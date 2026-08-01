package payments

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// PaymentAdapter is the only provider boundary available to future payment
// services. Implementations must translate provider SDK/API values into these
// provider-neutral DTOs before returning.
type PaymentAdapter interface {
	CreatePayment(context.Context, CreatePaymentRequest) (CreatePaymentResponse, error)
	GetPaymentStatus(context.Context, GetPaymentStatusRequest) (PaymentStatusResponse, error)
	VerifyWebhook(context.Context, VerifyWebhookRequest) (WebhookVerification, error)
	ParseWebhook(context.Context, ParseWebhookRequest) (WebhookEvent, error)
	RequestRefund(context.Context, RefundRequest) (RefundResponse, error)
	GetRefundStatus(context.Context, GetRefundStatusRequest) (RefundStatusResponse, error)
}

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusCaptured  PaymentStatus = "CAPTURED"
	PaymentStatusFailed    PaymentStatus = "FAILED"
	PaymentStatusExpired   PaymentStatus = "EXPIRED"
	PaymentStatusCancelled PaymentStatus = "CANCELLED"
)

func (s PaymentStatus) IsTerminal() bool {
	return s == PaymentStatusCaptured || s == PaymentStatusFailed ||
		s == PaymentStatusExpired || s == PaymentStatusCancelled
}

// PaymentInquiryScope identifies which provider-side object an inquiry
// response describes. A checkout session may reveal a payment-request
// identity, but it cannot prove payment capture.
type PaymentInquiryScope string

const (
	PaymentInquiryScopeCheckoutSession PaymentInquiryScope = "CHECKOUT_SESSION"
	PaymentInquiryScopePayment         PaymentInquiryScope = "PAYMENT"
)

func (s PaymentInquiryScope) IsValid() bool {
	return s == PaymentInquiryScopeCheckoutSession || s == PaymentInquiryScopePayment
}

type RefundStatus string

const (
	RefundStatusProcessing RefundStatus = "PROCESSING"
	RefundStatusSucceeded  RefundStatus = "SUCCEEDED"
	RefundStatusFailed     RefundStatus = "FAILED"
)

type AdapterErrorCode string

const (
	AdapterErrorRetryableTimeout      AdapterErrorCode = "RETRYABLE_TIMEOUT"
	AdapterErrorRetryableProvider     AdapterErrorCode = "RETRYABLE_PROVIDER"
	AdapterErrorRateLimited           AdapterErrorCode = "RATE_LIMITED"
	AdapterErrorAuthenticationFailed  AdapterErrorCode = "AUTHENTICATION_FAILED"
	AdapterErrorInvalidRequest        AdapterErrorCode = "INVALID_REQUEST"
	AdapterErrorIdempotencyConflict   AdapterErrorCode = "IDEMPOTENCY_CONFLICT"
	AdapterErrorReferenceMismatch     AdapterErrorCode = "REFERENCE_MISMATCH"
	AdapterErrorAmountMismatch        AdapterErrorCode = "AMOUNT_MISMATCH"
	AdapterErrorCurrencyMismatch      AdapterErrorCode = "CURRENCY_MISMATCH"
	AdapterErrorTerminalProvider      AdapterErrorCode = "TERMINAL_PROVIDER"
	AdapterErrorMalformedResponse     AdapterErrorCode = "MALFORMED_RESPONSE"
	AdapterErrorFutureCreatedSemantic AdapterErrorCode = "FUTURE_CREATED_SEMANTIC"
)

