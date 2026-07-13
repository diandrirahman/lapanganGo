package platformfinance

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/database"
)

func ptrString(s string) *string {
	return &s
}

func setupSnapshotTestDB(t *testing.T) *pgxpool.Pool {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test")
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, dsn)
	require.NoError(t, err)

	return pool
}

func insertDependencyFixture(t *testing.T, ctx context.Context, tx pgx.Tx) (userID, ownerID, venueID, courtID, termID, bookingID string) {
	userID = uuid.New().String()
	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, status, created_at, updated_at)
		VALUES ($1, 'Test User', $2, 'hash', 'ACTIVE', now(), now())
	`, userID, userID+"@test.com")
	require.NoError(t, err)

	ownerID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO owner_profiles (id, user_id, business_name, created_at, updated_at)
		VALUES ($1, $2, 'Test Business', now(), now())
	`, ownerID, userID)
	require.NoError(t, err)

	venueID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO venues (id, owner_profile_id, name, status, created_at, updated_at, address, city)
		VALUES ($1, $2, 'Test Venue', 'ACTIVE', now(), now(), 'Test Address', 'Test City')
	`, venueID, ownerID)
	require.NoError(t, err)

	var sportID string
	err = tx.QueryRow(ctx, "SELECT id FROM sports LIMIT 1").Scan(&sportID)
	require.NoError(t, err, "Expected at least one seeded sport in database")

	courtID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, created_at, updated_at)
		VALUES ($1, $2, $3, 'Test Court', 'INDOOR', 100000, now(), now())
	`, courtID, venueID, sportID)
	require.NoError(t, err)

	bookingID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO bookings (id, court_id, customer_id, booking_date, start_time, end_time, status, total_price, created_at, updated_at)
		VALUES ($1, $2, $3, CURRENT_DATE, '10:00:00', '11:00:00', 'COMPLETED', 175000, now(), now())
	`, bookingID, courtID, userID)
	require.NoError(t, err)

	termID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO platform_commercial_terms
		(id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, created_at)
		VALUES ($1, $2, 'Test Term', 'STANDARD', 'SIMULATION', 'NONE', 700, now(), null, now())
	`, termID, &ownerID)
	require.NoError(t, err)

	return userID, ownerID, venueID, courtID, termID, bookingID
}

func assertSnapshotMapping(t *testing.T, params CreateBookingFeeSnapshotParams, snapshot *BookingFeeSnapshot) {
	t.Helper()
	assert.Equal(t, params.BookingID, snapshot.BookingID)
	assert.Equal(t, params.OwnerProfileID, snapshot.OwnerProfileID)
	assert.Equal(t, params.VenueID, snapshot.VenueID)

	if params.CommercialTermID != nil {
		require.NotNil(t, snapshot.CommercialTermID)
		assert.Equal(t, *params.CommercialTermID, *snapshot.CommercialTermID)
	} else {
		assert.Nil(t, snapshot.CommercialTermID)
	}

	assert.Equal(t, params.TermsSource, snapshot.TermsSource)
	assert.Equal(t, params.BookingChannel, snapshot.BookingChannel)
	assert.Equal(t, params.FinanceMode, snapshot.FinanceMode)
	assert.Equal(t, SnapshotCurrencyIDR, snapshot.Currency)
	assert.Equal(t, SnapshotCurrencyExponentIDR, snapshot.CurrencyExponent)
	assert.Equal(t, params.OriginalPriceRupiah, snapshot.OriginalPriceRupiah)
	assert.Equal(t, params.OwnerPriceAdjustmentRupiah, snapshot.OwnerPriceAdjustmentRupiah)

	if params.PriceAdjustmentReason != nil {
		require.NotNil(t, snapshot.PriceAdjustmentReason)
		assert.Equal(t, *params.PriceAdjustmentReason, *snapshot.PriceAdjustmentReason)
	} else {
		assert.Nil(t, snapshot.PriceAdjustmentReason)
	}

	assert.Equal(t, params.FinalBookingPriceRupiah, snapshot.FinalBookingPriceRupiah)
	assert.Equal(t, params.CustomerServiceFeeRupiah, snapshot.CustomerServiceFeeRupiah)
	assert.Equal(t, params.CustomerChargeAmountRupiah, snapshot.CustomerChargeAmountRupiah)
	assert.Equal(t, params.CommissionBasisAmountRupiah, snapshot.CommissionBasisAmountRupiah)
	assert.Equal(t, params.CommissionBps, snapshot.CommissionBps)
	assert.Equal(t, params.CommissionAmountRupiah, snapshot.CommissionAmountRupiah)
	assert.Equal(t, params.OwnerNetAmountRupiah, snapshot.OwnerNetAmountRupiah)
	assert.Equal(t, params.CalculationVersion, snapshot.CalculationVersion)
	assert.False(t, snapshot.CreatedAt.IsZero())
}

