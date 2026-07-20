package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/config"
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
	fs := flag.NewFlagSet("detect-platform-finance-p0", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var batchSize int
	var maxIterations int

	fs.IntVar(&batchSize, "batch-size", 100, "Number of bookings to scan per batch")
	fs.IntVar(&maxIterations, "max-iterations", 1000000, "Maximum number of iterations")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "failed to parse arguments: %v\n", err)
		return 1
	}

	if batchSize < 1 || batchSize > 1000 {
		fmt.Fprintf(stderr, "invalid batch size. Must be between 1 and 1000\n")
		return 1
	}

	if maxIterations <= 0 {
		fmt.Fprintf(stderr, "invalid max iterations. Must be > 0\n")
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "invalid application configuration\n")
		return 1
	}

	dbUrl := getenv("POST_CUTOVER_DETECTOR_DATABASE_URL")
	if dbUrl == "" {
		dbUrl = cfg.DatabaseURL
	}
	if dbUrl == "" {
		fmt.Fprintf(stderr, "database URL not configured\n")
		return 1
	}

	poolCfg, err := pgxpool.ParseConfig(dbUrl)
	if err != nil {
		fmt.Fprintf(stderr, "failed to parse database url\n")
		return 1
	}
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "lapanggo-post-cutover-p0-detector"

	poolCtx, poolCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer poolCancel()

	pool, err := pgxpool.NewWithConfig(poolCtx, poolCfg)
	if err != nil {
		fmt.Fprintf(stderr, "failed to connect to database\n")
		return 1
	}
	defer pool.Close()

	// Wrap everything in a single transaction
	txCtx, txCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer txCancel()

	tx, err := pool.BeginTx(txCtx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		fmt.Fprintf(stderr, "failed to begin transaction\n")
		return 1
	}
	defer func() {
		if err := rollbackTx(tx); err != nil && exitCode != 1 {
			exitCode = 1
		}
	}()

	// Preflight
	preflight, err := platformfinance.LoadPostCutoverDetectorPreflight(txCtx, tx)
	if err != nil {
		fmt.Fprintf(stderr, "verdict=INTEGRITY_FAILURE\n")
		return 1
	}

	// Population
	population, err := platformfinance.LoadPostCutoverPopulation(txCtx, tx, preflight.CutoverAt)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load population\n")
		return 1
	}

	termResolver := platformfinance.NewCommercialTermResolver(tx)

	var batchesScanned int
	var classifiedCount int64
	var repairableOnline int64
	var repairableWalkIn int64
	var manualDecision int64

	reasonCounts := make(map[platformfinance.PostCutoverP0Reason]int64)

	var currentCursor *uuid.UUID
	hasMore := true

	for hasMore {
		if batchesScanned >= maxIterations {
			fmt.Fprintf(stderr, "verdict=INTEGRITY_FAILURE\n")
			return 1
		}

		batch, err := platformfinance.FetchPostCutoverP0Candidates(txCtx, tx, platformfinance.PostCutoverDetectorParams{
			CutoverAt:      preflight.CutoverAt,
			AfterBookingID: currentCursor,
			BatchSize:      batchSize,
		})
		if err != nil {
			fmt.Fprintf(stderr, "failed to fetch candidates\n")
			return 1
		}

		for _, candidate := range batch.Candidates {
			var term *platformfinance.CommercialTerm
			var resolverErr error
			if candidate.OwnerProfileID != nil {
				term, resolverErr = termResolver.ResolveEffectiveTerm(txCtx, candidate.OwnerProfileID.String(), candidate.CreatedAt.UTC())
			}

			// A cancelled detector context is an operational failure.
			if resolverErr != nil && txCtx.Err() != nil {
				fmt.Fprintf(stderr, "context deadline exceeded\n")
				return 1
			}

			result := platformfinance.ClassifyPostCutoverP0Candidate(candidate, term, resolverErr)
			if result.OperationalError != nil {
				fmt.Fprintf(stderr, "failed to resolve commercial term\n")
				return 1
			}

			classifiedCount++
			switch result.Classification {
			case platformfinance.ClassificationRepairablePolicyOnline:
				repairableOnline++
			case platformfinance.ClassificationRepairablePolicyWalkIn:
				repairableWalkIn++
			case platformfinance.ClassificationManualDecisionRequired:
				manualDecision++
				reasonCounts[result.Reason]++
			}
		}

		batchesScanned++
		hasMore = batch.HasMore
		currentCursor = batch.NextCursor

		if hasMore && (len(batch.Candidates) == 0 || currentCursor == nil) {
			fmt.Fprintf(stderr, "fatal: pagination invariant violated\n")
			return 1
		}
	}

	if classifiedCount != population.PostCutoverMissingSnapshot {
		fmt.Fprintf(stderr, "fatal: classified count %d != missing snapshot count %d\n", classifiedCount, population.PostCutoverMissingSnapshot)
		return 1
	}
	var reasonTotal int64
	for _, count := range reasonCounts {
		reasonTotal += count
	}
	if reasonTotal != manualDecision {
		fmt.Fprintf(stderr, "verdict=INTEGRITY_FAILURE\n")
		return 1
	}

	verdict := "CLEAR"
	if manualDecision > 0 {
		verdict = "BLOCKED_MANUAL_DECISION"
	} else if repairableOnline > 0 || repairableWalkIn > 0 {
		verdict = "QUARANTINED_P0"
	}

	fmt.Fprintf(stdout, "mode=POST_CUTOVER_P0_DETECTOR\n")
	fmt.Fprintf(stdout, "cutover_verified=true\n")
	fmt.Fprintf(stdout, "trigger_integrity=true\n")
	fmt.Fprintf(stdout, "transaction_read_only=true\n")
	fmt.Fprintf(stdout, "transaction_isolation=repeatable_read\n")
	fmt.Fprintf(stdout, "batch_size=%d\n", batchSize)
	fmt.Fprintf(stdout, "batches_scanned=%d\n", batchesScanned)
	fmt.Fprintf(stdout, "post_cutover_booking_total=%d\n", population.PostCutoverBookingTotal)
	fmt.Fprintf(stdout, "post_cutover_with_snapshot=%d\n", population.PostCutoverWithSnapshot)
	fmt.Fprintf(stdout, "post_cutover_missing_snapshot=%d\n", population.PostCutoverMissingSnapshot)
	fmt.Fprintf(stdout, "repairable_policy_online=%d\n", repairableOnline)
	fmt.Fprintf(stdout, "repairable_policy_walk_in=%d\n", repairableWalkIn)
	fmt.Fprintf(stdout, "manual_decision_required=%d\n", manualDecision)

	fmt.Fprintf(stdout, "reason_missing_effective_term=%d\n", reasonCounts[platformfinance.ReasonMissingEffectiveTerm])
	fmt.Fprintf(stdout, "reason_duplicate_effective_term=%d\n", reasonCounts[platformfinance.ReasonDuplicateEffectiveTerm])
	fmt.Fprintf(stdout, "reason_invalid_resolved_term=%d\n", reasonCounts[platformfinance.ReasonInvalidResolvedTerm])
	fmt.Fprintf(stdout, "reason_unsupported_finance_mode=%d\n", reasonCounts[platformfinance.ReasonUnsupportedFinanceMode])
	fmt.Fprintf(stdout, "reason_unsupported_collection_method=%d\n", reasonCounts[platformfinance.ReasonUnsupportedCollectionMethod])
	fmt.Fprintf(stdout, "reason_invalid_term_phase=%d\n", reasonCounts[platformfinance.ReasonInvalidTermPhase])
	fmt.Fprintf(stdout, "reason_invalid_commission_bps=%d\n", reasonCounts[platformfinance.ReasonInvalidCommissionBps])

	missingRef := reasonCounts[platformfinance.ReasonMissingCourtReference] + reasonCounts[platformfinance.ReasonMissingVenueReference] + reasonCounts[platformfinance.ReasonMissingOwnerReference]
	fmt.Fprintf(stdout, "reason_missing_reference=%d\n", missingRef)
	fmt.Fprintf(stdout, "reason_missing_court_reference=%d\n", reasonCounts[platformfinance.ReasonMissingCourtReference])
	fmt.Fprintf(stdout, "reason_missing_venue_reference=%d\n", reasonCounts[platformfinance.ReasonMissingVenueReference])
	fmt.Fprintf(stdout, "reason_missing_owner_reference=%d\n", reasonCounts[platformfinance.ReasonMissingOwnerReference])
	fmt.Fprintf(stdout, "reason_unknown_booking_channel=%d\n", reasonCounts[platformfinance.ReasonUnknownBookingChannel])

	invalidMoney := reasonCounts[platformfinance.ReasonMissingMoneyValue] + reasonCounts[platformfinance.ReasonFractionalMoneyValue] + reasonCounts[platformfinance.ReasonNegativeMoneyValue] + reasonCounts[platformfinance.ReasonMoneyOverflow]
	fmt.Fprintf(stdout, "reason_invalid_money=%d\n", invalidMoney)
	fmt.Fprintf(stdout, "reason_missing_money=%d\n", reasonCounts[platformfinance.ReasonMissingMoneyValue])
	fmt.Fprintf(stdout, "reason_fractional_money=%d\n", reasonCounts[platformfinance.ReasonFractionalMoneyValue])
	fmt.Fprintf(stdout, "reason_negative_money=%d\n", reasonCounts[platformfinance.ReasonNegativeMoneyValue])
	fmt.Fprintf(stdout, "reason_money_overflow=%d\n", reasonCounts[platformfinance.ReasonMoneyOverflow])

	priceMismatch := reasonCounts[platformfinance.ReasonDiscountArithmeticMismatch] + reasonCounts[platformfinance.ReasonBookingTotalFinalMismatch] + reasonCounts[platformfinance.ReasonOfflineSystemPriceMismatch] + reasonCounts[platformfinance.ReasonOfflineFinalPriceMismatch]
	fmt.Fprintf(stdout, "reason_price_mismatch=%d\n", priceMismatch)
	fmt.Fprintf(stdout, "reason_discount_arithmetic_mismatch=%d\n", reasonCounts[platformfinance.ReasonDiscountArithmeticMismatch])
	fmt.Fprintf(stdout, "reason_booking_total_final_mismatch=%d\n", reasonCounts[platformfinance.ReasonBookingTotalFinalMismatch])
	fmt.Fprintf(stdout, "reason_offline_system_price_mismatch=%d\n", reasonCounts[platformfinance.ReasonOfflineSystemPriceMismatch])
	fmt.Fprintf(stdout, "reason_offline_final_price_mismatch=%d\n", reasonCounts[platformfinance.ReasonOfflineFinalPriceMismatch])

	fmt.Fprintf(stdout, "reason_missing_promo_fact=%d\n", reasonCounts[platformfinance.ReasonPromoFactMissing])
	fmt.Fprintf(stdout, "reason_missing_offline_fact=%d\n", reasonCounts[platformfinance.ReasonMissingOfflineFact])
	fmt.Fprintf(stdout, "reason_duplicate_offline_fact=%d\n", reasonCounts[platformfinance.ReasonDuplicateOfflineFact])
	fmt.Fprintf(stdout, "reason_online_positive_adjustment=%d\n", reasonCounts[platformfinance.ReasonOnlinePositiveAdjustment])
	fmt.Fprintf(stdout, "reason_adjustment_without_reason=%d\n", reasonCounts[platformfinance.ReasonAdjustmentWithoutReason])
	fmt.Fprintf(stdout, "reason_calculator_mismatch=%d\n", reasonCounts[platformfinance.ReasonCalculatorResultMismatch])

	fmt.Fprintf(stdout, "pii_fields_emitted=0\n")
	fmt.Fprintf(stdout, "writes_performed=0\n")
	fmt.Fprintf(stdout, "verdict=%s\n", verdict)

	if verdict == "CLEAR" {
		return 0
	}
	return 2
}

func rollbackTx(tx pgx.Tx) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return tx.Rollback(cleanupCtx)
}
