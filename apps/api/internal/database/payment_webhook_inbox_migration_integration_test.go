package database_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestPaymentWebhookInboxMigration_FreshUpgradeEmptyDownAndReup(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 29)
	assertMigrationVersion(t, m, 29, false)
	assertPaymentWebhookInboxPresent(t, db, true)

	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty down migration 029 should succeed: %v", err)
	}
	assertMigrationVersion(t, m, 28, false)
	assertPaymentWebhookInboxPresent(t, db, false)

	if err := m.Migrate(29); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("upgrade from migration 028 to 029 failed: %v", err)
	}
	assertMigrationVersion(t, m, 29, false)
	assertPaymentWebhookInboxPresent(t, db, true)
}

func TestPaymentWebhookInboxMigration_ConstraintsFKAndLifecycle(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 29)

	now := time.Now().UTC().Truncate(time.Microsecond)
	firstID := insertWebhookEvent(t, db, "XENDIT|payment.capture|pay_fixture_capture_0001", "pay_fixture_capture_0001", nil, now)
	if firstID == "" {
		t.Fatal("valid synthetic webhook event did not return an ID")
	}
	assertExecFails(t, db, `
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key,
			primary_object_id, raw_body_hash, auth_contract_version, verification_state,
			processing_state, redacted_payload, correlation_id, received_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', 'payment.capture', $1, 'pay_fixture_duplicate_0001',
			repeat('b', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC',
			'RECEIVED', '{"state":"PENDING","amount_rupiah":125000,"currency":"IDR"}'::jsonb,
			'corr-webhook-duplicate-0001', $2, $2, $2)
	`, "XENDIT|payment.capture|pay_fixture_capture_0001", now)

	for _, mutation := range []string{
		`provider = 'OTHER'`, `provider_environment = 'LIVE'`, `event_type = 'refund.failed'`,
		`provider_event_key = 'XENDIT|payment.capture|different'`, `provider_event_id = 'event-fixture-1'`,
		`primary_object_id = 'different-object'`, `raw_body_hash = repeat('b', 64)`,
		`auth_contract_version = 'XENDIT_CALLBACK_TOKEN_V1_VERIFIED'`,
		`redacted_payload = '{"state":"FAILED"}'::jsonb`, `payment_attempt_id = gen_random_uuid()`,
		`correlation_id = 'different-correlation'`,
		`received_at = received_at + interval '1 second'`, `created_at = created_at + interval '1 second'`,
	} {
		assertExecFails(t, db, `UPDATE payment_webhook_events SET `+mutation+` WHERE id = $1`, firstID)
	}
	assertExecFails(t, db, `UPDATE payment_webhook_events SET processing_state = 'RECEIVED' WHERE id = $1`, firstID)
	if _, err := db.Exec(`
		UPDATE payment_webhook_events
		SET verification_state = 'VERIFIED', processing_state = 'PROCESSED',
			processed_at = updated_at + interval '1 second', updated_at = updated_at + interval '1 second'
		WHERE id = $1
	`, firstID); err != nil {
		t.Fatalf("legal webhook lifecycle transition: %v", err)
	}
	assertExecFails(t, db, `UPDATE payment_webhook_events SET updated_at = updated_at + interval '1 second' WHERE id = $1`, firstID)
	assertExecFails(t, db, `DELETE FROM payment_webhook_events WHERE id = $1`, firstID)
	assertExecFailsWithReplicaRole(t, db, `UPDATE payment_webhook_events SET raw_body_hash = repeat('c', 64) WHERE id = $1`, firstID)
	assertExecFailsWithReplicaRole(t, db, `DELETE FROM payment_webhook_events WHERE id = $1`, firstID)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:webhook-fk", "PENDING", nil, now)
	insertWebhookEvent(t, db, "XENDIT|payment.capture|pay_fixture_fk_0001", "pay_fixture_fk_0001", &attemptID, now.Add(time.Second))
	assertExecFails(t, db, `DELETE FROM payment_attempts WHERE id = $1`, attemptID)
	assertExecFails(t, db, `
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key, primary_object_id,
			raw_body_hash, auth_contract_version, verification_state, processing_state,
			redacted_payload, payment_attempt_id, correlation_id, received_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|missing-fk',
			'pay_fixture_missing_fk_0001', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL',
			'DIAGNOSTIC', 'RECEIVED', '{"state":"PENDING"}'::jsonb, $1,
			'corr-webhook-missing-fk-0001', $2, $2, $2)
	`, uuid.NewString(), now.Add(2*time.Second))

	for _, invalidInsert := range []string{
		`'OTHER', 'TEST', 'payment.capture', 'XENDIT|payment.capture|bad-provider', 'pay_bad', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED', '{"state":"PENDING"}'::jsonb, 'corr-bad-provider'`,
		`'XENDIT', 'LIVE', 'payment.capture', 'XENDIT|payment.capture|bad-environment', 'pay_bad', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED', '{"state":"PENDING"}'::jsonb, 'corr-bad-environment'`,
		`'XENDIT', 'TEST', 'payment.unrecognized', 'XENDIT|payment.unrecognized|bad-type', 'pay_bad', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED', '{"state":"PENDING"}'::jsonb, 'corr-bad-type'`,
		`'XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|bad-auth', 'pay_bad', repeat('a', 64), 'UNKNOWN', 'DIAGNOSTIC', 'RECEIVED', '{"state":"PENDING"}'::jsonb, 'corr-bad-auth'`,
		`'XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|bad-verification', 'pay_bad', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'UNKNOWN', 'RECEIVED', '{"state":"PENDING"}'::jsonb, 'corr-bad-verification'`,
		`'XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|bad-processing', 'pay_bad', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'UNKNOWN', '{"state":"PENDING"}'::jsonb, 'corr-bad-processing'`,
		`'XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|bad-primary', '', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED', '{"state":"PENDING"}'::jsonb, 'corr-bad-primary'`,
		`'XENDIT', 'TEST', 'payment.capture', 'random key with spaces', 'pay_bad', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED', '{"state":"PENDING"}'::jsonb, 'corr-bad-key'`,
		`'XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|bad-payload', 'pay_bad', repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED', '{"callback_token":"<redacted>"}'::jsonb, 'corr-bad-payload'`,
	} {
		assertExecFails(t, db, webhookInsertSQL(invalidInsert)+` , $1, $1, $1)`, now.Add(3*time.Second))
	}
	for _, badHash := range []any{"ABC", strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("a", 65), "g" + strings.Repeat("a", 63), nil} {
		assertExecFails(t, db, `
			INSERT INTO payment_webhook_events (
				provider, provider_environment, event_type, provider_event_key, primary_object_id,
				raw_body_hash, auth_contract_version, verification_state, processing_state,
				redacted_payload, correlation_id, received_at, created_at, updated_at
			) VALUES ('XENDIT', 'TEST', 'payment.capture', $1, 'pay_bad_hash', $2,
				'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED',
				'{"state":"PENDING"}'::jsonb, 'corr-bad-hash', $3, $3, $3)
		`, "XENDIT|payment.capture|bad-hash-"+uuid.NewString(), badHash, now.Add(4*time.Second))
	}
	assertExecFails(t, db, `
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key, primary_object_id,
			raw_body_hash, auth_contract_version, verification_state, processing_state,
			redacted_payload, correlation_id, received_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|invalid-json', 'pay_bad_json',
			repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED',
			'{invalid'::jsonb, 'corr-invalid-json', $1, $1, $1)
	`, now.Add(5*time.Second))
	assertExecFails(t, db, `
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key, primary_object_id,
			raw_body_hash, auth_contract_version, verification_state, processing_state,
			redacted_payload, correlation_id, received_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|null-payload', 'pay_null_payload',
			repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED',
			NULL, 'corr-null-payload', $1, $1, $1)
	`, now.Add(5*time.Second))
	assertExecFails(t, db, `
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key, primary_object_id,
			raw_body_hash, auth_contract_version, verification_state, processing_state,
			redacted_payload, correlation_id, received_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', 'payment.capture', 'XENDIT|payment.capture|bad-time', 'pay_bad_time',
			repeat('a', 64), 'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED',
			'{"state":"PENDING"}'::jsonb, 'corr-bad-time', $1 + interval '1 second', $1, $1)
	`, now.Add(6*time.Second))
	assertExecFails(t, db, `TRUNCATE payment_webhook_events`)
}

