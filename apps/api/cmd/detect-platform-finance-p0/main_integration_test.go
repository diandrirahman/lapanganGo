package main

import (
	"bytes"
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
	"github.com/stretchr/testify/require"
)

func TestCLIPostCutoverP0ClearIntegration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("TEST_INTEGRATION is not enabled")
	}
	databaseURL := os.Getenv("POST_CUTOVER_DETECTOR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("POST_CUTOVER_DETECTOR_TEST_DATABASE_URL is required for integration tests")
	}

	var stdout, stderr bytes.Buffer
	getenv := func(key string) string {
		if key == "POST_CUTOVER_DETECTOR_DATABASE_URL" {
			return databaseURL
		}
		return ""
	}

	code := run([]string{"--batch-size=2"}, getenv, &stdout, &stderr)
	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), "verdict=CLEAR")
	require.Contains(t, stdout.String(), "writes_performed=0")
	require.Contains(t, stdout.String(), "pii_fields_emitted=0")
	require.NotContains(t, stdout.String(), "postgres://")
	require.NotContains(t, stderr.String(), "postgres://")
	require.NotContains(t, strings.ToLower(stderr.String()), "sqlstate")
}

type cliDetectorFixture struct {
	pool   *pgxpool.Pool
	admin  *pgxpool.Pool
	dbURL  string
	dbName string
}

func newCLIDetectorFixture(t *testing.T) *cliDetectorFixture {
	t.Helper()
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("TEST_INTEGRATION is not enabled")
	}
	sourceURL := os.Getenv("POST_CUTOVER_DETECTOR_TEST_DATABASE_URL")
	if sourceURL == "" {
		t.Fatal("POST_CUTOVER_DETECTOR_TEST_DATABASE_URL is required for integration tests")
	}

	parsed, err := url.Parse(sourceURL)
	require.NoError(t, err)
	sourceConfig, err := pgxpool.ParseConfig(sourceURL)
	require.NoError(t, err)
	adminConfig, err := pgxpool.ParseConfig(sourceURL)
	require.NoError(t, err)
	adminConfig.ConnConfig.Database = "postgres"
	adminConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-detector-cli-test-admin"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	require.NoError(t, err)
	require.NoError(t, adminPool.Ping(ctx))

	dbName := "lapanggo_detector_cli_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	cloneConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-detector-cli-test"
	clonePool, err := pgxpool.NewWithConfig(ctx, cloneConfig)
	require.NoError(t, err)
	require.NoError(t, clonePool.Ping(ctx))

	fixture := &cliDetectorFixture{pool: clonePool, admin: adminPool, dbURL: cloneURL.String(), dbName: dbName}
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

func cliFixtureEnv(databaseURL string) func(string) string {
	return func(key string) string {
		if key == "POST_CUTOVER_DETECTOR_DATABASE_URL" {
			return databaseURL
		}
		return ""
	}
}

func TestCLIPostCutoverP0QuarantineIntegration(t *testing.T) {
	fixture := newCLIDetectorFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bookingID := insertCLIMissingBooking(t, fixture, ctx, 100000, 100000, 100000)

	var beforeBookings, beforeSnapshots int
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT COUNT(*) FROM bookings").Scan(&beforeBookings))
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT COUNT(*) FROM booking_fee_snapshots").Scan(&beforeSnapshots))

	var stdout, stderr bytes.Buffer
	code := run([]string{"--batch-size=1"}, cliFixtureEnv(fixture.dbURL), &stdout, &stderr)
	require.Equal(t, 2, code)
	require.Contains(t, stdout.String(), "post_cutover_missing_snapshot=1")
	require.Contains(t, stdout.String(), "verdict=QUARANTINED_P0")
	require.Contains(t, stdout.String(), "writes_performed=0")
	require.NotContains(t, stdout.String(), bookingID.String())
	require.Empty(t, stderr.String())

	var afterBookings, afterSnapshots int
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT COUNT(*) FROM bookings").Scan(&afterBookings))
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT COUNT(*) FROM booking_fee_snapshots").Scan(&afterSnapshots))
	require.Equal(t, beforeBookings, afterBookings)
	require.Equal(t, beforeSnapshots, afterSnapshots)
}

func TestCLIPostCutoverP0ManualDecisionIntegration(t *testing.T) {
	fixture := newCLIDetectorFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bookingID := insertCLIMissingBooking(t, fixture, ctx, 50000.50, 50000.50, 50000.50)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--batch-size=1"}, cliFixtureEnv(fixture.dbURL), &stdout, &stderr)
	require.Equal(t, 2, code)
	require.Contains(t, stdout.String(), "manual_decision_required=1")
	require.Contains(t, stdout.String(), "reason_fractional_money=1")
	require.Contains(t, stdout.String(), "reason_invalid_money=1")
	require.Contains(t, stdout.String(), "verdict=BLOCKED_MANUAL_DECISION")
	require.Contains(t, stdout.String(), "writes_performed=0")
	require.NotContains(t, stdout.String(), bookingID.String())
	require.Empty(t, stderr.String())
}

func TestCLIPostCutoverP0IntegrityFailureIntegration(t *testing.T) {
	fixture := newCLIDetectorFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, func() error {
		_, err := fixture.pool.Exec(ctx, "ALTER TABLE public.bookings DISABLE TRIGGER booking_snapshot_required_after_cutover")
		return err
	}())

	var stdout, stderr bytes.Buffer
	code := run(nil, cliFixtureEnv(fixture.dbURL), &stdout, &stderr)
	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "verdict=INTEGRITY_FAILURE")
	require.NotContains(t, stderr.String(), "postgres://")
	require.NotContains(t, stderr.String(), "SQLSTATE")
}

func insertCLIMissingBooking(t *testing.T, fixture *cliDetectorFixture, ctx context.Context, original, final, total float64) uuid.UUID {
	t.Helper()
	var cutoverAt time.Time
	var courtID, customerID uuid.UUID
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT snapshot_cutover_at FROM platform_finance_cutovers LIMIT 1").Scan(&cutoverAt))
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT id FROM courts LIMIT 1").Scan(&courtID))
	require.NoError(t, fixture.pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&customerID))
	require.NoError(t, func() error {
		_, err := fixture.pool.Exec(ctx, "ALTER TABLE public.bookings DISABLE TRIGGER booking_snapshot_required_after_cutover")
		return err
	}())
	bookingID := uuid.New()
	_, err := fixture.pool.Exec(ctx, `
		INSERT INTO bookings (
			id, customer_id, court_id, booking_date, start_time, end_time,
			original_price, discount_amount, final_price, total_price, status,
			created_at, updated_at
		) VALUES ($1, $2, $3, CURRENT_DATE, '10:00:00', '11:00:00',
		          $4, 0, $5, $6, 'COMPLETED', $7, $7)
	`, bookingID, customerID, courtID, original, final, total, cutoverAt.Add(time.Minute))
	require.NoError(t, err)
	_, err = fixture.pool.Exec(ctx, "ALTER TABLE public.bookings ENABLE TRIGGER booking_snapshot_required_after_cutover")
	require.NoError(t, err)
	return bookingID
}