var (
	ErrRetryableTimeout           = AdapterError{code: AdapterErrorRetryableTimeout}
	ErrRetryableProvider          = AdapterError{code: AdapterErrorRetryableProvider}
	ErrRateLimited                = AdapterError{code: AdapterErrorRateLimited}
	ErrAuthenticationFailed       = AdapterError{code: AdapterErrorAuthenticationFailed}
	ErrInvalidRequest             = AdapterError{code: AdapterErrorInvalidRequest}
	ErrAdapterIdempotencyConflict = AdapterError{code: AdapterErrorIdempotencyConflict}
	ErrReferenceMismatch          = AdapterError{code: AdapterErrorReferenceMismatch}
	ErrAmountMismatch             = AdapterError{code: AdapterErrorAmountMismatch}
	ErrCurrencyMismatch           = AdapterError{code: AdapterErrorCurrencyMismatch}
	ErrTerminalProvider           = AdapterError{code: AdapterErrorTerminalProvider}
	ErrMalformedResponse          = AdapterError{code: AdapterErrorMalformedResponse}
	ErrFakeAdapterUnscripted      = errors.New("fake payment adapter operation is not configured")
)

// AdapterError deliberately excludes provider text and payloads. Provider
// implementations may wrap a normalized code with retry-after metadata, but
// must not expose raw provider errors to callers or logs.
type AdapterError struct {
	code       AdapterErrorCode
	RetryAfter time.Duration
}

func (e AdapterError) Error() string {
	if e.code == "" {
		return "payment adapter error"
	}
	return "payment adapter error: " + string(e.code)
}

func (e AdapterError) Is(target error) bool {
	other, ok := target.(AdapterError)
	return ok && e.code != "" && e.code == other.code
}

func (e AdapterError) Code() AdapterErrorCode {
	return e.code
}

func NewAdapterError(code AdapterErrorCode, retryAfter time.Duration) AdapterError {
	if !code.IsValid() {
		code = AdapterErrorMalformedResponse
	}
	return AdapterError{code: code, RetryAfter: retryAfter}
}

func (c AdapterErrorCode) IsValid() bool {
	switch c {
	case AdapterErrorRetryableTimeout,
		AdapterErrorRetryableProvider,
		AdapterErrorRateLimited,
		AdapterErrorAuthenticationFailed,
		AdapterErrorInvalidRequest,
		AdapterErrorIdempotencyConflict,
		AdapterErrorReferenceMismatch,
		AdapterErrorAmountMismatch,
		AdapterErrorCurrencyMismatch,
		AdapterErrorTerminalProvider,
		AdapterErrorMalformedResponse,
		AdapterErrorFutureCreatedSemantic:
		return true
	default:
		return false
	}
}

// NormalizeAdapterError converts transport errors into the frozen taxonomy.
// Unknown adapter errors are intentionally reduced to a generic retryable
// provider error; raw text is never copied into the normalized error.
func NormalizeAdapterError(err error) AdapterError {
	if err == nil {
		return AdapterError{}
	}
	var adapterErr AdapterError
	if errors.As(err, &adapterErr) {
		return NewAdapterError(adapterErr.code, adapterErr.RetryAfter)
	}
	var adapterErrPointer *AdapterError
	if errors.As(err, &adapterErrPointer) && adapterErrPointer != nil {
		return NewAdapterError(adapterErrPointer.code, adapterErrPointer.RetryAfter)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrRetryableTimeout
	}
	if errors.Is(err, context.Canceled) {
		return NewAdapterError(AdapterErrorRetryableProvider, 0)
	}
	return ErrRetryableProvider
}

type CreatePaymentRequest struct {
	AttemptID        string
	AmountRupiah     int64
	Currency         Currency
	RequestedMethod  RequestedMethod
	IntegrationMode  IntegrationMode
	CaptureMethod    CaptureMethod
	LocalReference   string
	IdempotencyKey   string
	RequestHash      string
	ExpiresAt        time.Time
	SuccessReturnURL string
	CancelReturnURL  string
}

type CreatePaymentResponse struct {
	ProviderSessionID     string
	ProviderPaymentReqID  string
	ProviderPaymentID     string
	Status                PaymentStatus
	AmountRupiah          int64
	Currency              Currency
	CheckoutURL           string
	ExpiresAt             time.Time
	ProviderCorrelationID string
	StatusCode            string
}

