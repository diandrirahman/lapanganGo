package bookings_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/bookings"
	"lapangango-api/internal/notifications"
	"lapangango-api/internal/platformfinance"
	"lapangango-api/internal/promos"
)

const (
	testCommercialTermID = "00000000-0000-0000-0000-000000000003"
	testPromoID          = "00000000-0000-0000-0000-000000000002"
)

type mockNotifService struct {
	called int
}

func (m *mockNotifService) Create(ctx context.Context, params notifications.CreateNotificationParams) error {
	m.called++
	return nil
}
func (m *mockNotifService) CreateNotificationFromAppJob(ctx context.Context, param notifications.CreateNotificationParams) error {
	return nil
}
func (m *mockNotifService) ListByUser(ctx context.Context, userID string, page, limit int) (notifications.NotificationListResponse, error) {
	return notifications.NotificationListResponse{}, nil
}
func (m *mockNotifService) UnreadCount(ctx context.Context, userID string) (notifications.UnreadCountResponse, error) {
	return notifications.UnreadCountResponse{}, nil
}
func (m *mockNotifService) MarkRead(ctx context.Context, userID string, notificationID string) error {
	return nil
}
func (m *mockNotifService) MarkAllRead(ctx context.Context, userID string) error {
	return nil
}

type mockPromoRepo struct {
	promo *promos.Promo
}

func (m *mockPromoRepo) CreatePromo(ctx context.Context, p promos.Promo) (promos.Promo, error) {
	return p, nil
}
func (m *mockPromoRepo) ListOwnerPromos(ctx context.Context, ownerID string) ([]promos.Promo, error) {
	return nil, nil
}
func (m *mockPromoRepo) GetPromoByIDAndOwner(ctx context.Context, id, ownerID string) (promos.Promo, error) {
	return promos.Promo{}, nil
}
func (m *mockPromoRepo) UpdatePromo(ctx context.Context, id, ownerID string, params promos.UpdatePromoParams) (promos.Promo, error) {
	return promos.Promo{}, nil
}
func (m *mockPromoRepo) FindActivePromoByCode(ctx context.Context, ownerID, code string) (promos.Promo, error) {
	if m.promo == nil {
		return promos.Promo{}, errors.New("promo not found")
	}
	return *m.promo, nil
}
func (m *mockPromoRepo) IsVenueOwnedByOwner(ctx context.Context, ownerUserID, venueID string) (bool, error) {
	return true, nil
}
func (m *mockPromoRepo) GetCourtValidationInfo(ctx context.Context, courtID string) (promos.CourtValidationInfo, error) {
	return promos.CourtValidationInfo{}, nil
}
func (m *mockPromoRepo) DeletePromo(ctx context.Context, id, ownerID string) error { return nil }

type testTxRepoWrapper struct {
	*bookings.Repository
	testTx          pgx.Tx
	rollbackOnError bool
	execTxCalled    bool
}

func (w *testTxRepoWrapper) ExecuteBookingTx(ctx context.Context, fn func(pgx.Tx) error) error {
	w.execTxCalled = true
	err := fn(w.testTx)
	if err == nil || !w.rollbackOnError {
		return err
	}
	if rollbackErr := w.testTx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		return fmt.Errorf("booking transaction failed: %w; rollback failed: %v", err, rollbackErr)
	}
	return err
}

func (w *testTxRepoWrapper) FindVenueByIDAndOwnerProfileID(ctx context.Context, venueID, ownerProfileID string) (bookings.OwnerVenue, error) {
	query := `
		SELECT id::text, name
		FROM venues
		WHERE id = $1 AND owner_profile_id = $2
		LIMIT 1
	`
	var venue bookings.OwnerVenue
	err := w.testTx.QueryRow(ctx, query, venueID, ownerProfileID).Scan(&venue.ID, &venue.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return venue, pgx.ErrNoRows
		}
		return venue, err
	}
	return venue, nil
}

var _ bookings.BookingRepository = (*testTxRepoWrapper)(nil)

