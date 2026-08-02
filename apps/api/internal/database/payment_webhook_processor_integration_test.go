package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/payments"
	"lapangango-api/internal/paymentwebhookprocessor"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPaymentWebhookProcessor_DoesNotClaimUnprovenVerifiedCapture(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 30)
	assertMigrationVersion(t, m, 30, false)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	processor, err := paymentwebhookprocessor.NewProcessor(pool, payments.NewRepository(pool), audit.NewPlatformService(audit.NewPlatformRepository()))
	if err != nil {
		t.Fatal(err)
	}

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	now := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:webhook-processor", "PENDING", nil, now)
	key := "XENDIT|payment.capture|pay_processor_unproven_0001"
	insertVerifiedProcessorEvent(t, db, "payment.capture", key, "pay_processor_unproven_0001", &attemptID,
		map[string]any{"state": "CAPTURED", "amount_rupiah": 10000, "currency": "IDR", "payment_id": "pay_processor_0001"}, now)

	claimed, err := processor.ProcessOne(ctx)
	if err != nil || claimed {
		t.Fatalf("unproven verified capture must not be claimed: claimed=%t err=%v", claimed, err)
	}
	var bookingStatus, attemptState, eventState string
	var captureCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id=$1`, bookingID).Scan(&bookingStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id=$1`, attemptID).Scan(&attemptState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT processing_state FROM payment_webhook_events WHERE provider_event_key=$1`, key).Scan(&eventState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id=$1`, attemptID).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	if bookingStatus != "PENDING_PAYMENT" || attemptState != "PENDING" || eventState != "RECEIVED" || captureCount != 0 {
		t.Fatalf("unproven capture changed local state: booking=%s attempt=%s event=%s facts=%d", bookingStatus, attemptState, eventState, captureCount)
	}
}

func TestPaymentWebhookProcessor_DoesNotPaySessionOrInvalidCapture(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 30)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	processor, err := paymentwebhookprocessor.NewProcessor(pool, payments.NewRepository(pool), audit.NewPlatformService(audit.NewPlatformRepository()))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:session-pending", "PENDING", nil, now)
	insertVerifiedProcessorEvent(t, db, "payment_session.completed", "XENDIT|payment_session.completed|session_processor_0001", "session_processor_0001", &attemptID,
		map[string]any{"state": "PENDING", "amount_rupiah": 10000, "currency": "IDR", "payment_request_id": "request_processor_0001"}, now)
	if claimed, err := processor.ProcessOne(ctx); err != nil || !claimed {
		t.Fatalf("process completed session: claimed=%t err=%v", claimed, err)
	}
	assertProcessorBookingPending(t, pool, ctx, bookingID)

	badBookingID := seedPaymentAttemptBooking(t, db, true)
	badAttemptID := uuid.NewString()
	insertPaymentAttempt(t, db, badAttemptID, badBookingID, 1, "payment:create:processor-mismatch", "PENDING", nil, now)
	badKey := "XENDIT|payment.capture|pay_processor_bad_0001"
	insertVerifiedProcessorEvent(t, db, "payment.capture", badKey, "pay_processor_bad_0001", &badAttemptID,
		map[string]any{"state": "CAPTURED", "amount_rupiah": 9999, "currency": "IDR", "payment_id": "pay_processor_bad_0001"}, now.Add(time.Minute))
	if claimed, err := processor.ProcessOne(ctx); err != nil || claimed {
		t.Fatalf("unproven mismatched capture must not be claimed: claimed=%t err=%v", claimed, err)
	}
	assertProcessorBookingPending(t, pool, ctx, badBookingID)
	var eventState string
	if err := pool.QueryRow(ctx, `SELECT processing_state FROM payment_webhook_events WHERE provider_event_key=$1`, badKey).Scan(&eventState); err != nil {
		t.Fatal(err)
	}
	if eventState != "RECEIVED" {
		t.Fatalf("unproven mismatched capture state = %s; want RECEIVED", eventState)
	}

	diagnosticBookingID := seedPaymentAttemptBooking(t, db, true)
	diagnosticAttemptID := uuid.NewString()
	insertPaymentAttempt(t, db, diagnosticAttemptID, diagnosticBookingID, 1, "payment:create:diagnostic-not-processable", "PENDING", nil, now)
	insertDiagnosticProcessorEvent(t, db, "payment.capture", "XENDIT|payment.capture|pay_processor_diagnostic_0001", "pay_processor_diagnostic_0001", &diagnosticAttemptID,
		map[string]any{"state": "CAPTURED", "amount_rupiah": 10000, "currency": "IDR", "payment_id": "pay_processor_diagnostic_0001"}, now.Add(2*time.Minute))
	if claimed, err := processor.ProcessOne(ctx); err != nil || claimed {
		t.Fatalf("diagnostic event must not be claimed: claimed=%t err=%v", claimed, err)
	}
	assertProcessorBookingPending(t, pool, ctx, diagnosticBookingID)
}

