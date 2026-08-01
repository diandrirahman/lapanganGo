package payments

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"
)

const (
	XenditWebhookAuthContractVersion = "XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL"
	XenditWebhookCallbackTokenHeader = "x-callback-token"
	XenditWebhookMaxBodyBytes        = 256 * 1024
)

type WebhookErrorCode string

const (
	WebhookTokenMissing      WebhookErrorCode = "WEBHOOK_TOKEN_MISSING"
	WebhookTokenInvalid      WebhookErrorCode = "WEBHOOK_TOKEN_INVALID"
	WebhookBodyEmpty         WebhookErrorCode = "WEBHOOK_BODY_EMPTY"
	WebhookBodyTooLarge      WebhookErrorCode = "WEBHOOK_BODY_TOO_LARGE"
	WebhookJSONMalformed     WebhookErrorCode = "WEBHOOK_JSON_MALFORMED"
	WebhookEventUnsupported  WebhookErrorCode = "WEBHOOK_EVENT_UNSUPPORTED"
	WebhookSchemaUnsupported WebhookErrorCode = "WEBHOOK_SCHEMA_UNSUPPORTED"
	WebhookPrimaryIDMissing  WebhookErrorCode = "WEBHOOK_PRIMARY_ID_MISSING"
	WebhookEventKeyInvalid   WebhookErrorCode = "WEBHOOK_EVENT_KEY_INVALID"
	WebhookFutureEventTime   WebhookErrorCode = "WEBHOOK_FUTURE_EVENT_TIME"
	WebhookDuplicate         WebhookErrorCode = "WEBHOOK_DUPLICATE"
	WebhookReplayConflict    WebhookErrorCode = "WEBHOOK_REPLAY_CONFLICT"
	WebhookAmountInvalid     WebhookErrorCode = "WEBHOOK_AMOUNT_INVALID"
	WebhookCurrencyMismatch  WebhookErrorCode = "WEBHOOK_CURRENCY_MISMATCH"
	WebhookReferenceMismatch WebhookErrorCode = "WEBHOOK_REFERENCE_MISMATCH"
	WebhookEventTimeInvalid  WebhookErrorCode = "WEBHOOK_EVENT_TIME_INVALID"
)

// WebhookError contains only a stable safe category. It deliberately never
// carries a token, raw body, header value, or provider error text.
type WebhookError struct {
	code WebhookErrorCode
}

func (e WebhookError) Error() string {
	return "webhook verification failed: " + string(e.code)
}

func (e WebhookError) Is(target error) bool {
	other, ok := target.(WebhookError)
	return ok && e.code == other.code
}

func (e WebhookError) Code() WebhookErrorCode {
	return e.code
}

func newWebhookError(code WebhookErrorCode) error {
	return WebhookError{code: code}
}

// XenditTestWebhookVerifier implements the existing provider-neutral webhook
// method signatures for the frozen Test Mode callback-token contract only.
// It has no HTTP, database, logging, provider-network, or payment-state role.
type XenditTestWebhookVerifier struct {
	callbackToken []byte
}

func NewXenditTestWebhookVerifier(callbackToken string) (*XenditTestWebhookVerifier, error) {
	if callbackToken == "" || len(callbackToken) > 512 {
		return nil, newWebhookError(WebhookTokenInvalid)
	}
	return &XenditTestWebhookVerifier{callbackToken: []byte(callbackToken)}, nil
}

func (v *XenditTestWebhookVerifier) VerifyWebhook(_ context.Context, req VerifyWebhookRequest) (WebhookVerification, error) {
	if err := validateWebhookBody(req.RawBody, req.MaxBodyBytes); err != nil {
		return WebhookVerification{}, err
	}

	// The digest is intentionally derived from the exact received bytes before
	// any JSON decoding or normalization happens elsewhere.
	payloadHash := rawBodySHA256(req.RawBody)
	if v == nil || len(v.callbackToken) == 0 {
		return WebhookVerification{}, newWebhookError(WebhookTokenInvalid)
	}
	providedToken, present := req.Headers[XenditWebhookCallbackTokenHeader]
	if !present || providedToken == "" {
		return WebhookVerification{}, newWebhookError(WebhookTokenMissing)
	}
	if subtle.ConstantTimeCompare([]byte(providedToken), v.callbackToken) != 1 {
		return WebhookVerification{}, newWebhookError(WebhookTokenInvalid)
	}

	return WebhookVerification{
		Verified:            true,
		PayloadHash:         payloadHash,
		ReceivedAt:          req.ReceivedAt,
		AuthContractVersion: XenditWebhookAuthContractVersion,
	}, nil
}

