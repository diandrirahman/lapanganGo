package payments

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

func TestRepositoryCreateReplayAndStateGuard(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)

	params := validCreateParams(bookingID, "payment:create:attempt-a")
	attempt, err := repo.CreateOrReplayAttempt(ctx, params)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if attempt.AttemptNo != 1 || attempt.State != AttemptStateCreated || attempt.AmountRupiah != 10000 {
		t.Fatalf("unexpected first attempt: %#v", attempt)
	}

	replay, err := repo.CreateOrReplayAttempt(ctx, params)
	if err != nil {
		t.Fatalf("replay attempt: %v", err)
	}
	if replay.ID != attempt.ID {
		t.Fatalf("replay created another attempt: first=%s replay=%s", attempt.ID, replay.ID)
	}

	expiringBookingID := seedRepositoryBooking(t, ctx, pool, true)
	expiringParams := validCreateParams(expiringBookingID, "payment:create:expired-replay")
	expiringParams.ExpiresAt = time.Now().UTC().Add(2 * time.Second)
	expiringAttempt, err := repo.CreateOrReplayAttempt(ctx, expiringParams)
	if err != nil {
		t.Fatalf("create expiring attempt: %v", err)
	}
	time.Sleep(time.Until(expiringParams.ExpiresAt) + 50*time.Millisecond)
	expiredReplay, err := repo.CreateOrReplayAttempt(ctx, expiringParams)
	if err != nil {
		t.Fatalf("replay expired attempt: %v", err)
	}
	if expiredReplay.ID != expiringAttempt.ID {
		t.Fatalf("expired replay created another attempt: first=%s replay=%s", expiringAttempt.ID, expiredReplay.ID)
	}

	nextNo, err := repo.GetNextAttemptNumber(ctx, bookingID)
	if err != nil || nextNo != 2 {
		t.Fatalf("next attempt number = %d, %v; want 2, nil", nextNo, err)
	}
	attemptsByBooking, err := repo.GetAttemptsByBooking(ctx, bookingID)
	if err != nil || len(attemptsByBooking) != 1 || attemptsByBooking[0].ID != attempt.ID {
		t.Fatalf("attempts by booking = %#v, %v; want first attempt", attemptsByBooking, err)
	}
	if _, err := repo.CreateOrReplayAttempt(ctx, CreateAttemptParams{
		BookingID: bookingID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		RequestedMethod: RequestedMethodQRIS, IntegrationMode: IntegrationModePaymentLink,
		CaptureMethod: CaptureMethodAutomatic, LocalReference: params.LocalReference,
		RequestHash: strings.Repeat("f", 64), ExpiresAt: params.ExpiresAt,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key/different hash error = %v; want ErrIdempotencyConflict", err)
	}

	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("created to pending: %v", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStatePending, AttemptStateCaptured); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("direct capture transition error = %v; want ErrInvalidTransition", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCreated, AttemptStateCancelled); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("stale CAS error = %v; want ErrStateConflict", err)
	}

	capture := validCaptureParams(attempt.ID, "payment-1", time.Now().UTC().Add(-time.Second))
	providerRequestID := "payment-request-1"
	capture.ProviderPaymentReqID = &providerRequestID
	captured, err := repo.RecordCapture(ctx, capture)
	if err != nil {
		t.Fatalf("record capture: %v", err)
	}
	if captured.Duplicate || captured.LateCapture || captured.Attempt.State != AttemptStateCaptured {
		t.Fatalf("unexpected normal capture result: %#v", captured)
	}

	duplicate, err := repo.RecordCapture(ctx, capture)
	if err != nil {
		t.Fatalf("duplicate capture: %v", err)
	}
	if !duplicate.Duplicate || duplicate.Fact.ID != captured.Fact.ID {
		t.Fatalf("duplicate capture was not a no-op: %#v", duplicate)
	}

	webhookDuplicate := capture
	webhookDuplicate.Authority = "VERIFIED_WEBHOOK"
	webhookDuplicate.ObservedAt = capture.ObservedAt.Add(time.Second)
	webhookDuplicate.SourceReference = "webhook:event-payment-1"
	webhookDuplicate.PayloadHash = strings.Repeat("e", 64)
	duplicate, err = repo.RecordCapture(ctx, webhookDuplicate)
	if err != nil {
		t.Fatalf("cross-authority duplicate capture: %v", err)
	}
	if !duplicate.Duplicate || duplicate.Fact.ID != captured.Fact.ID {
		t.Fatalf("cross-authority capture was not an idempotent no-op: %#v", duplicate)
	}

	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCaptured, AttemptStateFailed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("captured downgrade error = %v; want ErrInvalidTransition", err)
	}

	var factCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&factCount); err != nil {
		t.Fatalf("count capture facts: %v", err)
	}
	if factCount != 1 {
		t.Fatalf("capture fact count = %d; want 1", factCount)
	}
}

