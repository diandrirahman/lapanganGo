package platformfinance

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test: TEST_INTEGRATION != 1")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	return pool
}

func insertTestOwner(t *testing.T, ctx context.Context, tx pgx.Tx, ownerID string) {
	userID := uuid.New().String()
	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, status, created_at, updated_at)
		VALUES ($1, 'Test User', $2, 'hash', 'ACTIVE', now(), now())
	`, userID, userID+"@test.com")
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO owner_profiles (id, user_id, business_name, created_at, updated_at)
		VALUES ($1, $2, 'Test Business', now(), now())
	`, ownerID, userID)
	require.NoError(t, err)
}

func insertTerm(t *testing.T, ctx context.Context, tx pgx.Tx, termID string, ownerID *string, phase, financeMode, collectionMethod string, bps int, validFrom time.Time, validUntil *time.Time) {
	_, err := tx.Exec(ctx, `
		INSERT INTO platform_commercial_terms
		(id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, created_at)
		VALUES ($1, $2, 'Test Term', $3, $4, $5, $6, $7, $8, now())
	`, termID, ownerID, phase, financeMode, collectionMethod, bps, validFrom, validUntil)
	require.NoError(t, err)
}

func countTerms(t *testing.T, ctx context.Context, tx pgx.Tx) int {
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM platform_commercial_terms`).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestResolveEffectiveTerm(t *testing.T) {
	pool := getTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	t.Run("A. Global fallback (owner valid without override)", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		resolver := NewCommercialTermResolver(tx)
		term, err := resolver.ResolveEffectiveTerm(ctx, ownerID, time.Now())
		require.NoError(t, err)
		require.NotNil(t, term)
		assert.Nil(t, term.OwnerProfileID)
		assert.Equal(t, "Global Default Term", term.Label)
		assert.Equal(t, 700, term.CommissionBps)
	})

	t.Run("B. Owner override", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		termID := uuid.New().String()
		now := time.Now()
		insertTerm(t, ctx, tx, termID, &ownerID, "STANDARD", "SIMULATION", "NONE", 500, now.Add(-time.Hour), nil)

		resolver := NewCommercialTermResolver(tx)
		term, err := resolver.ResolveEffectiveTerm(ctx, ownerID, now)
		require.NoError(t, err)
		require.NotNil(t, term)
		require.NotNil(t, term.OwnerProfileID)
		assert.Equal(t, ownerID, *term.OwnerProfileID)
		assert.Equal(t, 500, term.CommissionBps)
	})

	t.Run("C. Exact boundaries [)", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		t0 := time.Now().Add(24 * time.Hour)
		t1 := t0.Add(24 * time.Hour)
		t2 := t1.Add(24 * time.Hour)

		termA := uuid.New().String()
		insertTerm(t, ctx, tx, termA, &ownerID, "STANDARD", "SIMULATION", "NONE", 100, t0, &t1)

		termB := uuid.New().String()
		insertTerm(t, ctx, tx, termB, &ownerID, "STANDARD", "SIMULATION", "NONE", 200, t1, &t2)

		resolver := NewCommercialTermResolver(tx)

		// At t0 -> Term A
		res, err := resolver.ResolveEffectiveTerm(ctx, ownerID, t0)
		require.NoError(t, err)
		assert.Equal(t, termA, res.ID)

		// At t1 -> Term B (exclusive A, inclusive B)
		res, err = resolver.ResolveEffectiveTerm(ctx, ownerID, t1)
		require.NoError(t, err)
		assert.Equal(t, termB, res.ID)

		// At t2 -> Global fallback (exclusive B)
		res, err = resolver.ResolveEffectiveTerm(ctx, ownerID, t2)
		require.NoError(t, err)
		assert.Nil(t, res.OwnerProfileID)
	})

	t.Run("E. Scheduled dan historical exclusion", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		now := time.Now()
		pastFrom := now.Add(-48 * time.Hour)
		pastUntil := now.Add(-24 * time.Hour)

		futureFrom := now.Add(24 * time.Hour)
		futureUntil := now.Add(48 * time.Hour)

		insertTerm(t, ctx, tx, uuid.New().String(), &ownerID, "STANDARD", "SIMULATION", "NONE", 100, pastFrom, &pastUntil)
		insertTerm(t, ctx, tx, uuid.New().String(), &ownerID, "STANDARD", "SIMULATION", "NONE", 200, futureFrom, &futureUntil)

		resolver := NewCommercialTermResolver(tx)
		// Request at `now`, neither should match. It should fallback to global.
		res, err := resolver.ResolveEffectiveTerm(ctx, ownerID, now)
		require.NoError(t, err)
		assert.Nil(t, res.OwnerProfileID)
	})

	t.Run("F. Missing effective term", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		// Delete global term
		_, err = tx.Exec(ctx, "DELETE FROM platform_commercial_terms WHERE owner_profile_id IS NULL")
		require.NoError(t, err)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		resolver := NewCommercialTermResolver(tx)
		_, err = resolver.ResolveEffectiveTerm(ctx, ownerID, time.Now())
		assert.ErrorIs(t, err, ErrMissingEffectiveCommercialTerm)
	})

	t.Run("G. Duplicate owner fail-closed", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		// Drop constraint to allow duplicate
		_, err = tx.Exec(ctx, "ALTER TABLE platform_commercial_terms DROP CONSTRAINT excl_pct_no_overlap")
		require.NoError(t, err)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t1 := t0.Add(24 * time.Hour)

		// Insert duplicates manually
		insertTerm(t, ctx, tx, uuid.New().String(), &ownerID, "STANDARD", "SIMULATION", "NONE", 100, t0, &t1)
		insertTerm(t, ctx, tx, uuid.New().String(), &ownerID, "STANDARD", "SIMULATION", "NONE", 200, t0, &t1)

		resolver := NewCommercialTermResolver(tx)
		_, err = resolver.ResolveEffectiveTerm(ctx, ownerID, t0.Add(1*time.Hour))
		assert.ErrorIs(t, err, ErrDuplicateCommercialTerm)
	})

	t.Run("H. Duplicate global fail-closed", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, "ALTER TABLE platform_commercial_terms DROP CONSTRAINT excl_pct_no_overlap")
		require.NoError(t, err)

		// Delete existing global
		_, err = tx.Exec(ctx, "DELETE FROM platform_commercial_terms WHERE owner_profile_id IS NULL")
		require.NoError(t, err)

		t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t1 := t0.Add(24 * time.Hour)

		// Insert two identical global bounded terms
		insertTerm(t, ctx, tx, uuid.New().String(), nil, "STANDARD", "SIMULATION", "NONE", 100, t0, &t1)
		insertTerm(t, ctx, tx, uuid.New().String(), nil, "STANDARD", "SIMULATION", "NONE", 200, t0, &t1)

		ownerID := uuid.New().String()

		resolver := NewCommercialTermResolver(tx)
		_, err = resolver.ResolveEffectiveTerm(ctx, ownerID, t0.Add(1*time.Hour))
		assert.ErrorIs(t, err, ErrDuplicateCommercialTerm)
	})

	t.Run("I. Unsupported LIVE term", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		// Insert LIVE
		insertTerm(t, ctx, tx, uuid.New().String(), &ownerID, "STANDARD", "LIVE", "NONE", 100, time.Now().Add(-time.Hour), nil)

		resolver := NewCommercialTermResolver(tx)
		_, err = resolver.ResolveEffectiveTerm(ctx, ownerID, time.Now())
		assert.ErrorIs(t, err, ErrUnsupportedCommercialTermFinanceMode)
	})

	t.Run("J. Invalid data defense", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		// Drop check constraints so we can insert invalid data
		_, err = tx.Exec(ctx, "ALTER TABLE platform_commercial_terms DROP CONSTRAINT platform_commercial_terms_collection_method_check")
		require.NoError(t, err)

		_, err = tx.Exec(ctx, "ALTER TABLE platform_commercial_terms DROP CONSTRAINT platform_commercial_terms_commission_bps_check")
		require.NoError(t, err)

		_, err = tx.Exec(ctx, "ALTER TABLE platform_commercial_terms DROP CONSTRAINT platform_commercial_terms_phase_check")
		require.NoError(t, err)

		t0 := time.Now()

		testCases := []struct {
			name             string
			collectionMethod string
			bps              int
			phase            string
			validFrom        time.Time
			validUntil       *time.Time
		}{
			{"Invalid Collection Method", "CASH", 100, "STANDARD", t0, nil},
			{"Negative BPS", "NONE", -1, "STANDARD", t0, nil},
			{"Oversized BPS", "NONE", 3001, "STANDARD", t0, nil},
			{"Invalid Phase", "NONE", 100, "INVALID_PHASE", t0, nil},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ownerID := uuid.New().String()
				// We don't insertTestOwner to speed up, UUID validation will pass and it will query.
				// Wait, foreign key owner_profile_id prevents insert unless owner_profile exists.
				insertTestOwner(t, ctx, tx, ownerID)

				insertTerm(t, ctx, tx, uuid.New().String(), &ownerID, tc.phase, "SIMULATION", tc.collectionMethod, tc.bps, tc.validFrom, tc.validUntil)

				resolver := NewCommercialTermResolver(tx)
				_, err := resolver.ResolveEffectiveTerm(ctx, ownerID, time.Now())
				assert.ErrorIs(t, err, ErrInvalidResolvedCommercialTerm)
			})
		}
	})

	t.Run("K. Invalid inputs", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)
		resolver := NewCommercialTermResolver(tx)

		_, err = resolver.ResolveEffectiveTerm(ctx, "not-a-uuid", time.Now())
		assert.ErrorIs(t, err, ErrInvalidCommercialTermOwner)

		_, err = resolver.ResolveEffectiveTerm(ctx, "", time.Now())
		assert.ErrorIs(t, err, ErrInvalidCommercialTermOwner)

		_, err = resolver.ResolveEffectiveTerm(ctx, uuid.New().String(), time.Time{})
		assert.ErrorIs(t, err, ErrInvalidCommercialTermTimestamp)
	})

	t.Run("L. Read-only proof", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		ownerID := uuid.New().String()
		insertTestOwner(t, ctx, tx, ownerID)

		countBefore := countTerms(t, ctx, tx)

		resolver := NewCommercialTermResolver(tx)
		_, err = resolver.ResolveEffectiveTerm(ctx, ownerID, time.Now())
		require.NoError(t, err)

		countAfter := countTerms(t, ctx, tx)
		assert.Equal(t, countBefore, countAfter)
	})
}
