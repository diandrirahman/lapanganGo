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
		WHERE application_name IN (
			'lapanggo-backfill-dry-run',
			'lapanggo-backfill-apply'
		)
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
		envOverwrite map[string]string
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
			expectErr:    "cutover configuration mismatch",
			notExpectErr: []string{"postgres://", "localhost", "5432", "55433", "FATAL", "ERROR"},
		},
		{
			name:         "--apply dan --dry-run tidak boleh sama-sama aktif",
			args:         []string{"--apply=true", "--dry-run=true", "--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "Exactly one of --apply or --dry-run must be true",
		},
		{
			name:         "salah satu mode wajib aktif",
			args:         []string{"--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "Exactly one of --apply or --dry-run must be true",
		},
		{
			name:         "apply tanpa confirmation ditolak",
			args:         []string{"--apply=true", "--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "--apply requires --non-production-confirmed=true",
		},
		{
			name:         "apply tanpa expected count ditolak",
			args:         []string{"--apply=true", "--non-production-confirmed=true", "--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "--apply requires --expected-candidate-count",
		},
		{
			name:         "production target ditolak",
			args:         []string{"--apply=true", "--non-production-confirmed=true", "--expected-candidate-count=7", "--cutover-at=" + exactCutoverStr},
			envOverwrite: map[string]string{"BACKFILL_TARGET_ENVIRONMENT": "production"},
			expectedCode: 1,
			expectErr:    "BACKFILL_TARGET_ENVIRONMENT must be 'development' or 'staging'",
		},
		{
			name:         "database mismatch ditolak",
			args:         []string{"--apply=true", "--non-production-confirmed=true", "--expected-candidate-count=7", "--cutover-at=" + exactCutoverStr},
			envOverwrite: map[string]string{"BACKFILL_TARGET_ENVIRONMENT": "staging", "BACKFILL_EXPECTED_DATABASE_NAME": "wrong_db"},
			expectedCode: 1,
			expectErr:    "unable to verify target database",
		},
		{
			name:         "Invalid UUID cursor ditolak",
			args:         []string{"--dry-run=true", "--after-booking-id=invalid-uuid", "--cutover-at=" + exactCutoverStr},
			expectedCode: 1,
			expectErr:    "Invalid after-booking-id UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			customGetenv := func(key string) string {
				if tt.envOverwrite != nil {
					if val, ok := tt.envOverwrite[key]; ok {
						return val
					}
				}
				return getenv(key)
			}

			exitCode := run(tt.args, customGetenv, &stdout, &stderr)

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

func TestApply(t *testing.T) {
	fixture := newDisposableApplyFixture(t)
	pool := fixture.pool

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exactCutoverStr := storedCutoverString(t, ctx, pool)

	// 1. First Apply
	var stdout1, stderr1 bytes.Buffer
	args1 := []string{"--apply=true", "--non-production-confirmed=true", "--expected-candidate-count=7", "--batch-size=2", "--cutover-at=" + exactCutoverStr}
	exitCode1 := run(args1, fixture.getenv(), &stdout1, &stderr1)

	require.Equal(t, 0, exitCode1, "First apply should succeed: %s", stderr1.String())
	assert.Contains(t, stdout1.String(), "processed=7")
	assert.Contains(t, stdout1.String(), "inserted=7")
	assert.Contains(t, stdout1.String(), "idempotent_noop=0")
	assert.Contains(t, stdout1.String(), "commit_outcome=CONFIRMED")

	// Ensure no idle in transaction left
	assert.Equal(t, 0, countIdleInTransaction(t, ctx, pool), "Should not have idle transactions remaining")

	// 2. Rerun (Idempotent / No candidates left)
	var stdout2, stderr2 bytes.Buffer
	args2 := []string{"--apply=true", "--non-production-confirmed=true", "--expected-candidate-count=0", "--batch-size=2", "--cutover-at=" + exactCutoverStr}
	exitCode2 := run(args2, fixture.getenv(), &stdout2, &stderr2)

	require.Equal(t, 0, exitCode2, "Rerun apply should succeed: %s", stderr2.String())
	assert.Contains(t, stdout2.String(), "processed=0")
	assert.Contains(t, stdout2.String(), "inserted=0")
	assert.Contains(t, stdout2.String(), "idempotent_noop=0")
	assert.Contains(t, stdout2.String(), "commit_outcome=CONFIRMED")

	// Ensure no idle in transaction left
	assert.Equal(t, 0, countIdleInTransaction(t, ctx, pool), "Should not have idle transactions remaining")
}
