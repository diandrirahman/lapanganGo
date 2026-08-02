package paymentwebhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/payments"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db    *pgxpool.Pool
	audit audit.PlatformRepository
}

func NewPostgresRepository(db *pgxpool.Pool, auditRepository audit.PlatformRepository) *PostgresRepository {
	return &PostgresRepository{db: db, audit: auditRepository}
}

func (r *PostgresRepository) FindAttemptContext(ctx context.Context, event payments.WebhookEvent) (*AttemptContext, error) {
	if r == nil || r.db == nil {
		return nil, ErrDurabilityUnavailable
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, amount_rupiah, currency, provider_session_id, provider_payment_request_id, provider_payment_id
		FROM payment_attempts
		WHERE provider = 'XENDIT' AND provider_environment = 'TEST'
		  AND ((NULLIF($1, '') IS NOT NULL AND provider_session_id = NULLIF($1, ''))
		    OR (NULLIF($2, '') IS NOT NULL AND provider_payment_request_id = NULLIF($2, ''))
		    OR (NULLIF($3, '') IS NOT NULL AND provider_payment_id = NULLIF($3, '')))
	`, event.ProviderSessionID, event.ProviderPaymentReqID, event.ProviderPaymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var matches []AttemptContext
	for rows.Next() {
		var out AttemptContext
		var session, request, payment *string
		if err := rows.Scan(&out.ID, &out.AmountRupiah, &out.Currency, &session, &request, &payment); err != nil {
			return nil, err
		}
		if session != nil {
			out.PaymentSessionID = *session
		}
		if request != nil {
			out.PaymentRequestID = *request
		}
		if payment != nil {
			out.PaymentID = *payment
		}
		matches = append(matches, out)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(matches) != 1 {
		return nil, nil
	}
	return &matches[0], nil
}

func (r *PostgresRepository) Accept(ctx context.Context, params AcceptParams) (Acceptance, error) {
	if r == nil || r.db == nil || r.audit == nil || !validAcceptParams(params) {
		return Acceptance{}, ErrInvalidIngressInput
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Acceptance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existingHash, processing string
	err = tx.QueryRow(ctx, `SELECT raw_body_hash, processing_state FROM payment_webhook_events WHERE provider = 'XENDIT' AND provider_environment = 'TEST' AND provider_event_key = $1 FOR UPDATE`, params.Event.EventKey).Scan(&existingHash, &processing)
	if errors.Is(err, pgx.ErrNoRows) {
		payload, err := redactedPayload(params.Event)
		if err != nil {
			return Acceptance{}, err
		}
		processingState := initialLifecycle(params.Event)
		_, err = tx.Exec(ctx, `
			INSERT INTO payment_webhook_events (provider, provider_environment, event_type, provider_event_key, provider_event_id, primary_object_id, raw_body_hash, auth_contract_version, verification_state, processing_state, redacted_payload, payment_attempt_id, correlation_id, received_at, processed_at)
			VALUES ('XENDIT', 'TEST', $1,$2,NULL,$3,$4,$5,$6,$7::varchar,$8::jsonb,$9,$10,$11,
				CASE WHEN $7::varchar = 'TERMINAL' THEN transaction_timestamp() ELSE NULL END)
		`, params.Event.EventType, params.Event.EventKey, params.Event.PrimaryObjectID, params.Event.PayloadHash, params.AuthContract, params.Event.VerificationState, processingState, payload, params.PaymentAttemptID, params.CorrelationID, params.ReceivedAt)
		if err != nil {
			return Acceptance{}, err
		}
		if err := r.writeAudit(ctx, tx, audit.ActionWebhookReceived, params, "NEW", safeReason(params.Event.ReasonCode)); err != nil {
			return Acceptance{}, err
		}
		if err := r.writeAudit(ctx, tx, audit.ActionWebhookAuthPassed, params, "NEW", safeReason(params.Event.ReasonCode)); err != nil {
			return Acceptance{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Acceptance{}, err
		}
		return Acceptance{New: true}, nil
	}
	if err != nil {
		return Acceptance{}, err
	}
	classification, err := payments.ClassifyWebhookReplay(payments.WebhookReplayInput{ExistingEventFound: true, ExistingBodyHash: existingHash, IncomingBodyHash: params.Event.PayloadHash})
	if err != nil {
		return Acceptance{}, err
	}
	if classification.Decision == payments.WebhookReplayDuplicateSameBody {
		if err := r.writeAudit(ctx, tx, audit.ActionWebhookDuplicate, params, "DUPLICATE", safeReason(params.Event.ReasonCode)); err != nil {
			return Acceptance{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Acceptance{}, err
		}
		return Acceptance{Duplicate: true}, nil
	}
	if processing == "RECEIVED" || processing == "PROCESSING" || processing == "RETRYABLE" {
		_, err = tx.Exec(ctx, `UPDATE payment_webhook_events SET verification_state = 'QUARANTINED', processing_state = 'TERMINAL', processed_at = transaction_timestamp(), updated_at = transaction_timestamp() WHERE provider = 'XENDIT' AND provider_environment = 'TEST' AND provider_event_key = $1`, params.Event.EventKey)
		if err != nil {
			return Acceptance{}, err
		}
	}
	if err := r.writeAudit(ctx, tx, audit.ActionWebhookConflict, params, "CONFLICT", string(payments.AdapterErrorIdempotencyConflict)); err != nil {
		return Acceptance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Acceptance{}, err
	}
	return Acceptance{Conflict: true}, nil
}

func validAcceptParams(params AcceptParams) bool {
	return params.Event.EventKey != "" && params.Event.EventType != "" && params.Event.PrimaryObjectID != "" && params.Event.PayloadHash != "" && payments.IsXenditWebhookContractVersion(params.AuthContract) && params.CorrelationID != "" && !params.ReceivedAt.IsZero()
}

func initialLifecycle(event payments.WebhookEvent) string {
	if event.VerificationState == payments.WebhookVerificationQuarantined {
		return "TERMINAL"
	}
	return "RECEIVED"
}

func redactedPayload(event payments.WebhookEvent) (string, error) {
	payload := map[string]any{"state": string(event.State), "amount_rupiah": event.AmountRupiah, "currency": string(event.Currency), "source_reference": event.SourceReference}
	if event.ProviderPaymentID != "" {
		payload["payment_id"] = event.ProviderPaymentID
	}
	if event.ProviderPaymentReqID != "" {
		payload["payment_request_id"] = event.ProviderPaymentReqID
	}
	if event.ReasonCode != "" {
		payload["reason_code"] = event.ReasonCode
	}
	b, err := json.Marshal(payload)
	if err != nil || len(b) > 2048 {
		return "", fmt.Errorf("redacted webhook payload invalid")
	}
	return string(b), nil
}

func (r *PostgresRepository) writeAudit(ctx context.Context, tx pgx.Tx, action string, params AcceptParams, result, reason string) error {
	correlationID := params.CorrelationID
	return r.audit.Create(ctx, tx, audit.CreatePlatformAuditLogParams{ActorRole: "SYSTEM", Action: action, EntityType: audit.EntityPaymentWebhook, CorrelationID: &correlationID, Metadata: map[string]any{"provider": ProviderXendit, "environment": EnvironmentTest, "route_family": string(params.RouteFamily), "result": result, "reason": reason, "raw_body_hash": params.Event.PayloadHash}})
}

func safeReason(reason string) string {
	if reason == "" {
		return "NONE"
	}
	return reason
}

func (r *PostgresRepository) RecordUnsupported(ctx context.Context, params UnsupportedParams) error {
	if r == nil || r.db == nil || r.audit == nil || params.CorrelationID == "" || params.RawBodyHash == "" {
		return ErrInvalidIngressInput
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	correlationID := params.CorrelationID
	err = r.audit.Create(ctx, tx, audit.CreatePlatformAuditLogParams{ActorRole: "SYSTEM", Action: audit.ActionWebhookConflict, EntityType: audit.EntityPaymentWebhook, CorrelationID: &correlationID, Metadata: map[string]any{"provider": ProviderXendit, "environment": EnvironmentTest, "route_family": string(params.RouteFamily), "result": "UNSUPPORTED", "reason": "INVALID_REQUEST", "raw_body_hash": params.RawBodyHash}})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) RecordAuthFailure(ctx context.Context, params AuthFailureParams) error {
	if r == nil || r.db == nil || r.audit == nil || params.CorrelationID == "" || params.RawBodyHash == "" {
		return ErrInvalidIngressInput
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	correlationID := params.CorrelationID
	err = r.audit.Create(ctx, tx, audit.CreatePlatformAuditLogParams{ActorRole: "SYSTEM", Action: audit.ActionWebhookAuthFailed, EntityType: audit.EntityPaymentWebhook, CorrelationID: &correlationID, Metadata: map[string]any{"provider": ProviderXendit, "environment": EnvironmentTest, "route_family": string(params.RouteFamily), "result": "AUTH_FAILED", "reason": "AUTHENTICATION_FAILED", "raw_body_hash": params.RawBodyHash}})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