func TestPaymentWebhookInboxMigration_DownRefusesFactsAndSchemaIsSafe(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 29)
	insertWebhookEvent(t, db, "XENDIT|refund.succeeded|refund_fixture_succeeded_0001", "refund_fixture_succeeded_0001", nil, time.Now().UTC().Truncate(time.Microsecond))

	if err := m.Steps(-1); err == nil {
		t.Fatal("down migration must refuse while webhook facts exist")
	}
	var version int
	var dirty bool
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read migration state after refused down: %v", err)
	}
	if version != 28 || !dirty {
		t.Fatalf("refused down state = %d|%t; want 28|true", version, dirty)
	}
	var facts int
	if err := db.QueryRow(`SELECT count(*) FROM payment_webhook_events`).Scan(&facts); err != nil || facts != 1 {
		t.Fatalf("webhook facts after refused down = %d, err=%v; want 1", facts, err)
	}

	_, _ = m.Close()
	_ = db.Close()
	recoveryDB, recoveryM := openOutboxRecoveryMigrate(t, targetDSN)
	defer recoveryDB.Close()
	defer recoveryM.Close()
	if err := recoveryM.Force(29); err != nil {
		t.Fatalf("restore migration metadata to version 29: %v", err)
	}
	assertMigrationVersion(t, recoveryM, 29, false)

	var forbiddenColumns int
	if err := recoveryDB.QueryRow(`
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'payment_webhook_events'
		  AND (
		      lower(column_name) IN (
		          'raw_body', 'raw_payload', 'callback_token', 'authorization',
		          'signature', 'secret', 'api_key', 'pan', 'cvv', 'bank_credential',
		          'saved_payment_token', 'raw_headers', 'api_secret', 'card_pan',
		          'bank_credentials', 'saved_payment_method_token',
		          'authorization_header', 'raw_webhook_body'
		      )
		  )
	`).Scan(&forbiddenColumns); err != nil {
		t.Fatalf("scan forbidden webhook columns: %v", err)
	}
	if forbiddenColumns != 0 {
		t.Fatalf("forbidden webhook columns = %d; want 0", forbiddenColumns)
	}
	var cascades int
	if err := recoveryDB.QueryRow(`
		SELECT count(*)
		FROM pg_constraint
		WHERE conrelid = 'payment_webhook_events'::regclass
		  AND contype = 'f'
		  AND pg_get_constraintdef(oid) ILIKE '%ON DELETE CASCADE%'
	`).Scan(&cascades); err != nil {
		t.Fatalf("scan webhook FK delete actions: %v", err)
	}
	if cascades != 0 {
		t.Fatalf("webhook cascade FKs = %d; want 0", cascades)
	}
}