func TestGatewayFinalization_LateCaptureDoesNotPayExpiredBooking(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 30)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	now := time.Now().UTC().Truncate(time.Microsecond)
	bookingID := seedPaymentAttemptBooking(t, db, true)
	if _, err := db.Exec(`UPDATE bookings SET expires_at = $2 WHERE id = $1`, bookingID, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	attemptID := uuid.NewString()
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:late-capture", "PENDING", nil, now)

	repository := payments.NewRepository(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	result, err := repository.FinalizeGatewayCaptureTx(ctx, tx, payments.CaptureParams{
		AttemptID: attemptID, Provider: payments.ProviderXendit, ProviderEnvironment: payments.ProviderEnvironmentTest,
		ProviderPaymentID: "pay_processor_late_0001", AmountRupiah: 10000, Currency: payments.CurrencyIDR,
		CapturedAt: now, ObservedAt: now, Authority: "VERIFIED_WEBHOOK",
		SourceReference: "XENDIT|payment.capture|pay_processor_late_0001", PayloadHash: paymentHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if !result.LateCapture || result.BookingPaid {
		t.Fatalf("late finalization result = %#v", result)
	}
	assertProcessorBookingPending(t, pool, ctx, bookingID)
}

func TestPaymentWebhookProcessor_ReclaimsStaleProcessingAfterRestart(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 30)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	now := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:processor-reclaim", "PENDING", nil, now)
	key := "XENDIT|payment_session.completed|ps_processor_reclaim_0001"
	insertVerifiedProcessorEvent(t, db, "payment_session.completed", key, "ps_processor_reclaim_0001", &attemptID,
		map[string]any{"state": "PENDING", "amount_rupiah": 10000, "currency": "IDR", "payment_request_id": "pr_processor_reclaim_0001"}, now)
	if _, err := db.Exec(`UPDATE payment_webhook_events SET processing_state='PROCESSING', updated_at=transaction_timestamp() WHERE provider_event_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	recoveryProcessor, err := paymentwebhookprocessor.NewProcessorWithOptions(pool, payments.NewRepository(pool), audit.NewPlatformService(audit.NewPlatformRepository()), paymentwebhookprocessor.ProcessorOptions{ProcessingRecoveryAfter: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}

	if claimed, err := recoveryProcessor.ProcessOne(ctx); err != nil || !claimed {
		t.Fatalf("reclaim stale event: claimed=%t err=%v", claimed, err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT processing_state FROM payment_webhook_events WHERE provider_event_key=$1`, key).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "PROCESSED" {
		t.Fatalf("reclaimed event state = %s; want PROCESSED", state)
	}
	assertProcessorBookingPending(t, pool, ctx, bookingID)
}

func insertVerifiedProcessorEvent(t *testing.T, db *sql.DB, eventType, key, primaryObject string, attemptID *string, payload map[string]any, receivedAt time.Time) {
	insertProcessorEvent(t, db, "VERIFIED", eventType, key, primaryObject, attemptID, payload, receivedAt)
}

func insertDiagnosticProcessorEvent(t *testing.T, db *sql.DB, eventType, key, primaryObject string, attemptID *string, payload map[string]any, receivedAt time.Time) {
	insertProcessorEvent(t, db, "DIAGNOSTIC", eventType, key, primaryObject, attemptID, payload, receivedAt)
}

func insertProcessorEvent(t *testing.T, db *sql.DB, verification, eventType, key, primaryObject string, attemptID *string, payload map[string]any, receivedAt time.Time) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key, primary_object_id,
			raw_body_hash, auth_contract_version, verification_state, processing_state,
			redacted_payload, payment_attempt_id, correlation_id, received_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', $1, $2, $3, repeat('a', 64),
			'XENDIT_CALLBACK_TOKEN_V1_VERIFIED', $4, 'RECEIVED', $5::jsonb, $6,
			$7, $8, $8, $8)
	`, eventType, key, primaryObject, verification, string(encoded), attemptID, "webhook:processor:"+primaryObject, receivedAt); err != nil {
		t.Fatalf("insert verified processor event: %v", err)
	}
}

func assertProcessorBookingPending(t *testing.T, pool *pgxpool.Pool, ctx context.Context, bookingID string) {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id=$1`, bookingID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "PENDING_PAYMENT" {
		t.Fatalf("booking status = %s; want PENDING_PAYMENT", status)
	}
}
