package platformfinance_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"lapangango-api/internal/database"
)

func TestSQLRoundingFixture(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable")
	if err != nil {
		t.Skip("Database not available")
	}
	defer pool.Close()

	testCases := []struct {
		Amount int
		Expect int64
	}{
		{49, 3},
		{50, 4},
		{51, 4},
	}

	for _, tc := range testCases {
		var res int64
		err := pool.QueryRow(ctx, "SELECT CAST(ROUND($1 * 700.0 / 10000.0) AS bigint)", tc.Amount).Scan(&res)
		assert.NoError(t, err)
		assert.Equal(t, tc.Expect, res)
	}
}
