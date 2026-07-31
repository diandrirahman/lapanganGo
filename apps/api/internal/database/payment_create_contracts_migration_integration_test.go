package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestPaymentCreateContractsMigration_FreshUpgradeAndEmptyDown(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 27)

	assertMigrationVersion(t, m, 27, false)
	assertPaymentCreateContractsTablePresent(t, db, true)

	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty down migration 027 should succeed: %v", err)
	}
	assertMigrationVersion(t, m, 26, false)
	assertPaymentCreateContractsTablePresent(t, db, false)

	if err := m.Migrate(27); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("upgrade from migration 026 to 027 failed: %v", err)
	}
	assertMigrationVersion(t, m, 27, false)
	assertPaymentCreateContractsTablePresent(t, db, true)
}

func TestPaymentCreateContractsMigration_ExactMatchAndImmutability(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 27)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	reference := "pa-" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	now := time.Now().UTC().Truncate(time.Microsecond)
	requestedExpiry := now.Add(time.Hour)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, reference, "CREATED", nil, now)

	successURL := "https://demo.example.test/payments/return/" + reference + "/success"
	cancelURL := "https://demo.example.test/payments/return/" + reference + "/cancel"
	insertContract := `
		INSERT INTO payment_create_contracts (
			payment_attempt_id, request_hash, requested_expires_at,
			success_return_url, cancel_return_url
		) VALUES ($1, $2, $3, $4, $5)
	`
	assertExecFails(t, db, insertContract, attemptID, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", requestedExpiry, successURL, cancelURL)
	assertExecFails(t, db, insertContract, attemptID, paymentHash, requestedExpiry.Add(time.Second), successURL, cancelURL)
	assertExecFails(t, db, insertContract, attemptID, paymentHash, requestedExpiry, "https://demo.example.test/payments/return/other/success", cancelURL)
	assertExecFails(t, db, insertContract, attemptID, paymentHash, requestedExpiry, successURL, "https://other.example.test/payments/return/"+reference+"/cancel")
	assertExecFails(t, db, insertContract, uuid.NewString(), paymentHash, requestedExpiry, successURL, cancelURL)
	assertExecFails(t, db, insertContract, attemptID, paymentHash, requestedExpiry,
		"https://-/payments/return/"+reference+"/success",
		"https://-/payments/return/"+reference+"/cancel")
	assertExecFails(t, db, insertContract, attemptID, paymentHash, requestedExpiry,
		"https://..example/payments/return/"+reference+"/success",
		"https://..example/payments/return/"+reference+"/cancel")
	assertExecFails(t, db, insertContract, attemptID, paymentHash, requestedExpiry,
		"https://demo.example.test:99999/payments/return/"+reference+"/success",
		"https://demo.example.test:99999/payments/return/"+reference+"/cancel")
	assertExecFails(t, db, insertContract, attemptID, paymentHash, requestedExpiry,
		"https://demo.example.test:00080/payments/return/"+reference+"/success",
		"https://demo.example.test:00080/payments/return/"+reference+"/cancel")

	if _, err := db.Exec(insertContract, attemptID, paymentHash, requestedExpiry, successURL, cancelURL); err != nil {
		t.Fatalf("insert valid payment create contract: %v", err)
	}
	assertExecFails(t, db, `UPDATE payment_create_contracts SET requested_expires_at = requested_expires_at + interval '1 second' WHERE payment_attempt_id = $1`, attemptID)
	assertExecFails(t, db, `DELETE FROM payment_create_contracts WHERE payment_attempt_id = $1`, attemptID)
	assertExecFails(t, db, `TRUNCATE payment_create_contracts`)
	assertExecFailsWithReplicaRole(t, db, `UPDATE payment_create_contracts SET request_hash = repeat('b', 64) WHERE payment_attempt_id = $1`, attemptID)
	assertExecFailsWithReplicaRole(t, db, `DELETE FROM payment_create_contracts WHERE payment_attempt_id = $1`, attemptID)
	assertExecFailsWithReplicaRole(t, db, `TRUNCATE payment_create_contracts`)
	assertExecFails(t, db, `TRUNCATE payment_attempts CASCADE`)
	assertExecFailsWithReplicaRole(t, db, `TRUNCATE payment_attempts CASCADE`)

	if _, err := db.Exec(`
		UPDATE payment_attempts
		SET expires_at = expires_at - interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE id = $1
	`, attemptID); err != nil {
		t.Fatalf("provider result expiry remains independently mutable: %v", err)
	}
	var storedExpiry time.Time
	if err := db.QueryRow(`
		SELECT requested_expires_at
		FROM payment_create_contracts
		WHERE payment_attempt_id = $1
	`, attemptID).Scan(&storedExpiry); err != nil {
		t.Fatalf("read immutable requested expiry: %v", err)
	}
	if !storedExpiry.Equal(requestedExpiry) {
		t.Fatalf("requested expiry changed to %s; want %s", storedExpiry, requestedExpiry)
	}

	var alwaysTriggerCount int
	if err := db.QueryRow(`
		SELECT count(*)
		FROM pg_trigger
		WHERE tgrelid = 'payment_create_contracts'::regclass
		  AND NOT tgisinternal
		  AND tgenabled = 'A'
	`).Scan(&alwaysTriggerCount); err != nil {
		t.Fatalf("inspect payment create contract triggers: %v", err)
	}
	if alwaysTriggerCount != 3 {
		t.Fatalf("always trigger count = %d; want 3", alwaysTriggerCount)
	}
	assertExecFails(t, db, `DELETE FROM payment_attempts WHERE id = $1`, attemptID)
}

