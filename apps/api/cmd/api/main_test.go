package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lapangango-api/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRouterWiring_FinanceAdminDisabled(t *testing.T) {
	// Only run this test if a database is available (we can reuse the logic of starting a test pool or just skip if we can't easily mock)
	// Since setupRouter requires a real pgxpool due to all repositories being initialized, we should test it with a test database if available.
	
	// Skip for simple unit tests if no DB URL is provided.
	dbURL := "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	
	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skip("Skipping router test because no database is available")
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		t.Skip("Skipping router test because database ping failed")
	}

	cfg := config.Config{
		PlatformFinanceAdminEnabled: false,
		JWTSecret:                   "test-secret",
		JWTExpiresInHours:           24,
	}

	r, cancel, err := setupRouter(ctx, cfg, dbPool, false)
	if err != nil {
		t.Fatalf("failed to setup router: %v", err)
	}
	defer cancel()

	// Test a finance admin route - it should be a 404 Not Found since it's unregistered
	req, _ := http.NewRequest("GET", "/admin/finance/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found when finance admin is disabled, got %d", w.Code)
	}
	
	// Test a normal route - it should exist (might return 401 Unauthorized but NOT 404)
	req2, _ := http.NewRequest("GET", "/admin/users", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code == http.StatusNotFound {
		t.Errorf("Expected admin/users to exist, but got 404")
	}
}

func TestRouterWiring_FinanceAdminEnabled(t *testing.T) {
	dbURL := "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skip("Skipping router test because no database is available")
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		t.Skip("Skipping router test because database ping failed")
	}

	cfg := config.Config{
		PlatformFinanceAdminEnabled: true,
		JWTSecret:                   "test-secret",
		JWTExpiresInHours:           24,
	}

	r, cancel, err := setupRouter(ctx, cfg, dbPool, false)
	if err != nil {
		t.Fatalf("failed to setup router: %v", err)
	}
	defer cancel()

	req, _ := http.NewRequest("GET", "/admin/finance/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Since we are not authenticated, it should return 401 Unauthorized, proving the route exists
	if w.Code == http.StatusNotFound {
		t.Errorf("Expected route to exist and return 401, but got 404")
	}
}
