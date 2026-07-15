package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/httputil"
	"lapangango-api/internal/platformfinance"
)

func main() {
	os.Exit(run(
		os.Args[1:],
		os.Getenv,
		os.Stdout,
		os.Stderr,
	))
}

func run(
	args []string,
	getenv func(string) string,
	stdout io.Writer,
	stderr io.Writer,
) (exitCode int) {
	fs := flag.NewFlagSet("backfill-platform-finance", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var dryRun bool
	var batchSize int
	var afterBookingIDStr string
	var cutoverAtStr string
	var apply bool

	fs.BoolVar(&dryRun, "dry-run", false, "Must be true. Run backfill in dry-run mode.")
	fs.IntVar(&batchSize, "batch-size", 100, "Batch size for scanning (1-1000).")
	fs.StringVar(&afterBookingIDStr, "after-booking-id", "", "Optional cursor to resume from.")
	fs.StringVar(&cutoverAtStr, "cutover-at", "", "Expected exact cutover time (RFC3339).")
	fs.BoolVar(&apply, "apply", false, "Apply mode (not implemented).")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// 1. Validate flags
	if apply {
		fmt.Fprintln(stderr, "Error: --apply is not supported.")
		return 1
	}

	if !dryRun {
		fmt.Fprintln(stderr, "Error: --dry-run=true is required.")
		return 1
	}

	if batchSize < 1 || batchSize > 1000 {
		fmt.Fprintln(stderr, "Error: --batch-size must be between 1 and 1000.")
		return 1
	}

	var afterBookingID *uuid.UUID
	if afterBookingIDStr != "" {
		if !httputil.IsUUID(afterBookingIDStr) {
			fmt.Fprintln(stderr, "Error: --after-booking-id must be a valid UUID.")
			return 1
		}
		id := uuid.MustParse(afterBookingIDStr)
		afterBookingID = &id
	}

	if cutoverAtStr == "" {
		fmt.Fprintln(stderr, "Error: --cutover-at is required.")
		return 1
	}
	expectedCutoverAt, err := time.Parse(time.RFC3339Nano, cutoverAtStr)
	if err != nil {
		fmt.Fprintln(stderr, "Error: --cutover-at must be a valid RFC3339/RFC3339Nano timestamp.")
		return 1
	}

	// 2. Database Connection
	dbURL := getenv("BACKFILL_DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(stderr, "Error: BACKFILL_DATABASE_URL environment variable is required.")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		fmt.Fprintln(stderr, "Error: Failed to parse database configuration.")
		return 1
	}
	config.ConnConfig.RuntimeParams["application_name"] = "lapanggo-backfill-dry-run"

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Fprintln(stderr, "Error: Failed to establish database connection.")
		return 1
	}
	defer pool.Close()

	// 3. Open single transaction for the entire run
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		fmt.Fprintln(stderr, "Error: Failed to begin read-only transaction.")
		return 1
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		rollbackErr := tx.Rollback(cleanupCtx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			fmt.Fprintln(stderr, "Error: Read-only transaction cleanup failed.")
			exitCode = 1
		}
	}()

	// 4. Verify transaction mode
	var txMode, txIso string
	err = tx.QueryRow(ctx, "SELECT current_setting('transaction_read_only'), current_setting('transaction_isolation');").Scan(&txMode, &txIso)
	if err != nil {
		fmt.Fprintln(stderr, "Error: Failed to verify transaction isolation mode.")
		return 1
	}
	if txMode != "on" || txIso != "repeatable read" {
		fmt.Fprintln(stderr, "Error: Transaction is not strictly REPEATABLE READ READ ONLY.")
		return 1
	}

	// 5. Load and validate exact cutover
	storedCutoverAt, err := platformfinance.LoadStoredCutover(ctx, tx)
	if err != nil {
		fmt.Fprintln(stderr, "Error: Failed to load stored cutover from database.")
		return 1
	}
	if !storedCutoverAt.Equal(expectedCutoverAt.UTC()) {
		fmt.Fprintln(stderr, "Error: Provided cutover time does not match stored cutover exactly.")
		return 1
	}

	// 6. Iterate batches
	var batchesScanned int
	var candidateTotal int
	var marketplaceOnline int
	var ownerWalkIn int

	cursor := afterBookingID
	hasMore := true
	maxIterations := 100000 // Guard against infinite loop

	for hasMore {
		if batchesScanned >= maxIterations {
			fmt.Fprintln(stderr, "Error: Exceeded maximum batch iterations. Aborting to prevent infinite loop.")
			return 1
		}

		batch, err := platformfinance.FetchLegacyBackfillCandidates(ctx, tx, storedCutoverAt, batchSize, cursor)
		if err != nil {
			fmt.Fprintln(stderr, "Error: Failed to fetch candidates batch.")
			return 1
		}

		candidateTotal += batch.Count
		marketplaceOnline += batch.OnlineCount
		ownerWalkIn += batch.OfflineCount
		batchesScanned++

		if batch.HasMore {
			if batch.NextCursor == nil {
				fmt.Fprintln(stderr, "Error: Integrity failure, next cursor is missing but has more rows.")
				return 1
			}
			// ensure progress is made
			if cursor != nil && bytes.Compare(batch.NextCursor[:], cursor[:]) <= 0 {
				fmt.Fprintln(stderr, "Error: Integrity failure, cursor did not advance.")
				return 1
			}
			cursor = batch.NextCursor
			hasMore = true
		} else {
			hasMore = false
		}
	}

	// 7. Sanitized Output
	fmt.Fprintln(stdout, "mode=DRY_RUN")
	fmt.Fprintln(stdout, "cutover_match=true")
	fmt.Fprintf(stdout, "batch_size=%d\n", batchSize)
	fmt.Fprintf(stdout, "batches_scanned=%d\n", batchesScanned)
	fmt.Fprintf(stdout, "candidate_total=%d\n", candidateTotal)
	fmt.Fprintf(stdout, "marketplace_online=%d\n", marketplaceOnline)
	fmt.Fprintf(stdout, "owner_walk_in=%d\n", ownerWalkIn)
	fmt.Fprintln(stdout, "writes_performed=0")

	return 0
}