type GetPaymentStatusRequest struct {
	AttemptID            string
	ProviderSessionID    string
	ProviderPaymentReqID string
	ProviderPaymentID    string
	IdempotencyKey       string
}

type PaymentStatusResponse struct {
	Scope                PaymentInquiryScope
	ProviderSessionID    string
	ProviderPaymentReqID string
	ProviderPaymentID    string
	Status               PaymentStatus
	AmountRupiah         int64
	Currency             Currency
	CapturedAt           *time.Time
	// PayloadHash is an evidence hash produced by the adapter before any raw
	// provider response is discarded. It is never a raw provider payload.
	PayloadHash string
	ReasonCode  string
	StatusCode  string
}

type VerifyWebhookRequest struct {
	RawBody      []byte
	Headers      map[string]string
	ReceivedAt   time.Time
	MaxBodyBytes int
}

type WebhookVerification struct {
	Verified            bool
	ProviderEventID     string
	PayloadHash         string
	ReceivedAt          time.Time
	AuthContractVersion string
}

type ParseWebhookRequest struct {
	RawBody                  []byte
	ObservedAt               time.Time
	MaxBodyBytes             int
	ExpectedAmountRupiah     int64
	ExpectedCurrency         Currency
	ExpectedPaymentRequestID string
	ExpectedPaymentID        string
}

type WebhookVerificationState string

const (
	WebhookVerificationDiagnostic  WebhookVerificationState = "DIAGNOSTIC"
	WebhookVerificationQuarantined WebhookVerificationState = "QUARANTINED"
)

func (s WebhookVerificationState) IsValid() bool {
	return s == WebhookVerificationDiagnostic || s == WebhookVerificationQuarantined
}

type WebhookReplayDecision string

const (
	WebhookReplayNew               WebhookReplayDecision = "NEW"
	WebhookReplayDuplicateSameBody WebhookReplayDecision = "DUPLICATE_SAME_BODY"
	WebhookReplayConflicting       WebhookReplayDecision = "CONFLICTING_REPLAY"
)

type WebhookReplayClassification struct {
	Decision          WebhookReplayDecision
	VerificationState WebhookVerificationState
	ReasonCode        string
	MayMutate         bool
}

// WebhookReplayInput separates an absent prior event from an invalid prior
// hash. The body hashes are exact lowercase SHA-256 digests.
type WebhookReplayInput struct {
	ExistingEventFound bool
	ExistingBodyHash   string
	IncomingBodyHash   string
}

var ErrWebhookReplayInputInvalid = errors.New("invalid webhook replay input")

// ClassifyWebhookReplay is a pure decision for an already-resolved
// deterministic event identity. It validates all supplied hashes before
// returning a result that could permit a later state mutation.
func ClassifyWebhookReplay(input WebhookReplayInput) (WebhookReplayClassification, error) {
	if !isLowercaseSHA256(input.IncomingBodyHash) {
		return WebhookReplayClassification{}, ErrWebhookReplayInputInvalid
	}
	if !input.ExistingEventFound {
		if input.ExistingBodyHash != "" {
			return WebhookReplayClassification{}, ErrWebhookReplayInputInvalid
		}
		return WebhookReplayClassification{
			Decision:          WebhookReplayNew,
			VerificationState: WebhookVerificationDiagnostic,
			MayMutate:         true,
		}, nil
	}
	if !isLowercaseSHA256(input.ExistingBodyHash) {
		return WebhookReplayClassification{}, ErrWebhookReplayInputInvalid
	}
	if input.ExistingBodyHash == input.IncomingBodyHash {
		return WebhookReplayClassification{
			Decision:          WebhookReplayDuplicateSameBody,
			VerificationState: WebhookVerificationDiagnostic,
			MayMutate:         false,
		}, nil
	}
	return WebhookReplayClassification{
		Decision:          WebhookReplayConflicting,
		VerificationState: WebhookVerificationQuarantined,
		ReasonCode:        string(AdapterErrorIdempotencyConflict),
		MayMutate:         false,
	}, nil
}

func isLowercaseSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if (value[i] < '0' || value[i] > '9') && (value[i] < 'a' || value[i] > 'f') {
			return false
		}
	}
	return true
}

type WebhookEventState string

const (
	WebhookEventStatePending    WebhookEventState = "PENDING"
	WebhookEventStateCaptured   WebhookEventState = "CAPTURED"
	WebhookEventStateFailed     WebhookEventState = "FAILED"
	WebhookEventStateExpired    WebhookEventState = "EXPIRED"
	WebhookEventStateCancelled  WebhookEventState = "CANCELLED"
	WebhookEventStateProcessing WebhookEventState = "PROCESSING"
	WebhookEventStateSucceeded  WebhookEventState = "SUCCEEDED"
)

func (s WebhookEventState) IsValid() bool {
	switch s {
	case WebhookEventStatePending,
		WebhookEventStateCaptured,
		WebhookEventStateFailed,
		WebhookEventStateExpired,
		WebhookEventStateCancelled,
		WebhookEventStateProcessing,
		WebhookEventStateSucceeded:
		return true
	default:
		return false
	}
}

type WebhookEvent struct {
	EventKey             string
	ProviderEventID      string
	EventType            string
	PrimaryObjectID      string
	ProviderSessionID    string
	ProviderPaymentReqID string
	ProviderPaymentID    string
	ProviderRefundID     string
	State                WebhookEventState
	AmountRupiah         int64
	Currency             Currency
	OccurredAt           time.Time
	ObservedAt           time.Time
	SourceReference      string
	PayloadHash          string
	ReasonCode           string
	VerificationState    WebhookVerificationState
}

type RefundRequest struct {
	AttemptID            string
	ProviderPaymentReqID string
	ProviderPaymentID    string
	AmountRupiah         int64
	Currency             Currency
	IdempotencyKey       string
	RequestHash          string
}

type GetRefundStatusRequest struct {
	AttemptID        string
	ProviderRefundID string
	IdempotencyKey   string
}

type RefundResponse struct {
	ProviderRefundID     string
	ProviderPaymentReqID string
	Status               RefundStatus
	AmountRupiah         int64
	Currency             Currency
	ReasonCode           string
}

type RefundStatusResponse struct {
	ProviderRefundID     string
	ProviderPaymentReqID string
	Status               RefundStatus
	AmountRupiah         int64
	Currency             Currency
	ReasonCode           string
}

var (
	ErrInvalidAdapterConfig = errors.New("invalid payment adapter configuration")
	ErrAdapterNotTestMode   = errors.New("payment adapter must use Xendit Test Mode")
	ErrAdapterSecretMissing = errors.New("payment adapter secret is required when enabled")
	ErrAdapterURLInvalid    = errors.New("payment adapter endpoint must be an HTTPS origin")
	ErrAdapterTokenInvalid  = errors.New("payment adapter webhook token is invalid")
)

// AdapterConfig is backend-only configuration for a provider adapter. It is
// deliberately independent of request/response DTOs so credentials cannot be
// returned by adapter methods.
type AdapterConfig struct {
	Provider     Provider
	Environment  ProviderEnvironment
	Enabled      bool
	EndpointURL  string
	SecretKey    string
	WebhookToken string
}

func (c AdapterConfig) Validate() error {
	if c.Provider != ProviderXendit || c.Environment != ProviderEnvironmentTest {
		return ErrAdapterNotTestMode
	}
	if c.Enabled && strings.TrimSpace(c.SecretKey) == "" {
		return ErrAdapterSecretMissing
	}
	if c.EndpointURL != "" {
		parsed, err := url.Parse(c.EndpointURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
			return ErrAdapterURLInvalid
		}
	}
	if strings.ContainsAny(c.WebhookToken, "\r\n") || len(c.WebhookToken) > 512 {
		return ErrAdapterTokenInvalid
	}
	return nil
}

