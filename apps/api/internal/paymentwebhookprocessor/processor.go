// Package paymentwebhookprocessor finalizes only normalized, VERIFIED payment
// inbox facts. It never reads raw webhook data or calls a payment provider.
package paymentwebhookprocessor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/payments"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrProcessorUnavailable = errors.New("payment webhook processor unavailable")
	ErrInboxPayloadInvalid  = errors.New("payment webhook inbox payload invalid")
)

const processingRecoveryAfter = 5 * time.Minute

type Processor struct {
	db                *pgxpool.Pool
	attempts          *payments.Repository
	audit             audit.PlatformService
	processingTimeout time.Duration
}

func NewProcessor(db *pgxpool.Pool, attempts *payments.Repository, platformAudit audit.PlatformService) (*Processor, error) {
	return NewProcessorWithOptions(db, attempts, platformAudit, ProcessorOptions{})
}

type ProcessorOptions struct {
	// ProcessingRecoveryAfter is intentionally configurable only at
	// construction so tests can exercise restart recovery without waiting. A
	// zero value uses the production-safe five-minute recovery delay.
	ProcessingRecoveryAfter time.Duration
}

func NewProcessorWithOptions(db *pgxpool.Pool, attempts *payments.Repository, platformAudit audit.PlatformService, options ProcessorOptions) (*Processor, error) {
	if db == nil || attempts == nil || platformAudit == nil {
		return nil, ErrProcessorUnavailable
	}
	if options.ProcessingRecoveryAfter < 0 {
		return nil, ErrProcessorUnavailable
	}
	if options.ProcessingRecoveryAfter == 0 {
		options.ProcessingRecoveryAfter = processingRecoveryAfter
	}
	return &Processor{db: db, attempts: attempts, audit: platformAudit, processingTimeout: options.ProcessingRecoveryAfter}, nil
}