func TestPaymentWebhookInboxMigration_CanonicalFixtureKeysAndTypedPayload(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 29)

	manifest := loadWebhookFixtureManifest(t)
	seenKeys := make(map[string]bool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, fixture := range manifest.Fixtures {
		if len(fixture.Normalized) == 0 || seenKeys[fixture.EventKey] {
			continue
		}
		seenKeys[fixture.EventKey] = true

		payload := make(map[string]any)
		for key, value := range fixture.Normalized {
			payload[key] = value
		}
		if fixture.Reason != "NONE" {
			payload["reason_code"] = fixture.Reason
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal fixture %s normalized payload: %v", fixture.ID, err)
		}
		processingState := "RECEIVED"
		if fixture.Verification == "QUARANTINED" {
			processingState = "TERMINAL"
		}
		if err := insertWebhookEventPayload(
			db, fixture.EventType, fixture.EventKey, fixture.PrimaryObjectID,
			fixture.Verification, processingState, string(encoded), now,
		); err != nil {
			t.Fatalf("fixture %s was rejected: %v", fixture.ID, err)
		}
		now = now.Add(time.Second)
	}

	canonicalKey := "XENDIT|payment.capture|pay_fixture_canonical_0001"
	if err := insertWebhookEventPayload(
		db, "payment.capture", canonicalKey, "pay_fixture_canonical_0001",
		"DIAGNOSTIC", "RECEIVED", `{"state":"PENDING","amount_rupiah":125000,"currency":"IDR"}`, now,
	); err != nil {
		t.Fatalf("canonical IDR event rejected: %v", err)
	}
	assertWebhookPayloadRejected(t, db, "payment.capture", "550e8400-e29b-41d4-a716-446655440000", "pay_fixture_canonical_0001", "DIAGNOSTIC", `{"state":"PENDING","amount_rupiah":125000,"currency":"IDR"}`, now.Add(time.Second))
	assertWebhookPayloadRejected(t, db, "payment.capture", "XENDIT|payment.capture|alternate-random-key", "pay_fixture_canonical_0001", "DIAGNOSTIC", `{"state":"PENDING","amount_rupiah":125000,"currency":"IDR"}`, now.Add(2*time.Second))
	if err := insertWebhookEventPayload(
		db, "payment.capture", "XENDIT|payment.capture|pay_fixture_canonical_0002", "pay_fixture_canonical_0002",
		"DIAGNOSTIC", "RECEIVED", `{"state":"PENDING","amount_rupiah":125000,"currency":"IDR"}`, now.Add(3*time.Second),
	); err != nil {
		t.Fatalf("different canonical event rejected: %v", err)
	}
	assertExecFails(t, db, `
		UPDATE payment_webhook_events
		SET primary_object_id = 'pay_fixture_changed_0001'
		WHERE provider_event_key = $1
	`, canonicalKey)

	validMismatch := `{"state":"CAPTURED","amount_rupiah":125000,"currency":"USD","reason_code":"CURRENCY_MISMATCH"}`
	if err := insertWebhookEventPayload(
		db, "payment.capture", "XENDIT|payment.capture|pay_fixture_currency_quarantine_0001", "pay_fixture_currency_quarantine_0001",
		"QUARANTINED", "TERMINAL", validMismatch, now.Add(4*time.Second),
	); err != nil {
		t.Fatalf("quarantined USD currency mismatch rejected: %v", err)
	}
	var mismatchProcessingState string
	if err := db.QueryRow(`
		SELECT processing_state
		FROM payment_webhook_events
		WHERE provider_event_key = 'XENDIT|payment.capture|pay_fixture_currency_quarantine_0001'
	`).Scan(&mismatchProcessingState); err != nil {
		t.Fatalf("read quarantined USD currency mismatch processing state: %v", err)
	}
	if mismatchProcessingState != "TERMINAL" {
		t.Fatalf("quarantined USD currency mismatch processing state = %q; want TERMINAL", mismatchProcessingState)
	}
	assertWebhookPayloadRejected(t, db, "payment.capture", "XENDIT|payment.capture|pay_fixture_usd_diagnostic_0001", "pay_fixture_usd_diagnostic_0001", "DIAGNOSTIC", validMismatch, now.Add(5*time.Second))
	assertWebhookPayloadRejected(t, db, "payment.capture", "XENDIT|payment.capture|pay_fixture_usd_no_reason_0001", "pay_fixture_usd_no_reason_0001", "QUARANTINED", `{"state":"CAPTURED","amount_rupiah":125000,"currency":"USD"}`, now.Add(6*time.Second))
	assertWebhookPayloadRejected(t, db, "payment.capture", "XENDIT|payment.capture|pay_fixture_idr_mismatch_0001", "pay_fixture_idr_mismatch_0001", "QUARANTINED", `{"state":"CAPTURED","amount_rupiah":125000,"currency":"IDR","reason_code":"CURRENCY_MISMATCH"}`, now.Add(7*time.Second))

	for _, currency := range []string{"usd", "US", "USDD"} {
		assertWebhookPayloadRejected(t, db, "payment.capture", "XENDIT|payment.capture|pay_fixture_bad_currency_"+currency, "pay_fixture_bad_currency_"+currency, "DIAGNOSTIC", `{"state":"PENDING","amount_rupiah":125000,"currency":"`+currency+`"}`, now.Add(8*time.Second))
	}
	for index, payload := range []string{
		`{"state":"PENDING","amount_rupiah":"125000","currency":"IDR"}`,
		`{"state":"PENDING","amount_rupiah":125000.5,"currency":"IDR"}`,
		`{"state":"PENDING","amount_rupiah":-1,"currency":"IDR"}`,
		`{"state":"PENDING","amount_rupiah":0,"currency":"IDR"}`,
		`{"state":"PENDING","amount_rupiah":9223372036854775808,"currency":"IDR"}`,
		`{"state":"NOT_A_STATE","amount_rupiah":125000,"currency":"IDR"}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","reason_code":"NOT_A_REASON"}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","unknown":"value"}`,
		`{"state":{"raw":"PENDING"},"amount_rupiah":125000,"currency":"IDR"}`,
		`{"state":"PENDING","amount_rupiah":[125000],"currency":"IDR"}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","source_reference":"4111111111111111"}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","source_reference":"account-000001"}`,
	} {
		objectID := fmt.Sprintf("pay_fixture_invalid_payload_%d", index)
		assertWebhookPayloadRejected(t, db, "payment.capture", "XENDIT|payment.capture|"+objectID, objectID, "DIAGNOSTIC", payload, now.Add(9*time.Second))
	}

	for _, event := range []struct {
		eventType string
		state     string
		objectID  string
	}{
		{"payment_session.completed", "PENDING", "ps_fixture_minimal_completed_0001"},
		{"payment_session.expired", "EXPIRED", "ps_fixture_minimal_expired_0001"},
		{"payment.capture", "PENDING", "pay_fixture_minimal_0001"},
		{"refund.succeeded", "SUCCEEDED", "refund_fixture_minimal_succeeded_0001"},
		{"refund.failed", "FAILED", "refund_fixture_minimal_failed_0001"},
	} {
		key := "XENDIT|" + event.eventType + "|" + event.objectID
		if err := insertWebhookEventPayload(db, event.eventType, key, event.objectID, "DIAGNOSTIC", "RECEIVED", `{"state":"`+event.state+`","amount_rupiah":125000,"currency":"IDR","source_reference":"sha256:`+strings.Repeat("a", 64)+`"}`, now.Add(10*time.Second)); err != nil {
			t.Fatalf("minimal %s payload rejected: %v", event.eventType, err)
		}
	}
}

