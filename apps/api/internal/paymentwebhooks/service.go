package paymentwebhooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	"lapangango-api/internal/payments"
)

const ingressDeadline = 5 * time.Second
const maxJSONDepth = 16
const maxJSONMembers = 128

type Service struct {
	verifier   Verifier
	repository Repository
	now        Clock
}

func NewService(verifier Verifier, repository Repository, now Clock) (*Service, error) {
	if verifier == nil || repository == nil {
		return nil, ErrInvalidIngressInput
	}
	if now == nil {
		now = time.Now
	}
	return &Service{verifier: verifier, repository: repository, now: now}, nil
}

type ReceiveRequest struct {
	RouteFamily   RouteFamily
	RawBody       []byte
	Headers       map[string]string
	ReceivedAt    time.Time
	CorrelationID string
}

type ReceiveResult struct {
	Accepted bool
	Category string
	Status   int
}

func (s *Service) Receive(ctx context.Context, request ReceiveRequest) (ReceiveResult, error) {
	if s == nil || !isValidReceiveRequest(request) {
		return ReceiveResult{Category: InvalidCategory, Status: 400}, ErrInvalidIngressInput
	}
	if request.ReceivedAt.IsZero() {
		request.ReceivedAt = s.now().UTC()
	}
	ctx, cancel := context.WithTimeout(ctx, ingressDeadline)
	defer cancel()

	verification, err := s.verifier.VerifyWebhook(ctx, payments.VerifyWebhookRequest{
		RawBody: request.RawBody, Headers: request.Headers, ReceivedAt: request.ReceivedAt,
		MaxBodyBytes: payments.XenditWebhookMaxBodyBytes,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ReceiveResult{Category: UnavailableCategory, Status: 503}, ErrDurabilityUnavailable
		}
		result, responseErr := receiveVerificationError(err)
		if result.Status == 401 {
			// Authentication failures must never become accepted simply because
			// their sanitized audit sink is temporarily unavailable.
			_ = s.repository.RecordAuthFailure(ctx, AuthFailureParams{RouteFamily: request.RouteFamily, CorrelationID: request.CorrelationID, RawBodyHash: rawBodyHash(request.RawBody)})
		}
		return result, responseErr
	}
	if !verification.Verified {
		return ReceiveResult{Category: UnauthorizedCategory, Status: 401}, nil
	}
	if err := validateJSONStructure(request.RawBody); err != nil {
		return ReceiveResult{Category: InvalidCategory, Status: 400}, nil
	}

	event, err := s.verifier.ParseWebhook(ctx, payments.ParseWebhookRequest{
		RawBody: request.RawBody, ObservedAt: request.ReceivedAt, MaxBodyBytes: payments.XenditWebhookMaxBodyBytes,
	})
	if err != nil {
		if isUnsupportedParseError(err) {
			if auditErr := s.repository.RecordUnsupported(ctx, UnsupportedParams{RouteFamily: request.RouteFamily, CorrelationID: request.CorrelationID, RawBodyHash: verification.PayloadHash}); auditErr != nil {
				return ReceiveResult{Category: UnavailableCategory, Status: 503}, ErrDurabilityUnavailable
			}
		}
		return receiveParseError(err)
	}
	if !request.RouteFamily.Accepts(event.EventType) {
		event = quarantine(event, string(payments.AdapterErrorInvalidRequest))
	}

	// A first parse establishes a safe provider identity. A single immutable
	// attempt match may then provide comparison context for the second,
	// deterministic normalization pass. This is a read only operation.
	var paymentAttemptID *string
	if event.VerificationState != payments.WebhookVerificationQuarantined {
		attempt, lookupErr := s.repository.FindAttemptContext(ctx, event)
		if lookupErr != nil {
			return ReceiveResult{Category: UnavailableCategory, Status: 503}, ErrDurabilityUnavailable
		}
		if attempt != nil {
			paymentAttemptID = &attempt.ID
			event, err = s.verifier.ParseWebhook(ctx, payments.ParseWebhookRequest{
				RawBody: request.RawBody, ObservedAt: request.ReceivedAt, MaxBodyBytes: payments.XenditWebhookMaxBodyBytes,
				ExpectedAmountRupiah: attempt.AmountRupiah, ExpectedCurrency: attempt.Currency,
				ExpectedPaymentRequestID: attempt.PaymentRequestID, ExpectedPaymentID: attempt.PaymentID,
			})
			if err != nil {
				return receiveParseError(err)
			}
		}
	}

	_, err = s.repository.Accept(ctx, AcceptParams{
		Event: event, AuthContract: verification.AuthContractVersion, CorrelationID: request.CorrelationID,
		RouteFamily: request.RouteFamily, ReceivedAt: request.ReceivedAt, PaymentAttemptID: paymentAttemptID,
	})
	if err != nil {
		return ReceiveResult{Category: UnavailableCategory, Status: 503}, ErrDurabilityUnavailable
	}
	return ReceiveResult{Accepted: true, Category: AcceptedCategory, Status: 200}, nil
}

