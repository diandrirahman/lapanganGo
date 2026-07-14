package bookings_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/bookings"
	"lapangango-api/internal/platformfinance"
)

// --- UNIT TESTS ---

type mockTx struct {
	pgx.Tx
}

type mockSnapshotRepo struct {
	insertFn func(ctx context.Context, db platformfinance.SnapshotDBTX, params platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error)
}

func (m *mockSnapshotRepo) InsertSnapshot(ctx context.Context, db platformfinance.SnapshotDBTX, params platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
	if m.insertFn != nil {
		return m.insertFn(ctx, db, params)
	}
	return nil, nil
}

func (m *mockSnapshotRepo) GetSnapshot(ctx context.Context, db platformfinance.SnapshotDBTX, bookingID string) (*platformfinance.BookingFeeSnapshot, error) {
	return nil, nil
}

func TestSnapshotTransactionOrchestrator(t *testing.T) {
	ctx := context.Background()
	tx := &mockTx{}

	errSentinel := errors.New("sentinel error")
	ownerID := uuid.NewString()
	venueID := uuid.NewString()
	effectiveAt := time.Now().UTC()

	reqBase := bookings.SnapshotOrchestrationRequest{
		OwnerProfileID:             ownerID,
		VenueID:                    venueID,
		EffectiveAt:                effectiveAt,
		Channel:                    platformfinance.BookingChannelMarketplaceOnline,
		OriginalPriceRupiah:        100000,
		OwnerPriceAdjustmentRupiah: -10000,
		PriceAdjustmentReason:      " promo 10k ",
	}

	validTerm := &platformfinance.CommercialTerm{
		ID:            uuid.NewString(),
		CommissionBps: 700,
		FinanceMode:   platformfinance.SnapshotFinanceModeSimulation,
	}

	validCalcResult := platformfinance.CalculatorResult{
		FinalBookingPriceRupiah:     90000,
		CustomerChargeAmountRupiah:  90000,
		CommissionBasisAmountRupiah: 90000,
		CommissionBps:               700,
		CommissionAmountRupiah:      6300,
		OwnerNetAmountRupiah:        83700,
	}

	dummyResolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
		return validTerm, nil
	}
	dummyCalculator := func(p platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
		return validCalcResult, nil
	}
	dummyRepo := &mockSnapshotRepo{}

	t.Run("constructor guard - nil resolver", func(t *testing.T) {
		_, err := bookings.NewSnapshotOrchestrator(nil, dummyCalculator, dummyRepo)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("constructor guard - nil calculator", func(t *testing.T) {
		_, err := bookings.NewSnapshotOrchestrator(dummyResolver, nil, dummyRepo)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("constructor guard - nil repository", func(t *testing.T) {
		_, err := bookings.NewSnapshotOrchestrator(dummyResolver, dummyCalculator, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid request - zero EffectiveAt", func(t *testing.T) {
		req := reqBase
		req.EffectiveAt = time.Time{}
		o, err := bookings.NewSnapshotOrchestrator(dummyResolver, dummyCalculator, dummyRepo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = o.CreateBookingWithSnapshot(ctx, tx, req, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			return bookings.Booking{}, nil
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid request - nil insert callback", func(t *testing.T) {
		req := reqBase
		o, err := bookings.NewSnapshotOrchestrator(dummyResolver, dummyCalculator, dummyRepo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = o.CreateBookingWithSnapshot(ctx, tx, req, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid request - invalid channel", func(t *testing.T) {
		req := reqBase
		req.Channel = "INVALID_CHANNEL"
		o, err := bookings.NewSnapshotOrchestrator(dummyResolver, dummyCalculator, dummyRepo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = o.CreateBookingWithSnapshot(ctx, tx, req, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			return bookings.Booking{}, nil
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("resolver returns nil term", func(t *testing.T) {
		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			return nil, nil
		}
		o, _ := bookings.NewSnapshotOrchestrator(resolver, dummyCalculator, dummyRepo)
		_, _, err := o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			return bookings.Booking{}, nil
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("resolver returns term with empty ID", func(t *testing.T) {
		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			return &platformfinance.CommercialTerm{ID: ""}, nil
		}
		o, _ := bookings.NewSnapshotOrchestrator(resolver, dummyCalculator, dummyRepo)
		_, _, err := o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			return bookings.Booking{}, nil
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("resolver failure", func(t *testing.T) {
		resolverCalled := false
		calcCalled := false
		cbCalled := false
		repoCalled := false

		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			resolverCalled = true
			return nil, errSentinel
		}
		calculator := func(p platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			calcCalled = true
			return platformfinance.CalculatorResult{}, nil
		}
		repo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				repoCalled = true
				return nil, nil
			},
		}

		o, err := bookings.NewSnapshotOrchestrator(resolver, calculator, repo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			cbCalled = true
			return bookings.Booking{}, nil
		})

		if !errors.Is(err, errSentinel) {
			t.Fatalf("expected errSentinel, got %v", err)
		}
		if !resolverCalled {
			t.Error("resolver should be called")
		}
		if calcCalled || cbCalled || repoCalled {
			t.Error("calculator, cb, repo should not be called")
		}
	})

	t.Run("calculator failure", func(t *testing.T) {
		cbCalled := false
		repoCalled := false

		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			return validTerm, nil
		}
		calculator := func(p platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			return platformfinance.CalculatorResult{}, errSentinel
		}
		repo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				repoCalled = true
				return nil, nil
			},
		}

		o, err := bookings.NewSnapshotOrchestrator(resolver, calculator, repo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			cbCalled = true
			return bookings.Booking{}, nil
		})

		if !errors.Is(err, errSentinel) {
			t.Fatalf("expected errSentinel, got %v", err)
		}
		if cbCalled || repoCalled {
			t.Error("cb, repo should not be called")
		}
	})

	t.Run("booking callback failure", func(t *testing.T) {
		repoCalled := false

		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			return validTerm, nil
		}
		calculator := func(p platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			return validCalcResult, nil
		}
		repo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				repoCalled = true
				return nil, nil
			},
		}

		o, err := bookings.NewSnapshotOrchestrator(resolver, calculator, repo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(ctx context.Context, t pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			if t != tx {
				return bookings.Booking{}, errors.New("tx mismatch")
			}
			return bookings.Booking{}, errSentinel
		})

		if !errors.Is(err, errSentinel) {
			t.Fatalf("expected errSentinel, got %v", err)
		}
		if repoCalled {
			t.Error("repo should not be called")
		}
	})

	t.Run("snapshot failure", func(t *testing.T) {
		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			return validTerm, nil
		}
		calculator := func(p platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			return validCalcResult, nil
		}
		repo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				return nil, errSentinel
			},
		}

		o, err := bookings.NewSnapshotOrchestrator(resolver, calculator, repo)
		if err != nil {
			t.Fatal(err)
		}
		bookingID := uuid.NewString()
		b, s, err := o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			return bookings.Booking{ID: bookingID}, nil
		})

		if !errors.Is(err, errSentinel) {
			t.Fatalf("expected errSentinel, got %v", err)
		}
		if b.ID != "" || s != nil {
			t.Error("method must not return usable Booking/snapshot on error")
		}
	})

	t.Run("snapshot returns nil", func(t *testing.T) {
		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			return validTerm, nil
		}
		calculator := func(p platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			return validCalcResult, nil
		}
		repo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				return nil, nil // return nil without error
			},
		}

		o, err := bookings.NewSnapshotOrchestrator(resolver, calculator, repo)
		if err != nil {
			t.Fatal(err)
		}
		bookingID := uuid.NewString()
		b, s, err := o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(ctx context.Context, tx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			return bookings.Booking{ID: bookingID}, nil
		})

		if err == nil {
			t.Fatal("expected error")
		}
		if b.ID != "" || s != nil {
			t.Error("method must not return usable Booking/snapshot on error")
		}
	})

	t.Run("success mapping and call ordering", func(t *testing.T) {
		callOrder := []string{}
		var passedPricing bookings.CanonicalBookingPricing
		var passedSnapshot platformfinance.CreateBookingFeeSnapshotParams

		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			callOrder = append(callOrder, "resolve")
			if db != tx {
				return nil, errors.New("resolver tx mismatch")
			}
			return validTerm, nil
		}
		calculator := func(p platformfinance.CalculatorParams) (platformfinance.CalculatorResult, error) {
			callOrder = append(callOrder, "calculate")
			return validCalcResult, nil
		}
		bookingID := uuid.NewString()
		repo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				callOrder = append(callOrder, "snapshot")
				if db != tx {
					return nil, errors.New("repo tx mismatch")
				}
				passedSnapshot = p
				return &platformfinance.BookingFeeSnapshot{BookingID: bookingID}, nil
			},
		}

		o, err := bookings.NewSnapshotOrchestrator(resolver, calculator, repo)
		if err != nil {
			t.Fatal(err)
		}
		b, s, err := o.CreateBookingWithSnapshot(ctx, tx, reqBase, func(c context.Context, t pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			callOrder = append(callOrder, "insert")
			if t != tx {
				return bookings.Booking{}, errors.New("cb tx mismatch")
			}
			passedPricing = pricing
			return bookings.Booking{ID: bookingID}, nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.ID != bookingID {
			t.Error("expected booking id")
		}
		if s == nil || s.BookingID != bookingID {
			t.Error("expected snapshot with booking id")
		}

		if len(callOrder) != 4 || callOrder[0] != "resolve" || callOrder[1] != "calculate" || callOrder[2] != "insert" || callOrder[3] != "snapshot" {
			t.Errorf("incorrect call order: %v", callOrder)
		}

		// check canonical pricing
		if passedPricing.OriginalPriceRupiah != 100000 ||
			passedPricing.OwnerPriceAdjustmentRupiah != -10000 ||
			passedPricing.PriceAdjustmentReason != "promo 10k" ||
			passedPricing.FinalBookingPriceRupiah != 90000 ||
			passedPricing.CustomerChargeAmountRupiah != 90000 ||
			passedPricing.CommissionBasisAmountRupiah != 90000 ||
			passedPricing.EffectiveCommissionBps != 700 ||
			passedPricing.CommissionAmountRupiah != 6300 ||
			passedPricing.OwnerNetAmountRupiah != 83700 {
			t.Errorf("incorrect pricing mapping: %+v", passedPricing)
		}

		// check snapshot params
		if passedSnapshot.BookingID != bookingID ||
			passedSnapshot.OwnerProfileID != ownerID ||
			passedSnapshot.VenueID != venueID ||
			*passedSnapshot.CommercialTermID != validTerm.ID ||
			passedSnapshot.TermsSource != platformfinance.TermsSourcePolicy ||
			passedSnapshot.FinanceMode != validTerm.FinanceMode ||
			passedSnapshot.CalculationVersion != platformfinance.BookingFeeCalculationVersionV1 ||
			passedSnapshot.CommissionBps != 700 ||
			*passedSnapshot.PriceAdjustmentReason != "promo 10k" {
			t.Errorf("incorrect snapshot mapping: %+v", passedSnapshot)
		}
	})

	t.Run("walk in channel effective bps 0", func(t *testing.T) {
		reqWalkIn := reqBase
		reqWalkIn.Channel = platformfinance.BookingChannelOwnerWalkIn

		resolver := func(c context.Context, db platformfinance.CommercialTermQueryer, owner string, eff time.Time) (*platformfinance.CommercialTerm, error) {
			return validTerm, nil
		}
		calculator := platformfinance.CalculateBookingFees // Use actual calculator
		bookingID := uuid.NewString()
		repo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				if p.CommissionBps != 0 {
					t.Errorf("expected commission bps 0 for walk in, got %v", p.CommissionBps)
				}
				if p.CommissionAmountRupiah != 0 {
					t.Errorf("expected commission amount 0 for walk in, got %v", p.CommissionAmountRupiah)
				}
				return &platformfinance.BookingFeeSnapshot{BookingID: bookingID}, nil
			},
		}

		o, err := bookings.NewSnapshotOrchestrator(resolver, calculator, repo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = o.CreateBookingWithSnapshot(ctx, tx, reqWalkIn, func(c context.Context, dbTx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			if pricing.EffectiveCommissionBps != 0 {
				t.Errorf("expected effective bps 0 for walk in, got %v", pricing.EffectiveCommissionBps)
			}
			return bookings.Booking{ID: bookingID}, nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- INTEGRATION TESTS ---

func defaultResolverAdapter(ctx context.Context, db platformfinance.CommercialTermQueryer, ownerProfileID string, effectiveAt time.Time) (*platformfinance.CommercialTerm, error) {
	r := platformfinance.NewCommercialTermResolver(db)
	return r.ResolveEffectiveTerm(ctx, ownerProfileID, effectiveAt)
}

type integrationFixture struct {
	ownerProfileID string
	venueID        string
	userID         string
	courtID        string
	termID         string
	bookingID      string
	reqBase        bookings.SnapshotOrchestrationRequest
}

func setupIntegrationFixtures(t *testing.T, ctx context.Context, tx pgx.Tx, seededSportID string) integrationFixture {
	f := integrationFixture{
		ownerProfileID: uuid.NewString(),
		venueID:        uuid.NewString(),
		userID:         uuid.NewString(),
		courtID:        uuid.NewString(),
		termID:         uuid.NewString(),
		bookingID:      uuid.NewString(),
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash)
		VALUES ($1, 'Test User', $2, 'hash')
	`, f.userID, f.userID+"@example.com")
	if err != nil {
		t.Fatalf("setup user: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO owner_profiles (id, user_id, business_name)
		VALUES ($1, $2, 'Test Business')
	`, f.ownerProfileID, f.userID)
	if err != nil {
		t.Fatalf("setup owner: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO venues (id, owner_profile_id, name, address, city)
		VALUES ($1, $2, 'Test Venue', 'Address', 'City')
	`, f.venueID, f.ownerProfileID)
	if err != nil {
		t.Fatalf("setup venue: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, status)
		VALUES ($1, $2, $3, 'Test Court', 'INDOOR', 100000, 'ACTIVE')
	`, f.courtID, f.venueID, seededSportID)
	if err != nil {
		t.Fatalf("setup court: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO platform_commercial_terms (id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, created_by_user_id)
		VALUES ($1, $2, 'Integration Term', 'STANDARD', 'SIMULATION', 'NONE', 1000, '2020-01-01 00:00:00Z', $3)
	`, f.termID, f.ownerProfileID, f.userID)
	if err != nil {
		t.Fatalf("setup term: %v", err)
	}

	f.reqBase = bookings.SnapshotOrchestrationRequest{
		OwnerProfileID:             f.ownerProfileID,
		VenueID:                    f.venueID,
		EffectiveAt:                time.Now().UTC(),
		Channel:                    platformfinance.BookingChannelMarketplaceOnline,
		OriginalPriceRupiah:        100000,
		OwnerPriceAdjustmentRupiah: 0,
		PriceAdjustmentReason:      "",
	}
	return f
}

func checkLeaks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f integrationFixture) {
	queries := map[string]string{
		"users":                     "SELECT count(*) FROM users WHERE id = $1",
		"owner_profiles":            "SELECT count(*) FROM owner_profiles WHERE id = $1",
		"venues":                    "SELECT count(*) FROM venues WHERE id = $1",
		"courts":                    "SELECT count(*) FROM courts WHERE id = $1",
		"platform_commercial_terms": "SELECT count(*) FROM platform_commercial_terms WHERE id = $1",
		"bookings":                  "SELECT count(*) FROM bookings WHERE id = $1",
		"booking_fee_snapshots":     "SELECT count(*) FROM booking_fee_snapshots WHERE booking_id = $1",
	}

	args := map[string]string{
		"users":                     f.userID,
		"owner_profiles":            f.ownerProfileID,
		"venues":                    f.venueID,
		"courts":                    f.courtID,
		"platform_commercial_terms": f.termID,
		"bookings":                  f.bookingID,
		"booking_fee_snapshots":     f.bookingID,
	}

	for table, query := range queries {
		var count int
		err := pool.QueryRow(ctx, query, args[table]).Scan(&count)
		if err != nil {
			t.Errorf("error checking %s leak: %v", table, err)
		} else if count != 0 {
			t.Errorf("leaked data in %s: expected 0, got %d", table, count)
		}
	}
}

func TestSnapshotTransactionOrchestratorIntegration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test")
	}

	dbUrl := os.Getenv("TEST_DATABASE_URL")
	if dbUrl == "" {
		t.Fatal("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbUrl)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	var seededSportID string
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = pool.QueryRow(verifyCtx, "SELECT id FROM sports LIMIT 1").Scan(&seededSportID)
	verifyCancel()
	if err != nil {
		t.Fatalf("failed to find a seeded sport: %v", err)
	}

	t.Run("Success in caller transaction", func(t *testing.T) {
		ctxT, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctxT)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			err = tx.Rollback(ctxT)
			if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("rollback failed: %v", err)
			}
		}()

		f := setupIntegrationFixtures(t, ctxT, tx, seededSportID)

		snapshotRepo := platformfinance.NewBookingFeeSnapshotRepository()
		calculator := platformfinance.CalculateBookingFees
		o, err := bookings.NewSnapshotOrchestrator(defaultResolverAdapter, calculator, snapshotRepo)
		if err != nil {
			t.Fatal(err)
		}

		booking, snapshot, err := o.CreateBookingWithSnapshot(ctxT, tx, f.reqBase, func(c context.Context, dbTx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			_, e := dbTx.Exec(c, `
				INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, original_price, discount_amount, final_price, total_price, status)
				VALUES ($1, $2, $3, CURRENT_DATE, '10:00:00', '11:00:00', $4, $5, $6, $7, 'PENDING_PAYMENT')
			`, f.bookingID, f.userID, f.courtID, pricing.OriginalPriceRupiah, pricing.OriginalPriceRupiah-pricing.FinalBookingPriceRupiah, pricing.FinalBookingPriceRupiah, pricing.FinalBookingPriceRupiah)
			return bookings.Booking{ID: f.bookingID}, e
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if booking.ID != f.bookingID {
			t.Errorf("expected %s, got %s", f.bookingID, booking.ID)
		}

		if snapshot.BookingID != f.bookingID ||
			snapshot.OwnerProfileID != f.ownerProfileID ||
			snapshot.VenueID != f.venueID ||
			*snapshot.CommercialTermID != f.termID ||
			snapshot.TermsSource != platformfinance.TermsSourcePolicy ||
			snapshot.BookingChannel != platformfinance.BookingChannelMarketplaceOnline ||
			snapshot.FinanceMode != platformfinance.SnapshotFinanceModeSimulation ||
			snapshot.OriginalPriceRupiah != 100000 ||
			snapshot.OwnerPriceAdjustmentRupiah != 0 ||
			snapshot.PriceAdjustmentReason != nil ||
			snapshot.FinalBookingPriceRupiah != 100000 ||
			snapshot.CustomerServiceFeeRupiah != 0 ||
			snapshot.CustomerChargeAmountRupiah != 100000 ||
			snapshot.CommissionBasisAmountRupiah != 100000 ||
			snapshot.CommissionBps != 1000 ||
			snapshot.CommissionAmountRupiah != 10000 ||
			snapshot.OwnerNetAmountRupiah != 90000 ||
			snapshot.CalculationVersion != platformfinance.BookingFeeCalculationVersionV1 {
			t.Errorf("snapshot mismatch: %+v", snapshot)
		}

		// check inside tx
		var count int
		err = tx.QueryRow(ctxT, "SELECT count(*) FROM bookings WHERE id = $1", f.bookingID).Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("booking should exist inside tx: %v, count: %d", err, count)
		}

		err = tx.Rollback(ctxT)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback failed: %v", err)
		}

		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer verifyCancel()
		checkLeaks(t, verifyCtx, pool, f)
	})

	t.Run("Snapshot failure after actual booking insert", func(t *testing.T) {
		ctxT, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctxT)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			err = tx.Rollback(ctxT)
			if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("rollback failed: %v", err)
			}
		}()

		f := setupIntegrationFixtures(t, ctxT, tx, seededSportID)

		errSentinel := errors.New("db error")
		failRepo := &mockSnapshotRepo{
			insertFn: func(c context.Context, db platformfinance.SnapshotDBTX, p platformfinance.CreateBookingFeeSnapshotParams) (*platformfinance.BookingFeeSnapshot, error) {
				return nil, errSentinel
			},
		}
		calculator := platformfinance.CalculateBookingFees
		o, err := bookings.NewSnapshotOrchestrator(defaultResolverAdapter, calculator, failRepo)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = o.CreateBookingWithSnapshot(ctxT, tx, f.reqBase, func(c context.Context, dbTx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			_, e := dbTx.Exec(c, `
				INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, original_price, discount_amount, final_price, total_price, status)
				VALUES ($1, $2, $3, CURRENT_DATE, '10:00:00', '11:00:00', $4, $5, $6, $7, 'PENDING_PAYMENT')
			`, f.bookingID, f.userID, f.courtID, pricing.OriginalPriceRupiah, pricing.OriginalPriceRupiah-pricing.FinalBookingPriceRupiah, pricing.FinalBookingPriceRupiah, pricing.FinalBookingPriceRupiah)
			return bookings.Booking{ID: f.bookingID}, e
		})

		if !errors.Is(err, errSentinel) {
			t.Fatalf("expected errSentinel, got %v", err)
		}

		err = tx.Rollback(ctxT)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback failed: %v", err)
		}

		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer verifyCancel()
		checkLeaks(t, verifyCtx, pool, f)
	})

	t.Run("No nested transaction", func(t *testing.T) {
		ctxT, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		tx, err := pool.Begin(ctxT)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			err = tx.Rollback(ctxT)
			if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("rollback failed: %v", err)
			}
		}()

		f := setupIntegrationFixtures(t, ctxT, tx, seededSportID)

		snapshotRepo := platformfinance.NewBookingFeeSnapshotRepository()
		calculator := platformfinance.CalculateBookingFees
		o, err := bookings.NewSnapshotOrchestrator(defaultResolverAdapter, calculator, snapshotRepo)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = o.CreateBookingWithSnapshot(ctxT, tx, f.reqBase, func(c context.Context, dbTx pgx.Tx, pricing bookings.CanonicalBookingPricing) (bookings.Booking, error) {
			_, e := dbTx.Exec(c, `
				INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, original_price, discount_amount, final_price, total_price, status)
				VALUES ($1, $2, $3, CURRENT_DATE, '10:00:00', '11:00:00', $4, $5, $6, $7, 'PENDING_PAYMENT')
			`, f.bookingID, f.userID, f.courtID, pricing.OriginalPriceRupiah, pricing.OriginalPriceRupiah-pricing.FinalBookingPriceRupiah, pricing.FinalBookingPriceRupiah, pricing.FinalBookingPriceRupiah)
			return bookings.Booking{ID: f.bookingID}, e
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = tx.Rollback(ctxT)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback failed: %v", err)
		}

		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer verifyCancel()
		checkLeaks(t, verifyCtx, pool, f)
	})
}