func TestPaymentWebhookInboxMigration_ProviderIdentifierTypes(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 29)

	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, valid := range []struct {
		name     string
		objectID string
		payload  string
	}{
		{
			name:     "payment_id string",
			objectID: "pay_fixture_valid_payment_id_0001",
			payload:  `{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_id":"pay_fixture_provider_id_0001"}`,
		},
		{
			name:     "payment_request_id string",
			objectID: "pay_fixture_valid_payment_request_id_0001",
			payload:  `{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_request_id":"pr_fixture_provider_id_0001"}`,
		},
	} {
		key := "XENDIT|payment.capture|" + valid.objectID
		if err := insertWebhookEventPayload(db, "payment.capture", key, valid.objectID, "DIAGNOSTIC", "RECEIVED", valid.payload, now); err != nil {
			t.Fatalf("valid %s rejected: %v", valid.name, err)
		}
		now = now.Add(time.Second)
	}

	for index, payload := range []string{
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_id":123}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_request_id":456}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_id":true}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_request_id":{"id":"pr_fixture"}}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_id":["pay_fixture"]}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_request_id":null}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_id":""}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_request_id":"` + strings.Repeat("a", 192) + `"}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_id":"pay fixture invalid"}`,
		`{"state":"PENDING","amount_rupiah":125000,"currency":"IDR","payment_request_id":"callback-token-value"}`,
	} {
		objectID := fmt.Sprintf("pay_fixture_invalid_provider_id_%d", index)
		assertWebhookPayloadRejected(t, db, "payment.capture", "XENDIT|payment.capture|"+objectID, objectID, "DIAGNOSTIC", payload, now)
		now = now.Add(time.Second)
	}
}