type failingSnapshotRepo struct {
	err    error
	called bool
}

func (r *failingSnapshotRepo) InsertSnapshot(ctx context.Context, db platformfinance.SnapshotDBTX, params platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
	r.called = true
	return nil, r.err
}
func (r *failingSnapshotRepo) GetSnapshot(ctx context.Context, db platformfinance.SnapshotDBTX, bookingID string) (*platformfinance.BookingFeeSnapshot, error) {
	return nil, platformfinance.ErrBookingFeeSnapshotNotFound
}

type onlineFixture struct {
	ownerUserID    string
	customerID     string
	ownerProfileID string
	venueID        string
	courtID        string
}

func setupOnlineIntegrationFixtures(ctx context.Context, tx pgx.Tx, t *testing.T, commissionBps int) onlineFixture {
	t.Helper()
	var f onlineFixture

	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role)
		VALUES ('Test Owner', 'test_owner@example.com', '081111111', 'hash', 'OWNER')
		RETURNING id`).Scan(&f.ownerUserID); err != nil {
		t.Fatalf("insert owner user: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role)
		VALUES ('Test Customer', 'test_customer@example.com', '082222222', 'hash', 'CUSTOMER')
		RETURNING id`).Scan(&f.customerID); err != nil {
		t.Fatalf("insert customer user: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO owner_profiles (user_id, business_name)
		VALUES ($1, 'Test Business')
		RETURNING id`, f.ownerUserID).Scan(&f.ownerProfileID); err != nil {
		t.Fatalf("insert owner profile: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO venues (owner_profile_id, name, address, city, status)
		VALUES ($1, 'Test Venue', 'Address', 'City', 'ACTIVE')
		RETURNING id`, f.ownerProfileID).Scan(&f.venueID); err != nil {
		t.Fatalf("insert venue: %v", err)
	}

	var sportID string
	if err := tx.QueryRow(ctx, `SELECT id FROM sports LIMIT 1`).Scan(&sportID); err != nil {
		t.Fatalf("select seeded sport: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO courts (venue_id, sport_id, name, price_per_hour, status, location_type)
		VALUES ($1, $2, 'Court 1', 100000, 'ACTIVE', 'INDOOR')
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
		VALUES ($1, $2, 'Test Term', 'STANDARD', 'SIMULATION', 'NONE', $3, '2000-01-01')`, testCommercialTermID, f.ownerProfileID, commissionBps); err != nil {
		t.Fatalf("insert commercial term: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO owner_promos (id, owner_id, code, name, discount_type, discount_value, starts_at, ends_at)
		VALUES ($1, $2, 'CANONICAL', 'Promo', 'PERCENTAGE', 50, '2000-01-01', '2099-01-01')`, testPromoID, f.ownerUserID); err != nil {
		t.Fatalf("insert promo: %v", err)
	}
	return f
}

func checkOnlineLeaks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f onlineFixture) {
	t.Helper()
	checks := []struct {
		name  string
		query string
		arg   string
	}{
		{"owner user", "SELECT count(*) FROM users WHERE id = $1", f.ownerUserID},
		{"customer user", "SELECT count(*) FROM users WHERE id = $1", f.customerID},
		{"owner profile", "SELECT count(*) FROM owner_profiles WHERE id = $1", f.ownerProfileID},
		{"venue", "SELECT count(*) FROM venues WHERE id = $1", f.venueID},
		{"court", "SELECT count(*) FROM courts WHERE id = $1", f.courtID},
		{"commercial term", "SELECT count(*) FROM platform_commercial_terms WHERE id = $1", testCommercialTermID},
		{"bookings", "SELECT count(*) FROM bookings WHERE court_id = $1", f.courtID},
		{"snapshots", `SELECT count(*) FROM booking_fee_snapshots s JOIN bookings b ON b.id = s.booking_id WHERE b.court_id = $1`, f.courtID},
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

func futureBookingDate(t *testing.T) string {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	return time.Now().In(loc).AddDate(0, 0, 2).Format("2006-01-02")
}

func actualResolverAdapter(ctx context.Context, db platformfinance.CommercialTermQueryer, ownerProfileID string, effectiveAt time.Time) (*platformfinance.CommercialTerm, error) {
	return platformfinance.NewCommercialTermResolver(db).ResolveEffectiveTerm(ctx, ownerProfileID, effectiveAt)
}

func mustOrchestratorWith(t *testing.T, resolver bookings.ResolveEffectiveTermFunc, calculator bookings.CalculateBookingFeesFunc, snapshotRepo platformfinance.BookingFeeSnapshotRepository) bookings.SnapshotOrchestrator {
	t.Helper()
	orchestrator, err := bookings.NewSnapshotOrchestrator(resolver, calculator, snapshotRepo)
	if err != nil {
		t.Fatalf("new snapshot orchestrator: %v", err)
	}
	return orchestrator
}

func mustOrchestrator(t *testing.T, calculator bookings.CalculateBookingFeesFunc, snapshotRepo platformfinance.BookingFeeSnapshotRepository) bookings.SnapshotOrchestrator {
	t.Helper()
	return mustOrchestratorWith(t, actualResolverAdapter, calculator, snapshotRepo)
}

func runOnlineBookingTxTest(t *testing.T, name string, commissionBps int, testFn func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture)) {
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
			t.Fatalf("begin fixture transaction: %v", err)
		}

		fixtureReady := false
		var f onlineFixture
		defer func() {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				t.Errorf("rollback fixture transaction: %v", rollbackErr)
			}
			if !fixtureReady {
				return
			}
			verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer verifyCancel()
			checkOnlineLeaks(t, verifyCtx, pool, f)
		}()

		f = setupOnlineIntegrationFixtures(ctx, tx, t, commissionBps)
		fixtureReady = true
		wrapper := &testTxRepoWrapper{Repository: bookings.NewRepository(pool), testTx: tx}
		testFn(t, ctx, tx, wrapper, &mockNotifService{}, f)
	})
}

