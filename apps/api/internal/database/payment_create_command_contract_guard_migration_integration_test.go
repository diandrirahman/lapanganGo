package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestPaymentCreateCommandContractGuardMigration_FreshUpgradeAndEmptyDown(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()

	assertMigrationVersion(t, m, 28, false)
	assertPaymentCreateCommandContractGuardPresent(t, db, true)

	if err := m.Steps(-1); err != nil {
		t.Fatalf("empty down migration 028 should succeed: %v", err)
	}
	assertMigrationVersion(t, m, 27, false)
	assertPaymentCreateCommandContractGuardPresent(t, db, false)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("upgrade from migration 027 to 028 failed: %v", err)
	}
	assertMigrationVersion(t, m, 28, false)
	assertPaymentCreateCommandContractGuardPresent(t, db, true)
}

func TestPaymentCreateCommandContractGuardMigration_RequiresExactContract(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	reference := "pa-" + "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, reference, "CREATED", nil, now)

	payload := `{"attempt_id":"` + attemptID + `","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	insertCommand := `
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_CREATE', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
	`
	commandKey := "payment:create:" + bookingID + ":1"
	assertExecFails(t, db, insertCommand, attemptID, commandKey, paymentHash, payload)
	assertExecFailsWithReplicaRole(t, db, insertCommand, attemptID, commandKey, paymentHash, payload)

	insertPaymentCreateContractFixture(t, db, attemptID, reference, now.Add(time.Hour))
	if _, err := db.Exec(insertCommand, attemptID, commandKey, paymentHash, payload); err != nil {
		t.Fatalf("insert command with matching immutable create contract: %v", err)
	}

	assertPaymentCreateCommandContractGuardPresent(t, db, true)
}

func TestPaymentCreateCommandContractGuardMigration_ReservesPaymentInquiry(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	reference := "pa-" + "abababababababababababababababababababababababababababababab"
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, reference, "CREATED", nil, now)
	payload := `{"attempt_id":"` + attemptID + `","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	insertInquiry := `
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_INQUIRY', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
	`
	assertExecFails(
		t,
		db,
		insertInquiry,
		attemptID,
		"payment:inquiry:"+bookingID,
		paymentHash,
		payload,
	)
	assertExecFails(
		t,
		db,
		insertInquiry,
		attemptID,
		"payment:inquiry:"+attemptID,
		paymentHash,
		payload,
	)
	if _, err := db.Exec(`UPDATE payment_attempts SET state = 'PENDING' WHERE id = $1`, attemptID); err != nil {
		t.Fatalf("mark payment attempt uncertain before inquiry: %v", err)
	}
	var commandID string
	if err := db.QueryRow(
		insertInquiry+" RETURNING id::text",
		attemptID,
		"payment:inquiry:"+attemptID,
		paymentHash,
		payload,
	).Scan(&commandID); err != nil {
		t.Fatalf("insert reserved payment inquiry command: %v", err)
	}

	leaseOwner := "worker:" + uuid.NewString()
	leaseToken := uuid.NewString()
	if _, err := db.Exec(`
		UPDATE payment_provider_commands
		SET state = 'LEASED',
		    attempt_count = 1,
		    lease_owner = $2,
		    lease_token = $3,
		    lease_expires_at = transaction_timestamp() + interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE id = $1
	`, commandID, leaseOwner, leaseToken); err != nil {
		t.Fatalf("lease payment inquiry command: %v", err)
	}
	if _, err := db.Exec(`UPDATE payment_attempts SET state = 'CANCELLED' WHERE id = $1`, attemptID); err != nil {
		t.Fatalf("race payment attempt to terminal state: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE payment_provider_commands
		SET state = 'SUCCEEDED',
		    provider_reference = $4,
		    lease_owner = NULL,
		    lease_token = NULL,
		    lease_expires_at = NULL,
		    last_error_code = NULL,
		    completed_at = transaction_timestamp(),
		    updated_at = transaction_timestamp()
		WHERE id = $1
		  AND state = 'LEASED'
		  AND lease_owner = $2
		  AND lease_token = $3
	`, commandID, leaseOwner, leaseToken, "sha256:"+paymentHash); err != nil {
		t.Fatalf("finish leased inquiry after terminal race: %v", err)
	}
}

