package bookings_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lapangango-api/internal/bookings"
	"lapangango-api/internal/httputil"
	"lapangango-api/internal/platformfinance"
)

type offlineFixture struct {
	ownerUserID    string
	ownerProfileID string
	venueID        string
	courtID        string
	staffUserID    string
}

func setupOfflineIntegrationFixtures(ctx context.Context, tx pgx.Tx, t *testing.T, commissionBps int) offlineFixture {
	t.Helper()
	var f offlineFixture

	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role)
		VALUES ('Test Owner Offline', 'test_owner_off@example.com', '083333333', 'hash', 'OWNER')
		RETURNING id`).Scan(&f.ownerUserID); err != nil {
		t.Fatalf("insert owner user: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role)
		VALUES ('Test Staff Offline', 'test_staff_off@example.com', '084444444', 'hash', 'STAFF')
		RETURNING id`).Scan(&f.staffUserID); err != nil {
		t.Fatalf("insert staff user: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO owner_profiles (user_id, business_name)
		VALUES ($1, 'Test Business Offline')
		RETURNING id`, f.ownerUserID).Scan(&f.ownerProfileID); err != nil {
		t.Fatalf("insert owner profile: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO venues (owner_profile_id, name, address, city, status)
		VALUES ($1, 'Test Venue Offline', 'Address', 'City', 'ACTIVE')
		RETURNING id`, f.ownerProfileID).Scan(&f.venueID); err != nil {
		t.Fatalf("insert venue: %v", err)
	}

	var sportID string
	if err := tx.QueryRow(ctx, `SELECT id FROM sports LIMIT 1`).Scan(&sportID); err != nil {
		t.Fatalf("select seeded sport: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO courts (venue_id, sport_id, name, price_per_hour, status, location_type)
		VALUES ($1, $2, 'Court Offline', 100000, 'ACTIVE', 'INDOOR')
		RETURNING id`, f.venueID, sportID).Scan(&f.courtID); err != nil {
		t.Fatalf("insert court: %v", err)
	}
	for day := 0; day < 7; day++ {
		if _, err := tx.Exec(ctx, `
			INSERT INTO court_operating_hours (court_id, day_of_week, open_time, close_time, is_closed)
			VALUES ($1, $2, '00:00:00', '23:59:59', false)`, f.courtID, day); err != nil {
			t.Fatalf("insert operating hours: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform_commercial_terms (id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from)
		VALUES ($1, $2, 'Test Term', 'STANDARD', 'SIMULATION', 'NONE', $3, '2000-01-01')`, "00000000-0000-0000-0000-000000000004", f.ownerProfileID, commissionBps); err != nil {
		t.Fatalf("insert commercial term: %v", err)
	}
	return f
}

func checkOfflineLeaks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f offlineFixture) {
	t.Helper()
	checks := []struct {
		name  string
		query string
		arg   string
	}{
		{"owner user", "SELECT count(*) FROM users WHERE id = $1", f.ownerUserID},
		{"staff user", "SELECT count(*) FROM users WHERE id = $1", f.staffUserID},
		{"owner profile", "SELECT count(*) FROM owner_profiles WHERE id = $1", f.ownerProfileID},
		{"venue", "SELECT count(*) FROM venues WHERE id = $1", f.venueID},
		{"court", "SELECT count(*) FROM courts WHERE id = $1", f.courtID},
		{"commercial term", "SELECT count(*) FROM platform_commercial_terms WHERE id = $1", "00000000-0000-0000-0000-000000000004"},
		{"bookings", "SELECT count(*) FROM bookings WHERE court_id = $1", f.courtID},
		{"offline customers", "SELECT count(*) FROM offline_booking_customers WHERE booking_id IN (SELECT id FROM bookings WHERE court_id = $1)", f.courtID},
		{"snapshots", "SELECT count(*) FROM booking_fee_snapshots s JOIN bookings b ON b.id = s.booking_id WHERE b.court_id = $1", f.courtID},
		{"ledgers", "SELECT count(*) FROM owner_finance_transactions WHERE venue_id = $1", f.venueID},
	}
	for _, check := range checks {
		var count int
		if err := pool.QueryRow(ctx, check.query, check.arg).Scan(&count); err != nil {
			t.Errorf("check %s leak: %v", check.name, err)
			continue
		}
		if count != 0 {
			t.Errorf("leaked %s: got %d", check.name, count)
		}
	}
}

func futureOfflineBookingDate(t *testing.T) string {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	return time.Now().In(loc).AddDate(0, 0, 2).Format("2006-01-02")
}

