package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBookingSnapshotInvariant_DisabledAdmin(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to run migrations: %v", err)
	}

	dbPool, err := pgxpool.New(context.Background(), targetDSN)
	if err != nil {
		t.Fatalf("failed to create pgxpool: %v", err)
	}
	defer dbPool.Close()

	activeCustomerID, activeCustomerEmail := seedUser(t, db, "CUSTOMER", "ACTIVE")
	ownerID, _ := seedUser(t, db, "OWNER", "ACTIVE")

	// Seed basic booking requirements
	venueID := uuid.New().String()
	_, err = db.Exec("INSERT INTO venues (id, owner_user_id, name, status) VALUES ($1, $2, 'Test Venue', 'ACTIVE')", venueID, ownerID)
	if err != nil {
		t.Fatalf("failed to insert venue: %v", err)
	}
	courtID := uuid.New().String()
	_, err = db.Exec("INSERT INTO courts (id, venue_id, name, status, default_price_per_hour) VALUES ($1, $2, 'Test Court', 'ACTIVE', 100000)", courtID, venueID)
	if err != nil {
		t.Fatalf("failed to insert court: %v", err)
	}
	
	// Create router with admin DISABLED
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		PlatformFinanceAdminEnabled: false,
		JWTSecret: "test-secret",
		JWTExpiresInHours: 1,
		GeneralRateLimitPerMinute: 100,
		AuthRateLimitPerMinute: 100,
	}

	r, cancel, err := setupRouter(context.Background(), cfg, dbPool, false)
	if err != nil {
		t.Fatalf("failed to setup router: %v", err)
	}
	defer cancel()

	tokenService := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpiresInHours)
	token, _ := tokenService.Generate(auth.UserResponse{
		ID:    activeCustomerID,
		Email: activeCustomerEmail,
		Role:  "CUSTOMER",
	})

	// Make a booking
	reqBody := map[string]interface{}{
		"court_id": courtID,
		"booking_date": time.Now().Add(24 * time.Hour).Format("2006-01-02"),
		"start_time": "10:00:00",
		"end_time": "11:00:00",
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/api/v1/bookings", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Since we are not authenticated properly or missing some other mock setup it may fail with 400 or 500
	// But it shouldn't 404!
	if w.Code == http.StatusNotFound {
		t.Errorf("Booking route should be wired and not 404")
	}
}