func webhookInsertSQL(values string) string {
	return `INSERT INTO payment_webhook_events (
		provider, provider_environment, event_type, provider_event_key, primary_object_id,
		raw_body_hash, auth_contract_version, verification_state, processing_state,
		redacted_payload, correlation_id, received_at, created_at, updated_at
	) VALUES (` + values
}

func insertWebhookEvent(t *testing.T, db *sql.DB, eventKey, primaryObjectID string, attemptID *string, now time.Time) string {
	t.Helper()
	parts := strings.SplitN(eventKey, "|", 3)
	if len(parts) != 3 {
		t.Fatalf("fixture event key is not canonical: %q", eventKey)
	}
	var id string
	err := db.QueryRow(`
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key, primary_object_id,
			raw_body_hash, auth_contract_version, verification_state, processing_state,
			redacted_payload, payment_attempt_id, correlation_id, received_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', $1, $2, $3, $4,
			'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', 'DIAGNOSTIC', 'RECEIVED',
			'{"state":"PENDING","amount_rupiah":125000,"currency":"IDR"}'::jsonb,
			$5, $6, $7, $7, $7)
		RETURNING id::text
	`, parts[1], eventKey, primaryObjectID, paymentHash, attemptID, "corr-"+strings.ReplaceAll(eventKey, "|", "-"), now).Scan(&id)
	if err != nil {
		t.Fatalf("insert valid payment webhook event: %v", err)
	}
	return id
}