func runOfflineBookingTxTest(t *testing.T, name string, commissionBps int, testFn func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture)) {
	t.Run(name, func(t *testing.T) {
		if os.Getenv("TEST_INTEGRATION") != "1" {
			t.Skip("set TEST_INTEGRATION=1 to run disposable database integration tests")
		}
		dbURL := os.Getenv("TEST_DATABASE_URL")
		if dbURL == "" {
			t.Fatal("TEST_DATABASE_URL is not set")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("connect database: %v", err)
		}
		defer pool.Close()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}

		var f offlineFixture
		fixtureReady := false
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()

			if err := tx.Rollback(cleanupCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("rollback test tx: %v", err)
			}
			if fixtureReady {
				checkOfflineLeaks(t, cleanupCtx, pool, f)
			}
		}()

		f = setupOfflineIntegrationFixtures(ctx, tx, t, commissionBps)
		fixtureReady = true
		wrapper := &testTxRepoWrapper{
			Repository:      bookings.NewRepository(pool),
			testTx:          tx,
			rollbackOnError: false,
		}

		testFn(t, ctx, tx, wrapper, f)
	})
}

func TestOfflineBookingIntegration(t *testing.T) {
	runOfflineBookingTxTest(t, "normal offline booking no override", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		svc := bookings.NewService(wrapper, 15, nil, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:      f.venueID,
			CourtID:      f.courtID,
			BookingDate:  futureOfflineBookingDate(t),
			StartTime:    "10:00",
			EndTime:      "12:00",
			CustomerName: "Offline Cust",
			TotalPrice:   200000,
			Status:       "PAID",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.ownerUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              true,
		}
		resp, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if err != nil {
			t.Fatalf("create offline booking: %v", err)
		}

		var snapshotCount int
		var channel, mode, reason string
		var adj, bps, amt, basis, net int64
		if err := tx.QueryRow(ctx, `
			SELECT count(*), max(booking_channel), max(finance_mode), max(COALESCE(price_adjustment_reason, '')), 
			       max(owner_price_adjustment_rupiah), max(commission_bps), max(commission_amount_rupiah),
			       max(commission_basis_amount_rupiah), max(owner_net_amount_rupiah)
			FROM booking_fee_snapshots WHERE booking_id = $1
		`, resp.ID).Scan(&snapshotCount, &channel, &mode, &reason, &adj, &bps, &amt, &basis, &net); err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		if snapshotCount != 1 || channel != string(platformfinance.BookingChannelOwnerWalkIn) || reason != "" || adj != 0 || bps != 0 || amt != 0 || mode != "SIMULATION" || basis != 200000 || net != 200000 {
			t.Fatalf("unexpected snapshot: count=%d, channel=%s, reason=%s, adj=%d, bps=%d, amt=%d, mode=%s, basis=%d, net=%d", snapshotCount, channel, reason, adj, bps, amt, mode, basis, net)
		}

		var legacyReason *string
		if err := tx.QueryRow(ctx, `SELECT price_override_reason FROM offline_booking_customers WHERE booking_id = $1`, resp.ID).Scan(&legacyReason); err != nil {
			t.Fatalf("read offline_booking_customers: %v", err)
		}
		if legacyReason != nil {
			t.Fatalf("expected legacy reason nil, got %v", *legacyReason)
		}

		var ledgerCount int
		var lOwner, lCreator, lType, lSource, lCategory string
		var lAmt float64
		if err := tx.QueryRow(ctx, `
			SELECT count(*), max(owner_id::text), max(created_by_user_id::text), max(type), max(source), max(category), max(amount)
			FROM owner_finance_transactions WHERE booking_id = $1
		`, resp.ID).Scan(&ledgerCount, &lOwner, &lCreator, &lType, &lSource, &lCategory, &lAmt); err != nil {
			t.Fatalf("read ledger: %v", err)
		}
		if ledgerCount != 1 || lOwner != f.ownerUserID || lCreator != f.ownerUserID || lType != "INCOME" || lSource != "BOOKING" || lCategory != "BOOKING_PAYMENT" || lAmt != 200000 {
			t.Fatalf("unexpected ledger: count=%d, owner=%s, creator=%s, type=%s, source=%s, category=%s, amount=%f", ledgerCount, lOwner, lCreator, lType, lSource, lCategory, lAmt)
		}
	})

	runOfflineBookingTxTest(t, "offline booking discount", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		svc := bookings.NewService(wrapper, 15, nil, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:             f.venueID,
			CourtID:             f.courtID,
			BookingDate:         futureOfflineBookingDate(t),
			StartTime:           "10:00",
			EndTime:             "12:00",
			CustomerName:        "Offline Cust",
			TotalPrice:          150000,
			Status:              "PAID",
			PriceOverrideReason: "Discount teman",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.ownerUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              true,
		}
		resp, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if err != nil {
			t.Fatalf("create offline booking: %v", err)
		}

		var adj, bps, amt int64
		var reason string
		if err := tx.QueryRow(ctx, `
			SELECT price_adjustment_reason, owner_price_adjustment_rupiah, commission_bps, commission_amount_rupiah
			FROM booking_fee_snapshots WHERE booking_id = $1
		`, resp.ID).Scan(&reason, &adj, &bps, &amt); err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		if bps != 0 || amt != 0 || adj != -50000 || reason != "Discount teman" {
			t.Fatalf("unexpected snapshot discount: bps=%d, amt=%d, adj=%d, reason=%q", bps, amt, adj, reason)
		}

		var legacyReason *string
		if err := tx.QueryRow(ctx, `SELECT price_override_reason FROM offline_booking_customers WHERE booking_id = $1`, resp.ID).Scan(&legacyReason); err != nil {
			t.Fatalf("read offline_booking_customers: %v", err)
		}
		if legacyReason == nil || *legacyReason != "Discount teman" {
			t.Fatalf("expected legacy reason 'Discount teman', got %v", legacyReason)
		}

		var lAmt float64
		if err := tx.QueryRow(ctx, `SELECT amount FROM owner_finance_transactions WHERE booking_id = $1`, resp.ID).Scan(&lAmt); err != nil {
			t.Fatalf("read ledger: %v", err)
		}
		if lAmt != 150000 {
			t.Fatalf("unexpected ledger amount: %f", lAmt)
		}
	})

	runOfflineBookingTxTest(t, "offline booking markup", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		svc := bookings.NewService(wrapper, 15, nil, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:             f.venueID,
			CourtID:             f.courtID,
			BookingDate:         futureOfflineBookingDate(t),
			StartTime:           "10:00",
			EndTime:             "12:00",
			CustomerName:        "Offline Cust",
			TotalPrice:          250000,
			Status:              "PAID",
			PriceOverrideReason: "Markup alat",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.ownerUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              true,
		}
		resp, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if err != nil {
			t.Fatalf("create offline booking: %v", err)
		}

		var adj, bps, amt int64
		var reason string
		if err := tx.QueryRow(ctx, `
			SELECT price_adjustment_reason, owner_price_adjustment_rupiah, commission_bps, commission_amount_rupiah
			FROM booking_fee_snapshots WHERE booking_id = $1
		`, resp.ID).Scan(&reason, &adj, &bps, &amt); err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		if bps != 0 || amt != 0 || adj != 50000 || reason != "Markup alat" {
			t.Fatalf("unexpected snapshot markup: bps=%d, amt=%d, adj=%d, reason=%q", bps, amt, adj, reason)
		}

		var legacyReason *string
		if err := tx.QueryRow(ctx, `SELECT price_override_reason FROM offline_booking_customers WHERE booking_id = $1`, resp.ID).Scan(&legacyReason); err != nil {
			t.Fatalf("read offline_booking_customers: %v", err)
		}
		if legacyReason == nil || *legacyReason != "Markup alat" {
			t.Fatalf("expected legacy reason 'Markup alat', got %v", legacyReason)
		}

		var legacyDiscount float64
		if err := tx.QueryRow(ctx, `SELECT discount_amount FROM bookings WHERE id = $1`, resp.ID).Scan(&legacyDiscount); err != nil {
			t.Fatalf("read bookings discount: %v", err)
		}
		if legacyDiscount != 0 {
			t.Fatalf("expected legacy discount to be 0 for markup, got %f", legacyDiscount)
		}

		var lAmt float64
		if err := tx.QueryRow(ctx, `SELECT amount FROM owner_finance_transactions WHERE booking_id = $1`, resp.ID).Scan(&lAmt); err != nil {
			t.Fatalf("read ledger: %v", err)
		}
		if lAmt != 250000 {
			t.Fatalf("unexpected ledger amount: %f", lAmt)
		}
	})

	runOfflineBookingTxTest(t, "staff offline booking with venue access", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		svc := bookings.NewService(wrapper, 15, nil, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:      f.venueID,
			CourtID:      f.courtID,
			BookingDate:  futureOfflineBookingDate(t),
			StartTime:    "10:00",
			EndTime:      "12:00",
			CustomerName: "Offline Cust",
			TotalPrice:   200000,
			Status:       "PAID",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.staffUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              false,
			AllowedVenueIDs:      []string{f.venueID},
		}
		resp, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if err != nil {
			t.Fatalf("create offline booking: %v", err)
		}

		var lOwner, lCreator string
		if err := tx.QueryRow(ctx, `SELECT max(owner_id::text), max(created_by_user_id::text) FROM owner_finance_transactions WHERE booking_id = $1`, resp.ID).Scan(&lOwner, &lCreator); err != nil {
			t.Fatalf("read ledger: %v", err)
		}
		if lOwner != f.ownerUserID {
			t.Fatalf("unexpected owner: expected %s (effective owner), got %s", f.ownerUserID, lOwner)
		}
		if lCreator != f.staffUserID {
			t.Fatalf("unexpected creator: expected %s, got %s", f.staffUserID, lCreator)
		}
	})

	runOfflineBookingTxTest(t, "staff offline booking without venue access", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		svc := bookings.NewService(wrapper, 15, nil, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:      f.venueID,
			CourtID:      f.courtID,
			BookingDate:  futureOfflineBookingDate(t),
			StartTime:    "10:00",
			EndTime:      "12:00",
			CustomerName: "Offline Cust",
			TotalPrice:   200000,
			Status:       "PAID",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.staffUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              false,
			AllowedVenueIDs:      []string{"some-other-venue"},
		}
		_, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if !errors.Is(err, bookings.ErrForbidden) {
			t.Fatalf("expected forbidden error, got %v", err)
		}
		if wrapper.execTxCalled {
			t.Fatalf("expected ExecuteBookingTx NOT to be called")
		}
	})

	runOfflineBookingTxTest(t, "calculator failure rolls back offline booking", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		calculatorErr := errors.New("calculator failure")
		wrapper.rollbackOnError = true
		orchestrator := mustOrchestratorWith(t, actualResolverAdapter, func(platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			return platformfinance.CalculatorResult{}, calculatorErr
		}, platformfinance.NewBookingFeeSnapshotRepository())
		svc := bookings.NewService(wrapper, 15, nil, nil, orchestrator)

		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:      f.venueID,
			CourtID:      f.courtID,
			BookingDate:  futureOfflineBookingDate(t),
			StartTime:    "10:00",
			EndTime:      "12:00",
			CustomerName: "Offline Cust",
			TotalPrice:   200000,
			Status:       "PAID",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.ownerUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              true,
		}
		_, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if !errors.Is(err, calculatorErr) {
			t.Fatalf("expected calculator error, got %v", err)
		}
	})

	runOfflineBookingTxTest(t, "resolver failure rolls back offline booking", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		resolverErr := errors.New("resolver failure")
		wrapper.rollbackOnError = true
		orchestrator := mustOrchestratorWith(t, func(context.Context, platformfinance.CommercialTermQueryer, string, time.Time) (*platformfinance.CommercialTerm, error) {
			return nil, resolverErr
		}, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository())
		svc := bookings.NewService(wrapper, 15, nil, nil, orchestrator)

		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:      f.venueID,
			CourtID:      f.courtID,
			BookingDate:  futureOfflineBookingDate(t),
			StartTime:    "10:00",
			EndTime:      "12:00",
			CustomerName: "Offline Cust",
			TotalPrice:   200000,
			Status:       "PAID",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.ownerUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              true,
		}
		_, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if !errors.Is(err, resolverErr) {
			t.Fatalf("expected resolver error, got %v", err)
		}
	})

	runOfflineBookingTxTest(t, "snapshot failure rolls back offline booking", 700, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, f offlineFixture) {
		snapshotErr := errors.New("snapshot failure")
		wrapper.rollbackOnError = true
		failingRepo := &failingSnapshotRepo{err: snapshotErr}
		orchestrator := mustOrchestratorWith(t, actualResolverAdapter, platformfinance.CalculateBookingFees, failingRepo)
		svc := bookings.NewService(wrapper, 15, nil, nil, orchestrator)

		req := bookings.OwnerCreateOfflineBookingRequest{
			VenueID:      f.venueID,
			CourtID:      f.courtID,
			BookingDate:  futureOfflineBookingDate(t),
			StartTime:    "10:00",
			EndTime:      "12:00",
			CustomerName: "Offline Cust",
			TotalPrice:   200000,
			Status:       "PAID",
		}
		ownerCtx := httputil.OwnerContext{
			ActorUserID:          f.ownerUserID,
			EffectiveOwnerUserID: f.ownerUserID,
			OwnerProfileID:       f.ownerProfileID,
			IsOwner:              true,
		}
		_, err := svc.OwnerCreateOfflineBooking(ctx, ownerCtx, req)
		if !errors.Is(err, snapshotErr) {
			t.Fatalf("expected snapshot error, got %v", err)
		}
		if !failingRepo.called {
			t.Fatalf("expected failing repo to be called")
		}
	})
}
