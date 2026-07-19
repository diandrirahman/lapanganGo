package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type cliReconcileFixture struct {
	pool   *pgxpool.Pool
	admin  *pgxpool.Pool
	dbURL  string
	dbName string
}

func newCLIReconcileFixture(t *testing.T) *cliReconcileFixture {
	t.Helper()
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("TEST_INTEGRATION is not enabled")
	}
	sourceURL := os.Getenv("RECONCILIATION_CLI_TEST_DATABASE_URL")
	if sourceURL == "" {
		t.Fatal("RECONCILIATION_CLI_TEST_DATABASE_URL is required for integration tests")
	}

	parsed, err := url.Parse(sourceURL)
	require.NoError(t, err)
	sourceConfig, err := pgxpool.ParseConfig(sourceURL)
	require.NoError(t, err)
	adminConfig, err := pgxpool.ParseConfig(sourceURL)
	require.NoError(t, err)
	adminConfig.ConnConfig.Database = "postgres"
	adminConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-reconcile-cli-test-admin"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	require.NoError(t, err)
	require.NoError(t, adminPool.Ping(ctx))

	dbName := "lapanggo_reconcile_cli_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = adminPool.Exec(ctx, fmt.Sprintf(
		"CREATE DATABASE %s TEMPLATE %s",
		pgx.Identifier{dbName}.Sanitize(),
		pgx.Identifier{sourceConfig.ConnConfig.Database}.Sanitize(),
	))
	require.NoError(t, err)

	cloneURL := *parsed
	cloneURL.Path = "/" + dbName
	cloneConfig, err := pgxpool.ParseConfig(cloneURL.String())
	require.NoError(t, err)
	cloneConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-reconcile-cli-test"
	clonePool, err := pgxpool.NewWithConfig(ctx, cloneConfig)
	require.NoError(t, err)
	require.NoError(t, clonePool.Ping(ctx))

	fixture := &cliReconcileFixture{pool: clonePool, admin: adminPool, dbURL: cloneURL.String(), dbName: dbName}
	t.Cleanup(func() {
		clonePool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, dropErr := adminPool.Exec(cleanupCtx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", pgx.Identifier{dbName}.Sanitize()))
		adminPool.Close()
		if dropErr != nil {
			t.Errorf("drop disposable CLI database: %v", dropErr)
		}
	})
	return fixture
}

func (f *cliReconcileFixture) enforceReadOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := f.admin.Exec(ctx, fmt.Sprintf("ALTER DATABASE %s SET default_transaction_read_only = on", pgx.Identifier{f.dbName}.Sanitize()))
	require.NoError(t, err)
	
	// Close existing pool to force new connections that pick up the read-only setting
	f.pool.Close()
	
	cloneConfig, err := pgxpool.ParseConfig(f.dbURL)
	require.NoError(t, err)
	cloneConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-reconcile-cli-test-ro"
	clonePool, err := pgxpool.NewWithConfig(ctx, cloneConfig)
	require.NoError(t, err)
	f.pool = clonePool
}

func cliFixtureEnv(databaseURL string) func(string) string {
	return func(key string) string {
		if key == "RECONCILIATION_DATABASE_URL" {
			return databaseURL
		}
		return ""
	}
}

func getDBCounts(t *testing.T, pool *pgxpool.Pool) (int, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var bookings, snapshots int
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM bookings").Scan(&bookings))
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM booking_fee_snapshots").Scan(&snapshots))
	return bookings, snapshots
}