// ProcessOne claims at most one event. The boolean reports whether an event
// was claimed, allowing the worker loop to sleep when the inbox is idle.
func (p *Processor) ProcessOne(ctx context.Context) (bool, error) {
	if p == nil || p.db == nil || p.attempts == nil || p.audit == nil {
		return false, ErrProcessorUnavailable
	}
	tx, err := p.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := reclaimStaleProcessing(ctx, tx, p.processingTimeout); err != nil {
		return false, err
	}

	event, err := claimNext(ctx, tx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := p.processClaimed(ctx, tx, event); err != nil {
		var terminal terminalError
		if errors.As(err, &terminal) {
			_ = tx.Rollback(ctx)
			return true, p.markTerminal(ctx, event, terminal.reason)
		}
		return true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return true, err
	}
	return true, nil
}

// reclaimStaleProcessing makes a crash-recovered inbox event eligible again
// without changing its immutable facts. The migration-029 lifecycle permits
// PROCESSING -> RETRYABLE, and the subsequent claim owns RETRYABLE work.
func reclaimStaleProcessing(ctx context.Context, tx pgx.Tx, timeout time.Duration) error {
	_, err := tx.Exec(ctx, `
		UPDATE payment_webhook_events
		SET processing_state = 'RETRYABLE', updated_at = transaction_timestamp()
		WHERE provider = 'XENDIT'
		  AND provider_environment = 'TEST'
		  AND verification_state = 'VERIFIED'
		  AND processing_state = 'PROCESSING'
	  AND updated_at <= transaction_timestamp() - ($1::bigint * interval '1 microsecond')
	`, timeout.Microseconds())
	return err
}

type inboxEvent struct {
	ID               string
	EventType        string
	EventKey         string
	RawBodyHash      string
	Payload          []byte
	PaymentAttemptID *string
	CorrelationID    string
	ReceivedAt       time.Time
}

func claimNext(ctx context.Context, tx pgx.Tx) (inboxEvent, error) {
	var event inboxEvent
	err := tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM payment_webhook_events
			WHERE provider = 'XENDIT'
			  AND provider_environment = 'TEST'
			  AND verification_state = 'VERIFIED'
			  AND processing_state IN ('RECEIVED', 'RETRYABLE')
			  -- Only these Payment Session events are covered by the current
			  -- controlled proof. payment.capture remains inquiry-only until a
			  -- separate capture contract proof is accepted.
			  AND event_type IN ('payment_session.completed', 'payment_session.expired')
			ORDER BY received_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE payment_webhook_events event
		SET processing_state = 'PROCESSING', updated_at = transaction_timestamp()
		FROM candidate
		WHERE event.id = candidate.id
		RETURNING event.id::text, event.event_type, event.provider_event_key,
		          event.raw_body_hash, event.redacted_payload::text,
		          event.payment_attempt_id::text, event.correlation_id, event.received_at
	`).Scan(&event.ID, &event.EventType, &event.EventKey, &event.RawBodyHash, &event.Payload,
		&event.PaymentAttemptID, &event.CorrelationID, &event.ReceivedAt)
	return event, err
}

type normalizedPayload struct {
	State            string `json:"state"`
	AmountRupiah     int64  `json:"amount_rupiah"`
	Currency         string `json:"currency"`
	PaymentID        string `json:"payment_id"`
	PaymentRequestID string `json:"payment_request_id"`
}

func decodePayload(raw []byte) (normalizedPayload, error) {
	var payload normalizedPayload
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil || !validPayload(payload) {
		return normalizedPayload{}, ErrInboxPayloadInvalid
	}
	return payload, nil
}

func validPayload(payload normalizedPayload) bool {
	if payload.AmountRupiah <= 0 || payload.Currency != string(payments.CurrencyIDR) {
		return false
	}
	return payload.State == "PENDING" || payload.State == "CAPTURED" || payload.State == "FAILED" ||
		payload.State == "EXPIRED" || payload.State == "CANCELLED"
}

type terminalError struct{ reason string }

func (e terminalError) Error() string { return e.reason }

func terminal(reason string) error { return terminalError{reason: reason} }

func (p *Processor) processClaimed(ctx context.Context, tx pgx.Tx, event inboxEvent) error {
	if _, err := decodePayload(event.Payload); err != nil {
		return terminal("INVALID_REQUEST")
	}

	switch event.EventType {
	case "payment_session.completed":
		// Checkout completion proves neither capture nor amount settlement.
		return p.markProcessedTx(ctx, tx, event, "NONE")
	case "payment_session.expired":
		if err := p.applyNonCaptureState(ctx, tx, event, payments.AttemptStateExpired); err != nil {
			return err
		}
		return p.markProcessedTx(ctx, tx, event, "NONE")
	default:
		return terminal("INVALID_REQUEST")
	}
}

func (p *Processor) applyNonCaptureState(ctx context.Context, tx pgx.Tx, event inboxEvent, target payments.AttemptState) error {
	if event.PaymentAttemptID == nil {
		return terminal("REFERENCE_MISMATCH")
	}
	attempt, err := p.attempts.GetAttemptTx(ctx, tx, *event.PaymentAttemptID)
	if err != nil {
		if errors.Is(err, payments.ErrAttemptNotFound) {
			return terminal("REFERENCE_MISMATCH")
		}
		return err
	}
	if attempt.State == target || attempt.State == payments.AttemptStateCaptured {
		return nil
	}
	if attempt.State != payments.AttemptStatePending {
		return nil
	}
	_, err = p.attempts.TransitionStateTx(ctx, tx, attempt.ID, payments.AttemptStatePending, target)
	if errors.Is(err, payments.ErrStateConflict) {
		return nil
	}
	return err
}

func (p *Processor) markProcessedTx(ctx context.Context, tx pgx.Tx, event inboxEvent, reason string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE payment_webhook_events
		SET processing_state = 'PROCESSED', processed_at = transaction_timestamp(), updated_at = transaction_timestamp()
		WHERE id = $1::uuid AND processing_state = 'PROCESSING'
	`, event.ID); err != nil {
		return err
	}
	return p.recordWebhookAudit(ctx, tx, event, "PROCESSED", reason)
}

func (p *Processor) markTerminal(ctx context.Context, event inboxEvent, reason string) error {
	tx, err := p.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		UPDATE payment_webhook_events
		SET processing_state = 'TERMINAL', processed_at = transaction_timestamp(), updated_at = transaction_timestamp()
		WHERE id = $1::uuid AND verification_state = 'VERIFIED' AND processing_state IN ('RECEIVED', 'RETRYABLE', 'PROCESSING')
	`, event.ID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("terminalize webhook event: %w", payments.ErrStateConflict)
	}
	if err := p.recordWebhookAudit(ctx, tx, event, "TERMINAL", reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Processor) recordWebhookAudit(ctx context.Context, tx pgx.Tx, event inboxEvent, result, reason string) error {
	correlationID := event.CorrelationID
	return p.audit.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorRole: "SYSTEM", Action: audit.ActionWebhookReplay, EntityType: audit.EntityPaymentWebhook,
		CorrelationID: &correlationID,
		Metadata: map[string]any{
			"provider": "XENDIT", "environment": "TEST", "route_family": routeFamily(event.EventType),
			"result": result, "reason": reason, "raw_body_hash": event.RawBodyHash,
		},
	})
}

func routeFamily(eventType string) string {
	if eventType == "payment_session.completed" || eventType == "payment_session.expired" {
		return "payment_session"
	}
	return "payment"
}
