package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/platformfinance"
)

func TestApply_BatchFailureRollback(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ids := candidateBookingIDs(t, ctx, fixture.pool, 3)
	require.Len(t, ids, 3)

	// The third candidate belongs to batch two when batch-size=2. A fractional
	// source price makes calculation fail after the first batch has committed.
	_, err := fixture.pool.Exec(ctx, "UPDATE bookings SET total_price = 100000.50 WHERE id = $1", ids[2])
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	exitCode := run(
		[]string{
			"--apply=true",
			"--non-production-confirmed=true",
			"--expected-candidate-count=7",
			"--batch-size=2",
			"--cutover-at=" + storedCutoverString(t, ctx, fixture.pool),
		},
		fixture.getenv(),
		&stdout,
		&stderr,
	)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "Batch processing failed")
	assert.Equal(t, 5, countTableRows(t, ctx, fixture.pool, "booking_fee_snapshots"))
	assert.Equal(t, 0, countIdleInTransaction(t, ctx, fixture.pool))
}

func TestApply_ConcurrentRunnerRejected(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := fixture.pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	var locked bool
	require.NoError(t, conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&locked))
	require.True(t, locked)
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer unlockCancel()
		var unlocked bool
		require.NoError(t, conn.QueryRow(unlockCtx, "SELECT pg_advisory_unlock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&unlocked))
		require.True(t, unlocked)
	}()

	var stdout, stderr bytes.Buffer
	exitCode := run(
		[]string{
			"--apply=true",
			"--non-production-confirmed=true",
			"--expected-candidate-count=7",
			"--batch-size=2",
			"--cutover-at=" + storedCutoverString(t, ctx, fixture.pool),
		},
		fixture.getenv(),
		&stdout,
		&stderr,
	)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "another backfill runner is active or lock failed")
	assert.Equal(t, 3, countTableRows(t, ctx, fixture.pool, "booking_fee_snapshots"))
}

func TestApply_ConflictExactAndMismatch(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := fixture.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	t.Cleanup(func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rollbackCancel()
		err := tx.Rollback(rollbackCtx)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback conflict test transaction: %v", err)
		}
	})

	cutover, err := platformfinance.LoadStoredCutover(ctx, tx)
	require.NoError(t, err)
	batch, err := platformfinance.FetchLegacyBackfillCandidates(ctx, tx, cutover, 1, nil, true)
	require.NoError(t, err)
	require.Len(t, batch.Candidates, 1)

	snapshot, err := platformfinance.CalculateLegacySnapshot(batch.Candidates[0], cutover)
	require.NoError(t, err)
	params := snapshotParams(snapshot)
	repository := platformfinance.NewLegacyBackfillSnapshotRepository()

	_, inserted, err := repository.InsertIdempotentBackfillSnapshot(ctx, tx, params)
	require.NoError(t, err)
	require.True(t, inserted)

	_, inserted, err = repository.InsertIdempotentBackfillSnapshot(ctx, tx, params)
	require.NoError(t, err)
	require.False(t, inserted)

	mismatch := params
	if mismatch.BookingChannel == platformfinance.BookingChannelMarketplaceOnline {
		mismatch.BookingChannel = platformfinance.BookingChannelOwnerWalkIn
	} else {
		mismatch.BookingChannel = platformfinance.BookingChannelMarketplaceOnline
	}
	_, _, err = repository.InsertIdempotentBackfillSnapshot(ctx, tx, mismatch)
	require.ErrorIs(t, err, platformfinance.ErrBackfillSnapshotConflict)
}

