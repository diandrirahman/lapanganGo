package admin_test

import (
	"context"
	"os"
	"testing"
	"time"

	"lapangango-api/internal/admin"
	"lapangango-api/internal/database"

	"github.com/stretchr/testify/assert"
)

func TestAdminRepository_GetVenues_OwnerProfileFilter(t *testing.T) {
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

	repo := admin.NewRepository(pool)

	// Generic fetch
	_, count, err := repo.GetVenues(ctx, admin.VenueQuery{})
	assert.NoError(t, err)
	
	// Fetch with filter
	venuesFiltered, countFiltered, err := repo.GetVenues(ctx, admin.VenueQuery{
		OwnerProfileID: "00000000-0000-0000-0000-000000000000",
	})
	assert.NoError(t, err)

	// Since 000... doesn't exist, countFiltered should be 0
	if count > 0 {
		assert.Equal(t, 0, countFiltered)
		assert.Empty(t, venuesFiltered)
	}
}