func (v *XenditTestWebhookVerifier) ParseWebhook(_ context.Context, req ParseWebhookRequest) (WebhookEvent, error) {
	if v == nil || len(v.callbackToken) == 0 {
		return WebhookEvent{}, newWebhookError(WebhookTokenInvalid)
	}
	if err := validateWebhookBody(req.RawBody, req.MaxBodyBytes); err != nil {
		return WebhookEvent{}, err
	}
	if req.ObservedAt.IsZero() {
		return WebhookEvent{}, newWebhookError(WebhookEventTimeInvalid)
	}

	envelope, _, unexpectedData, err := parseXenditWebhookEnvelope(req.RawBody)
	if err != nil {
		return WebhookEvent{}, err
	}
	state, err := normalizedXenditWebhookState(envelope.eventType, envelope.status)
	if err != nil {
		return WebhookEvent{}, err
	}
	primaryObjectID, err := xenditPrimaryObjectID(envelope.eventType, envelope)
	if err != nil {
		return WebhookEvent{}, err
	}
	if !ValidProviderIdentity(primaryObjectID, true) {
		return WebhookEvent{}, newWebhookError(WebhookEventKeyInvalid)
	}
	if err := validateXenditProviderIDs(envelope); err != nil {
		return WebhookEvent{}, err
	}

	event := WebhookEvent{
		EventType:            envelope.eventType,
		PrimaryObjectID:      primaryObjectID,
		ProviderSessionID:    envelope.paymentSessionID,
		ProviderPaymentReqID: envelope.paymentRequestID,
		ProviderPaymentID:    envelope.paymentID,
		ProviderRefundID:     envelope.refundID,
		State:                state,
		AmountRupiah:         envelope.amountRupiah,
		Currency:             Currency(envelope.currency),
		OccurredAt:           envelope.createdAt,
		ObservedAt:           req.ObservedAt.UTC(),
		PayloadHash:          rawBodySHA256(req.RawBody),
		VerificationState:    WebhookVerificationDiagnostic,
	}
	event.EventKey = deterministicXenditEventKey(event.EventType, event.PrimaryObjectID)
	event.SourceReference = "sha256:" + event.PayloadHash

	if unexpectedData {
		return quarantineWebhookEvent(event, string(AdapterErrorInvalidRequest)), nil
	}
	if event.Currency != CurrencyIDR {
		return quarantineWebhookEvent(event, string(AdapterErrorCurrencyMismatch)), nil
	}
	if req.ExpectedCurrency != "" && event.Currency != req.ExpectedCurrency {
		return quarantineWebhookEvent(event, string(AdapterErrorCurrencyMismatch)), nil
	}
	if req.ExpectedAmountRupiah > 0 && event.AmountRupiah != req.ExpectedAmountRupiah {
		return quarantineWebhookEvent(event, string(AdapterErrorAmountMismatch)), nil
	}
	if (req.ExpectedPaymentRequestID != "" && event.ProviderPaymentReqID != req.ExpectedPaymentRequestID) ||
		(req.ExpectedPaymentID != "" && event.ProviderPaymentID != req.ExpectedPaymentID) {
		return quarantineWebhookEvent(event, string(AdapterErrorReferenceMismatch)), nil
	}
	if event.OccurredAt.After(req.ObservedAt.UTC().Add(5 * time.Minute)) {
		return quarantineWebhookEvent(event, string(AdapterErrorFutureCreatedSemantic)), nil
	}
	if envelope.reasonCode == "" {
		return event, nil
	}
	if envelope.reasonCode == string(AdapterErrorTerminalProvider) && event.State == WebhookEventStateFailed {
		event.ReasonCode = envelope.reasonCode
		return event, nil
	}
	// Provider payload reason text is not authoritative. All other normalized
	// reasons are derived above from the observed event and request context.
	return quarantineWebhookEvent(event, string(AdapterErrorInvalidRequest)), nil
}

