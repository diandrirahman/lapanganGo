package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"lapangango-api/internal/config"
	"lapangango-api/internal/database"
	"lapangango-api/internal/httputil"
	"lapangango-api/internal/platformfinance"
)

func main() {
	var apply bool
	var maintenanceConfirmed bool
	var calcVersion string
	var releaseRef string
	var actorUserID string
	var lockTimeoutStr string

	flag.BoolVar(&apply, "apply", false, "Apply the cutover mutation")
	flag.BoolVar(&maintenanceConfirmed, "maintenance-confirmed", false, "Confirm that booking-create maintenance window is active")
	flag.StringVar(&calcVersion, "calculation-version", "", "Calculation version (must be booking-fee-v1)")
	flag.StringVar(&releaseRef, "release-reference", "", "Release reference (trimmed, non-empty, max 255)")
	flag.StringVar(&actorUserID, "user-id", "", "ACTIVE SUPER_ADMIN user UUID")
	flag.StringVar(&lockTimeoutStr, "lock-timeout", "30s", "Postgres lock timeout duration")

	flag.Parse()

	// 1. Basic validation of flags
	if calcVersion == "" {
		fmt.Fprintln(os.Stderr, "Error: calculation version is required")
		os.Exit(1)
	}
	if calcVersion != platformfinance.ActiveBookingFeeCalculationVersion {
		fmt.Fprintln(os.Stderr, "Error: invalid calculation version")
		os.Exit(1)
	}

	releaseRef = strings.TrimSpace(releaseRef)
	if releaseRef == "" || len(releaseRef) > 255 {
		fmt.Fprintln(os.Stderr, "Error: release reference is invalid or too long")
		os.Exit(1)
	}

	if actorUserID == "" {
		fmt.Fprintln(os.Stderr, "Error: actor user id is required")
		os.Exit(1)
	}
	if !httputil.IsUUID(actorUserID) {
		fmt.Fprintln(os.Stderr, "Error: actor user id is not a valid UUID")
		os.Exit(1)
	}

	lockTimeout, err := time.ParseDuration(lockTimeoutStr)
	if err != nil || lockTimeout <= 0 || lockTimeout > 10*time.Minute {
		fmt.Fprintln(os.Stderr, "Error: lock timeout is invalid")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: invalid application configuration")
		os.Exit(1)
	}

	// Preflight context is bounded to 15 seconds, separate from apply lock timeout budget
	preflightCtx, preflightCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer preflightCancel()

	dbPool, err := database.NewPostgresPool(preflightCtx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to establish database connection")
		os.Exit(1)
	}
	defer dbPool.Close()

	// Preflight validation: check migrations
	var schemaVersion int
	var dirty bool
	err = dbPool.QueryRow(preflightCtx, `SELECT version, dirty FROM schema_migrations`).Scan(&schemaVersion, &dirty)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to query schema migrations state")
		os.Exit(1)
	}
	if schemaVersion < 21 || dirty {
		fmt.Fprintln(os.Stderr, "Error: database schema version is invalid or dirty")
		os.Exit(1)
	}

	// Preflight validation: verify trigger contract using shared helper
	triggerValid, err := platformfinance.VerifyTriggerIntegrity(preflightCtx, dbPool)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to verify database integrity trigger")
		os.Exit(1)
	}
	if !triggerValid {
		fmt.Fprintln(os.Stderr, "Error: required deferred booking snapshot trigger is missing or misconfigured")
		os.Exit(1)
	}

	// Verify actor from DB (role and status)
	var role, status string
	err = dbPool.QueryRow(preflightCtx, "SELECT role::text, status::text FROM users WHERE id = $1", actorUserID).Scan(&role, &status)
	if err != nil {
		if err == pgx.ErrNoRows {
			fmt.Fprintln(os.Stderr, "Error: actor is not authorized or inactive")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Error: database query failure during actor verification")
		os.Exit(1)
	}

	if role != "SUPER_ADMIN" || status != "ACTIVE" {
		fmt.Fprintln(os.Stderr, "Error: actor is not authorized or inactive")
		os.Exit(1)
	}

	// Check if already active
	var cutoverExists bool
	err = dbPool.QueryRow(preflightCtx, "SELECT EXISTS(SELECT 1 FROM platform_finance_cutovers WHERE id = 1)").Scan(&cutoverExists)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: database query failure during cutover check")
		os.Exit(1)
	}

	if cutoverExists {
		fmt.Println("Status: Cutover is ALREADY ACTIVE")
		if apply {
			fmt.Fprintln(os.Stderr, "Error: Cutover has already been activated, cannot re-apply")
			os.Exit(2)
		}
		os.Exit(0)
	}

	// Dry run / preflight mode
	if !apply {
		fmt.Println("--- FINANCE CUTOVER PREFLIGHT (DRY-RUN) ---")
		fmt.Println("Preflight status: VALID")
		fmt.Printf("Calculation Version: %s\n", calcVersion)
		fmt.Printf("Release Reference:   %s\n", releaseRef)
		fmt.Printf("Lock Timeout:        %s\n", lockTimeout)
		fmt.Println("Ready for cutover. Run with --apply --maintenance-confirmed to execute.")
		os.Exit(0)
	}

	// Apply mode
	if !maintenanceConfirmed {
		fmt.Fprintln(os.Stderr, "Error: --maintenance-confirmed is required when running with --apply")
		os.Exit(1)
	}

	activator, err := platformfinance.NewCutoverActivator(dbPool)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to initialize cutover activator")
		os.Exit(1)
	}

	// Create a new apply context representing the lock budget
	applyCtx, applyCancel := context.WithTimeout(context.Background(), lockTimeout+5*time.Second)
	defer applyCancel()

	record, err := activator.ActivateCutover(applyCtx, platformfinance.ActivateCutoverParams{
		CalculationVersion: calcVersion,
		ReleaseReference:   releaseRef,
		ActorUserID:        actorUserID,
		LockTimeout:        lockTimeout,
	})
	if err != nil {
		switch {
		case errors.Is(err, platformfinance.ErrCutoverAlreadyActive):
			fmt.Fprintln(os.Stderr, "Error: cutover is already active")
		case errors.Is(err, platformfinance.ErrCutoverActorForbidden):
			fmt.Fprintln(os.Stderr, "Error: actor is not authorized or inactive")
		case errors.Is(err, platformfinance.ErrInvalidCutoverParams):
			fmt.Fprintln(os.Stderr, "Error: invalid parameters provided")
		case errors.Is(err, platformfinance.ErrCutoverLockTimeout):
			fmt.Fprintln(os.Stderr, "Error: acquisition lock timed out")
		case errors.Is(err, platformfinance.ErrCutoverIntegrity):
			fmt.Fprintln(os.Stderr, "Error: trigger/schema integrity check failed")
		default:
			fmt.Fprintln(os.Stderr, "Error: activation failed due to an internal system error")
		}
		os.Exit(3)
	}

	// Sanitize output, printing only specified details
	fmt.Printf("SUCCESS: Cutover activated\n")
	fmt.Printf("Timestamp: %s\n", record.SnapshotCutoverAt.Format(time.RFC3339Nano))
	fmt.Printf("CalculationVersion: %s\n", record.CalculationVersion)
	fmt.Printf("ReleaseReference: %s\n", record.ReleaseReference)
}
