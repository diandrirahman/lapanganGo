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
	ownerProfileID := uuid.New().String()
	_, err = db.Exec(`INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($1, $2, 'Invariant Owner', 'APPROVED')`, ownerProfileID, ownerID)
	if err != nil {
		t.Fatalf("failed to insert owner profile: %v", err)
	}
	venueID := uuid.New().String()
	_, err = db.Exec(`INSERT INTO venues (id, owner_profile_id, name, address, city, status) VALUES ($1, $2, 'Test Venue', 'Test Address', 'Jakarta', 'ACTIVE')`, venueID, ownerProfileID)
	if err != nil {
		t.Fatalf("failed to insert venue: %v", err)
	}
	var sportID string
	if err = db.QueryRow("SELECT id FROM sports WHERE name = 'Futsal' LIMIT 1").Scan(&sportID); err != nil {
		t.Fatalf("failed to find seeded sport: %v", err)
	}
	courtID := uuid.New().String()
	_, err = db.Exec(`INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, status) VALUES ($1, $2, $3, 'Test Court', 'INDOOR', 100000, 'ACTIVE')`, courtID, venueID, sportID)
	if err != nil {
		t.Fatalf("failed to insert court: %v", err)
	}
	bookingDate := time.Now().In(time.FixedZone("WIB", 7*60*60)).Add(24 * time.Hour).Format("2006-01-02")
	parsedDate, err := time.Parse("2006-01-02", bookingDate)
	if err != nil {
		t.Fatalf("failed to parse booking date: %v", err)
	}
	_, err = db.Exec(`INSERT INTO court_operating_hours (court_id, day_of_week, open_time, close_time, is_closed) VALUES ($1, $2, '08:00', '22:00', false)`, courtID, int(parsedDate.Weekday()))
	if err != nil {
		t.Fatalf("failed to insert operating hours: %v", err)
	}

	// Create router with admin DISABLED
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		PlatformFinanceAdminEnabled: false,
		JWTSecret:                   "test-secret",
		JWTExpiresInHours:           1,
		GeneralRateLimitPerMinute:   100,
		AuthRateLimitPerMinute:      100,
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
		"court_id":     courtID,
		"booking_date": bookingDate,
		"start_time":   "10:00",
		"end_time":     "11:00",
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/bookings", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected booking creation to succeed while finance admin is disabled, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Booking struct {
			ID string `json:"id"`
		} `json:"booking"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode booking response: %v", err)
	}
	if response.Booking.ID == "" {
		t.Fatal("booking response did not include booking id")
	}
	var snapshotCount int
	if err := db.QueryRow("SELECT count(*) FROM booking_fee_snapshots WHERE booking_id = $1", response.Booking.ID).Scan(&snapshotCount); err != nil {
		t.Fatalf("failed to query booking snapshot: %v", err)
	}
	if snapshotCount != 1 {
		t.Fatalf("expected exactly one booking fee snapshot, got %d", snapshotCount)
	}
}
