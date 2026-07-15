package platformfinance

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type disposableDetectorFixture struct {
	pool        *pgxpool.Pool
	databaseURL string
	database    string
}

func newDisposableDetectorFixture(t *testing.T) *disposableDetectorFixture {
	t.Helper()
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test: TEST_INTEGRATION not set")
	}

	sourceURL := os.Getenv("POST_CUTOVER_DETECTOR_TEST_DATABASE_URL")
	if sourceURL == "" {
		t.Fatal("POST_CUTOVER_DETECTOR_TEST_DATABASE_URL is required for integration tests")
	}

	parsedURL, err := url.Parse(sourceURL)
	require.NoError(t, err)
	require.Contains(t, []string{"postgres", "postgresql"}, parsedURL.Scheme)

	sourceConfig, err := pgxpool.ParseConfig(sourceURL)
	require.NoError(t, err)
	sourceDatabase := sourceConfig.ConnConfig.Database
	require.NotEmpty(t, sourceDatabase)

	adminConfig, err := pgxpool.ParseConfig(sourceURL)
	require.NoError(t, err)
	adminConfig.ConnConfig.Database = "postgres"
	adminConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-detector-test-admin"

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()

	adminPool, err := pgxpool.NewWithConfig(setupCtx, adminConfig)
	require.NoError(t, err)
	require.NoError(t, adminPool.Ping(setupCtx))

	cloneDatabase := "lapanggo_detector_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	createSQL := fmt.Sprintf(
		"CREATE DATABASE %s TEMPLATE %s",
		pgx.Identifier{cloneDatabase}.Sanitize(),
		pgx.Identifier{sourceDatabase}.Sanitize(),
	)
	_, err = adminPool.Exec(setupCtx, createSQL)
	if err != nil {
		adminPool.Close()
		t.Fatalf("create disposable detector database: %v", err)
	}

	cloneURL := *parsedURL
	cloneURL.Path = "/" + cloneDatabase
	cloneConfig, err := pgxpool.ParseConfig(cloneURL.String())
	require.NoError(t, err)
	cloneConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-detector-test-harness"

	clonePool, err := pgxpool.NewWithConfig(setupCtx, cloneConfig)
	if err != nil {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", pgx.Identifier{cloneDatabase}.Sanitize()))
		dropCancel()
		adminPool.Close()
		t.Fatalf("connect disposable detector database: %v", err)
	}
	require.NoError(t, clonePool.Ping(setupCtx))

	t.Cleanup(func() {
		clonePool.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, dropErr := adminPool.Exec(
			cleanupCtx,
			fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", pgx.Identifier{cloneDatabase}.Sanitize()),
		)
		adminPool.Close()
		if dropErr != nil {
			t.Errorf("drop disposable detector database: %v", dropErr)
		}
	})

	return &disposableDetectorFixture{
		pool:        clonePool,
		databaseURL: cloneURL.String(),
		database:    cloneDatabase,
	}
}

func detectorCountTableRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&count)
	require.NoError(t, err)
	return count
}

type tableCounts struct {
	Users                    int
	OwnerProfiles            int
	Venues                   int
	Courts                   int
	Bookings                 int
	OfflineBookingCustomers  int
	BookingFeeSnapshots      int
	PlatformCommercialTerms  int
	PlatformFinanceCutovers  int
	OwnerFinanceTransactions int
	PlatformAuditLogs        int
}

func getTableCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) tableCounts {
	return tableCounts{
		Users:                    detectorCountTableRows(t, ctx, pool, "users"),
		OwnerProfiles:            detectorCountTableRows(t, ctx, pool, "owner_profiles"),
		Venues:                   detectorCountTableRows(t, ctx, pool, "venues"),
		Courts:                   detectorCountTableRows(t, ctx, pool, "courts"),
		Bookings:                 detectorCountTableRows(t, ctx, pool, "bookings"),
		OfflineBookingCustomers:  detectorCountTableRows(t, ctx, pool, "offline_booking_customers"),
		BookingFeeSnapshots:      detectorCountTableRows(t, ctx, pool, "booking_fee_snapshots"),
		PlatformCommercialTerms:  detectorCountTableRows(t, ctx, pool, "platform_commercial_terms"),
		PlatformFinanceCutovers:  detectorCountTableRows(t, ctx, pool, "platform_finance_cutovers"),
		OwnerFinanceTransactions: detectorCountTableRows(t, ctx, pool, "owner_finance_transactions"),
		PlatformAuditLogs:        detectorCountTableRows(t, ctx, pool, "platform_audit_logs"),
	}
}