type RedactedAdapterConfig struct {
	Provider            Provider
	Environment         ProviderEnvironment
	Enabled             bool
	EndpointConfigured  bool
	SecretConfigured    bool
	WebhookTokenPresent bool
}

func (c AdapterConfig) Redacted() RedactedAdapterConfig {
	return RedactedAdapterConfig{
		Provider:            c.Provider,
		Environment:         c.Environment,
		Enabled:             c.Enabled,
		EndpointConfigured:  c.EndpointURL != "",
		SecretConfigured:    c.SecretKey != "",
		WebhookTokenPresent: c.WebhookToken != "",
	}
}

type FakeAdapterScript struct {
	CreatePayment    func(context.Context, CreatePaymentRequest) (CreatePaymentResponse, error)
	GetPaymentStatus func(context.Context, GetPaymentStatusRequest) (PaymentStatusResponse, error)
	VerifyWebhook    func(context.Context, VerifyWebhookRequest) (WebhookVerification, error)
	ParseWebhook     func(context.Context, ParseWebhookRequest) (WebhookEvent, error)
	RequestRefund    func(context.Context, RefundRequest) (RefundResponse, error)
	GetRefundStatus  func(context.Context, GetRefundStatusRequest) (RefundStatusResponse, error)
}

// FakeAdapter is deterministic and makes no network calls. It exists solely
// for service/contract tests before an actual provider adapter is authorized.
type FakeAdapter struct {
	script FakeAdapterScript
}

func NewFakeAdapter(script FakeAdapterScript) *FakeAdapter {
	return &FakeAdapter{script: script}
}

func (f *FakeAdapter) CreatePayment(ctx context.Context, req CreatePaymentRequest) (CreatePaymentResponse, error) {
	if f == nil || f.script.CreatePayment == nil {
		return CreatePaymentResponse{}, ErrFakeAdapterUnscripted
	}
	return f.script.CreatePayment(ctx, req)
}

func (f *FakeAdapter) GetPaymentStatus(ctx context.Context, req GetPaymentStatusRequest) (PaymentStatusResponse, error) {
	if f == nil || f.script.GetPaymentStatus == nil {
		return PaymentStatusResponse{}, ErrFakeAdapterUnscripted
	}
	return f.script.GetPaymentStatus(ctx, req)
}

func (f *FakeAdapter) VerifyWebhook(ctx context.Context, req VerifyWebhookRequest) (WebhookVerification, error) {
	if f == nil || f.script.VerifyWebhook == nil {
		return WebhookVerification{}, ErrFakeAdapterUnscripted
	}
	return f.script.VerifyWebhook(ctx, req)
}

func (f *FakeAdapter) ParseWebhook(ctx context.Context, req ParseWebhookRequest) (WebhookEvent, error) {
	if f == nil || f.script.ParseWebhook == nil {
		return WebhookEvent{}, ErrFakeAdapterUnscripted
	}
	return f.script.ParseWebhook(ctx, req)
}

func (f *FakeAdapter) RequestRefund(ctx context.Context, req RefundRequest) (RefundResponse, error) {
	if f == nil || f.script.RequestRefund == nil {
		return RefundResponse{}, ErrFakeAdapterUnscripted
	}
	return f.script.RequestRefund(ctx, req)
}

func (f *FakeAdapter) GetRefundStatus(ctx context.Context, req GetRefundStatusRequest) (RefundStatusResponse, error) {
	if f == nil || f.script.GetRefundStatus == nil {
		return RefundStatusResponse{}, ErrFakeAdapterUnscripted
	}
	return f.script.GetRefundStatus(ctx, req)
}

var _ PaymentAdapter = (*FakeAdapter)(nil)
