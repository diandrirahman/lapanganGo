package main

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

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

func countIdleInTransaction(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	query := `
		SELECT COUNT(*)
		FROM pg_stat_activity
		WHERE application_name = 'lapanggo-backfill-dry-run'
		  AND state = 'idle in transaction'
	`
	err := pool.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestCLIMain(t *testing.T) {
	pool := getIntegrationDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initialBookings := countTableRows(t, ctx, pool, "bookings")
	initialSnapshots := countTableRows(t, ctx, pool, "booking_fee_snapshots")
	initialCutovers := countTableRows(t, ctx, pool, "platform_finance_cutovers")

	// Get exact cutover
	var cutover time.Time
	err := pool.QueryRow(ctx, "SELECT snapshot_cutover_at FROM platform_finance_cutovers LIMIT 1").Scan(&cutover)
	require.NoError(t, err)
	exactCutoverStr := cutover.UTC().Format(time.RFC3339Nano)

	getenv := func(key string) string {
		if key == "BACKFILL_DATABASE_URL" {
			return os.Getenv("TEST_DATABASE_URL")
		}
		return os.Getenv(key)
	}

	tests := []struct {
		name         string
		args         []string
		expectedCode int
		expectOut    string
		expectErr    string
		notExpectErr []string
	}{
		{
			name:         "Exact cutover menghasilkan exit code 0",
			args:         []string{"--dry-run=true", "--batch-size=2", "--cutover-at=" + exactCutoverStr},
			expectedCode: 0,
			expectOut:    "candidate_total=7",
		},
		{
			name:         "Cutover berbeda satu jam menghasilkan exit code 1",
			args:         []string{"--dry-run=true", "--batch-size=2", "--cutover-at=" + cutover.Add(1*time.Hour).UTC().Format(time.RFC3339Nano)},
			expectedCode: 1,
			expectErr:    "Provided cutover time does not match stored cutover exactly",
			notExpectErr: []string{"postgres://", "localhost", "5432", "55433", "FATAL", "ERROR"},
		},
		{
			name:         "--dry-run=false menghasilkan exit code 1",
			args:         []string{"--dry-run=false", "--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "--dry-run=true is required",
		},
		{
			name:         "--apply ditolak",
			args:         []string{"--dry-run=true", "--apply=true", "--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "--apply is not supported",
		},
		{
			name:         "Invalid UUID cursor ditolak",
			args:         []string{"--dry-run=true", "--after-booking-id=invalid-uuid", "--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "--after-booking-id must be a valid UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := run(tt.args, getenv, &stdout, &stderr)

			assert.Equal(t, tt.expectedCode, exitCode)

			if tt.expectOut != "" {
				assert.Contains(t, stdout.String(), tt.expectOut)
			}

			errStr := stderr.String()
			if tt.expectErr != "" {
				assert.Contains(t, errStr, tt.expectErr)
			}

			// Ensure generic error doesn't leak PII or DB details
			for _, unexp := range tt.notExpectErr {
				assert.NotContains(t, errStr, unexp)
			}

			// 8. Setelah setiap error path tidak ada session `idle in transaction` dari CLI
			idleCount := countIdleInTransaction(t, ctx, pool)
			assert.Equal(t, 0, idleCount, "Should not have idle transactions remaining")
		})
	}

	// 7. Before/after counts tetap
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	finalBookings := countTableRows(t, cleanupCtx, pool, "bookings")
	finalSnapshots := countTableRows(t, cleanupCtx, pool, "booking_fee_snapshots")
	finalCutovers := countTableRows(t, cleanupCtx, pool, "platform_finance_cutovers")

	assert.Equal(t, initialBookings, finalBookings, "Bookings count should not change")
	assert.Equal(t, initialSnapshots, finalSnapshots, "Snapshots count should not change")
	assert.Equal(t, initialCutovers, finalCutovers, "Cutovers count should not change")
}