func TestPaymentCreateContractsMigration_DownRefusesImmutableRows(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()
	migrateToVersion(t, m, 27)

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	reference := "pa-" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, reference, "CREATED", nil, now)
	if _, err := db.Exec(`
		INSERT INTO payment_create_contracts (
			payment_attempt_id, request_hash, requested_expires_at,
			success_return_url, cancel_return_url
		) VALUES ($1, $2, $3, $4, $5)
	`, attemptID, paymentHash, now.Add(time.Hour),
		"https://demo.example.test/payments/return/"+reference+"/success",
		"https://demo.example.test/payments/return/"+reference+"/cancel"); err != nil {
		t.Fatalf("insert contract before refused down: %v", err)
	}

	if err := m.Steps(-1); err == nil {
		t.Fatal("down migration must refuse while immutable payment create contracts exist")
	}
	var preserved int
	if err := db.QueryRow(`SELECT count(*) FROM payment_create_contracts WHERE payment_attempt_id = $1`, attemptID).Scan(&preserved); err != nil {
		t.Fatalf("read preserved contract after refused down: %v", err)
	}
	if preserved != 1 {
		t.Fatalf("preserved contract count = %d; want 1", preserved)
	}
	var version int
	var dirty bool
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read migration state after refused down: %v", err)
	}
	if version != 26 || !dirty {
		t.Fatalf("refused down migration state = %d|%t; want 26|true", version, dirty)
	}
	_, _ = m.Close()
	_ = db.Close()

	recoveryDB, recoveryM := openOutboxRecoveryMigrate(t, targetDSN)
	defer recoveryDB.Close()
	defer recoveryM.Close()
	if err := recoveryM.Force(27); err != nil {
		t.Fatalf("restore migration metadata to preserved schema version 27: %v", err)
	}
	assertMigrationVersion(t, recoveryM, 27, false)
	assertPaymentCreateContractsTablePresent(t, recoveryDB, true)
}

func assertPaymentCreateContractsTablePresent(t *testing.T, db *sql.DB, want bool) {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('public.payment_create_contracts') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("inspect payment_create_contracts presence: %v", err)
	}
	if exists != want {
		t.Fatalf("payment_create_contracts presence = %t; want %t", exists, want)
	}
}
