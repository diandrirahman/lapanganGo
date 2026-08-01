package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const xenditWebhookTestToken = "xendit-test-callback-token"

type xenditWebhookFixtureManifest struct {
	Fixtures []xenditWebhookFixtureExpectation `json:"fixtures"`
}

type xenditWebhookFixtureExpectation struct {
	ID              string         `json:"id"`
	File            string         `json:"file"`
	EventType       string         `json:"event_type"`
	EventKey        string         `json:"event_key"`
	PrimaryObjectID string         `json:"primary_object_id"`
	Hash            string         `json:"hash"`
	Auth            string         `json:"auth"`
	Verification    string         `json:"verification"`
	Reason          string         `json:"reason"`
	Normalized      map[string]any `json:"normalized"`
}

func TestXenditTestWebhookVerifierAuthentication(t *testing.T) {
	verifier := newXenditWebhookVerifier(t)
	raw := mustXenditWebhookFixture(t, "payment_pending.json")
	receivedAt := time.Date(2026, time.January, 15, 10, 20, 0, 0, time.UTC)

	verification, err := verifier.VerifyWebhook(context.Background(), VerifyWebhookRequest{
		RawBody:    raw,
		Headers:    map[string]string{XenditWebhookCallbackTokenHeader: xenditWebhookTestToken},
		ReceivedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("valid current token rejected: %v", err)
	}
	if !verification.Verified || verification.AuthContractVersion != XenditWebhookAuthContractVersion || verification.ReceivedAt != receivedAt {
		t.Fatalf("unexpected verification result: %+v", verification)
	}

	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    WebhookErrorCode
	}{
		{name: "missing", headers: nil, want: WebhookTokenMissing},
		{name: "empty", headers: map[string]string{XenditWebhookCallbackTokenHeader: ""}, want: WebhookTokenMissing},
		{name: "wrong same length", headers: map[string]string{XenditWebhookCallbackTokenHeader: strings.Repeat("x", len(xenditWebhookTestToken))}, want: WebhookTokenInvalid},
		{name: "wrong different length", headers: map[string]string{XenditWebhookCallbackTokenHeader: "wrong"}, want: WebhookTokenInvalid},
		{name: "unicode variant", headers: map[string]string{XenditWebhookCallbackTokenHeader: xenditWebhookTestToken + "\u00a0"}, want: WebhookTokenInvalid},
		{name: "whitespace variant", headers: map[string]string{XenditWebhookCallbackTokenHeader: " " + xenditWebhookTestToken}, want: WebhookTokenInvalid},
		{name: "previous token", headers: map[string]string{XenditWebhookCallbackTokenHeader: "xendit-previous-callback-token"}, want: WebhookTokenInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := verifier.VerifyWebhook(context.Background(), VerifyWebhookRequest{RawBody: raw, Headers: tc.headers, ReceivedAt: receivedAt})
			assertWebhookErrorCode(t, err, tc.want)
			if err != nil && (strings.Contains(err.Error(), xenditWebhookTestToken) || strings.Contains(err.Error(), string(raw))) {
				t.Fatalf("safe verification error leaked token or raw body: %q", err)
			}
		})
	}
}