func TestPostCutoverP0_Integration(t *testing.T) {
	fixture := newDisposableDetectorFixture(t)
	pool := fixture.pool
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Ensure trigger is valid initially
	valid, err := VerifyTriggerIntegrity(ctx, pool)
	require.NoError(t, err)
	require.True(t, valid, "trigger should be valid initially on clone")

	// Get a court_id and customer_id to use
	var courtID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT id FROM courts LIMIT 1").Scan(&courtID)
	require.NoError(t, err)

	var customerID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&customerID)
	require.NoError(t, err)

	// Step 3: Disable trigger to insert corrupt data
	_, err = pool.Exec(ctx, "ALTER TABLE public.bookings DISABLE TRIGGER booking_snapshot_required_after_cutover")
	require.NoError(t, err)

	// Get cutover time
	var cutoverAt time.Time
	err = pool.QueryRow(ctx, "SELECT snapshot_cutover_at FROM platform_finance_cutovers LIMIT 1").Scan(&cutoverAt)
	require.NoError(t, err)

	// Step 4: Insert missing snapshot bookings (post-cutover)
	// 1 Online Repairable
	onlineBookingID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO bookings (
			id, created_at, updated_at, customer_id, court_id, status,
			original_price, final_price, total_price, discount_amount, booking_date, start_time, end_time,
			promo_id, promo_code
		) VALUES (
			$1, $2, $2, $3, $4, 'COMPLETED',
			100000, 100000, 100000, 0, CURRENT_DATE, '10:00:00', '11:00:00',
			NULL, NULL
		)
	`, onlineBookingID, cutoverAt.Add(1*time.Minute), customerID, courtID)
	require.NoError(t, err)

	// 1 Offline Repairable
	offlineBookingID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO bookings (
			id, created_at, updated_at, customer_id, court_id, status,
			original_price, final_price, total_price, discount_amount, booking_date, start_time, end_time
		) VALUES (
			$1, $2, $2, $3, $4, 'COMPLETED',
			100000, 90000, 90000, 0, CURRENT_DATE, '11:00:00', '12:00:00'
		)
	`, offlineBookingID, cutoverAt.Add(2*time.Minute), customerID, courtID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO offline_booking_customers (
			booking_id, name, phone, system_price, final_price, price_override_reason
		) VALUES (
			$1, 'Test Offline', '081234567890', 100000, 90000, 'Member discount'
		)
	`, offlineBookingID)
	require.NoError(t, err)

	// 1 Manual Decision: fractional money is allowed by NUMERIC schema but not by
	// the whole-rupiah snapshot contract.
	manualBookingID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO bookings (
			id, created_at, updated_at, customer_id, court_id, status,
			original_price, final_price, total_price, discount_amount, booking_date, start_time, end_time
		) VALUES (
			$1, $2, $2, $3, $4, 'COMPLETED',
			50000.50, 50000.50, 50000.50, 0, CURRENT_DATE, '12:00:00', '13:00:00'
		)
	`, manualBookingID, cutoverAt.Add(3*time.Minute), customerID, courtID)
	require.NoError(t, err)

	// Step 5: Enable trigger again
	_, err = pool.Exec(ctx, "ALTER TABLE public.bookings ENABLE TRIGGER booking_snapshot_required_after_cutover")
	require.NoError(t, err)

	// Step 6: Verify trigger integrity
	valid, err = VerifyTriggerIntegrity(ctx, pool)
	require.NoError(t, err)
	require.True(t, valid)

	// Step 7: Record counts BEFORE detector
	countsBefore := getTableCounts(t, ctx, pool)

	// Step 8: Run detector inside a transaction
	txCtx, txCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer txCancel()

	tx, err := pool.BeginTx(txCtx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	require.NoError(t, err)

	preflight, err := LoadPostCutoverDetectorPreflight(txCtx, tx)
	require.NoError(t, err)

	population, err := LoadPostCutoverPopulation(txCtx, tx, preflight.CutoverAt)
	require.NoError(t, err)

	assert.Equal(t, int64(3), population.PostCutoverMissingSnapshot, "Should detect 3 missing snapshots")

	termResolver := NewCommercialTermResolver(tx)

	batch, err := FetchPostCutoverP0Candidates(txCtx, tx, PostCutoverDetectorParams{
		CutoverAt: preflight.CutoverAt,
		BatchSize: 100,
	})
	require.NoError(t, err)

	var repairableOnline int
	var repairableOffline int
	var manualDecision int
	for _, c := range batch.Candidates {
		var term *CommercialTerm
		var resolverErr error
		if c.OwnerProfileID != nil {
			term, resolverErr = termResolver.ResolveEffectiveTerm(txCtx, c.OwnerProfileID.String(), c.CreatedAt.UTC())
		}
		res := ClassifyPostCutoverP0Candidate(c, term, resolverErr)
		if res.Classification == ClassificationRepairablePolicyOnline {
			repairableOnline++
		} else if res.Classification == ClassificationRepairablePolicyWalkIn {
			repairableOffline++
		} else {
			manualDecision++
		}
	}

	assert.Equal(t, 1, repairableOnline)
	assert.Equal(t, 1, repairableOffline)
	assert.Equal(t, 1, manualDecision)

	// Step 9: Rollback tx
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	err = tx.Rollback(cleanupCtx)
	require.NoError(t, err)

	// Step 10: Record counts AFTER detector
	countsAfter := getTableCounts(t, ctx, pool)
	assert.Equal(t, countsBefore, countsAfter, "No table counts should change")

	// Ensure no idle in transaction
	var idleCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM pg_stat_activity WHERE application_name = 'lapanggo-detector-test-harness' AND state = 'idle in transaction'").Scan(&idleCount)
	require.NoError(t, err)
	assert.Equal(t, 0, idleCount)
}
