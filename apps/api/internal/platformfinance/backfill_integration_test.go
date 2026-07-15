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

func getIntegrationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test: TEST_INTEGRATION not set")
	}

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)

	err = pool.Ping(ctx)
	require.NoError(t, err)

	return pool
}

func countTableRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestLegacyBackfillIntegration(t *testing.T) {
	pool := getIntegrationDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initial checks for invariant proof
	initialBookings := countTableRows(t, ctx, pool, "bookings")
	initialSnapshots := countTableRows(t, ctx, pool, "booking_fee_snapshots")
	initialCutovers := countTableRows(t, ctx, pool, "platform_finance_cutovers")

	// Create REPEATABLE READ READ ONLY transaction
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	require.NoError(t, err)

	// Ensure transaction is cleaned up
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		err := tx.Rollback(cleanupCtx)
		if err != nil && err != pgx.ErrTxClosed {
			t.Fatalf("Failed to rollback cleanup context: %v", err)
		}

		// Proof of no mutation
		finalBookings := countTableRows(t, cleanupCtx, pool, "bookings")
		finalSnapshots := countTableRows(t, cleanupCtx, pool, "booking_fee_snapshots")
		finalCutovers := countTableRows(t, cleanupCtx, pool, "platform_finance_cutovers")

		assert.Equal(t, initialBookings, finalBookings, "Bookings count should not change")
		assert.Equal(t, initialSnapshots, finalSnapshots, "Snapshots count should not change")
		assert.Equal(t, initialCutovers, finalCutovers, "Cutovers count should not change")
	}()

	// 2. SHOW transaction_read_only = on
	// 3. SHOW transaction_isolation = repeatable read
	var txMode, txIso string
	err = tx.QueryRow(ctx, "SELECT current_setting('transaction_read_only'), current_setting('transaction_isolation');").Scan(&txMode, &txIso)
	require.NoError(t, err)
	assert.Equal(t, "on", txMode)
	assert.Equal(t, "repeatable read", txIso)

	// Fetch exact cutover
	cutover, err := LoadStoredCutover(ctx, tx)
	require.NoError(t, err)

	// Cutover mismatch testing is handled in main_test.go
	// Perform batch loop 2,2,2,1
	batchSize := 2
	hasMore := true
	var cursor *uuid.UUID

	var totalCandidate, online, offline int
	var batchCounts []int
	for hasMore {
		batch, err := FetchLegacyBackfillCandidates(ctx, tx, cutover, batchSize, cursor)
		require.NoError(t, err)

		if batch.Count > 0 {
			batchCounts = append(batchCounts, batch.Count)
			totalCandidate += batch.Count
			online += batch.OnlineCount
			offline += batch.OfflineCount
		}

		if batch.HasMore {
			cursor = batch.NextCursor
			require.NotNil(t, cursor)
		} else {
			hasMore = false
		}
	}

	// 4. Batch size 2 menghasilkan 2,2,2,1
	assert.Equal(t, []int{2, 2, 2, 1}, batchCounts)

	// 5,6,7. Check total, online, offline
	assert.Equal(t, 7, totalCandidate)
	assert.Equal(t, 3, online)
	assert.Equal(t, 4, offline)

	// 8. Resume setelah UUID spesifik
	resumeCursor := uuid.MustParse("00000000-0000-4000-8000-000000000504")
	resumeBatch1, err := FetchLegacyBackfillCandidates(ctx, tx, cutover, 10, &resumeCursor)
	require.NoError(t, err)
	assert.Equal(t, 3, resumeBatch1.Count)
	assert.Equal(t, 1, resumeBatch1.OnlineCount)
	assert.Equal(t, 2, resumeBatch1.OfflineCount)
}