func TestApply_UnlockCleanup(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	exitCode := run(
		[]string{
			"--apply=true",
			"--non-production-confirmed=true",
			"--expected-candidate-count=999",
			"--batch-size=2",
			"--cutover-at=" + storedCutoverString(t, ctx, fixture.pool),
		},
		fixture.getenv(),
		&stdout,
		&stderr,
	)
	require.Equal(t, 1, exitCode)

	conn, err := fixture.pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	var locked bool
	require.NoError(t, conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&locked))
	require.True(t, locked)
	var unlocked bool
	require.NoError(t, conn.QueryRow(ctx, "SELECT pg_advisory_unlock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&unlocked))
	require.True(t, unlocked)
	assert.Equal(t, 0, countIdleInTransaction(t, ctx, fixture.pool))
}

func TestApply_UnlockFailure(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	operations := defaultRunnerOperations()
	operations.unlock = func(context.Context, *pgxpool.Conn) (bool, error) {
		return false, errors.New("injected unlock failure")
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithOperations(
		[]string{
			"--apply=true",
			"--non-production-confirmed=true",
			"--expected-candidate-count=999",
			"--batch-size=2",
			"--cutover-at=" + storedCutoverString(t, ctx, fixture.pool),
		},
		fixture.getenv(),
		&stdout,
		&stderr,
		operations,
	)
	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "failed to unlock advisory lock")

	conn, err := fixture.pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	var locked bool
	require.NoError(t, conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&locked))
	require.True(t, locked)
	var unlocked bool
	require.NoError(t, conn.QueryRow(ctx, "SELECT pg_advisory_unlock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&unlocked))
	require.True(t, unlocked)
}

func TestApply_CommitOutcomeUnknown(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	operations := defaultRunnerOperations()
	operations.commit = func(context.Context, pgx.Tx) error {
		return errors.New("injected ambiguous commit outcome")
	}

	var stdout, stderr bytes.Buffer
	exitCode := runWithOperations(
		[]string{
			"--apply=true",
			"--non-production-confirmed=true",
			"--expected-candidate-count=7",
			"--batch-size=2",
			"--cutover-at=" + storedCutoverString(t, ctx, fixture.pool),
		},
		fixture.getenv(),
		&stdout,
		&stderr,
		operations,
	)
	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "COMMIT_OUTCOME_UNKNOWN")
	assert.Equal(t, 3, countTableRows(t, ctx, fixture.pool, "booking_fee_snapshots"))
	assert.Equal(t, 0, countIdleInTransaction(t, ctx, fixture.pool))
}

func TestApply_NoCollateralMutation(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tables := []string{
		"bookings",
		"booking_fee_snapshots",
		"platform_finance_cutovers",
		"owner_finance_transactions",
		"platform_audit_logs",
		"owner_audit_logs",
		"booking_refund_requests",
		"owner_promos",
		"platform_commercial_terms",
	}
	beforeCounts := make(map[string]int, len(tables))
	for _, table := range tables {
		beforeCounts[table] = countTableRows(t, ctx, fixture.pool, table)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run(
		[]string{
			"--apply=true",
			"--non-production-confirmed=true",
			"--expected-candidate-count=7",
			"--batch-size=2",
			"--cutover-at=" + storedCutoverString(t, ctx, fixture.pool),
		},
		fixture.getenv(),
		&stdout,
		&stderr,
	)
	require.Equal(t, 0, exitCode, "apply should succeed: %s", stderr.String())

	for _, table := range tables {
		after := countTableRows(t, ctx, fixture.pool, table)
		if table == "booking_fee_snapshots" {
			assert.Equal(t, beforeCounts[table]+7, after)
		} else {
			assert.Equal(t, beforeCounts[table], after, "table %s should not mutate", table)
		}
	}
	assert.Equal(t, 0, countIdleInTransaction(t, ctx, fixture.pool))
}

func snapshotParams(snapshot platformfinance.BookingFeeSnapshot) platformfinance.CreateBookingFeeSnapshotParams {
	return platformfinance.CreateBookingFeeSnapshotParams{
		BookingID:                   snapshot.BookingID,
		OwnerProfileID:              snapshot.OwnerProfileID,
		VenueID:                     snapshot.VenueID,
		CommercialTermID:            snapshot.CommercialTermID,
		TermsSource:                 snapshot.TermsSource,
		BookingChannel:              snapshot.BookingChannel,
		FinanceMode:                 snapshot.FinanceMode,
		OriginalPriceRupiah:         snapshot.OriginalPriceRupiah,
		OwnerPriceAdjustmentRupiah:  snapshot.OwnerPriceAdjustmentRupiah,
		PriceAdjustmentReason:       snapshot.PriceAdjustmentReason,
		FinalBookingPriceRupiah:     snapshot.FinalBookingPriceRupiah,
		CustomerServiceFeeRupiah:    snapshot.CustomerServiceFeeRupiah,
		CustomerChargeAmountRupiah:  snapshot.CustomerChargeAmountRupiah,
		CommissionBasisAmountRupiah: snapshot.CommissionBasisAmountRupiah,
		CommissionBps:               snapshot.CommissionBps,
		CommissionAmountRupiah:      snapshot.CommissionAmountRupiah,
		OwnerNetAmountRupiah:        snapshot.OwnerNetAmountRupiah,
		CalculationVersion:          snapshot.CalculationVersion,
	}
}