func TestRepositoryConcurrentCreateAndCapture(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)

	paramsA := validCreateParams(bookingID, "payment:create:concurrent-a")
	paramsB := validCreateParams(bookingID, "payment:create:concurrent-b")
	results := make(chan PaymentAttempt, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, params := range []CreateAttemptParams{paramsA, paramsB} {
		wg.Add(1)
		go func(params CreateAttemptParams) {
			defer wg.Done()
			attempt, err := repo.CreateOrReplayAttempt(ctx, params)
			if err != nil {
				errs <- err
				return
			}
			results <- attempt
		}(params)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent create: %v", err)
	}
	var attempts []PaymentAttempt
	for attempt := range results {
		attempts = append(attempts, attempt)
	}
	if len(attempts) != 2 || attempts[0].AttemptNo == attempts[1].AttemptNo {
		t.Fatalf("concurrent create attempt numbers = %#v; want distinct", attempts)
	}

	if _, err := repo.TransitionState(ctx, attempts[0].ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("prepare concurrent capture: %v", err)
	}
	capture := validCaptureParams(attempts[0].ID, "concurrent-payment", time.Now().UTC().Add(-time.Second))
	resultsCapture := make(chan CaptureResult, 2)
	errsCapture := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repo.RecordCapture(ctx, capture)
			if err != nil {
				errsCapture <- err
				return
			}
			resultsCapture <- result
		}()
	}
	wg.Wait()
	close(resultsCapture)
	close(errsCapture)
	for err := range errsCapture {
		t.Fatalf("concurrent capture: %v", err)
	}
	var duplicateCount int
	for result := range resultsCapture {
		if result.Duplicate {
			duplicateCount++
		}
	}
	if duplicateCount != 1 {
		t.Fatalf("duplicate concurrent capture count = %d; want 1", duplicateCount)
	}

	if _, err := repo.TransitionState(ctx, attempts[1].ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("prepare second capture: %v", err)
	}
	secondCapture := validCaptureParams(attempts[1].ID, "second-booking-payment", time.Now().UTC().Add(-time.Second))
	if _, err := repo.RecordCapture(ctx, secondCapture); !errors.Is(err, ErrAlreadyCaptured) {
		t.Fatalf("second booking capture error = %v; want ErrAlreadyCaptured", err)
	}
	secondState, err := repo.GetAttemptByID(ctx, attempts[1].ID)
	if err != nil {
		t.Fatalf("get second attempt after capture conflict: %v", err)
	}
	if secondState.State != AttemptStatePending || secondState.CapturedAt != nil {
		t.Fatalf("capture conflict changed second attempt: %#v", secondState)
	}
}

