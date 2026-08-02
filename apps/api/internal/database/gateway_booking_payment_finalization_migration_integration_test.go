package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestGatewayBookingPaymentFinalizationMigration_FreshUpgradeAndSafeDown(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 30)
	assertMigrationVersion(t, m, 30, false)

	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty migration 030 down: %v", err)
	}
	assertMigrationVersion(t, m, 29, false)
	if err := m.Migrate(30); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("upgrade migration 030: %v", err)
	}
	assertMigrationVersion(t, m, 30, false)
}

func TestGatewayBookingPaymentFinalizationMigration_RejectsLegacyButAllowsCapturedAtomicPath(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 30)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:gateway-finalization", "PENDING", nil, now)
	assertExecFails(t, db, `UPDATE bookings SET status = 'PAID' WHERE id = $1`, bookingID)
	assertExecFails(t, db, `UPDATE bookings SET status = 'CONFIRMED' WHERE id = $1`, bookingID)

	var customerID string
	if err := db.QueryRow(`SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	capturedAt := now.Add(time.Minute)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE payment_attempts
		SET state = 'CAPTURED', captured_at = $2, provider_payment_id = 'pay_gateway_finalization_0001', updated_at = $2
		WHERE id = $1
	`, attemptID, capturedAt); err != nil {
		t.Fatalf("stage capture: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO payment_capture_facts (
			id, payment_attempt_id, provider, provider_environment, provider_payment_id,
			amount_rupiah, currency, captured_at, observed_at, authority, source_reference, payload_hash
		) VALUES ($1, $2, 'XENDIT', 'TEST', 'pay_gateway_finalization_0001',
			10000, 'IDR', $3, $3, 'VERIFIED_WEBHOOK', 'XENDIT|payment.capture|pay_gateway_finalization_0001', $4)
	`, uuid.NewString(), attemptID, capturedAt, paymentHash); err != nil {
		t.Fatalf("stage capture fact: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO platform_audit_logs (
			actor_user_id, actor_role, action, entity_type, entity_id, correlation_id, metadata
		) VALUES ($1, 'SYSTEM', 'payment_state_transition', 'PAYMENT_ATTEMPT', $2, $3,
			'{"from_state":"PENDING","to_state":"CAPTURED","attempt_no":1,"late_capture":false}'::jsonb)
	`, customerID, attemptID, "webhook:gateway-finalization-0001"); err != nil {
		t.Fatalf("stage capture audit: %v", err)
	}
	if _, err := tx.Exec(`UPDATE bookings SET status = 'PAID' WHERE id = $1`, bookingID); err != nil {
		t.Fatalf("gateway paid transition: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit gateway finalization: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM bookings WHERE id = $1`, bookingID).Scan(&status); err != nil || status != "PAID" {
		t.Fatalf("booking status = %q, err=%v; want PAID", status, err)
	}
}

func TestGatewayBookingPaymentFinalizationMigration_DownRefusesGatewayPaidFacts(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 30)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	capturedAt := now.Add(time.Minute)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:gateway-down-refusal", "CAPTURED", &capturedAt, now)
	if _, err := db.Exec(`
		INSERT INTO payment_capture_facts (
			id, payment_attempt_id, provider, provider_environment, provider_payment_id,
			amount_rupiah, currency, captured_at, observed_at, authority, source_reference, payload_hash
		) VALUES ($1, $2, 'XENDIT', 'TEST', 'pay_gateway_down_0001', 10000, 'IDR',
			$3, $3, 'AUTHENTICATED_INQUIRY', 'payment:inquiry:gateway-down', $4)
	`, uuid.NewString(), attemptID, capturedAt, paymentHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE bookings SET status = 'PAID' WHERE id = $1`, bookingID); err == nil {
		t.Fatal("direct PAID update must remain blocked without capture audit")
	}

	// The refusal contract is exercised with a committed gateway-paid row. The
	// direct update remains blocked, so seed the audit and final state in one
	// explicit transaction exactly as the gateway repository does.
	var customerID string
	if err := db.QueryRow(`SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO platform_audit_logs (actor_user_id, actor_role, action, entity_type, entity_id, correlation_id, metadata)
		VALUES ($1, 'SYSTEM', 'payment_state_transition', 'PAYMENT_ATTEMPT', $2, $3,
		'{"from_state":"PENDING","to_state":"CAPTURED","attempt_no":1,"late_capture":false}'::jsonb)
	`, customerID, attemptID, "webhook:gateway-down-0001"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE bookings SET status = 'PAID' WHERE id = $1`, bookingID); err != nil {
		t.Fatal(err)
	}

	if err := m.Steps(-1); err == nil {
		t.Fatal("migration 030 down must refuse gateway-paid facts")
	}
	var version int
	var dirty bool
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != 29 || !dirty {
		t.Fatalf("refused 030 down state = %d|%t; want 29|true", version, dirty)
	}
}

var _ = sql.ErrNoRows