func validateWebhookBody(rawBody []byte, requestedMaximum int) error {
	if len(rawBody) == 0 {
		return newWebhookError(WebhookBodyEmpty)
	}
	maximum := XenditWebhookMaxBodyBytes
	if requestedMaximum > 0 && requestedMaximum < maximum {
		maximum = requestedMaximum
	}
	if requestedMaximum < 0 || len(rawBody) > maximum {
		return newWebhookError(WebhookBodyTooLarge)
	}
	return nil
}

func rawBodySHA256(rawBody []byte) string {
	digest := sha256.Sum256(rawBody)
	return hex.EncodeToString(digest[:])
}

type xenditWebhookEnvelope struct {
	eventType        string
	createdAt        time.Time
	paymentSessionID string
	paymentRequestID string
	paymentID        string
	refundID         string
	status           string
	amountRupiah     int64
	currency         string
	reasonCode       string
}

func parseXenditWebhookEnvelope(rawBody []byte) (xenditWebhookEnvelope, map[string]json.RawMessage, bool, error) {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &outer); err != nil {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookJSONMalformed)
	}
	if !hasOnlyKeys(outer, "event", "version", "created", "data") {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookSchemaUnsupported)
	}
	eventType, err := requiredJSONString(outer, "event", WebhookEventUnsupported)
	if err != nil || !isFrozenXenditEventType(eventType) {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookEventUnsupported)
	}
	version, err := requiredJSONString(outer, "version", WebhookSchemaUnsupported)
	if err != nil || version != "2024-11-11" {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookSchemaUnsupported)
	}
	created, err := requiredJSONString(outer, "created", WebhookEventTimeInvalid)
	if err != nil {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookEventTimeInvalid)
	}
	createdAt, err := time.Parse(time.RFC3339, created)
	if err != nil || createdAt.Format(time.RFC3339) != created {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookEventTimeInvalid)
	}

	dataRaw, ok := outer["data"]
	if !ok {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookSchemaUnsupported)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookSchemaUnsupported)
	}
	unexpectedData := !hasOnlyKeys(data,
		"payment_session_id", "payment_request_id", "payment_id", "refund_id",
		"status", "amount", "currency", "reason_code",
	)

	status, err := requiredJSONString(data, "status", WebhookSchemaUnsupported)
	if err != nil {
		return xenditWebhookEnvelope{}, nil, false, err
	}
	currency, err := requiredJSONString(data, "currency", WebhookCurrencyMismatch)
	if err != nil || !xenditCurrencyPattern.MatchString(currency) {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookCurrencyMismatch)
	}
	amountRupiah, err := requiredXenditAmount(data)
	if err != nil {
		return xenditWebhookEnvelope{}, nil, false, err
	}
	paymentSessionID, err := optionalJSONString(data, "payment_session_id", WebhookSchemaUnsupported)
	if err != nil {
		return xenditWebhookEnvelope{}, nil, false, err
	}
	paymentRequestID, err := optionalJSONString(data, "payment_request_id", WebhookSchemaUnsupported)
	if err != nil {
		return xenditWebhookEnvelope{}, nil, false, err
	}
	paymentID, err := optionalJSONString(data, "payment_id", WebhookSchemaUnsupported)
	if err != nil {
		return xenditWebhookEnvelope{}, nil, false, err
	}
	refundID, err := optionalJSONString(data, "refund_id", WebhookSchemaUnsupported)
	if err != nil {
		return xenditWebhookEnvelope{}, nil, false, err
	}
	reasonCode, err := optionalJSONString(data, "reason_code", WebhookSchemaUnsupported)
	if err != nil || !isSafeXenditReasonCode(reasonCode) {
		return xenditWebhookEnvelope{}, nil, false, newWebhookError(WebhookSchemaUnsupported)
	}

	return xenditWebhookEnvelope{
		eventType:        eventType,
		createdAt:        createdAt,
		paymentSessionID: paymentSessionID,
		paymentRequestID: paymentRequestID,
		paymentID:        paymentID,
		refundID:         refundID,
		status:           status,
		amountRupiah:     amountRupiah,
		currency:         currency,
		reasonCode:       reasonCode,
	}, data, unexpectedData, nil
}

