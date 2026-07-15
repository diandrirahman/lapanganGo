package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/platformfinance"
)

func rollbackTx(tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	err := tx.Rollback(ctx)
	if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return err
	}
	return nil
}

func main() {
	os.Exit(run(
		os.Args[1:],
		os.Getenv,
		os.Stdout,
		os.Stderr,
	))
}

type runnerOperations struct {
	commit func(context.Context, pgx.Tx) error
	unlock func(context.Context, *pgxpool.Conn) (bool, error)
}

func defaultRunnerOperations() runnerOperations {
	return runnerOperations{
		commit: func(ctx context.Context, tx pgx.Tx) error {
			return tx.Commit(ctx)
		},
		unlock: func(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
			var unlocked bool
			err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&unlocked)
			return unlocked, err
		},
	}
}

func run(args []string, getenv func(string) string, stdout io.Writer, stderr io.Writer) int {
	return runWithOperations(args, getenv, stdout, stderr, defaultRunnerOperations())
}

func runWithOperations(args []string, getenv func(string) string, stdout io.Writer, stderr io.Writer, operations runnerOperations) (exitCode int) {
	flagSet := flag.NewFlagSet("backfill-platform-finance", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	apply := flagSet.Bool("apply", false, "Write backfill snapshots to database")
	dryRun := flagSet.Bool("dry-run", false, "Calculate candidates without writing")
	nonProdConfirmed := flagSet.Bool("non-production-confirmed", false, "Confirm this is not production for apply")
	expectedCount := flagSet.Int("expected-candidate-count", -1, "Expected candidate count for apply guard")
	cutoverStr := flagSet.String("cutover-at", "", "Optional: Cutover timestamp override (RFC3339)")
	afterBookingStr := flagSet.String("after-booking-id", "", "Optional: Resume UUID cursor")
	batchSize := flagSet.Int("batch-size", 100, "Candidates per batch")

	if err := flagSet.Parse(args); err != nil {
		return 1
	}

	if *apply == *dryRun {
		fmt.Fprintf(stderr, "Error: Exactly one of --apply or --dry-run must be true\n")
		return 1
	}

	if *apply {
		if !*nonProdConfirmed {
			fmt.Fprintf(stderr, "Error: --apply requires --non-production-confirmed=true\n")
			return 1
		}
		if *expectedCount < 0 {
			fmt.Fprintf(stderr, "Error: --apply requires --expected-candidate-count\n")
			return 1
		}
		env := strings.TrimSpace(getenv("BACKFILL_TARGET_ENVIRONMENT"))
		if env != "development" && env != "staging" {
			fmt.Fprintf(stderr, "Error: BACKFILL_TARGET_ENVIRONMENT must be 'development' or 'staging'\n")
			return 1
		}
		expectedDB := strings.TrimSpace(getenv("BACKFILL_EXPECTED_DATABASE_NAME"))
		if expectedDB == "" {
			fmt.Fprintf(stderr, "Error: BACKFILL_EXPECTED_DATABASE_NAME must be set\n")
			return 1
		}
	}

	var afterID *uuid.UUID
	if *afterBookingStr != "" {
		id, err := uuid.Parse(*afterBookingStr)
		if err != nil {
			fmt.Fprintf(stderr, "Error: Invalid after-booking-id UUID\n")
			return 1
		}
		afterID = &id
	}

	var parsedCutover *time.Time
	if *cutoverStr != "" {
		pt, err := time.Parse(time.RFC3339, *cutoverStr)
		if err != nil {
			fmt.Fprintf(stderr, "Error: Invalid cutover-at time format\n")
			return 1
		}
		utc := pt.UTC()
		parsedCutover = &utc
	}

	dbURL := getenv("BACKFILL_DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintf(stderr, "Error: BACKFILL_DATABASE_URL environment variable is required\n")
		return 1
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		fmt.Fprintln(stderr, "Error: Failed to configure database connection")
		return 1
	}

	applicationName := "lapanggo-backfill-dry-run"
	if *apply {
		applicationName = "lapanggo-backfill-apply"
	}
	config.ConnConfig.RuntimeParams["application_name"] = applicationName

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	pool, err := pgxpool.NewWithConfig(ctx, config)
	cancel()
	if err != nil {
		fmt.Fprintf(stderr, "Error: Failed to connect to database\n")
		return 1
	}
	defer pool.Close()

	if *apply {
		expectedDB := strings.TrimSpace(getenv("BACKFILL_EXPECTED_DATABASE_NAME"))
		var currentDB string
		ctxCheck, cancelCheck := context.WithTimeout(context.Background(), 5*time.Second)
		err := pool.QueryRow(ctxCheck, "SELECT current_database()").Scan(&currentDB)
		cancelCheck()
		if err != nil {
			fmt.Fprintf(stderr, "Error: unable to verify target database\n")
			return 1
		}
		if currentDB != expectedDB {
			fmt.Fprintf(stderr, "Error: unable to verify target database\n")
			return 1
		}
	}

	var conn *pgxpool.Conn
	connectionDiscarded := false
	if *apply {
		var err error
		ctxAcquire, cancelAcquire := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err = pool.Acquire(ctxAcquire)
		cancelAcquire()
		if err != nil {
			fmt.Fprintf(stderr, "Error: failed to acquire connection for advisory lock\n")
			return 1
		}

		ctxLock, cancelLock := context.WithTimeout(context.Background(), 5*time.Second)
		var locked bool
		err = conn.QueryRow(ctxLock, "SELECT pg_try_advisory_lock(hashtextextended('lapanggo:platformfinance:legacy-backfill-v1', 0))").Scan(&locked)
		cancelLock()

		if err != nil || !locked {
			fmt.Fprintf(stderr, "Error: another backfill runner is active or lock failed\n")
			closeCtx, cClose := context.WithTimeout(context.Background(), 5*time.Second)
			conn.Conn().Close(closeCtx)
			cClose()
			conn.Release()
			return 1
		}

		defer func() {
			if conn != nil {
				if !connectionDiscarded {
					unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
					unlocked, errUnlock := operations.unlock(unlockCtx, conn)
					unlockCancel()
					if errUnlock != nil || !unlocked {
						closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
						_ = conn.Conn().Close(closeCtx)
						closeCancel()
						connectionDiscarded = true
						fmt.Fprintf(stderr, "Error: failed to unlock advisory lock\n")
						exitCode = 1
					}
				}
				conn.Release()
			}
		}()
	}

	// Read cutover
	ctxCutover, c := context.WithTimeout(context.Background(), 5*time.Second)
	storedCutover, err := platformfinance.LoadStoredCutover(ctxCutover, pool)
	c()
	if err != nil {
		fmt.Fprintf(stderr, "Error: unable to load cutover configuration\n")
		return 1
	}

	var cutover time.Time
	if parsedCutover != nil {
		if !parsedCutover.Equal(storedCutover) {
			fmt.Fprintf(stderr, "Error: cutover configuration mismatch\n")
			return 1
		}
		cutover = *parsedCutover
	} else {
		cutover = storedCutover
	}

	// Expected Count Guard
	if *apply {
		ctxCount, cCount := context.WithTimeout(context.Background(), 10*time.Second)
		var totalCandidates int
		countQuery := `
		SELECT count(*)
		FROM bookings b
		WHERE b.created_at < $1
		  AND ($2::uuid IS NULL OR b.id > $2::uuid)
		  AND NOT EXISTS (
			  SELECT 1
			  FROM booking_fee_snapshots bfs
			  WHERE bfs.booking_id = b.id
		  )
		`
		err := pool.QueryRow(ctxCount, countQuery, cutover, afterID).Scan(&totalCandidates)
		cCount()
		if err != nil {
			fmt.Fprintf(stderr, "Error: unable to count backfill candidates\n")
			return 1
		}
		if totalCandidates != *expectedCount {
			fmt.Fprintf(stderr, "Error: candidate count mismatch: expected %d, got %d\n", *expectedCount, totalCandidates)
			fmt.Fprintf(stdout, "mode=APPLY\nexpected_candidate_count=%d\nprocessed=0\ninserted=0\nidempotent_noop=0\n", *expectedCount)
			return 1
		}
	}

	// Dry run mode uses one big read-only transaction, but wait, the plan says for dry-run:
	// "1 long-running REPEATABLE READ READ ONLY transaction (unchanged from 2B3-01)."
	// Wait, for apply, we use transaction PER BATCH on the `conn`.
	// Let's implement dry-run first.
	if *dryRun {
		ctxTx, cancelTx := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancelTx()

		tx, err := pool.BeginTx(ctxTx, pgx.TxOptions{
			IsoLevel:   pgx.RepeatableRead,
			AccessMode: pgx.ReadOnly,
		})
		if err != nil {
			fmt.Fprintf(stderr, "Error: Transaction begin failed\n")
			return 1
		}
		defer func() {
			if err := rollbackTx(tx); err != nil {
				fmt.Fprintf(stderr, "Error: read-only transaction cleanup failed\n")
				exitCode = 1
			}
		}()

		var processed, onlineTotal, offlineTotal, batchesScanned int
		var currentCursor *uuid.UUID = afterID

		for {
			batchesScanned++
			batch, err := platformfinance.FetchLegacyBackfillCandidates(ctxTx, tx, cutover, *batchSize, currentCursor, false)
			if err != nil {
				fmt.Fprintf(stderr, "Error: Fetch candidates failed\n")
				return 1
			}

			processed += batch.Count
			onlineTotal += batch.OnlineCount
			offlineTotal += batch.OfflineCount

			if !batch.HasMore {
				break
			}
			currentCursor = batch.NextCursor
		}

		fmt.Fprintf(stdout, "mode=DRY_RUN\n")
		fmt.Fprintf(stdout, "cutover_match=true\n")
		fmt.Fprintf(stdout, "batch_size=%d\n", *batchSize)
		fmt.Fprintf(stdout, "batches_scanned=%d\n", batchesScanned)
		fmt.Fprintf(stdout, "candidate_total=%d\n", processed)
		fmt.Fprintf(stdout, "marketplace_online=%d\n", onlineTotal)
		fmt.Fprintf(stdout, "owner_walk_in=%d\n", offlineTotal)
		fmt.Fprintf(stdout, "writes_performed=0\n")
		return 0
	}

	// Apply mode
	repo := platformfinance.NewLegacyBackfillSnapshotRepository()
	var processed, inserted, noop, batchesCommitted int
	var lastCommittedCursor *uuid.UUID = afterID
	var currentCursor *uuid.UUID = afterID

	for {
		ctxBatch, cancelBatch := context.WithTimeout(context.Background(), 2*time.Minute)
		tx, err := conn.BeginTx(ctxBatch, pgx.TxOptions{
			IsoLevel:   pgx.RepeatableRead,
			AccessMode: pgx.ReadWrite,
		})
		if err != nil {
			cancelBatch()
			fmt.Fprintf(stderr, "Error: Transaction begin failed for batch\n")
			if lastCommittedCursor != nil {
				fmt.Fprintf(stderr, "last_committed_cursor=%s\n", lastCommittedCursor.String())
			}
			return 1
		}

		// Revalidate cutover in tx
		txCutover, err := platformfinance.LoadStoredCutover(ctxBatch, tx)
		if err != nil || !txCutover.Equal(cutover) {
			if rbErr := rollbackTx(tx); rbErr != nil {
				fmt.Fprintf(stderr, "Error: transaction cleanup failed\n")
				exitCode = 1
			}
			cancelBatch()
			fmt.Fprintf(stderr, "Error: Cutover mismatch in transaction\n")
			if exitCode != 0 {
				return exitCode
			}
			return 1
		}

		batch, err := platformfinance.FetchLegacyBackfillCandidates(ctxBatch, tx, cutover, *batchSize, currentCursor, true)
		if err != nil {
			if rbErr := rollbackTx(tx); rbErr != nil {
				fmt.Fprintf(stderr, "Error: transaction cleanup failed\n")
				exitCode = 1
			}
			cancelBatch()
			fmt.Fprintf(stderr, "Error: Fetch candidates failed\n")
			if lastCommittedCursor != nil {
				fmt.Fprintf(stderr, "last_committed_cursor=%s\n", lastCommittedCursor.String())
			}
			if exitCode != 0 {
				return exitCode
			}
			return 1
		}

		if batch.Count == 0 {
			if rbErr := rollbackTx(tx); rbErr != nil {
				fmt.Fprintf(stderr, "Error: transaction cleanup failed\n")
				exitCode = 1
			}
			cancelBatch()
			if exitCode != 0 {
				return exitCode
			}
			break
		}

		batchInserted := 0
		batchNoop := 0

		var processErr error
		for _, cand := range batch.Candidates {
			snap, err := platformfinance.CalculateLegacySnapshot(cand, cutover)
			if err != nil {
				processErr = err
				break
			}

			params := platformfinance.CreateBookingFeeSnapshotParams{
				BookingID:                   snap.BookingID,
				OwnerProfileID:              snap.OwnerProfileID,
				VenueID:                     snap.VenueID,
				CommercialTermID:            snap.CommercialTermID,
				TermsSource:                 snap.TermsSource,
				BookingChannel:              snap.BookingChannel,
				FinanceMode:                 snap.FinanceMode,
				OriginalPriceRupiah:         snap.OriginalPriceRupiah,
				OwnerPriceAdjustmentRupiah:  snap.OwnerPriceAdjustmentRupiah,
				PriceAdjustmentReason:       snap.PriceAdjustmentReason,
				FinalBookingPriceRupiah:     snap.FinalBookingPriceRupiah,
				CustomerServiceFeeRupiah:    snap.CustomerServiceFeeRupiah,
				CustomerChargeAmountRupiah:  snap.CustomerChargeAmountRupiah,
				CommissionBasisAmountRupiah: snap.CommissionBasisAmountRupiah,
				CommissionBps:               snap.CommissionBps,
				CommissionAmountRupiah:      snap.CommissionAmountRupiah,
				OwnerNetAmountRupiah:        snap.OwnerNetAmountRupiah,
				CalculationVersion:          snap.CalculationVersion,
			}

			res, isNew, err := repo.InsertIdempotentBackfillSnapshot(ctxBatch, tx, params)
			if err != nil {
				processErr = err
				break
			}
			_ = res

			if isNew {
				batchInserted++
			} else {
				batchNoop++
			}
		}

		if processErr != nil {
			if rbErr := rollbackTx(tx); rbErr != nil {
				fmt.Fprintf(stderr, "Error: transaction cleanup failed\n")
				exitCode = 1
			}
			cancelBatch()
			fmt.Fprintf(stderr, "Error: Batch processing failed\n")
			if lastCommittedCursor != nil {
				fmt.Fprintf(stderr, "last_committed_cursor=%s\n", lastCommittedCursor.String())
			}
			if exitCode != 0 {
				return exitCode
			}
			return 1
		}

		commitCtx, cCommit := context.WithTimeout(context.Background(), 5*time.Second)
		commitErr := operations.commit(commitCtx, tx)
		cCommit()
		cancelBatch()
		if commitErr != nil {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = conn.Conn().Close(closeCtx)
			closeCancel()
			connectionDiscarded = true
			fmt.Fprintf(stderr, "Error: COMMIT_OUTCOME_UNKNOWN\n")
			if lastCommittedCursor != nil {
				fmt.Fprintf(stderr, "last_committed_cursor=%s\n", lastCommittedCursor.String())
			}
			return 1
		}

		// Only on success:
		batchesCommitted++
		processed += batch.Count
		inserted += batchInserted
		noop += batchNoop
		lastCommittedCursor = batch.NextCursor
		currentCursor = batch.NextCursor

		if !batch.HasMore {
			break
		}
	}

	if processed != *expectedCount {
		fmt.Fprintf(stderr, "Error: processed candidate count does not match reviewed count\n")
		return 1
	}

	fmt.Fprintf(stdout, "mode=APPLY\n")
	fmt.Fprintf(stdout, "cutover_match=true\n")
	fmt.Fprintf(stdout, "expected_candidate_count=%d\n", *expectedCount)
	fmt.Fprintf(stdout, "batches_committed=%d\n", batchesCommitted)
	fmt.Fprintf(stdout, "processed=%d\n", processed)
	fmt.Fprintf(stdout, "inserted=%d\n", inserted)
	fmt.Fprintf(stdout, "idempotent_noop=%d\n", noop)
	if lastCommittedCursor != nil {
		fmt.Fprintf(stdout, "last_committed_cursor=%s\n", lastCommittedCursor.String())
	}
	fmt.Fprintf(stdout, "commit_outcome=CONFIRMED\n")
	return 0
}