func TestXenditTestWebhookVerifierHashesExactRawBytes(t *testing.T) {
	verifier := newXenditWebhookVerifier(t)
	raw := mustXenditWebhookFixture(t, "payment_pending.json")
	verify := func(t *testing.T, body []byte) WebhookVerification {
		t.Helper()
		result, err := verifier.VerifyWebhook(context.Background(), VerifyWebhookRequest{
			RawBody: body,
			Headers: map[string]string{XenditWebhookCallbackTokenHeader: xenditWebhookTestToken},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	fixtureResult := verify(t, raw)
	if fixtureResult.PayloadHash != "3b72ed29a798f9825f64e52696f7bcecd983536b83d4622f306c15111dfca45e" {
		t.Fatalf("fixture raw hash = %q", fixtureResult.PayloadHash)
	}
	changedByte := append([]byte(nil), raw...)
	changedByte[len(changedByte)-2] ^= 1
	if got := verify(t, changedByte).PayloadHash; got == fixtureResult.PayloadHash {
		t.Fatal("one-byte body change retained the raw hash")
	}
	if got := verify(t, append([]byte(" "), raw...)).PayloadHash; got == fixtureResult.PayloadHash {
		t.Fatal("whitespace change retained the raw hash")
	}
	if got := verify(t, append(append([]byte(nil), raw...), '\n')).PayloadHash; got == fixtureResult.PayloadHash {
		t.Fatal("trailing newline change retained the raw hash")
	}

	_, err := verifier.VerifyWebhook(context.Background(), VerifyWebhookRequest{RawBody: nil, Headers: map[string]string{XenditWebhookCallbackTokenHeader: xenditWebhookTestToken}})
	assertWebhookErrorCode(t, err, WebhookBodyEmpty)
	_, err = verifier.VerifyWebhook(context.Background(), VerifyWebhookRequest{
		RawBody:      mustXenditWebhookFixture(t, "oversized_body.spec"),
		Headers:      map[string]string{XenditWebhookCallbackTokenHeader: xenditWebhookTestToken},
		MaxBodyBytes: XenditWebhookMaxBodyBytes,
	})
	assertWebhookErrorCode(t, err, WebhookBodyTooLarge)
}

func TestXenditTestWebhookParserFrozenFixtures(t *testing.T) {
	verifier := newXenditWebhookVerifier(t)
	manifest := loadXenditWebhookFixtureManifest(t)
	defaultObservedAt := time.Date(2026, time.January, 15, 11, 0, 0, 0, time.UTC)
	errorsByFixture := map[string]WebhookErrorCode{
		"malformed-json":           WebhookJSONMalformed,
		"unsupported-event":        WebhookEventUnsupported,
		"unsupported-version":      WebhookSchemaUnsupported,
		"missing-primary-identity": WebhookPrimaryIDMissing,
		"oversized-body":           WebhookBodyTooLarge,
		"invalid-amount":           WebhookAmountInvalid,
	}

	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			raw := mustXenditWebhookFixture(t, fixture.File)
			request := ParseWebhookRequest{RawBody: raw, ObservedAt: defaultObservedAt}
			switch fixture.ID {
			case "amount-mismatch":
				request.ExpectedAmountRupiah = 125000
			case "reference-mismatch":
				request.ExpectedPaymentRequestID = "pr_fixture_expected_0001"
			case "future-created":
				request.ObservedAt = time.Date(2026, time.January, 15, 10, 5, 0, 0, time.UTC)
			}
			event, err := verifier.ParseWebhook(context.Background(), request)
			if want, rejects := errorsByFixture[fixture.ID]; rejects {
				assertWebhookErrorCode(t, err, want)
				return
			}
			if err != nil {
				t.Fatalf("fixture parser rejected: %v", err)
			}
			if event.EventKey != fixture.EventKey || event.PrimaryObjectID != fixture.PrimaryObjectID || event.EventType != fixture.EventType || event.PayloadHash != fixture.Hash {
				t.Fatalf("fixture identity/hash mismatch: %+v", event)
			}
			if fixture.ID == "currency-mismatch" && (event.VerificationState != WebhookVerificationQuarantined || event.ReasonCode != string(AdapterErrorCurrencyMismatch)) {
				t.Fatalf("currency mismatch was not quarantined safely: %+v", event)
			}
			if fixture.ID == "amount-mismatch" && (event.VerificationState != WebhookVerificationQuarantined || event.ReasonCode != string(AdapterErrorAmountMismatch)) {
				t.Fatalf("amount mismatch was not quarantined safely: %+v", event)
			}
			if fixture.ID == "reference-mismatch" && (event.VerificationState != WebhookVerificationQuarantined || event.ReasonCode != string(AdapterErrorReferenceMismatch)) {
				t.Fatalf("reference mismatch was not quarantined safely: %+v", event)
			}
			if fixture.ID == "future-created" && (event.VerificationState != WebhookVerificationQuarantined || event.ReasonCode != string(AdapterErrorFutureCreatedSemantic)) {
				t.Fatalf("future event was not quarantined safely: %+v", event)
			}
			if fixture.ID == "sensitive-redacted" {
				if event.VerificationState != WebhookVerificationQuarantined || event.ReasonCode != string(AdapterErrorInvalidRequest) {
					t.Fatalf("sensitive payload was not quarantined safely: %+v", event)
				}
				assertNormalizedWebhookEventRedacted(t, event)
			}
		})
	}
}

