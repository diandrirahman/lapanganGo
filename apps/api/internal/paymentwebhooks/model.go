package paymentwebhooks

import (
	"context"
	"errors"
	"time"

	"lapangango-api/internal/payments"
)

const (
	ProviderXendit           = "XENDIT"
	EnvironmentTest          = "TEST"
	AcceptedCategory         = "accepted"
	UnauthorizedCategory     = "unauthorized"
	InvalidCategory          = "invalid_request"
	UnsupportedMediaCategory = "unsupported_media_type"
	PayloadTooLargeCategory  = "payload_too_large"
	RateLimitedCategory      = "rate_limited"
	UnavailableCategory      = "temporarily_unavailable"
)

type RouteFamily string

const (
	RoutePaymentSession RouteFamily = "payment_session"
	RoutePayment        RouteFamily = "payment"
	RouteRefund         RouteFamily = "refund"
)

func (f RouteFamily) Accepts(eventType string) bool {
	switch f {
	case RoutePaymentSession:
		return eventType == "payment_session.completed" || eventType == "payment_session.expired"
	case RoutePayment:
		return eventType == "payment.capture"
	case RouteRefund:
		return eventType == "refund.succeeded" || eventType == "refund.failed"
	default:
		return false
	}
}

type Verifier interface {
	VerifyWebhook(context.Context, payments.VerifyWebhookRequest) (payments.WebhookVerification, error)
	ParseWebhook(context.Context, payments.ParseWebhookRequest) (payments.WebhookEvent, error)
}

type AttemptContext struct {
	ID               string
	AmountRupiah     int64
	Currency         payments.Currency
	PaymentSessionID string
	PaymentRequestID string
	PaymentID        string
}

type Repository interface {
	FindAttemptContext(context.Context, payments.WebhookEvent) (*AttemptContext, error)
	Accept(context.Context, AcceptParams) (Acceptance, error)
	RecordUnsupported(context.Context, UnsupportedParams) error
	RecordAuthFailure(context.Context, AuthFailureParams) error
}

type AcceptParams struct {
	Event            payments.WebhookEvent
	AuthContract     string
	CorrelationID    string
	RouteFamily      RouteFamily
	ReceivedAt       time.Time
	PaymentAttemptID *string
}

type Acceptance struct {
	New       bool
	Duplicate bool
	Conflict  bool
}

type UnsupportedParams struct {
	RouteFamily   RouteFamily
	CorrelationID string
	RawBodyHash   string
}

type AuthFailureParams struct {
	RouteFamily   RouteFamily
	CorrelationID string
	RawBodyHash   string
}

var ErrInvalidIngressInput = errors.New("invalid webhook ingress input")
var ErrDurabilityUnavailable = errors.New("webhook durability unavailable")

type Clock func() time.Time