type webhookFixtureManifest struct {
	Fixtures []webhookFixture `json:"fixtures"`
}

type webhookFixture struct {
	ID              string         `json:"id"`
	EventType       string         `json:"event_type"`
	EventKey        string         `json:"event_key"`
	PrimaryObjectID string         `json:"primary_object_id"`
	Verification    string         `json:"verification"`
	Reason          string         `json:"reason"`
	Normalized      map[string]any `json:"normalized"`
}

func loadWebhookFixtureManifest(t *testing.T) webhookFixtureManifest {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate webhook fixture test")
	}
	manifestPath := filepath.Join(filepath.Dir(sourceFile), "..", "payments", "testdata", "xendit_webhooks_v1", "manifest.json")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read frozen webhook manifest: %v", err)
	}
	var manifest webhookFixtureManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatalf("parse frozen webhook manifest: %v", err)
	}
	return manifest
}

func insertWebhookEventPayload(db *sql.DB, eventType, eventKey, primaryObjectID, verificationState, processingState, payload string, now time.Time) error {
	var processedAt any
	if processingState == "TERMINAL" {
		processedAt = now.Add(time.Second)
	}
	_, err := db.Exec(`
		INSERT INTO payment_webhook_events (
			provider, provider_environment, event_type, provider_event_key, primary_object_id,
			raw_body_hash, auth_contract_version, verification_state, processing_state,
			redacted_payload, correlation_id, received_at, processed_at, created_at, updated_at
		) VALUES ('XENDIT', 'TEST', $1, $2, $3, $4,
			'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL', $5, $6, $7::jsonb,
			$8, $9, $10, $9, $9)
	`, eventType, eventKey, primaryObjectID, paymentHash, verificationState, processingState, payload,
		"corr-"+strings.ReplaceAll(eventKey, "|", "-"), now, processedAt)
	return err
}

func assertWebhookPayloadRejected(t *testing.T, db *sql.DB, eventType, eventKey, primaryObjectID, verificationState, payload string, now time.Time) {
	t.Helper()
	processingState := "RECEIVED"
	if verificationState == "QUARANTINED" {
		processingState = "TERMINAL"
	}
	if err := insertWebhookEventPayload(db, eventType, eventKey, primaryObjectID, verificationState, processingState, payload, now); err == nil {
		t.Fatalf("webhook payload/key was accepted unexpectedly: event=%s key=%s payload=%s", eventType, eventKey, payload)
	}
}

func assertPaymentWebhookInboxPresent(t *testing.T, db *sql.DB, want bool) {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('public.payment_webhook_events') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("inspect payment_webhook_events presence: %v", err)
	}
	if exists != want {
		t.Fatalf("payment_webhook_events presence = %t; want %t", exists, want)
	}
}