func TestBookingFeeSnapshotRepository_A_ExactPolicyInsertRead(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, ownerID, venueID, _, termID, bookingID := insertDependencyFixture(t, ctx, tx)

	repo := NewBookingFeeSnapshotRepository()

	reason := "Diskon"
	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            &termID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         200000,
		OwnerPriceAdjustmentRupiah:  -25000,
		PriceAdjustmentReason:       &reason,
		FinalBookingPriceRupiah:     175000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  175000,
		CommissionBasisAmountRupiah: 175000,
		CommissionBps:               700,
		CommissionAmountRupiah:      12250,
		OwnerNetAmountRupiah:        162750,
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	inserted, err := repo.InsertSnapshot(ctx, tx, params)
	require.NoError(t, err)

	require.NotNil(t, inserted)
	assertSnapshotMapping(t, params, inserted)

	read, err := repo.GetSnapshot(ctx, tx, bookingID)
	require.NoError(t, err)
	assertSnapshotMapping(t, params, read)
	assert.Equal(t, inserted, read)
}

func TestBookingFeeSnapshotRepository_B_ExactLegacyInsertRead(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, ownerID, venueID, _, _, bookingID := insertDependencyFixture(t, ctx, tx)

	repo := NewBookingFeeSnapshotRepository()

	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            nil,
		TermsSource:                 TermsSourceLegacyNoCommission,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         175000,
		OwnerPriceAdjustmentRupiah:  0,
		PriceAdjustmentReason:       nil,
		FinalBookingPriceRupiah:     175000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  175000,
		CommissionBasisAmountRupiah: 175000,
		CommissionBps:               0,
		CommissionAmountRupiah:      0,
		OwnerNetAmountRupiah:        175000,
		CalculationVersion:          LegacyBackfillCalculationVersionV1,
	}

	inserted, err := repo.InsertSnapshot(ctx, tx, params)
	require.NoError(t, err)

	require.NotNil(t, inserted)
	assertSnapshotMapping(t, params, inserted)

	read, err := repo.GetSnapshot(ctx, tx, bookingID)
	require.NoError(t, err)
	assertSnapshotMapping(t, params, read)
	assert.Equal(t, inserted, read)
}