func TestXenditTestWebhookParserTimestampSemanticsAndDeterminism(t *testing.T) {
	verifier := newXenditWebhookVerifier(t)
	raw := mustXenditWebhookFixture(t, "payment_pending.json")
	created := time.Date(2026, time.January, 15, 10, 3, 0, 0, time.UTC)

	for _, tc := range []struct {
		name        string
		observedAt  time.Time
		quarantined bool
	}{
		{name: "valid", observedAt: created},
		{name: "exactly five minutes", observedAt: created.Add(-5 * time.Minute)},
		{name: "more than five minutes", observedAt: created.Add(-5*time.Minute - time.Second), quarantined: true},
		{name: "old event", observedAt: created.Add(365 * 24 * time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{RawBody: raw, ObservedAt: tc.observedAt})
			if err != nil {
				t.Fatal(err)
			}
			if tc.quarantined {
				if event.VerificationState != WebhookVerificationQuarantined || event.ReasonCode != string(AdapterErrorFutureCreatedSemantic) {
					t.Fatalf("future semantic result = %+v", event)
				}
				return
			}
			if event.VerificationState != WebhookVerificationDiagnostic {
				t.Fatalf("non-future event was quarantined: %+v", event)
			}
		})
	}

	malformed := []byte(`{"event":"payment.capture","version":"2024-11-11","created":"not-a-time","data":{"payment_id":"pay_fixture_pending_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`)
	_, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{RawBody: malformed, ObservedAt: created})
	assertWebhookErrorCode(t, err, WebhookEventTimeInvalid)

	first, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{RawBody: raw, ObservedAt: created})
	if err != nil {
		t.Fatal(err)
	}
	second, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{RawBody: raw, ObservedAt: created})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated parsing was non-deterministic: first=%+v second=%+v", first, second)
	}
}

func TestClassifyWebhookReplay(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    WebhookReplayInput
		decision WebhookReplayDecision
		state    WebhookVerificationState
		reason   string
		mutates  bool
	}{
		{name: "new", input: WebhookReplayInput{IncomingBodyHash: strings.Repeat("a", 64)}, decision: WebhookReplayNew, state: WebhookVerificationDiagnostic, mutates: true},
		{name: "same hash", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: strings.Repeat("a", 64), IncomingBodyHash: strings.Repeat("a", 64)}, decision: WebhookReplayDuplicateSameBody, state: WebhookVerificationDiagnostic},
		{name: "different hash", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: strings.Repeat("a", 64), IncomingBodyHash: strings.Repeat("b", 64)}, decision: WebhookReplayConflicting, state: WebhookVerificationQuarantined, reason: string(AdapterErrorIdempotencyConflict)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ClassifyWebhookReplay(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision != tc.decision || result.VerificationState != tc.state || result.ReasonCode != tc.reason || result.MayMutate != tc.mutates {
				t.Fatalf("replay classification = %+v", result)
			}
		})
	}
}

func TestClassifyWebhookReplayRejectsInvalidHashes(t *testing.T) {
	validHash := strings.Repeat("a", 64)
	malformedUnicodeHash := strings.Repeat("é", 32)
	for _, tc := range []struct {
		name  string
		input WebhookReplayInput
	}{
		{name: "incoming empty", input: WebhookReplayInput{}},
		{name: "incoming uppercase", input: WebhookReplayInput{IncomingBodyHash: strings.Repeat("A", 64)}},
		{name: "incoming short", input: WebhookReplayInput{IncomingBodyHash: strings.Repeat("a", 63)}},
		{name: "incoming long", input: WebhookReplayInput{IncomingBodyHash: strings.Repeat("a", 65)}},
		{name: "incoming non hex", input: WebhookReplayInput{IncomingBodyHash: strings.Repeat("g", 64)}},
		{name: "incoming malformed", input: WebhookReplayInput{IncomingBodyHash: strings.Repeat("a", 63) + "-"}},
		{name: "incoming whitespace prefix", input: WebhookReplayInput{IncomingBodyHash: " " + strings.Repeat("a", 63)}},
		{name: "incoming whitespace suffix", input: WebhookReplayInput{IncomingBodyHash: strings.Repeat("a", 63) + " "}},
		{name: "incoming algorithm prefix", input: WebhookReplayInput{IncomingBodyHash: "sha256:" + validHash}},
		{name: "incoming malformed unicode", input: WebhookReplayInput{IncomingBodyHash: malformedUnicodeHash}},
		{name: "existing empty", input: WebhookReplayInput{ExistingEventFound: true, IncomingBodyHash: validHash}},
		{name: "existing uppercase", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: strings.Repeat("A", 64), IncomingBodyHash: validHash}},
		{name: "existing short", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: strings.Repeat("a", 63), IncomingBodyHash: validHash}},
		{name: "existing long", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: strings.Repeat("a", 65), IncomingBodyHash: validHash}},
		{name: "existing non hex", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: strings.Repeat("g", 64), IncomingBodyHash: validHash}},
		{name: "existing whitespace", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: " " + strings.Repeat("a", 63), IncomingBodyHash: validHash}},
		{name: "existing algorithm prefix", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: "sha256:" + validHash, IncomingBodyHash: validHash}},
		{name: "existing malformed unicode", input: WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: malformedUnicodeHash, IncomingBodyHash: validHash}},
		{name: "missing event with supplied existing hash", input: WebhookReplayInput{ExistingBodyHash: validHash, IncomingBodyHash: validHash}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ClassifyWebhookReplay(tc.input)
			if !errors.Is(err, ErrWebhookReplayInputInvalid) {
				t.Fatalf("replay error = %v; want %v", err, ErrWebhookReplayInputInvalid)
			}
			if result.MayMutate {
				t.Fatalf("invalid replay input may mutate: %+v", result)
			}
		})
	}
}

