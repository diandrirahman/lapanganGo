package platformfinance_test

import (
	"context"
	"os"
	"testing"
	"time"

	"lapangango-api/internal/database"
	"lapangango-api/internal/platformfinance"

	"github.com/stretchr/testify/assert"
)

func TestRepository_GetSummaryData(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Skip("Database not available")
	}
	defer pool.Close()

	repo := platformfinance.NewRepository(pool)

	start := time.Now().UTC().AddDate(0, -1, 0)
	end := time.Now().UTC().AddDate(0, 0, 1)

	// Since we are running in tests, if duplicate ledger is present it will fail closed.
	res, err := repo.GetSummaryData(ctx, start, end, "", "")
	if err != nil {
		if err == platformfinance.ErrDuplicateLedgerDetected {
			// Expected if data is bad
		} else {
			assert.NoError(t, err)
			assert.NotNil(t, res)
			if res != nil {
				assert.NotZero(t, res.AsOf)
			}
		}
	} else {
		assert.NotNil(t, res)
	}
}

func TestRepository_GetPaginatedBreakdownProjection(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Skip("Database not available")
	}
	defer pool.Close()

	repo := platformfinance.NewRepository(pool)
	res, err := repo.GetPaginatedBreakdown(ctx,
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		"", "", "owner", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Rows) > 2 || res.TotalItems < len(res.Rows) {
		t.Fatalf("invalid paginated projection response: %#v", res)
	}
}
