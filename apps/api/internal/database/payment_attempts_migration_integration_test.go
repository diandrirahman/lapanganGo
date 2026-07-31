package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

const paymentHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestPaymentAttemptsMigration_FreshUpgradeAndEmptyDown(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 26)

	assertMigrationVersion(t, m, 26, false)
	assertPaymentTablesPresent(t, db, true)

	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty down migration 026 should succeed: %v", err)
	}
	assertMigrationVersion(t, m, 25, false)
	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty down migration 025 should succeed: %v", err)
	}
	assertMigrationVersion(t, m, 24, false)
	assertPaymentTablesPresent(t, db, false)

	if err := m.Migrate(26); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("upgrade from migration 024 to 026 failed: %v", err)
	}
	assertMigrationVersion(t, m, 26, false)
	assertPaymentTablesPresent(t, db, true)
}

func TestPaymentAttemptsMigration_ConstraintsAndImmutability(t *testing.T) {
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
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, "payment:create:attempt-one", "PENDING", nil, now)

	assertExecFails(t, db, `
		INSERT INTO payment_attempts (
			id, booking_id, attempt_no, provider, provider_environment, requested_method,
			integration_mode, capture_method, state, currency, amount_rupiah,
			local_reference, request_hash, expires_at
		) VALUES ($1, $2, 2, 'XENDIT', 'TEST', 'QRIS', 'PAYMENT_LINK', 'AUTOMATIC',
			'PENDING', 'USD', 10000, 'payment:create:invalid-currency', $3, $4)
	`, uuid.NewString(), bookingID, paymentHash, now.Add(time.Hour))

	assertExecFails(t, db, `
		INSERT INTO payment_attempts (
			id, booking_id, attempt_no, provider, provider_environment, requested_method,
			integration_mode, capture_method, state, currency, amount_rupiah,
			local_reference, request_hash, expires_at
		) VALUES ($1, $2, 2, 'XENDIT', 'TEST', 'QRIS', 'PAYMENT_LINK', 'AUTOMATIC',
			'PENDING', 'IDR', 0, 'payment:create:zero', $3, $4)
	`, uuid.NewString(), bookingID, paymentHash, now.Add(time.Hour))

	assertExecFails(t, db, `
		INSERT INTO payment_attempts (
			id, booking_id, attempt_no, provider, provider_environment, requested_method,
			integration_mode, capture_method, state, currency, amount_rupiah,
			local_reference, request_hash, expires_at
		) VALUES ($1, $2, 1, 'XENDIT', 'TEST', 'QRIS', 'PAYMENT_LINK', 'AUTOMATIC',
			'PENDING', 'IDR', 10000, 'payment:create:duplicate-attempt', $3, $4)
	`, uuid.NewString(), bookingID, paymentHash, now.Add(time.Hour))

	bookingWithoutSnapshotID := seedPaymentAttemptBooking(t, db, false)
	assertExecFails(t, db, `
		INSERT INTO payment_attempts (
			id, booking_id, attempt_no, provider, provider_environment, requested_method,
			integration_mode, capture_method, state, currency, amount_rupiah,
			local_reference, request_hash, expires_at
		) VALUES ($1, $2, 1, 'XENDIT', 'TEST', 'QRIS', 'PAYMENT_LINK', 'AUTOMATIC',
			'PENDING', 'IDR', 10000, 'payment:create:no-snapshot', $3, $4)
	`, uuid.NewString(), bookingWithoutSnapshotID, paymentHash, now.Add(time.Hour))

	capturedAt := now.Add(2 * time.Minute)
	if _, err := db.Exec(`
		UPDATE payment_attempts
		SET state = 'CAPTURED', captured_at = $2, provider_payment_id = 'test-payment-1', updated_at = $2
		WHERE id = $1
	`, attemptID, capturedAt); err != nil {
		t.Fatalf("failed to set first attempt captured: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO payment_capture_facts (
			id, payment_attempt_id, provider, provider_environment, provider_payment_id,
			amount_rupiah, currency, captured_at, observed_at, authority, source_reference, payload_hash
		) VALUES ($1, $2, 'XENDIT', 'TEST', 'test-payment-1', 10000, 'IDR', $3, $4,
			'AUTHENTICATED_INQUIRY', 'inquiry:test-payment-1', $5)
	`, uuid.NewString(), attemptID, capturedAt, capturedAt.Add(time.Second), paymentHash); err != nil {
		t.Fatalf("failed to insert matching immutable capture fact: %v", err)
	}

	assertExecFails(t, db, `UPDATE payment_attempts SET captured_at = captured_at + interval '1 second' WHERE id = $1`, attemptID)
	assertExecFails(t, db, `UPDATE payment_capture_facts SET source_reference = 'changed' WHERE payment_attempt_id = $1`, attemptID)
	assertExecFails(t, db, `DELETE FROM payment_capture_facts WHERE payment_attempt_id = $1`, attemptID)

	secondAttemptID := uuid.NewString()
	insertPaymentAttempt(t, db, secondAttemptID, bookingID, 2, "payment:create:attempt-two", "PENDING", nil, now)
	assertExecFails(t, db, `
		UPDATE payment_attempts
		SET state = 'CAPTURED', captured_at = $2, provider_payment_id = 'test-payment-2', updated_at = $2
		WHERE id = $1
	`, secondAttemptID, capturedAt.Add(time.Minute))

	otherBookingID := seedPaymentAttemptBooking(t, db, true)
	otherAttemptID := uuid.NewString()
	otherCapturedAt := capturedAt.Add(2 * time.Minute)
	insertPaymentAttempt(t, db, otherAttemptID, otherBookingID, 1, "payment:create:other", "CAPTURED", &otherCapturedAt, now)
	assertExecFails(t, db, `
		INSERT INTO payment_capture_facts (
			id, payment_attempt_id, provider, provider_environment, provider_payment_id,
			amount_rupiah, currency, captured_at, observed_at, authority, source_reference, payload_hash
		) VALUES ($1, $2, 'XENDIT', 'TEST', 'test-payment-2', 9999, 'IDR', $3, $4,
			'VERIFIED_WEBHOOK', 'webhook:test-payment-2', $5)
	`, uuid.NewString(), otherAttemptID, otherCapturedAt, otherCapturedAt.Add(time.Second), paymentHash)

	assertExecFails(t, db, `
		INSERT INTO payment_capture_facts (
			id, payment_attempt_id, provider, provider_environment, provider_payment_id,
			amount_rupiah, currency, captured_at, observed_at, authority, source_reference, payload_hash
		) VALUES ($1, $2, 'XENDIT', 'TEST', 'test-payment-1', 10000, 'IDR', $3, $4,
			'VERIFIED_WEBHOOK', 'webhook:test-payment-1-duplicate', $5)
	`, uuid.NewString(), otherAttemptID, otherCapturedAt, otherCapturedAt.Add(time.Second), paymentHash)
}

func TestPaymentAttemptsMigration_DownRefusesExistingAttempts(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 26)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	insertPaymentAttempt(t, db, uuid.NewString(), bookingID, 1, "payment:create:down-refusal", "PENDING", nil, time.Now().UTC())

	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty down migration 026 should succeed before testing 025 refusal: %v", err)
	}
	assertMigrationVersion(t, m, 25, false)
	if err := m.Steps(-1); err == nil {
		t.Fatal("down migration must refuse when a payment attempt exists")
	}

	var attempts int
	if err := db.QueryRow(`SELECT count(*) FROM payment_attempts`).Scan(&attempts); err != nil {
		t.Fatalf("failed to count attempts after refused down migration: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("refused down migration changed payment attempts: got %d rows", attempts)
	}
}

func seedPaymentAttemptBooking(t *testing.T, db *sql.DB, withSnapshot bool) string {
	t.Helper()
	customerID := uuid.NewString()
	ownerUserID := uuid.NewString()
	ownerProfileID := uuid.NewString()
	venueID := uuid.NewString()
	courtID := uuid.NewString()
	bookingID := uuid.NewString()
	suffix := uuid.NewString()

	if _, err := db.Exec(`
		INSERT INTO users (id, name, email, password_hash, role, status)
		VALUES ($1, 'payment customer', $2, 'hash', 'CUSTOMER', 'ACTIVE'),
		       ($3, 'payment owner', $4, 'hash', 'OWNER', 'ACTIVE')
	`, customerID, "customer-"+suffix+"@example.test", ownerUserID, "owner-"+suffix+"@example.test"); err != nil {
		t.Fatalf("failed to seed payment users: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO owner_profiles (id, user_id, business_name, verification_status)
		VALUES ($1, $2, 'Payment Test Owner', 'APPROVED')
	`, ownerProfileID, ownerUserID); err != nil {
		t.Fatalf("failed to seed payment owner profile: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO venues (id, owner_profile_id, name, address, city, status)
		VALUES ($1, $2, 'Payment Test Venue', 'Test address', 'Jakarta', 'ACTIVE')
	`, venueID, ownerProfileID); err != nil {
		t.Fatalf("failed to seed payment venue: %v", err)
	}
	var sportID string
	if err := db.QueryRow(`SELECT id FROM sports WHERE name = 'Futsal' LIMIT 1`).Scan(&sportID); err != nil {
		t.Fatalf("failed to find seeded Futsal sport: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, status)
		VALUES ($1, $2, $3, 'Payment Test Court', 'INDOOR', 10000, 'ACTIVE')
	`, courtID, venueID, sportID); err != nil {
		t.Fatalf("failed to seed payment court: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, total_price, status)
		VALUES ($1, $2, $3, CURRENT_DATE + 1, '10:00', '11:00', 10000, 'PENDING_PAYMENT')
	`, bookingID, customerID, courtID); err != nil {
		t.Fatalf("failed to seed payment booking: %v", err)
	}
	if !withSnapshot {
		return bookingID
	}

	var termID string
	if err := db.QueryRow(`SELECT id FROM platform_commercial_terms WHERE owner_profile_id IS NULL LIMIT 1`).Scan(&termID); err != nil {
		t.Fatalf("failed to find global commercial term: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO booking_fee_snapshots (
			booking_id, owner_profile_id, venue_id, commercial_term_id, terms_source,
			booking_channel, finance_mode, original_price_rupiah, owner_price_adjustment_rupiah,
			final_booking_price_rupiah, customer_charge_amount_rupiah,
			commission_basis_amount_rupiah, commission_bps, commission_amount_rupiah,
			owner_net_amount_rupiah, calculation_version
		) VALUES ($1, $2, $3, $4, 'POLICY', 'MARKETPLACE_ONLINE', 'SIMULATION',
			10000, 0, 10000, 10000, 10000, 700, 700, 9300, 'PAYMENT_TEST_V1')
	`, bookingID, ownerProfileID, venueID, termID); err != nil {
		t.Fatalf("failed to seed payment booking snapshot: %v", err)
	}

	return bookingID
}

func insertPaymentAttempt(t *testing.T, db *sql.DB, attemptID, bookingID string, attemptNo int, reference, state string, capturedAt *time.Time, now time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO payment_attempts (
			id, booking_id, attempt_no, provider, provider_environment, requested_method,
			integration_mode, capture_method, state, currency, amount_rupiah,
			local_reference, request_hash, expires_at, captured_at, created_at, updated_at
		) VALUES ($1, $2, $3, 'XENDIT', 'TEST', 'QRIS', 'PAYMENT_LINK', 'AUTOMATIC',
			$4, 'IDR', 10000, $5, $6, $7, $8, $9, $9)
	`, attemptID, bookingID, attemptNo, state, reference, paymentHash, now.Add(time.Hour), capturedAt, now)
	if err != nil {
		t.Fatalf("failed to insert payment attempt: %v", err)
	}
}

func assertMigrationVersion(t *testing.T, m *migrate.Migrate, wantVersion uint, wantDirty bool) {
	t.Helper()
	version, dirty, err := m.Version()
	if err != nil {
		t.Fatalf("failed to read migration version: %v", err)
	}
	if version != wantVersion || dirty != wantDirty {
		t.Fatalf("unexpected migration state: got %d|%t, want %d|%t", version, dirty, wantVersion, wantDirty)
	}
}

func migrateToVersion(t *testing.T, m *migrate.Migrate, version uint) {
	t.Helper()
	if err := m.Migrate(version); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate to version %d: %v", version, err)
	}
}

func assertPaymentTablesPresent(t *testing.T, db *sql.DB, want bool) {
	t.Helper()
	for _, table := range []string{"payment_attempts", "payment_capture_facts"} {
		var exists bool
		if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
			t.Fatalf("failed to query %s presence: %v", table, err)
		}
		if exists != want {
			t.Fatalf("unexpected table presence for %s: got %t, want %t", table, exists, want)
		}
	}
}

func assertExecFails(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err == nil {
		t.Fatalf("expected SQL statement to fail")
	}
}
