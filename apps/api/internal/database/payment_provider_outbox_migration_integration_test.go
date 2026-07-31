package database_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/google/uuid"
)

func TestPaymentProviderOutboxMigration_FreshUpgradeAndEmptyDown(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 26)

	assertMigrationVersion(t, m, 26, false)
	assertOutboxTablePresent(t, db, true)

	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty down migration 026 should succeed: %v", err)
	}
	assertMigrationVersion(t, m, 25, false)
	assertOutboxTablePresent(t, db, false)

	if err := m.Migrate(26); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("upgrade from migration 025 to 026 failed: %v", err)
	}
	assertMigrationVersion(t, m, 26, false)
	assertOutboxTablePresent(t, db, true)
}

func TestPaymentProviderOutboxMigration_ConstraintsAndRestrict(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 26)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:outbox-test", "PENDING", nil, now)
	payload := `{"attempt_id":"` + attemptID + `","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	hash := paymentHash
	key := "payment:create:" + bookingID + ":1"

	insertCommand := `
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_CREATE', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
	`
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":"`+attemptID+`","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS","source_reference":"xnd_development_secret"}`)
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":"`+attemptID+`","amount_rupiah":10000,"currency":"IDR","requested_method":"4111111111111111"}`)
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":"`+attemptID+`","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS","source_reference":"1234567890"}`)
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":{"id":"`+attemptID+`"},"amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`)
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":"`+attemptID+`","amount_rupiah":10000,"currency":null,"requested_method":"QRIS"}`)
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":"`+attemptID+`","amount_rupiah":10000,"currency":"IDR","requested_method":null}`)
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":"`+attemptID+`","amount_rupiah":9223372036854775808,"currency":"IDR","requested_method":"QRIS"}`)
	assertExecFails(t, db, insertCommand, attemptID, key, "ABC", payload)
	assertExecFails(t, db, insertCommand, attemptID, "payment:create:"+bookingID+":2", hash, payload)
	assertExecFails(t, db, insertCommand, attemptID, key, hash, `{"attempt_id":"`+attemptID+`","amount_rupiah":10001,"currency":"IDR","requested_method":"QRIS"}`)
	assertExecFails(t, db, `
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_INQUIRY', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
	`, attemptID, "payment:inquiry:"+attemptID, hash, payload)
	assertExecFails(t, db, `
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload, state,
			attempt_count, completed_at, provider_reference
		) VALUES (
			'PAYMENT_CREATE', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb,
			'SUCCEEDED', 1, transaction_timestamp(), ('sha256:' || repeat('a', 64))
		)
	`, attemptID, key, hash, payload)
	if _, err := db.Exec(insertCommand, attemptID, key, hash, payload); err != nil {
		t.Fatalf("insert valid provider command: %v", err)
	}
	assertExecFails(t, db, `TRUNCATE payment_provider_commands`)
	assertExecFails(t, db, `TRUNCATE payment_attempts CASCADE`)
	assertExecFailsWithReplicaRole(t, db, `TRUNCATE payment_provider_commands`)
	assertExecFailsWithReplicaRole(t, db, `DELETE FROM payment_provider_commands WHERE idempotency_key = $1`, key)
	assertExecFailsWithReplicaRole(t, db, `
		UPDATE payment_provider_commands
		SET request_hash = repeat('b', 64)
		WHERE idempotency_key = $1
	`, key)
	var alwaysTriggerCount int
	if err := db.QueryRow(`
		SELECT count(*)
		FROM pg_trigger
		WHERE tgrelid = 'payment_provider_commands'::regclass
		  AND NOT tgisinternal
		  AND tgenabled = 'A'
	`).Scan(&alwaysTriggerCount); err != nil {
		t.Fatalf("inspect always triggers: %v", err)
	}
	if alwaysTriggerCount != 3 {
		t.Fatalf("always trigger count = %d; want 3", alwaysTriggerCount)
	}
	var commandCount int
	if err := db.QueryRow(`SELECT count(*) FROM payment_provider_commands WHERE idempotency_key = $1`, key).Scan(&commandCount); err != nil {
		t.Fatalf("count command after refused truncate: %v", err)
	}
	if commandCount != 1 {
		t.Fatalf("command count after refused truncate = %d; want 1", commandCount)
	}
	assertExecFails(t, db, insertCommand, attemptID, key, hash, payload)
	assertExecFails(t, db, `
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('REFUND_CREATE', 'PAYMENT_REFUND', $1, NULL, $2, $3, $4::jsonb)
	`, uuid.NewString(), "refund:create:outbox-not-ready", hash, payload)
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET state = 'LEASED', lease_owner = 'worker-without-token',
		    lease_expires_at = transaction_timestamp() + interval '1 minute'
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET state = 'RETRYABLE', attempt_count = 1, last_error_code = 'AUTHENTICATION_FAILED'
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET state = 'RETRYABLE', attempt_count = 1, last_error_code = NULL
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET state = 'SUCCEEDED', attempt_count = 1,
		    completed_at = transaction_timestamp(),
		    provider_reference = ('sha256:' || repeat('a', 64)),
		    updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET state = 'LEASED', attempt_count = 1,
		    lease_owner = 'worker:xnd_development_secret',
		    lease_token = gen_random_uuid(),
		    lease_expires_at = transaction_timestamp() + interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`, key)

	workerOwner := "worker:" + uuid.NewString()
	primaryKeyMutation := `
		UPDATE payment_provider_commands
		SET id = gen_random_uuid(),
		    state = 'LEASED', attempt_count = 1, lease_owner = $2,
		    lease_token = gen_random_uuid(),
		    lease_expires_at = transaction_timestamp() + interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`
	assertExecFails(t, db, primaryKeyMutation, key, workerOwner)
	assertExecFailsWithReplicaRole(t, db, primaryKeyMutation, key, workerOwner)

	if _, err := db.Exec(`
		UPDATE payment_provider_commands
		SET state = 'LEASED', attempt_count = 1, lease_owner = $2,
		    lease_token = gen_random_uuid(),
		    lease_expires_at = transaction_timestamp() + interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`, key, workerOwner); err != nil {
		t.Fatalf("legal pending-to-leased transition: %v", err)
	}
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET state = 'PENDING', lease_owner = NULL, lease_token = NULL,
		    lease_expires_at = NULL, updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `DELETE FROM payment_provider_commands WHERE idempotency_key = $1`, key)
	for _, unsafeReference := range []string{
		"4111111111111111",
		"ref-4111-1111-1111-1111",
		"ref_1234567890",
		"sk_test_abc123",
		"xnd_development_secret",
		"https://provider.example/reference",
		"provider_ref_4111111111111111",
		"sha256:" + strings.Repeat("a", 63),
		"sha256:" + strings.Repeat("A", 64),
	} {
		assertExecFails(t, db, `
			UPDATE payment_provider_commands
			SET state = 'SUCCEEDED', completed_at = transaction_timestamp(),
			    provider_reference = $2,
			    lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
			    last_error_code = NULL, updated_at = transaction_timestamp()
			WHERE idempotency_key = $1
		`, key, unsafeReference)
	}
	safeProviderReference := "sha256:" + strings.Repeat("a", 64)
	if _, err := db.Exec(`
		UPDATE payment_provider_commands
		SET state = 'SUCCEEDED', completed_at = transaction_timestamp(),
		    provider_reference = $2,
		    lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
		    last_error_code = NULL, updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`, key, safeProviderReference); err != nil {
		t.Fatalf("legal leased-to-succeeded transition: %v", err)
	}
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET state = 'PENDING', completed_at = NULL, provider_reference = NULL,
		    updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET updated_at = transaction_timestamp()
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `DELETE FROM payment_provider_commands WHERE idempotency_key = $1`, key)
	assertExecFails(t, db, `
		UPDATE payment_provider_commands
		SET request_hash = repeat('b', 64)
		WHERE idempotency_key = $1
	`, key)
	assertExecFails(t, db, `DELETE FROM payment_attempts WHERE id = $1`, attemptID)

	if err := m.Steps(-1); err == nil {
		t.Fatal("down migration must refuse while provider command exists")
	}
	_, _ = m.Close()
	_ = db.Close()

	recoveryDB, recoveryM := openOutboxRecoveryMigrate(t, targetDSN)
	defer recoveryDB.Close()
	defer recoveryM.Close()
	assertMigrationVersion(t, recoveryM, 25, true)
	var preserved int
	if err := recoveryDB.QueryRow(`SELECT count(*) FROM payment_provider_commands WHERE idempotency_key = $1`, key).Scan(&preserved); err != nil {
		t.Fatalf("read preserved command after refused rollback: %v", err)
	}
	if preserved != 1 {
		t.Fatalf("preserved command count after refused rollback = %d; want 1", preserved)
	}
	if err := recoveryM.Force(26); err != nil {
		t.Fatalf("restore migration metadata to preserved schema version 26: %v", err)
	}
	assertMigrationVersion(t, recoveryM, 26, false)
	assertOutboxTablePresent(t, recoveryDB, true)
}

func openOutboxRecoveryMigrate(t *testing.T, targetDSN string) (*sql.DB, *migrate.Migrate) {
	t.Helper()
	db, err := sql.Open("postgres", targetDSN)
	if err != nil {
		t.Fatalf("open outbox recovery database: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping outbox recovery database: %v", err)
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		db.Close()
		t.Fatalf("create outbox recovery migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../../../db/migrations", "postgres", driver)
	if err != nil {
		db.Close()
		t.Fatalf("create outbox recovery migration instance: %v", err)
	}
	return db, m
}

func assertOutboxTablePresent(t *testing.T, db *sql.DB, want bool) {
	t.Helper()
	var present bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'payment_provider_commands'
		)
	`).Scan(&present); err != nil {
		t.Fatalf("failed to inspect payment provider outbox table: %v", err)
	}
	if present != want {
		t.Fatalf("payment_provider_commands present = %v; want %v", present, want)
	}
}

func assertExecFailsWithReplicaRole(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin replica-role assertion: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("set local replica role: %v", err)
	}
	if _, err := tx.Exec(query, args...); err == nil {
		t.Fatalf("expected statement to fail with replica role: %s", query)
	}
}