func TestOnlineBookingIntegration(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		commissionBps      int
		expectedCommission int64
	}{
		{"normal booking 0 bps", 0, 0},
		{"normal booking 500 bps", 500, 10000},
		{"normal booking 700 bps", 700, 14000},
	} {
		testCase := testCase
		runOnlineBookingTxTest(t, testCase.name, testCase.commissionBps, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
			svc := bookings.NewService(wrapper, 15, notif, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
			req := bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "12:00"}
			response, err := svc.CreateBooking(ctx, f.customerID, req)
			if err != nil {
				t.Fatalf("create booking: %v", err)
			}
			if response.OriginalPrice == nil || *response.OriginalPrice != 200000 || response.FinalPrice == nil || *response.FinalPrice != 200000 || response.DiscountAmount != 0 {
				t.Fatalf("unexpected normal response: %+v", response)
			}
			var ownerID, venueID string
			var basis, amount int64
			var bps int
			if err := tx.QueryRow(ctx, `SELECT owner_profile_id::text, venue_id::text, commission_basis_amount_rupiah, commission_bps, commission_amount_rupiah FROM booking_fee_snapshots WHERE booking_id = $1`, response.ID).Scan(&ownerID, &venueID, &basis, &bps, &amount); err != nil {
				t.Fatalf("read normal snapshot: %v", err)
			}
			if ownerID != f.ownerProfileID || venueID != f.venueID || basis != 200000 || bps != testCase.commissionBps || amount != testCase.expectedCommission {
				t.Fatalf("unexpected normal snapshot owner=%s venue=%s basis=%d bps=%d amount=%d", ownerID, venueID, basis, bps, amount)
			}
			if notif.called != 1 {
				t.Fatalf("expected one post-commit notification, got %d", notif.called)
			}
		})
	}

	runOnlineBookingTxTest(t, "legacy float noise remains valid whole rupiah", 0, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
		if _, err := tx.Exec(ctx, `UPDATE courts SET price_per_hour = 150000 WHERE id = $1`, f.courtID); err != nil {
			t.Fatalf("set court price: %v", err)
		}
		svc := bookings.NewService(wrapper, 15, notif, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		request := bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "10:11"}
		response, err := svc.CreateBooking(ctx, f.customerID, request)
		if err != nil {
			t.Fatalf("create 11-minute booking: %v", err)
		}
		if response.TotalPrice != 27500 {
			t.Fatalf("expected Rp27500, got %v", response.TotalPrice)
		}
	})

	runOnlineBookingTxTest(t, "fractional price rejected", 0, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
		if _, err := tx.Exec(ctx, `UPDATE courts SET price_per_hour = 100000.50 WHERE id = $1`, f.courtID); err != nil {
			t.Fatalf("set fractional court price: %v", err)
		}
		svc := bookings.NewService(wrapper, 15, notif, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		_, err := svc.CreateBooking(ctx, f.customerID, bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "11:00"})
		if err == nil {
			t.Fatal("expected fractional price error")
		}
		var bookingsCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE court_id = $1`, f.courtID).Scan(&bookingsCount); err != nil {
			t.Fatalf("count rejected bookings: %v", err)
		}
		if bookingsCount != 0 {
			t.Fatalf("fractional input created %d bookings", bookingsCount)
		}
	})

	runOnlineBookingTxTest(t, "sequential retry prevents duplicate", 0, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
		svc := bookings.NewService(wrapper, 15, notif, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		req := bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "12:00"}
		first, err := svc.CreateBooking(ctx, f.customerID, req)
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := svc.CreateBooking(ctx, f.customerID, req); !errors.Is(err, bookings.ErrOverlapBooking) {
			t.Fatalf("expected ErrOverlapBooking on retry, got %v", err)
		}
		var bookingCount, snapshotCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE court_id = $1`, f.courtID).Scan(&bookingCount); err != nil {
			t.Fatalf("count retry bookings: %v", err)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM booking_fee_snapshots WHERE booking_id = $1`, first.ID).Scan(&snapshotCount); err != nil {
			t.Fatalf("count retry snapshots: %v", err)
		}
		if bookingCount != 1 || snapshotCount != 1 {
			t.Fatalf("expected one booking and one snapshot, got %d and %d", bookingCount, snapshotCount)
		}
	})

	for _, testCase := range []struct {
		name               string
		promo              promos.Promo
		expectedFinal      float64
		expectedCommission int64
	}{
		{"percentage promo uses final basis", promos.Promo{ID: testPromoID, Code: "CANONICAL", DiscountType: "PERCENTAGE", DiscountValue: 50, Status: "ACTIVE", StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().AddDate(1, 0, 0)}, 100000, 5000},
		{"fixed promo uses final basis", promos.Promo{ID: testPromoID, Code: "CANONICAL", DiscountType: "FIXED_AMOUNT", DiscountValue: 50000, Status: "ACTIVE", StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().AddDate(1, 0, 0)}, 150000, 7500},
	} {
		testCase := testCase
		runOnlineBookingTxTest(t, testCase.name, 500, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
			promoRepo := &mockPromoRepo{promo: &testCase.promo}
			svc := bookings.NewService(wrapper, 15, notif, promoRepo, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
			requestCode := "request-code"
			response, err := svc.CreateBooking(ctx, f.customerID, bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "12:00", PromoCode: &requestCode})
			if err != nil {
				t.Fatalf("create promo booking: %v", err)
			}
			if response.OriginalPrice == nil || *response.OriginalPrice != 200000 || response.FinalPrice == nil || *response.FinalPrice != testCase.expectedFinal || response.DiscountAmount != 200000-testCase.expectedFinal {
				t.Fatalf("unexpected promo response: %+v", response)
			}
			var adjustment, basis, amount int64
			var reason string
			if err := tx.QueryRow(ctx, `SELECT owner_price_adjustment_rupiah, commission_basis_amount_rupiah, commission_amount_rupiah, price_adjustment_reason FROM booking_fee_snapshots WHERE booking_id = $1`, response.ID).Scan(&adjustment, &basis, &amount, &reason); err != nil {
				t.Fatalf("read promo snapshot: %v", err)
			}
			if adjustment != int64(testCase.expectedFinal)-200000 || basis != int64(testCase.expectedFinal) || amount != testCase.expectedCommission || reason != "PROMO:CANONICAL" {
				t.Fatalf("unexpected promo snapshot adjustment=%d basis=%d amount=%d reason=%q", adjustment, basis, amount, reason)
			}
		})
	}

	runOnlineBookingTxTest(t, "zero-value promo is rejected", 500, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
		promoRepo := &mockPromoRepo{promo: &promos.Promo{ID: testPromoID, Code: "CANONICAL", DiscountType: "FIXED_AMOUNT", DiscountValue: 0.001, Status: "ACTIVE", StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().AddDate(1, 0, 0)}}
		svc := bookings.NewService(wrapper, 15, notif, promoRepo, mustOrchestrator(t, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		code := "tiny"
		if _, err := svc.CreateBooking(ctx, f.customerID, bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "12:00", PromoCode: &code}); !errors.Is(err, bookings.ErrInvalidPromoPrice) {
			t.Fatalf("expected ErrInvalidPromoPrice, got %v", err)
		}
	})

	runOnlineBookingTxTest(t, "calculator failure rolls back without notification", 500, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
		calculatorErr := errors.New("calculator failure")
		wrapper.rollbackOnError = true
		svc := bookings.NewService(wrapper, 15, notif, nil, mustOrchestrator(t, func(platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			return platformfinance.CalculatorResult{}, calculatorErr
		}, platformfinance.NewBookingFeeSnapshotRepository()))
		if _, err := svc.CreateBooking(ctx, f.customerID, bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "12:00"}); !errors.Is(err, calculatorErr) {
			t.Fatalf("expected calculator error, got %v", err)
		}
		if notif.called != 0 {
			t.Fatalf("notification called after calculator failure")
		}
	})

	runOnlineBookingTxTest(t, "resolver failure rolls back without notification", 500, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
		resolverErr := errors.New("resolver failure")
		wrapper.rollbackOnError = true
		resolver := func(ctx context.Context, db platformfinance.CommercialTermQueryer, ownerProfileID string, effectiveAt time.Time) (*platformfinance.CommercialTerm, error) {
			return nil, resolverErr
		}
		svc := bookings.NewService(wrapper, 15, notif, nil, mustOrchestratorWith(t, resolver, platformfinance.CalculateBookingFees, platformfinance.NewBookingFeeSnapshotRepository()))
		if _, err := svc.CreateBooking(ctx, f.customerID, bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "12:00"}); !errors.Is(err, resolverErr) {
			t.Fatalf("expected resolver error, got %v", err)
		}
		if notif.called != 0 {
			t.Fatalf("notification called after resolver failure")
		}
	})

	runOnlineBookingTxTest(t, "snapshot failure rolls back booking without notification", 500, func(t *testing.T, ctx context.Context, tx pgx.Tx, wrapper *testTxRepoWrapper, notif *mockNotifService, f onlineFixture) {
		snapshotErr := errors.New("snapshot failure")
		failingRepo := &failingSnapshotRepo{err: snapshotErr}
		wrapper.rollbackOnError = true
		svc := bookings.NewService(wrapper, 15, notif, nil, mustOrchestrator(t, platformfinance.CalculateBookingFees, failingRepo))
		if _, err := svc.CreateBooking(ctx, f.customerID, bookings.CreateBookingRequest{CourtID: f.courtID, BookingDate: futureBookingDate(t), StartTime: "10:00", EndTime: "12:00"}); !errors.Is(err, snapshotErr) {
			t.Fatalf("expected snapshot error, got %v", err)
		}
		if !failingRepo.called || notif.called != 0 {
			t.Fatalf("expected snapshot attempt and zero notifications, called=%t notifications=%d", failingRepo.called, notif.called)
		}
	})
}
