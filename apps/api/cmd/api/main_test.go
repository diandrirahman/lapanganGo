package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"lapangango-api/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSetupRouterFailsClosedWhenPaymentWorkerActivationIsRequested(t *testing.T) {
	cfg := config.Config{PaymentCreateEnabled: true}
	_, _, err := setupRouter(context.Background(), cfg, nil, true)
	if !errors.Is(err, errPaymentProviderAdapterContractBlocked) {
		t.Fatalf("error = %v; want provider adapter contract guard", err)
	}
}

func TestSetupRouterWebhookRoutesAreFlagGatedAndIsolated(t *testing.T) {
	base := config.Config{
		JWTSecret:                 "test-jwt-secret",
		JWTExpiresInHours:         1,
		GeneralRateLimitPerMinute: 0,
		AuthRateLimitPerMinute:    100,
	}

	disabled, cancel, err := setupRouter(context.Background(), base, nil, false)
	if err != nil {
		t.Fatalf("setup disabled router: %v", err)
	}
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", nil)
	response := httptest.NewRecorder()
	disabled.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled webhook status=%d; want 404", response.Code)
	}

	enabledConfig := base
	enabledConfig.PaymentWebhookIngressEnabled = true
	enabledConfig.PaymentWebhookContractVersion = "XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL"
	enabledConfig.XenditWebhookToken = "test-callback-token"
	enabled, enabledCancel, err := setupRouter(context.Background(), enabledConfig, nil, false)
	if err != nil {
		t.Fatalf("setup enabled router: %v", err)
	}
	defer enabledCancel()
	for attempt := 0; attempt < 2; attempt++ {
		request = httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", nil)
		request.Header.Set("Content-Type", "text/plain")
		response = httptest.NewRecorder()
		enabled.ServeHTTP(response, request)
		if response.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("enabled webhook attempt %d status=%d; route inherited JWT or general limiter", attempt+1, response.Code)
		}
	}
}

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