func TestBookingFeeSnapshotRepository_C_CallerRollback(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Create persistent dependencies first
	txDep, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer txDep.Rollback(ctx)
	userID, ownerID, venueID, courtID, termID, bookingID := insertDependencyFixture(t, ctx, txDep)
	err = txDep.Commit(ctx)
	require.NoError(t, err)

	// Clean up after test
	defer func() {
		cleanupCtx, cancelClean := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelClean()

		txClean, err := pool.Begin(cleanupCtx)
		require.NoError(t, err)

		defer txClean.Rollback(cleanupCtx)

		// Term
		_, err = txClean.Exec(cleanupCtx, "DELETE FROM platform_commercial_terms WHERE id = $1", termID)
		require.NoError(t, err)

		// Booking
		_, err = txClean.Exec(cleanupCtx, "DELETE FROM bookings WHERE id = $1", bookingID)
		require.NoError(t, err)

		// Court
		_, err = txClean.Exec(cleanupCtx, "DELETE FROM courts WHERE id = $1", courtID)
		require.NoError(t, err)

		// Venue
		_, err = txClean.Exec(cleanupCtx, "DELETE FROM venues WHERE id = $1", venueID)
		require.NoError(t, err)

		// Owner Profile
		_, err = txClean.Exec(cleanupCtx, "DELETE FROM owner_profiles WHERE id = $1", ownerID)
		require.NoError(t, err)

		// User
		_, err = txClean.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)

		err = txClean.Commit(cleanupCtx)
		require.NoError(t, err)
	}()

	repo := NewBookingFeeSnapshotRepository()

	// Use a new transaction to insert and then rollback
	txApp, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer txApp.Rollback(ctx)

	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            &termID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         100000,
		OwnerPriceAdjustmentRupiah:  0,
		FinalBookingPriceRupiah:     100000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  100000,
		CommissionBasisAmountRupiah: 100000,
		CommissionBps:               700,
		CommissionAmountRupiah:      7000,
		OwnerNetAmountRupiah:        93000,
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	_, err = repo.InsertSnapshot(ctx, txApp, params)
	require.NoError(t, err)

	_, err = repo.GetSnapshot(ctx, txApp, bookingID)
	require.NoError(t, err) // Should exist inside tx

	// Rollback
	err = txApp.Rollback(ctx)
	require.NoError(t, err)

	// Query from pool (outside tx) should be not found
	_, err = repo.GetSnapshot(ctx, pool, bookingID)
	assert.ErrorIs(t, err, ErrBookingFeeSnapshotNotFound)
}

func TestBookingFeeSnapshotRepository_D_DuplicateDenied(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, ownerID, venueID, _, termID, bookingID := insertDependencyFixture(t, ctx, tx)

	repo := NewBookingFeeSnapshotRepository()

	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            &termID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         100000,
		OwnerPriceAdjustmentRupiah:  0,
		FinalBookingPriceRupiah:     100000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  100000,
		CommissionBasisAmountRupiah: 100000,
		CommissionBps:               700,
		CommissionAmountRupiah:      7000,
		OwnerNetAmountRupiah:        93000,
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	_, err = repo.InsertSnapshot(ctx, tx, params)
	require.NoError(t, err)

	_, err = repo.InsertSnapshot(ctx, tx, params)
	assert.ErrorIs(t, err, ErrBookingFeeSnapshotAlreadyExists)
}

func TestBookingFeeSnapshotRepository_E_TermChangeImmutable(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, ownerID, venueID, _, termID, bookingID := insertDependencyFixture(t, ctx, tx)

	repo := NewBookingFeeSnapshotRepository()

	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            &termID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         100000,
		OwnerPriceAdjustmentRupiah:  0,
		FinalBookingPriceRupiah:     100000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  100000,
		CommissionBasisAmountRupiah: 100000,
		CommissionBps:               700,
		CommissionAmountRupiah:      7000,
		OwnerNetAmountRupiah:        93000,
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	before, err := repo.InsertSnapshot(ctx, tx, params)
	require.NoError(t, err)

	// Update commercial term
	_, err = tx.Exec(ctx, "UPDATE platform_commercial_terms SET commission_bps = 800 WHERE id = $1", termID)
	require.NoError(t, err)

	after, err := repo.GetSnapshot(ctx, tx, bookingID)
	require.NoError(t, err)

	assert.Equal(t, before, after)
	assert.Equal(t, 700, after.CommissionBps)
}