func TestPaymentCreateCommandContractGuardMigration_DownRefusesCreateCommands(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	reference := "pa-" + "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, reference, "CREATED", nil, now)
	insertPaymentCreateContractFixture(t, db, attemptID, reference, now.Add(time.Hour))
	payload := `{"attempt_id":"` + attemptID + `","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	if _, err := db.Exec(`
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_CREATE', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
	`, attemptID, "payment:create:"+bookingID+":1", paymentHash, payload); err != nil {
		t.Fatalf("insert command before refused guard down: %v", err)
	}

	if err := m.Steps(-1); err == nil {
		t.Fatal("down migration must refuse while payment create commands exist")
	}
	var version int
	var dirty bool
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read migration state after refused guard down: %v", err)
	}
	if version != 27 || !dirty {
		t.Fatalf("refused guard down state = %d|%t; want 27|true", version, dirty)
	}
	assertPaymentCreateCommandContractGuardPresent(t, db, true)

	_, _ = m.Close()
	_ = db.Close()
	recoveryDB, recoveryM := openOutboxRecoveryMigrate(t, targetDSN)
	defer recoveryDB.Close()
	defer recoveryM.Close()
	if err := recoveryM.Force(28); err != nil {
		t.Fatalf("restore migration metadata to preserved schema version 28: %v", err)
	}
	assertMigrationVersion(t, recoveryM, 28, false)
	assertPaymentCreateCommandContractGuardPresent(t, recoveryDB, true)
}

func TestPaymentCreateCommandContractGuardMigration_IsolatesLegacyAndRecordsCancellation(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	reference := "pa-" + "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, reference, "CREATED", nil, now)
	insertPaymentCreateContractFixture(t, db, attemptID, reference, now.Add(time.Hour))
	payload := `{"attempt_id":"` + attemptID + `","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	var commandID string
	if err := db.QueryRow(`
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_CREATE', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
		RETURNING id::text
	`, attemptID, "payment:create:"+bookingID+":1", paymentHash, payload).Scan(&commandID); err != nil {
		t.Fatalf("insert guarded create command: %v", err)
	}

	assertExecFails(t, db, `UPDATE bookings SET status = 'CONFIRMED' WHERE id = $1`, bookingID)
	assertExecFailsWithReplicaRole(t, db, `UPDATE bookings SET status = 'WAITING_VERIFICATION' WHERE id = $1`, bookingID)
	assertExecFails(t, db, `
		INSERT INTO owner_finance_transactions (
			owner_id, venue_id, booking_id, created_by_user_id,
			type, source, category, amount, transaction_date, description
		)
		SELECT op.user_id, v.id, b.id, op.user_id,
		       'INCOME', 'BOOKING', 'BOOKING_PAYMENT', b.total_price,
		       CURRENT_DATE, 'must be blocked'
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON op.id = v.owner_profile_id
		WHERE b.id = $1
	`, bookingID)
	assertExecFails(t, db, `UPDATE bookings SET status = 'CANCELLED' WHERE id = $1`, bookingID)
	assertExecFailsWithReplicaRole(t, db, `UPDATE bookings SET status = 'CANCELLED' WHERE id = $1`, bookingID)
	assertExecFailsWithReplicaRole(t, db, `
		UPDATE payment_attempts
		SET amount_rupiah = amount_rupiah + 1
		WHERE id = $1
	`, attemptID)

	var customerID string
	if err := db.QueryRow(`SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatalf("read cancellation actor: %v", err)
	}
	assertExecFails(t, db, `
		INSERT INTO payment_create_cancellations (
			payment_attempt_id, command_id, actor_user_id, reason
		) VALUES ($1, $2, $3, 'BOOKING_CANCELLED')
	`, attemptID, commandID, customerID)
	assertExecFails(t, db, `UPDATE payment_attempts SET state = 'CANCELLED' WHERE id = $1`, attemptID)
	assertDeferredExecFailsWithReplicaRole(
		t,
		db,
		`UPDATE payment_attempts SET state = 'CANCELLED' WHERE id = $1`,
		attemptID,
	)

	cancelTx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin atomic cancellation: %v", err)
	}
	defer cancelTx.Rollback()
	if _, err := cancelTx.Exec(`UPDATE payment_attempts SET state = 'CANCELLED' WHERE id = $1`, attemptID); err != nil {
		t.Fatalf("stage cancelled attempt: %v", err)
	}
	if _, err := cancelTx.Exec(`
		INSERT INTO payment_create_cancellations (
			payment_attempt_id, command_id, actor_user_id, reason
		) VALUES ($1, $2, $3, 'BOOKING_CANCELLED')
	`, attemptID, commandID, customerID); err != nil {
		t.Fatalf("stage valid cancellation tombstone: %v", err)
	}
	if _, err := cancelTx.Exec(`
		INSERT INTO platform_audit_logs (
			actor_user_id, actor_role, action, entity_type,
			entity_id, correlation_id, metadata
		) VALUES (
			$1, 'CUSTOMER', 'payment_state_transition', 'PAYMENT_ATTEMPT',
			$2, $3, '{"from_state":"CREATED","to_state":"CANCELLED","attempt_no":1,"late_capture":false}'::jsonb
		)
	`, customerID, attemptID, reference); err != nil {
		t.Fatalf("stage cancellation audit: %v", err)
	}
	if _, err := cancelTx.Exec(`UPDATE bookings SET status = 'CANCELLED' WHERE id = $1`, bookingID); err != nil {
		t.Fatalf("stage cancelled booking: %v", err)
	}
	if err := cancelTx.Commit(); err != nil {
		t.Fatalf("commit atomic sandbox cancellation: %v", err)
	}
	leaseCancelledCommand := `
		UPDATE payment_provider_commands
		SET state = 'LEASED',
		    attempt_count = 1,
		    lease_owner = $2,
		    lease_token = $3,
		    lease_expires_at = transaction_timestamp() + interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE id = $1
	`
	assertExecFails(
		t,
		db,
		leaseCancelledCommand,
		commandID,
		"worker:"+uuid.NewString(),
		uuid.NewString(),
	)
	assertExecFailsWithReplicaRole(
		t,
		db,
		leaseCancelledCommand,
		commandID,
		"worker:"+uuid.NewString(),
		uuid.NewString(),
	)
	assertExecFails(t, db, `
		UPDATE payment_create_cancellations
		SET actor_user_id = NULL
		WHERE payment_attempt_id = $1
	`, attemptID)
	assertExecFailsWithReplicaRole(t, db, `
		DELETE FROM payment_create_cancellations
		WHERE payment_attempt_id = $1
	`, attemptID)

	legacyBookingID := seedPaymentAttemptBooking(t, db, true)
	if _, err := db.Exec(`
		INSERT INTO owner_finance_transactions (
			owner_id, venue_id, booking_id, created_by_user_id,
			type, source, category, amount, transaction_date, description
		)
		SELECT op.user_id, v.id, b.id, op.user_id,
		       'INCOME', 'BOOKING', 'BOOKING_PAYMENT', b.total_price,
		       CURRENT_DATE, 'legacy fact before sandbox'
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON op.id = v.owner_profile_id
		WHERE b.id = $1
	`, legacyBookingID); err != nil {
		t.Fatalf("insert pre-existing legacy owner income: %v", err)
	}
	assertExecFails(t, db, `
		INSERT INTO payment_attempts (
			id, booking_id, attempt_no, provider, provider_environment, requested_method,
			integration_mode, capture_method, state, currency, amount_rupiah,
			local_reference, request_hash, expires_at, created_at, updated_at
		) VALUES ($1, $2, 1, 'XENDIT', 'TEST', 'QRIS', 'PAYMENT_LINK', 'AUTOMATIC',
			'CREATED', 'IDR', 10000, $3, $4, $5, $6, $6)
	`, uuid.NewString(), legacyBookingID,
		"pa-"+"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		paymentHash, now.Add(time.Hour), now)

	dispatchedBookingID := seedPaymentAttemptBooking(t, db, true)
	dispatchedAttemptID := uuid.NewString()
	dispatchedReference := "pa-" + "121212121212121212121212121212121212121212121212121212121212"
	insertPaymentAttempt(t, db, dispatchedAttemptID, dispatchedBookingID, 1, dispatchedReference, "CREATED", nil, now)
	insertPaymentCreateContractFixture(t, db, dispatchedAttemptID, dispatchedReference, now.Add(time.Hour))
	dispatchedPayload := `{"attempt_id":"` + dispatchedAttemptID + `","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	var dispatchedCommandID string
	if err := db.QueryRow(`
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_CREATE', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
		RETURNING id::text
	`, dispatchedAttemptID, "payment:create:"+dispatchedBookingID+":1", paymentHash, dispatchedPayload).Scan(&dispatchedCommandID); err != nil {
		t.Fatalf("insert command before dispatch proof: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE payment_provider_commands
		SET state = 'LEASED',
		    attempt_count = 1,
		    lease_owner = $2,
		    lease_token = $3,
		    lease_expires_at = transaction_timestamp() + interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE id = $1
	`, dispatchedCommandID, "worker:"+uuid.NewString(), uuid.NewString()); err != nil {
		t.Fatalf("mark create command dispatched: %v", err)
	}
	assertExecFails(t, db, `UPDATE payment_attempts SET state = 'CANCELLED' WHERE id = $1`, dispatchedAttemptID)
	assertExecFails(t, db, `
		INSERT INTO payment_create_cancellations (
			payment_attempt_id, command_id, actor_user_id, reason
		) VALUES ($1, $2, NULL, 'BOOKING_CANCELLED')
	`, dispatchedAttemptID, dispatchedCommandID)
}

func TestPaymentCreateCancellationGuardSerializesAgainstConcurrentClaim(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()

	bookingID := seedPaymentAttemptBooking(t, db, true)
	attemptID := uuid.NewString()
	reference := "pa-" + "343434343434343434343434343434343434343434343434343434343434"
	now := time.Now().UTC().Truncate(time.Microsecond)
	insertPaymentAttempt(t, db, attemptID, bookingID, 1, reference, "CREATED", nil, now)
	insertPaymentCreateContractFixture(t, db, attemptID, reference, now.Add(time.Hour))
	payload := `{"attempt_id":"` + attemptID + `","amount_rupiah":10000,"currency":"IDR","requested_method":"QRIS"}`
	var commandID string
	if err := db.QueryRow(`
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload
		) VALUES ('PAYMENT_CREATE', 'PAYMENT_ATTEMPT', $1, $1, $2, $3, $4::jsonb)
		RETURNING id::text
	`, attemptID, "payment:create:"+bookingID+":1", paymentHash, payload).Scan(&commandID); err != nil {
		t.Fatalf("insert create command: %v", err)
	}

	cancelTx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin cancellation transaction: %v", err)
	}
	defer cancelTx.Rollback()
	if _, err := cancelTx.Exec(`UPDATE payment_attempts SET state = 'CANCELLED' WHERE id = $1`, attemptID); err != nil {
		t.Fatalf("stage cancelled attempt: %v", err)
	}
	var cancelPID int
	if err := cancelTx.QueryRow(`SELECT pg_backend_pid()`).Scan(&cancelPID); err != nil {
		t.Fatalf("read cancellation backend pid: %v", err)
	}

	claimTx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin claim transaction: %v", err)
	}
	defer claimTx.Rollback()
	if _, err := claimTx.Exec(`
		SELECT pg_advisory_xact_lock(
			hashtextextended('payments:booking-flow:' || $1::uuid::text, 0)
		)
	`, bookingID); err != nil {
		t.Fatalf("lock booking flow for claim: %v", err)
	}

	insertResult := make(chan error, 1)
	go func() {
		_, insertErr := cancelTx.Exec(`
			INSERT INTO payment_create_cancellations (
				payment_attempt_id, command_id, actor_user_id, reason
			) VALUES ($1, $2, NULL, 'BOOKING_CANCELLED')
		`, attemptID, commandID)
		insertResult <- insertErr
	}()
	waitForAdvisoryWait(t, db, cancelPID)

	if _, err := claimTx.Exec(`
		UPDATE payment_provider_commands
		SET state = 'LEASED',
		    attempt_count = 1,
		    lease_owner = $2,
		    lease_token = $3,
		    lease_expires_at = transaction_timestamp() + interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE id = $1
	`, commandID, "worker:"+uuid.NewString(), uuid.NewString()); err != nil {
		t.Fatalf("lease command while cancellation is serialized: %v", err)
	}
	if err := claimTx.Commit(); err != nil {
		t.Fatalf("commit claim transaction: %v", err)
	}

	if err := <-insertResult; err == nil {
		t.Fatal("cancellation tombstone was accepted after the command became leased")
	}
	if err := cancelTx.Rollback(); err != nil && err != sql.ErrTxDone {
		t.Fatalf("rollback rejected cancellation transaction: %v", err)
	}

	var commandState, attemptState, bookingState string
	var tombstoneCount int
	if err := db.QueryRow(`SELECT state FROM payment_provider_commands WHERE id = $1`, commandID).Scan(&commandState); err != nil {
		t.Fatalf("read command state: %v", err)
	}
	if err := db.QueryRow(`SELECT state FROM payment_attempts WHERE id = $1`, attemptID).Scan(&attemptState); err != nil {
		t.Fatalf("read attempt state: %v", err)
	}
	if err := db.QueryRow(`SELECT status FROM bookings WHERE id = $1`, bookingID).Scan(&bookingState); err != nil {
		t.Fatalf("read booking state: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM payment_create_cancellations WHERE command_id = $1`, commandID).Scan(&tombstoneCount); err != nil {
		t.Fatalf("count cancellation tombstones: %v", err)
	}
	if commandState != "LEASED" || attemptState != "CREATED" ||
		bookingState != "PENDING_PAYMENT" || tombstoneCount != 0 {
		t.Fatalf(
			"serialized result command=%s attempt=%s booking=%s tombstones=%d; want LEASED/CREATED/PENDING_PAYMENT/0",
			commandState,
			attemptState,
			bookingState,
			tombstoneCount,
		)
	}
}

func waitForAdvisoryWait(t *testing.T, db *sql.DB, backendPID int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE pid = $1
				  AND wait_event_type = 'Lock'
				  AND wait_event = 'advisory'
			)
		`, backendPID).Scan(&waiting); err != nil {
			t.Fatalf("inspect cancellation advisory wait: %v", err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cancellation transaction did not wait on the booking advisory lock")
}

func insertPaymentCreateContractFixture(t *testing.T, db *sql.DB, attemptID, reference string, requestedExpiry time.Time) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO payment_create_contracts (
			payment_attempt_id, request_hash, requested_expires_at,
			success_return_url, cancel_return_url
		) VALUES ($1, $2, $3, $4, $5)
	`, attemptID, paymentHash, requestedExpiry,
		"https://demo.example.test/payments/return/"+reference+"/success",
		"https://demo.example.test/payments/return/"+reference+"/cancel"); err != nil {
		t.Fatalf("insert payment create contract fixture: %v", err)
	}
}

func assertDeferredExecFailsWithReplicaRole(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin deferred replica-role assertion: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("set local replica role for deferred assertion: %v", err)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		t.Fatalf("deferred replica-role statement failed before commit: %v", err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatalf("expected deferred replica-role transaction to fail: %s", query)
	}
}

func assertPaymentCreateCommandContractGuardPresent(t *testing.T, db *sql.DB, want bool) {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid = 'payment_provider_commands'::regclass
			   AND tgname = 'validate_payment_create_command_contract'
			   AND tgenabled = 'A')
			+
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid = 'bookings'::regclass
			   AND tgname = 'guard_sandbox_booking_payment_isolation'
			   AND tgenabled = 'A')
			+
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid = 'owner_finance_transactions'::regclass
			   AND tgname = 'guard_sandbox_owner_cash_isolation'
			   AND tgenabled = 'A')
			+
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid = 'payment_attempts'::regclass
			   AND tgname = 'guard_sandbox_payment_attempt_isolation'
			   AND tgenabled = 'A')
			+
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid = 'payment_attempts'::regclass
			   AND tgname = 'guard_payment_attempt_update'
			   AND tgenabled = 'A')
			+
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid = 'payment_attempts'::regclass
			   AND tgname = 'validate_atomic_local_payment_cancellation'
			   AND tgenabled = 'A')
			+
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid = 'payment_provider_commands'::regclass
			   AND tgname = 'guard_cancelled_payment_create_command_lifecycle'
			   AND tgenabled = 'A')
			+
			CASE WHEN to_regclass('payment_create_cancellations') IS NULL THEN 0 ELSE 1 END
	`).Scan(&count); err != nil {
		t.Fatalf("inspect payment create command contract guard: %v", err)
	}
	if (count == 8) != want {
		t.Fatalf("payment create command contract guard count = %d; want present=%t", count, want)
	}
}