func isValidReceiveRequest(request ReceiveRequest) bool {
	return request.CorrelationID != "" && (request.RouteFamily == RoutePaymentSession || request.RouteFamily == RoutePayment || request.RouteFamily == RouteRefund)
}

func receiveVerificationError(err error) (ReceiveResult, error) {
	var webhookErr payments.WebhookError
	if errors.As(err, &webhookErr) {
		switch webhookErr.Code() {
		case payments.WebhookTokenMissing, payments.WebhookTokenInvalid:
			return ReceiveResult{Category: UnauthorizedCategory, Status: 401}, nil
		case payments.WebhookBodyTooLarge:
			return ReceiveResult{Category: PayloadTooLargeCategory, Status: 413}, nil
		case payments.WebhookBodyEmpty:
			return ReceiveResult{Category: InvalidCategory, Status: 400}, nil
		}
	}
	return ReceiveResult{Category: InvalidCategory, Status: 400}, err
}

func receiveParseError(err error) (ReceiveResult, error) {
	var webhookErr payments.WebhookError
	if errors.As(err, &webhookErr) {
		switch webhookErr.Code() {
		case payments.WebhookEventUnsupported, payments.WebhookSchemaUnsupported:
			// The handler records a sanitized audit receipt for this case. The
			// current inbox schema intentionally cannot represent an unknown type.
			return ReceiveResult{Accepted: true, Category: AcceptedCategory, Status: 200}, nil
		case payments.WebhookBodyTooLarge:
			return ReceiveResult{Category: PayloadTooLargeCategory, Status: 413}, nil
		}
	}
	return ReceiveResult{Category: InvalidCategory, Status: 400}, nil
}

func isUnsupportedParseError(err error) bool {
	var webhookErr payments.WebhookError
	return errors.As(err, &webhookErr) && (webhookErr.Code() == payments.WebhookEventUnsupported || webhookErr.Code() == payments.WebhookSchemaUnsupported)
}

func quarantine(event payments.WebhookEvent, reason string) payments.WebhookEvent {
	event.VerificationState = payments.WebhookVerificationQuarantined
	event.ReasonCode = reason
	return event
}

func rawBodyHash(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

type jsonFrame struct {
	kind         json.Delim
	members      int
	expectingKey bool
}

func validateJSONStructure(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var stack []jsonFrame
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := countJSONValue(&stack, token); err != nil {
			return err
		}
	}
	if len(stack) != 0 {
		return ErrInvalidIngressInput
	}
	return nil
}

func countJSONValue(stack *[]jsonFrame, token json.Token) error {
	if delimiter, ok := token.(json.Delim); ok && (delimiter == '}' || delimiter == ']') {
		if len(*stack) == 0 {
			return ErrInvalidIngressInput
		}
		top := (*stack)[len(*stack)-1]
		if (delimiter == '}' && top.kind != '{') || (delimiter == ']' && top.kind != '[') || (top.kind == '{' && !top.expectingKey) {
			return ErrInvalidIngressInput
		}
		*stack = (*stack)[:len(*stack)-1]
		if len(*stack) > 0 && (*stack)[len(*stack)-1].kind == '{' {
			(*stack)[len(*stack)-1].expectingKey = true
		}
		return nil
	}
	if len(*stack) > 0 {
		parent := &(*stack)[len(*stack)-1]
		if parent.kind == '{' && parent.expectingKey {
			if _, ok := token.(string); !ok {
				return ErrInvalidIngressInput
			}
			parent.members++
			parent.expectingKey = false
			if parent.members > maxJSONMembers {
				return ErrInvalidIngressInput
			}
			return nil
		}
		if parent.kind == '[' && token != json.Delim(']') {
			parent.members++
			if parent.members > maxJSONMembers {
				return ErrInvalidIngressInput
			}
		}
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{', '[':
			if len(*stack)+1 > maxJSONDepth {
				return ErrInvalidIngressInput
			}
			if len(*stack) > 0 && (*stack)[len(*stack)-1].kind == '{' {
				(*stack)[len(*stack)-1].expectingKey = true
			}
			*stack = append(*stack, jsonFrame{kind: value, expectingKey: value == '{'})
		}
	default:
		if len(*stack) > 0 && (*stack)[len(*stack)-1].kind == '{' {
			(*stack)[len(*stack)-1].expectingKey = true
		}
	}
	return nil
}