func TestRepositoryLateCaptureAndMismatchRollback(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)

	withoutSnapshot := seedRepositoryBooking(t, ctx, pool, false)
	if _, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(withoutSnapshot, "payment:create:no-snapshot")); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("missing snapshot error = %v; want ErrSnapshotNotFound", err)
	}

	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingID, "payment:create:late"))
	if err != nil {
		t.Fatalf("create late attempt: %v", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("late attempt to pending: %v", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStatePending, AttemptStateExpired); err != nil {
		t.Fatalf("late attempt to expired: %v", err)
	}

	captureTime := time.Now().UTC().Add(-time.Second)
	late, err := repo.RecordCapture(ctx, validCaptureParams(attempt.ID, "late-payment", captureTime))
	if err != nil {
		t.Fatalf("record late capture: %v", err)
	}
	if !late.LateCapture || late.Attempt.State != AttemptStateCaptured {
		t.Fatalf("late capture result = %#v; want late captured", late)
	}

	bookingMismatch := seedRepositoryBooking(t, ctx, pool, true)
	mismatchAttempt, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingMismatch, "payment:create:mismatch"))
	if err != nil {
		t.Fatalf("create mismatch attempt: %v", err)
	}
	if _, err := repo.TransitionState(ctx, mismatchAttempt.ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("mismatch attempt to pending: %v", err)
	}
	mismatch := validCaptureParams(mismatchAttempt.ID, "mismatch-payment", captureTime)
	mismatch.AmountRupiah = 9999
	if _, err := repo.RecordCapture(ctx, mismatch); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("amount mismatch error = %v; want ErrCaptureConflict", err)
	}
	current, err := repo.GetAttemptByID(ctx, mismatchAttempt.ID)
	if err != nil {
		t.Fatalf("get attempt after mismatch: %v", err)
	}
	if current.State != AttemptStatePending || current.CapturedAt != nil {
		t.Fatalf("mismatch changed attempt: %#v", current)
	}
	var facts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, mismatchAttempt.ID).Scan(&facts); err != nil {
		t.Fatalf("count mismatch facts: %v", err)
	}
	if facts != 0 {
		t.Fatalf("mismatch created %d capture facts; want 0", facts)
	}
}

func openPaymentTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	if os.Getenv("TEST_ROLLBACK_HARDENING_DISPOSABLE") != "1" {
		t.Skip("disposable payment repository test is opt-in")
	}
	adminDSN := os.Getenv("ROLLBACK_HARDENING_TEST_DATABASE_URL")
	if adminDSN == "" {
		t.Fatal("ROLLBACK_HARDENING_TEST_DATABASE_URL is required")
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil || parsed.Path == "" || parsed.Path == "/" {
		t.Fatalf("invalid admin DSN")
	}
	dbName := "lapangango_payment_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	adminDB, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
		adminDB.Close()
		t.Fatalf("create disposable database: %v", err)
	}
	adminDB.Close()
	parsed.Path = "/" + dbName
	targetDSN := parsed.String()

	targetDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		t.Fatalf("open target database: %v", err)
	}
	driver, err := postgres.WithInstance(targetDB, &postgres.Config{})
	if err != nil {
		t.Fatalf("create migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../../../db/migrations", "postgres", driver)
	if err != nil {
		t.Fatalf("create migration runner: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("run migrations: %v", err)
	}
	m.Close()
	targetDB.Close()

	pool, err := pgxpool.New(context.Background(), targetDSN)
	if err != nil {
		t.Fatalf("open target pool: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("ping target pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupDB, err := sql.Open("postgres", adminDSN)
		if err == nil {
			defer cleanupDB.Close()
			cleanupDB.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, dbName)
			cleanupDB.Exec("DROP DATABASE " + dbName)
		}
	})
	return context.Background(), pool
}

