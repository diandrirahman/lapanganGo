package payments

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreatePaymentIdempotencyDerivationsAreStableAndOpaque(t *testing.T) {
	bookingID := uuid.NewString()
	first := deterministicLocalReference(bookingID, "client-request-1")
	replay := deterministicLocalReference(bookingID, "client-request-1")
	other := deterministicLocalReference(bookingID, "client-request-2")
	if first != replay {
		t.Fatalf("same request key produced different local references: %q vs %q", first, replay)
	}
	if first == other || len(first) > 64 || !strings.HasPrefix(first, "pa-") {
		t.Fatalf("local reference is not deterministic opaque data: %q", first)
	}
	if strings.Contains(first, bookingID) || strings.Contains(first, "client-request") {
		t.Fatalf("local reference leaked request data: %q", first)
	}

	expiresAt := time.Date(2026, time.July, 29, 12, 34, 56, 123456789, time.FixedZone("WIB", 7*60*60))
	input := createRequestHashInput{
		AmountRupiah:     10000,
		BookingID:        bookingID,
		CancelReturnURL:  "https://demo.example.test/payments/return/" + first + "/cancel",
		ExpiresAt:        expiresAt,
		LocalReference:   first,
		RequestedMethod:  RequestedMethodQRIS,
		SuccessReturnURL: "https://demo.example.test/payments/return/" + first + "/success",
	}
	hashA := createRequestHash(input)
	hashB := createRequestHash(input)
	input.RequestedMethod = RequestedMethodCard
	hashC := createRequestHash(input)
	if hashA != hashB || hashA == hashC || len(hashA) != 64 {
		t.Fatalf("request hash derivation is not stable or method-bound: %q %q %q", hashA, hashB, hashC)
	}
}

func TestCreateRequestHashKnownCanonicalVector(t *testing.T) {
	const reference = "pa-0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab"
	input := createRequestHashInput{
		AmountRupiah:     10000,
		BookingID:        "11111111-1111-4111-8111-111111111111",
		CancelReturnURL:  "https://demo.example.test/payments/return/" + reference + "/cancel",
		ExpiresAt:        time.Date(2026, time.July, 29, 5, 34, 56, 123456789, time.UTC),
		LocalReference:   reference,
		RequestedMethod:  RequestedMethodQRIS,
		SuccessReturnURL: "https://demo.example.test/payments/return/" + reference + "/success",
	}
	const want = "7d9fbaa8e461e4a3b583946bc671c0ba979426f4cdf3b433523ddc71d64a2037"
	if got := createRequestHash(input); got != want {
		t.Fatalf("canonical request hash = %q; want %q", got, want)
	}
}

func TestCreateRequestHashCoversStableProviderFacts(t *testing.T) {
	const reference = "pa-0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab"
	base := createRequestHashInput{
		AmountRupiah:     10000,
		BookingID:        uuid.NewString(),
		CancelReturnURL:  "https://demo.example.test/payments/return/" + reference + "/cancel",
		ExpiresAt:        time.Date(2026, time.July, 29, 5, 34, 56, 0, time.UTC),
		LocalReference:   reference,
		RequestedMethod:  RequestedMethodQRIS,
		SuccessReturnURL: "https://demo.example.test/payments/return/" + reference + "/success",
	}
	want := createRequestHash(base)
	mutations := []createRequestHashInput{
		func() createRequestHashInput {
			value := base
			value.ExpiresAt = value.ExpiresAt.Add(time.Second)
			return value
		}(),
		func() createRequestHashInput { value := base; value.SuccessReturnURL += "-other"; return value }(),
		func() createRequestHashInput { value := base; value.CancelReturnURL += "-other"; return value }(),
		func() createRequestHashInput { value := base; value.LocalReference += "f"; return value }(),
	}
	for _, mutation := range mutations {
		if got := createRequestHash(mutation); got == want {
			t.Fatalf("stable provider fact mutation did not change request hash: %#v", mutation)
		}
	}
}

func TestPaymentIdempotencyKeyValidation(t *testing.T) {
	valid := []string{"request-1", uuid.NewString()}
	for _, key := range valid {
		if !validIdempotencyKey(key) {
			t.Errorf("valid idempotency key rejected: %q", key)
		}
	}
	invalid := []string{"", " leading", "trailing ", "line\nbreak", "combined,keys", strings.Repeat("x", 129)}
	for _, key := range invalid {
		if validIdempotencyKey(key) {
			t.Errorf("invalid idempotency key accepted: %q", key)
		}
	}
}