func TestCLIReconciliationCleanIntegration(t *testing.T) {
	fixture := newCLIReconcileFixture(t)
	fixture.enforceReadOnly(t)
	
	beforeBookings, beforeSnapshots := getDBCounts(t, fixture.pool)

	var stdout, stderr bytes.Buffer
	getenv := cliFixtureEnv(fixture.dbURL)
	ops := defaultRunnerOperations()

	code := run([]string{"--start-date=2000-01-01", "--end-date=2000-01-31"}, getenv, &stdout, &stderr, ops)

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	require.Equal(t, "1", parsed["version"])

	report := parsed["report"].(map[string]interface{})
	require.Equal(t, true, report["clean"])
	require.Equal(t, "CLEAN", report["status"])

	afterBookings, afterSnapshots := getDBCounts(t, fixture.pool)
	require.Equal(t, beforeBookings, afterBookings, "Zero writes violated: bookings count changed")
	require.Equal(t, beforeSnapshots, afterSnapshots, "Zero writes violated: snapshots count changed")
}

func TestCLIReconciliationFaultIntegration(t *testing.T) {
	fixture := newCLIReconcileFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Inject a missing snapshot fault BEFORE enforcing read-only
	var cutoverAt time.Time
	var courtID, customerID uuid.UUID
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT snapshot_cutover_at FROM platform_finance_cutovers LIMIT 1").Scan(&cutoverAt))
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT id FROM courts LIMIT 1").Scan(&courtID))
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&customerID))
	require.NoError(t, func() error {
		_, err := fixture.pool.Exec(ctx, "ALTER TABLE public.bookings DISABLE TRIGGER booking_snapshot_required_after_cutover")
		return err
	}())

	faultTime := cutoverAt.Add(time.Minute)
	jakartaLoc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	faultDate := faultTime.In(jakartaLoc).Format("2006-01-02")

	_, err = fixture.pool.Exec(ctx, `
		INSERT INTO bookings (
			id, customer_id, court_id, booking_date, start_time, end_time,
			original_price, discount_amount, final_price, total_price, status,
			created_at, updated_at
		) VALUES ($1, $2, $3, CURRENT_DATE, '10:00:00', '11:00:00',
		          100000, 0, 100000, 100000, 'COMPLETED', $4, $4)
	`, uuid.New(), customerID, courtID, faultTime)
	require.NoError(t, err)

	_, err = fixture.pool.Exec(ctx, "ALTER TABLE public.bookings ENABLE TRIGGER booking_snapshot_required_after_cutover")
	require.NoError(t, err)
	
	// ENFORCE READ-ONLY before running the CLI
	fixture.enforceReadOnly(t)

	beforeBookings, beforeSnapshots := getDBCounts(t, fixture.pool)

	var stdout, stderr bytes.Buffer
	getenv := cliFixtureEnv(fixture.dbURL)
	ops := defaultRunnerOperations()

	code := run([]string{"--start-date=" + faultDate, "--end-date=" + faultDate}, getenv, &stdout, &stderr, ops)

	require.Equal(t, 1, code, "Should exit with 1 on reconciliation failure")
	require.Empty(t, stderr.String(), "No raw db errors in stderr")

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	require.Equal(t, "1", parsed["version"])

	report := parsed["report"].(map[string]interface{})
	require.Equal(t, false, report["clean"])
	require.Equal(t, "EXCEPTIONS", report["status"])

	checks := report["checks"].([]interface{})
	hasMissingSnapshot := false
	for _, c := range checks {
		check := c.(map[string]interface{})
		if check["code"] == "PAID_SNAPSHOT_SOURCE_MATCH" {
			if check["status"] == "FAIL" {
				hasMissingSnapshot = true
				exceptions := check["exceptions"].([]interface{})
				require.NotEmpty(t, exceptions)
				firstException := exceptions[0].(map[string]interface{})
				require.Equal(t, faultDate, firstException["bucket_date"], "Exception bucket date should match injected fault Jakarta date")
			}
		}
	}
	require.True(t, hasMissingSnapshot, "Missing snapshot should be detected")

	afterBookings, afterSnapshots := getDBCounts(t, fixture.pool)
	require.Equal(t, beforeBookings, afterBookings, "Zero writes violated")
	require.Equal(t, beforeSnapshots, afterSnapshots, "Zero writes violated")
}
