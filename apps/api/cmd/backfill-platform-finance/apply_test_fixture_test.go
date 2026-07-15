package main

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
	"github.com/stretchr/testify/require"
)

type disposableApplyFixture struct {
	pool        *pgxpool.Pool
	databaseURL string
	database    string
}

func newDisposableApplyFixture(t *testing.T) *disposableApplyFixture {
	t.Helper()
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test: TEST_INTEGRATION not set")
	}

	sourceURL := os.Getenv("TEST_DATABASE_URL")
	if sourceURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
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
	adminConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-backfill-test-admin"

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()

	adminPool, err := pgxpool.NewWithConfig(setupCtx, adminConfig)
	require.NoError(t, err)
	require.NoError(t, adminPool.Ping(setupCtx))

	cloneDatabase := "lapanggo_backfill_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	createSQL := fmt.Sprintf(
		"CREATE DATABASE %s TEMPLATE %s",
		pgx.Identifier{cloneDatabase}.Sanitize(),
		pgx.Identifier{sourceDatabase}.Sanitize(),
	)
	_, err = adminPool.Exec(setupCtx, createSQL)
	if err != nil {
		adminPool.Close()
		t.Fatalf("create disposable apply database: %v", err)
	}

	cloneURL := *parsedURL
	cloneURL.Path = "/" + cloneDatabase
	cloneConfig, err := pgxpool.ParseConfig(cloneURL.String())
	require.NoError(t, err)
	cloneConfig.ConnConfig.RuntimeParams["application_name"] = "lapanggo-backfill-test-harness"

	clonePool, err := pgxpool.NewWithConfig(setupCtx, cloneConfig)
	if err != nil {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", pgx.Identifier{cloneDatabase}.Sanitize()))
		dropCancel()
		adminPool.Close()
		t.Fatalf("connect disposable apply database: %v", err)
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
			t.Errorf("drop disposable apply database: %v", dropErr)
		}
	})

	return &disposableApplyFixture{
		pool:        clonePool,
		databaseURL: cloneURL.String(),
		database:    cloneDatabase,
	}
}

func (f *disposableApplyFixture) getenv() func(string) string {
	return func(key string) string {
		switch key {
		case "BACKFILL_DATABASE_URL":
			return f.databaseURL
		case "BACKFILL_TARGET_ENVIRONMENT":
			return "development"
		case "BACKFILL_EXPECTED_DATABASE_NAME":
			return f.database
		default:
			return os.Getenv(key)
		}
	}
}

func storedCutoverString(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var cutover time.Time
	require.NoError(t, pool.QueryRow(ctx, "SELECT snapshot_cutover_at FROM platform_finance_cutovers LIMIT 1").Scan(&cutover))
	return cutover.UTC().Format(time.RFC3339Nano)
}

func candidateBookingIDs(t *testing.T, ctx context.Context, pool *pgxpool.Pool, limit int) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT b.id
		FROM bookings b
		CROSS JOIN platform_finance_cutovers c
		WHERE b.created_at < c.snapshot_cutover_at
		  AND NOT EXISTS (
			  SELECT 1 FROM booking_fee_snapshots bfs WHERE bfs.booking_id = b.id
		  )
		ORDER BY b.id ASC
		LIMIT $1
	`, limit)
	require.NoError(t, err)
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}