func seedRepositoryBooking(t *testing.T, ctx context.Context, pool *pgxpool.Pool, withSnapshot bool) string {
	t.Helper()
	customerID := uuid.NewString()
	ownerUserID := uuid.NewString()
	ownerProfileID := uuid.NewString()
	venueID := uuid.NewString()
	courtID := uuid.NewString()
	bookingID := uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role, status)
		VALUES ($1, 'payment repository customer', $2, 'hash', 'CUSTOMER', 'ACTIVE'),
		       ($3, 'payment repository owner', $4, 'hash', 'OWNER', 'ACTIVE')
	`, customerID, "customer-"+suffix+"@example.test", ownerUserID, "owner-"+suffix+"@example.test"); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($1, $2, 'Payment Repository Owner', 'APPROVED')`, ownerProfileID, ownerUserID); err != nil {
		t.Fatalf("seed owner profile: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO venues (id, owner_profile_id, name, address, city, status) VALUES ($1, $2, $3, 'Test address', 'Jakarta', 'ACTIVE')`, venueID, ownerProfileID, "Payment Repository Venue "+suffix); err != nil {
		t.Fatalf("seed venue: %v", err)
	}
	var sportID string
	if err := pool.QueryRow(ctx, `SELECT id FROM sports WHERE name = 'Futsal' LIMIT 1`).Scan(&sportID); err != nil {
		t.Fatalf("find sport: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, status) VALUES ($1, $2, $3, $4, 'INDOOR', 10000, 'ACTIVE')`, courtID, venueID, sportID, "Payment Repository Court "+suffix); err != nil {
		t.Fatalf("seed court: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, total_price, status) VALUES ($1, $2, $3, CURRENT_DATE + 1, '10:00', '11:00', 10000, 'PENDING_PAYMENT')`, bookingID, customerID, courtID); err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	if !withSnapshot {
		return bookingID
	}
	var termID string
	if err := pool.QueryRow(ctx, `SELECT id FROM platform_commercial_terms WHERE owner_profile_id IS NULL LIMIT 1`).Scan(&termID); err != nil {
		t.Fatalf("find commercial term: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO booking_fee_snapshots (
			booking_id, owner_profile_id, venue_id, commercial_term_id, terms_source,
			booking_channel, finance_mode, original_price_rupiah, owner_price_adjustment_rupiah,
			final_booking_price_rupiah, customer_charge_amount_rupiah, commission_basis_amount_rupiah,
			commission_bps, commission_amount_rupiah, owner_net_amount_rupiah, calculation_version
		) VALUES ($1, $2, $3, $4, 'POLICY', 'MARKETPLACE_ONLINE', 'SIMULATION',
			10000, 0, 10000, 10000, 10000, 700, 700, 9300, 'PAYMENT_REPOSITORY_TEST_V1')
	`, bookingID, ownerProfileID, venueID, termID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	return bookingID
}

func validCreateParams(bookingID, localReference string) CreateAttemptParams {
	return CreateAttemptParams{
		BookingID: bookingID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		RequestedMethod: RequestedMethodQRIS, IntegrationMode: IntegrationModePaymentLink,
		CaptureMethod: CaptureMethodAutomatic, LocalReference: localReference,
		RequestHash: validPaymentHash, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
}

func validCaptureParams(attemptID, providerPaymentID string, capturedAt time.Time) CaptureParams {
	return CaptureParams{
		AttemptID: attemptID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		ProviderPaymentID: providerPaymentID, AmountRupiah: 10000, Currency: CurrencyIDR,
		CapturedAt: capturedAt, ObservedAt: capturedAt.Add(time.Second), Authority: "AUTHENTICATED_INQUIRY",
		SourceReference: "inquiry:" + providerPaymentID, PayloadHash: validPaymentHash,
	}
}

func TestAllowedStateTransition(t *testing.T) {
	legal := []struct {
		from AttemptState
		to   AttemptState
	}{
		{AttemptStateCreated, AttemptStatePending},
		{AttemptStateCreated, AttemptStateCancelled},
		{AttemptStatePending, AttemptStateFailed},
		{AttemptStatePending, AttemptStateExpired},
		{AttemptStatePending, AttemptStateCancelled},
	}
	for _, tc := range legal {
		if !allowedStateTransition(tc.from, tc.to) {
			t.Errorf("allowedStateTransition(%q, %q) = false", tc.from, tc.to)
		}
	}

	denied := []struct {
		from AttemptState
		to   AttemptState
	}{
		{AttemptStateCreated, AttemptStateCaptured},
		{AttemptStatePending, AttemptStateCaptured},
		{AttemptStateCaptured, AttemptStatePending},
		{AttemptStateCaptured, AttemptStateFailed},
		{AttemptStateFailed, AttemptStatePending},
		{AttemptStateExpired, AttemptStatePending},
		{AttemptStateCancelled, AttemptStatePending},
	}
	for _, tc := range denied {
		if allowedStateTransition(tc.from, tc.to) {
			t.Errorf("allowedStateTransition(%q, %q) = true", tc.from, tc.to)
		}
	}
}