func TestPaymentAttemptResponseDoesNotExposeProviderFields(t *testing.T) {
	attempt := PaymentAttempt{
		ID:                   uuid.NewString(),
		BookingID:            uuid.NewString(),
		State:                AttemptStateCreated,
		ExpiresAt:            mustTestTime(),
		LocalReference:       "pa-safe-reference",
		RequestHash:          strings.Repeat("a", 64),
		ProviderPaymentID:    stringPtrForTest("provider-payment"),
		ProviderPaymentReqID: stringPtrForTest("provider-request"),
		CheckoutURL:          stringPtrForTest("https://checkout.example.test/session/opaque"),
	}
	response := paymentAttemptResponse(attempt)
	if _, ok := response["provider_payment_id"]; ok {
		t.Fatal("provider payment ID leaked in create response")
	}
	if _, ok := response["checkout_url"]; ok {
		t.Fatal("checkout URL leaked in create response before asynchronous worker")
	}
}

func TestSafeCheckoutURLAllowsOnlyXenditTestHosts(t *testing.T) {
	allowed := []string{
		"https://checkout-staging.xendit.co/sessions/ps-661f87c614802d6c402cd82d0",
		"https://dev.xen.to/kGxPCi76",
	}
	for _, value := range allowed {
		if !safeCheckoutURL(value) {
			t.Errorf("Xendit Test checkout URL rejected: %q", value)
		}
	}

	rejected := []string{
		"https://evil.example/checkout",
		"https://checkout.xendit.co/sessions/ps-661f87c614802d6c402cd82d0",
		"https://xen.to/kGxPCi60",
		"https://checkout-staging.xendit.co.evil.example/sessions/ps-661f87c614802d6c402cd82d0",
		"https://checkout-staging.xendit.co/sessions/ps-661f87c614802d6c402cd82d0?redirect=https://evil.example",
		"https://user@checkout-staging.xendit.co/sessions/ps-661f87c614802d6c402cd82d0",
		"http://checkout-staging.xendit.co/sessions/ps-661f87c614802d6c402cd82d0",
	}
	for _, value := range rejected {
		if safeCheckoutURL(value) {
			t.Errorf("unsafe checkout URL accepted: %q", value)
		}
	}
}

func TestPaymentAttemptViewExposesCheckoutOnlyWhileEligibleAndPending(t *testing.T) {
	checkoutURL := "https://checkout-staging.xendit.co/sessions/ps-661f87c614802d6c402cd82d0"
	for _, state := range []AttemptState{
		AttemptStateCreated,
		AttemptStatePending,
		AttemptStateCaptured,
		AttemptStateFailed,
		AttemptStateExpired,
		AttemptStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			view := paymentAttemptView(PaymentAttempt{
				ID:          uuid.NewString(),
				BookingID:   uuid.NewString(),
				State:       state,
				CheckoutURL: &checkoutURL,
			}, true)
			if state == AttemptStatePending {
				if view.CheckoutURL == nil || *view.CheckoutURL != checkoutURL {
					t.Fatalf("pending checkout URL = %v; want safe URL", view.CheckoutURL)
				}
				return
			}
			if view.CheckoutURL != nil {
				t.Fatalf("%s checkout URL = %q; want omitted", state, *view.CheckoutURL)
			}
		})
	}

	ineligible := paymentAttemptView(PaymentAttempt{
		ID:          uuid.NewString(),
		BookingID:   uuid.NewString(),
		State:       AttemptStatePending,
		CheckoutURL: &checkoutURL,
	}, false)
	if ineligible.CheckoutURL != nil {
		t.Fatalf("ineligible pending checkout URL = %q; want omitted", *ineligible.CheckoutURL)
	}
}

func TestNormalizedPaymentReturnURLs(t *testing.T) {
	const reference = "pa-0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab"
	success, cancel, err := normalizedPaymentReturnURLs("https://DEMO.EXAMPLE.TEST", reference)
	if err != nil {
		t.Fatalf("normalize payment return URLs: %v", err)
	}
	if success != "https://demo.example.test/payments/return/"+reference+"/success" ||
		cancel != "https://demo.example.test/payments/return/"+reference+"/cancel" {
		t.Fatalf("unexpected normalized return URLs: %q %q", success, cancel)
	}
	for _, origin := range []string{"", "http://demo.example.test", "https://demo.example.test/path", "https://user@demo.example.test"} {
		if _, _, err := normalizedPaymentReturnURLs(origin, reference); err == nil {
			t.Errorf("invalid return origin accepted: %q", origin)
		}
	}
	if _, _, err := normalizedPaymentReturnURLs("https://demo.example.test", "booking-customer@example.test"); err == nil {
		t.Fatal("non-opaque attempt reference accepted in return URL")
	}
}

func TestCreatePaymentFailsClosedWithoutAuditService(t *testing.T) {
	orchestrator := NewOrchestrator(nil, nil, nil, nil, OrchestratorOptions{})
	_, err := orchestrator.CreatePayment(
		context.Background(),
		uuid.NewString(),
		uuid.NewString(),
		"missing-audit",
		CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS},
	)
	if !errors.Is(err, ErrPaymentAuditUnavailable) {
		t.Fatalf("missing audit error = %v; want ErrPaymentAuditUnavailable", err)
	}
}

func stringPtrForTest(value string) *string { return &value }

func mustTestTime() (value time.Time) {
	return time.Unix(1, 0).UTC()
}