func requiredJSONString(values map[string]json.RawMessage, key string, code WebhookErrorCode) (string, error) {
	value, present := values[key]
	if !present {
		return "", newWebhookError(code)
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil || decoded == "" {
		return "", newWebhookError(code)
	}
	return decoded, nil
}

func optionalJSONString(values map[string]json.RawMessage, key string, code WebhookErrorCode) (string, error) {
	value, present := values[key]
	if !present {
		return "", nil
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil || decoded == "" {
		return "", newWebhookError(code)
	}
	return decoded, nil
}

func requiredXenditAmount(values map[string]json.RawMessage) (int64, error) {
	value, present := values["amount"]
	if !present {
		return 0, newWebhookError(WebhookAmountInvalid)
	}
	amount, err := ParsePositiveRupiah(string(value))
	if err != nil {
		return 0, newWebhookError(WebhookAmountInvalid)
	}
	return amount, nil
}

func xenditPrimaryObjectID(eventType string, event xenditWebhookEnvelope) (string, error) {
	switch eventType {
	case "payment_session.completed", "payment_session.expired":
		if event.paymentSessionID == "" {
			return "", newWebhookError(WebhookPrimaryIDMissing)
		}
		return event.paymentSessionID, nil
	case "payment.capture":
		if event.paymentID != "" {
			return event.paymentID, nil
		}
		if event.paymentRequestID != "" {
			return event.paymentRequestID, nil
		}
	case "refund.succeeded", "refund.failed":
		if event.refundID != "" {
			return event.refundID, nil
		}
	}
	return "", newWebhookError(WebhookPrimaryIDMissing)
}

func validateXenditProviderIDs(event xenditWebhookEnvelope) error {
	for _, identifier := range []string{event.paymentSessionID, event.paymentRequestID, event.paymentID, event.refundID} {
		if identifier != "" && !ValidProviderIdentity(identifier, true) {
			return newWebhookError(WebhookEventKeyInvalid)
		}
	}
	return nil
}

func normalizedXenditWebhookState(eventType, status string) (WebhookEventState, error) {
	switch eventType {
	case "payment_session.completed":
		if status == "COMPLETED" {
			return WebhookEventStatePending, nil
		}
	case "payment_session.expired":
		if status == "EXPIRED" {
			return WebhookEventStateExpired, nil
		}
	case "payment.capture":
		switch status {
		case "PENDING":
			return WebhookEventStatePending, nil
		case "SUCCEEDED":
			return WebhookEventStateCaptured, nil
		case "FAILED":
			return WebhookEventStateFailed, nil
		case "EXPIRED":
			return WebhookEventStateExpired, nil
		case "CANCELED":
			return WebhookEventStateCancelled, nil
		}
	case "refund.succeeded":
		if status == "SUCCEEDED" {
			return WebhookEventStateSucceeded, nil
		}
	case "refund.failed":
		if status == "FAILED" {
			return WebhookEventStateFailed, nil
		}
	}
	return "", newWebhookError(WebhookSchemaUnsupported)
}

func quarantineWebhookEvent(event WebhookEvent, reason string) WebhookEvent {
	event.VerificationState = WebhookVerificationQuarantined
	event.ReasonCode = reason
	return event
}

func deterministicXenditEventKey(eventType, primaryObjectID string) string {
	return string(ProviderXendit) + "|" + eventType + "|" + primaryObjectID
}

func hasOnlyKeys(values map[string]json.RawMessage, allowed ...string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range values {
		if _, ok := allowedSet[key]; !ok {
			return false
		}
	}
	return true
}

func isFrozenXenditEventType(eventType string) bool {
	switch eventType {
	case "payment_session.completed", "payment_session.expired", "payment.capture", "refund.succeeded", "refund.failed":
		return true
	default:
		return false
	}
}

func isSafeXenditReasonCode(reason string) bool {
	return reason == "" || AdapterErrorCode(reason).IsValid()
}

var xenditCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