func TestBookingFeeSnapshotRepository_F_MoneySerializationInt64(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, ownerID, venueID, _, termID, bookingID := insertDependencyFixture(t, ctx, tx)

	repo := NewBookingFeeSnapshotRepository()

	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            &termID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         math.MaxInt64,
		OwnerPriceAdjustmentRupiah:  0,
		FinalBookingPriceRupiah:     math.MaxInt64,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  math.MaxInt64,
		CommissionBasisAmountRupiah: math.MaxInt64,
		CommissionBps:               3000,
		CommissionAmountRupiah:      2767011611056432742,
		OwnerNetAmountRupiah:        6456360425798343065, // MaxInt64 - 2767011611056432742
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	inserted, err := repo.InsertSnapshot(ctx, tx, params)
	require.NoError(t, err)

	read, err := repo.GetSnapshot(ctx, tx, bookingID)
	require.NoError(t, err)

	assertSnapshotMapping(t, params, read)
	assert.Equal(t, inserted, read)
}

func TestBookingFeeSnapshotRepository_G_Validation(t *testing.T) {
	repo := NewBookingFeeSnapshotRepository()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	validUUID := uuid.New().String()
	validParams := CreateBookingFeeSnapshotParams{
		BookingID:                   validUUID,
		OwnerProfileID:              validUUID,
		VenueID:                     validUUID,
		CommercialTermID:            &validUUID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         100000,
		OwnerPriceAdjustmentRupiah:  0,
		FinalBookingPriceRupiah:     100000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  100000,
		CommissionBasisAmountRupiah: 100000,
		CommissionBps:               700,
		CommissionAmountRupiah:      7000,
		OwnerNetAmountRupiah:        93000,
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	testCases := []struct {
		name        string
		mutator     func(p *CreateBookingFeeSnapshotParams)
		expectedErr error
	}{
		{
			"POLICY without commercial term",
			func(p *CreateBookingFeeSnapshotParams) { p.CommercialTermID = nil },
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"POLICY with invalid version",
			func(p *CreateBookingFeeSnapshotParams) { p.CalculationVersion = "v2" },
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"LEGACY with commercial term",
			func(p *CreateBookingFeeSnapshotParams) {
				p.TermsSource = TermsSourceLegacyNoCommission
				p.CalculationVersion = LegacyBackfillCalculationVersionV1
				p.CommissionBps = 0
				p.CommissionAmountRupiah = 0
				p.CommercialTermID = &validUUID // Invalid
			},
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"LEGACY with invalid version",
			func(p *CreateBookingFeeSnapshotParams) {
				p.TermsSource = TermsSourceLegacyNoCommission
				p.CommercialTermID = nil
				p.CommissionBps = 0
				p.CommissionAmountRupiah = 0
				p.CalculationVersion = BookingFeeCalculationVersionV1 // Invalid
			},
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"LEGACY with nonzero BPS",
			func(p *CreateBookingFeeSnapshotParams) {
				p.TermsSource = TermsSourceLegacyNoCommission
				p.CommercialTermID = nil
				p.CalculationVersion = LegacyBackfillCalculationVersionV1
				p.CommissionBps = 700 // Invalid
				p.CommissionAmountRupiah = 0
			},
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"LEGACY with nonzero commission",
			func(p *CreateBookingFeeSnapshotParams) {
				p.TermsSource = TermsSourceLegacyNoCommission
				p.CommercialTermID = nil
				p.CalculationVersion = LegacyBackfillCalculationVersionV1
				p.CommissionBps = 0
				p.CommissionAmountRupiah = 7000 // Invalid
			},
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"LIVE finance mode",
			func(p *CreateBookingFeeSnapshotParams) { p.FinanceMode = "LIVE" },
			ErrUnsupportedSnapshotFinanceMode,
		},
		{
			"Invalid booking channel",
			func(p *CreateBookingFeeSnapshotParams) { p.BookingChannel = "INVALID" },
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"OWNER_WALK_IN with nonzero commission",
			func(p *CreateBookingFeeSnapshotParams) {
				p.BookingChannel = BookingChannelOwnerWalkIn
				p.CommissionBps = 700 // Invalid
			},
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"Adjustment nonzero without reason",
			func(p *CreateBookingFeeSnapshotParams) {
				p.OwnerPriceAdjustmentRupiah = 1000
				p.PriceAdjustmentReason = nil
			},
			ErrInvalidBookingFeeSnapshot,
		},
		{
			"Adjustment nonzero with empty reason",
			func(p *CreateBookingFeeSnapshotParams) {
				p.OwnerPriceAdjustmentRupiah = 1000
				p.PriceAdjustmentReason = ptrString("   ")
			},
			ErrInvalidBookingFeeSnapshot,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := validParams
			tc.mutator(&p)
			_, err := repo.InsertSnapshot(ctx, nil, p) // db is nil because validation fails before db call
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestBookingFeeSnapshotRepository_H_DatabaseConstraintPropagation(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, ownerID, venueID, _, termID, bookingID := insertDependencyFixture(t, ctx, tx)

	repo := NewBookingFeeSnapshotRepository()

	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            &termID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         100000,
		OwnerPriceAdjustmentRupiah:  0,
		FinalBookingPriceRupiah:     100000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  100000,
		CommissionBasisAmountRupiah: 100000,
		CommissionBps:               700,
		CommissionAmountRupiah:      7000,
		OwnerNetAmountRupiah:        93001, // Arithmetic mismatch (should be 93000)
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	_, err = repo.InsertSnapshot(ctx, tx, params)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidBookingFeeSnapshot) // Should be DB error

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	assert.Equal(t, "23514", pgErr.Code) // check_violation
}

func TestBookingFeeSnapshotRepository_I_NotFound(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBookingFeeSnapshotRepository()
	_, err := repo.GetSnapshot(ctx, pool, uuid.New().String())
	assert.ErrorIs(t, err, ErrBookingFeeSnapshotNotFound)
}

func TestBookingFeeSnapshotRepository_K_DatabaseImmutabilityRegression(t *testing.T) {
	pool := setupSnapshotTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, ownerID, venueID, _, termID, bookingID := insertDependencyFixture(t, ctx, tx)

	repo := NewBookingFeeSnapshotRepository()

	params := CreateBookingFeeSnapshotParams{
		BookingID:                   bookingID,
		OwnerProfileID:              ownerID,
		VenueID:                     venueID,
		CommercialTermID:            &termID,
		TermsSource:                 TermsSourcePolicy,
		BookingChannel:              BookingChannelMarketplaceOnline,
		FinanceMode:                 SnapshotFinanceModeSimulation,
		OriginalPriceRupiah:         100000,
		OwnerPriceAdjustmentRupiah:  0,
		FinalBookingPriceRupiah:     100000,
		CustomerServiceFeeRupiah:    0,
		CustomerChargeAmountRupiah:  100000,
		CommissionBasisAmountRupiah: 100000,
		CommissionBps:               700,
		CommissionAmountRupiah:      7000,
		OwnerNetAmountRupiah:        93000,
		CalculationVersion:          BookingFeeCalculationVersionV1,
	}

	_, err = repo.InsertSnapshot(ctx, tx, params)
	require.NoError(t, err)

	// Try to update via SQL directly
	// We use Savepoint so tx is not broken
	_, err = tx.Exec(ctx, "SAVEPOINT sp1")
	require.NoError(t, err)

	_, err = tx.Exec(ctx, "UPDATE booking_fee_snapshots SET final_booking_price_rupiah = 0 WHERE booking_id = $1", bookingID)
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	assert.Contains(t, pgErr.Message, "strictly forbidden")

	_, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT sp1")
	require.NoError(t, err)

	_, err = tx.Exec(ctx, "DELETE FROM booking_fee_snapshots WHERE booking_id = $1", bookingID)
	require.Error(t, err)
	require.True(t, errors.As(err, &pgErr))
	assert.Contains(t, pgErr.Message, "strictly forbidden")
}