func TestWebhookNormalizedReasonCodesMatchInboxContract(t *testing.T) {
	verifier := newXenditWebhookVerifier(t)
	observedAt := time.Date(2026, time.January, 15, 11, 0, 0, 0, time.UTC)
	migration029ReasonAllowlist := map[string]struct{}{
		"RETRYABLE_TIMEOUT": {}, "RETRYABLE_PROVIDER": {}, "RATE_LIMITED": {},
		"AUTHENTICATION_FAILED": {}, "INVALID_REQUEST": {}, "IDEMPOTENCY_CONFLICT": {},
		"REFERENCE_MISMATCH": {}, "AMOUNT_MISMATCH": {}, "CURRENCY_MISMATCH": {},
		"TERMINAL_PROVIDER": {}, "MALFORMED_RESPONSE": {}, "FUTURE_CREATED_SEMANTIC": {},
	}

	currencyEvent, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{
		RawBody:    mustXenditWebhookFixture(t, "payment_currency_mismatch.json"),
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	futureEvent, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{
		RawBody:    mustXenditWebhookFixture(t, "payment_future_created.json"),
		ObservedAt: time.Date(2026, time.January, 15, 10, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	referenceEvent, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{
		RawBody:                  mustXenditWebhookFixture(t, "payment_reference_mismatch.json"),
		ObservedAt:               observedAt,
		ExpectedPaymentRequestID: "pr_fixture_expected_0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	amountEvent, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{
		RawBody:              mustXenditWebhookFixture(t, "payment_amount_mismatch.json"),
		ObservedAt:           observedAt,
		ExpectedAmountRupiah: 125000,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminalEvent, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{
		RawBody:    mustXenditWebhookFixture(t, "payment_failed.json"),
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	duplicate, err := ClassifyWebhookReplay(WebhookReplayInput{
		ExistingEventFound: true,
		ExistingBodyHash:   strings.Repeat("a", 64),
		IncomingBodyHash:   strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := ClassifyWebhookReplay(WebhookReplayInput{
		ExistingEventFound: true,
		ExistingBodyHash:   strings.Repeat("a", 64),
		IncomingBodyHash:   strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{name: "currency mismatch", got: currencyEvent.ReasonCode, want: "CURRENCY_MISMATCH"},
		{name: "reference mismatch", got: referenceEvent.ReasonCode, want: "REFERENCE_MISMATCH"},
		{name: "amount mismatch", got: amountEvent.ReasonCode, want: "AMOUNT_MISMATCH"},
		{name: "duplicate", got: duplicate.ReasonCode, want: ""},
		{name: "conflicting replay", got: conflict.ReasonCode, want: "IDEMPOTENCY_CONFLICT"},
		{name: "future semantic", got: futureEvent.ReasonCode, want: "FUTURE_CREATED_SEMANTIC"},
		{name: "terminal provider", got: terminalEvent.ReasonCode, want: "TERMINAL_PROVIDER"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("reason code = %q; want %q", tc.got, tc.want)
			}
			_, allowed := migration029ReasonAllowlist[tc.got]
			if strings.HasPrefix(tc.got, "WEBHOOK_") || (tc.got != "" && !allowed) {
				t.Fatalf("reason code is outside the frozen inbox allowlist: %q", tc.got)
			}
		})
	}

	for _, reason := range []AdapterErrorCode{
		AdapterErrorRetryableTimeout, AdapterErrorRetryableProvider, AdapterErrorRateLimited,
		AdapterErrorAuthenticationFailed, AdapterErrorInvalidRequest, AdapterErrorIdempotencyConflict,
		AdapterErrorReferenceMismatch, AdapterErrorAmountMismatch, AdapterErrorCurrencyMismatch,
		AdapterErrorTerminalProvider, AdapterErrorMalformedResponse, AdapterErrorFutureCreatedSemantic,
	} {
		if _, allowed := migration029ReasonAllowlist[string(reason)]; !allowed {
			t.Fatalf("provider-neutral reason is outside migration 029 allowlist: %q", reason)
		}
	}
}

func TestXenditTestWebhookParserQuarantinesContextInvalidReasonCode(t *testing.T) {
	verifier := newXenditWebhookVerifier(t)
	raw := mustXenditWebhookFixture(t, "payment_pending.json")
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("fixture data is not an object")
	}

	for _, claimedReason := range []string{
		"RETRYABLE_TIMEOUT",
		"RETRYABLE_PROVIDER",
		"RATE_LIMITED",
		"CURRENCY_MISMATCH",
		"AMOUNT_MISMATCH",
		"REFERENCE_MISMATCH",
		"FUTURE_CREATED_SEMANTIC",
		"IDEMPOTENCY_CONFLICT",
		"AUTHENTICATION_FAILED",
		"INVALID_REQUEST",
		"TERMINAL_PROVIDER",
		"MALFORMED_RESPONSE",
	} {
		t.Run(claimedReason, func(t *testing.T) {
			probe := make(map[string]any, len(payload))
			for key, value := range payload {
				probe[key] = value
			}
			probeData := make(map[string]any, len(data))
			for key, value := range data {
				probeData[key] = value
			}
			probeData["reason_code"] = claimedReason
			probe["data"] = probeData
			probeRaw, err := json.Marshal(probe)
			if err != nil {
				t.Fatal(err)
			}

			event, err := verifier.ParseWebhook(context.Background(), ParseWebhookRequest{
				RawBody:    probeRaw,
				ObservedAt: time.Date(2026, time.January, 15, 11, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			if event.VerificationState != WebhookVerificationQuarantined || event.ReasonCode != string(AdapterErrorInvalidRequest) {
				t.Fatalf("context-invalid reason was not safely quarantined: %+v", event)
			}
		})
	}
}

func TestXenditTestWebhookParserRejectsNonStringProviderIDs(t *testing.T) {
	verifier := newXenditWebhookVerifier(t)
	raw := mustXenditWebhookFixture(t, "payment_capture_succeeded.json")
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "number", value: float64(123)},
		{name: "boolean", value: true},
		{name: "object", value: map[string]any{"id": "pay_fixture_capture_0001"}},
		{name: "array", value: []any{"pay_fixture_capture_0001"}},
		{name: "null", value: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := make(map[string]any, len(payload))
			for key, value := range payload {
				probe[key] = value
			}
			originalData, ok := payload["data"].(map[string]any)
			if !ok {
				t.Fatal("fixture data is not an object")
			}
			probeData := make(map[string]any, len(originalData))
			for key, value := range originalData {
				probeData[key] = value
			}
			probeData["payment_id"] = tc.value
			probe["data"] = probeData
			probeRaw, err := json.Marshal(probe)
			if err != nil {
				t.Fatal(err)
			}
			_, err = verifier.ParseWebhook(context.Background(), ParseWebhookRequest{RawBody: probeRaw, ObservedAt: time.Date(2026, time.January, 15, 11, 0, 0, 0, time.UTC)})
			assertWebhookErrorCode(t, err, WebhookSchemaUnsupported)
		})
	}
}

func newXenditWebhookVerifier(t *testing.T) *XenditTestWebhookVerifier {
	t.Helper()
	verifier, err := NewXenditTestWebhookVerifier(xenditWebhookTestToken)
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func assertWebhookErrorCode(t *testing.T, err error, want WebhookErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("webhook error = nil; want %s", want)
	}
	var webhookErr WebhookError
	if !errors.As(err, &webhookErr) || webhookErr.Code() != want {
		t.Fatalf("webhook error = %v; want %s", err, want)
	}
}

func loadXenditWebhookFixtureManifest(t *testing.T) xenditWebhookFixtureManifest {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(xenditWebhookFixtureDir(t), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest xenditWebhookFixtureManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func mustXenditWebhookFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := fixtureRawBytes(xenditWebhookFixtureDir(t), name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func xenditWebhookFixtureDir(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate webhook fixture test")
	}
	return filepath.Join(filepath.Dir(sourceFile), "testdata", "xendit_webhooks_v1")
}

func assertNormalizedWebhookEventRedacted(t *testing.T, event WebhookEvent) {
	t.Helper()
	encoded := fmt.Sprintf("%+v", event)
	for _, forbidden := range []string{"<redacted>", "callback", "token", "customer", "checkout", "pan", "cvv"} {
		if strings.Contains(strings.ToLower(encoded), forbidden) {
			t.Fatalf("normalized event retained forbidden value/field %q: %s", forbidden, encoded)
		}
	}
}
