package bookings_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"lapangango-api/internal/bookings"
	"lapangango-api/internal/database"
)

func TestInsertOfflineBookingTx_SnapshotHarga(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set, skipping repository integration test")
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Initialize repository
	repo := bookings.NewRepository(pool)

	// Create minimal fixture in a transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	// Always rollback after test
	defer tx.Rollback(ctx)

	// Since we use UUIDs, let's let postgres generate them for sports, or we select one existing sport
	var sportID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM sports LIMIT 1`).Scan(&sportID)
	if err != nil {
		// Insert one if not exists
		err = tx.QueryRow(ctx, `INSERT INTO sports (name) VALUES ('Test Sport') RETURNING id::text`).Scan(&sportID)
		if err != nil {
			t.Fatalf("Failed to setup sport: %v", err)
		}
	}

	uniqueSuffix := time.Now().UnixNano()
	ownerEmail := fmt.Sprintf("owner-%d@test.local", uniqueSuffix)
	ownerPhone := fmt.Sprintf("08%d", uniqueSuffix)

	var ownerUserID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role) 
		VALUES ('Test Owner', $1, $2, 'hash', 'OWNER') 
		RETURNING id::text
	`, ownerEmail, ownerPhone).Scan(&ownerUserID)
	if err != nil {
		t.Fatalf("Failed to insert owner user: %v", err)
	}

	var ownerProfileID string
	businessName := fmt.Sprintf("Test Business %d", uniqueSuffix)
	err = tx.QueryRow(ctx, `
		INSERT INTO owner_profiles (user_id, business_name)
		VALUES ($1, $2)
		RETURNING id::text
	`, ownerUserID, businessName).Scan(&ownerProfileID)
	if err != nil {
		t.Fatalf("Failed to insert owner profile: %v", err)
	}

	var venueID string
	err = tx.QueryRow(ctx, `
		INSERT INTO venues (owner_profile_id, name, address, city, status) 
		VALUES ($1, 'Test Venue', 'Addr', 'City', 'ACTIVE') 
		RETURNING id::text
	`, ownerProfileID).Scan(&venueID)
	if err != nil {
		t.Fatalf("Failed to insert venue: %v", err)
	}

	var courtID string
	err = tx.QueryRow(ctx, `
		INSERT INTO courts (venue_id, sport_id, name, price_per_hour, status, location_type)
		VALUES ($1, $2, 'Test Court', 150000, 'ACTIVE', 'INDOOR')
		RETURNING id::text
	`, venueID, sportID).Scan(&courtID)
	if err != nil {
		t.Fatalf("Failed to insert court: %v", err)
	}

	t.Run("Case 1: Offline booking tanpa override", func(t *testing.T) {
		// Run in nested transaction or savepoint to isolate tests
		testTx, err := tx.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin test transaction: %v", err)
		}
		defer testTx.Rollback(ctx)

		params := bookings.CreateOfflineBookingParams{
			VenueID:             venueID,
			CourtID:             courtID,
			Date:                time.Now().Format("2006-01-02"),
			StartTime:           "10:00",
			EndTime:             "12:00",
			SystemPrice:         150000,
			FinalPrice:          150000,
			Status:              "PAID",
			OwnerUserID:         ownerUserID,
			CreatedByUserID:     ownerUserID,
			CustomerName:        "Test Customer",
			CustomerPhone:       func(s string) *string { return &s }("0812345678"),
			CustomerEmail:       nil,
			Note:                nil,
			PriceOverrideReason: nil,
		}

		b, err := repo.InsertOfflineBookingTx(ctx, testTx, params)
		if err != nil {
			t.Fatalf("Failed to insert offline booking: %v", err)
		}

		// Verify returned booking object
		if b.OriginalPrice == nil || *b.OriginalPrice != 150000 {
			t.Errorf("Expected b.OriginalPrice to be 150000, got %v", b.OriginalPrice)
		}
		if b.DiscountAmount != 0 {
			t.Errorf("Expected b.DiscountAmount to be 0, got %f", b.DiscountAmount)
		}
		if b.FinalPrice == nil || *b.FinalPrice != 150000 {
			t.Errorf("Expected b.FinalPrice to be 150000, got %v", b.FinalPrice)
		}
		if b.TotalPrice != 150000 {
			t.Errorf("Expected b.TotalPrice to be 150000, got %f", b.TotalPrice)
		}

		// Verify database table bookings
		var dbOrig *float64
		var dbDisc float64
		var dbFinal *float64
		var dbTotal float64
		err = testTx.QueryRow(ctx, `SELECT original_price, discount_amount, final_price, total_price FROM bookings WHERE id = $1`, b.ID).Scan(&dbOrig, &dbDisc, &dbFinal, &dbTotal)
		if err != nil {
			t.Fatalf("Failed to query bookings: %v", err)
		}
		if dbOrig == nil || *dbOrig != 150000 {
			t.Errorf("DB original_price expected 150000, got %v", dbOrig)
		}
		if dbDisc != 0 {
			t.Errorf("DB discount_amount expected 0, got %v", dbDisc)
		}
		if dbFinal == nil || *dbFinal != 150000 {
			t.Errorf("DB final_price expected 150000, got %v", dbFinal)
		}
		if dbTotal != 150000 {
			t.Errorf("DB total_price expected 150000, got %v", dbTotal)
		}

		// Verify offline_booking_customers
		var obcSys float64
		var obcFinal float64
		var obcReason *string
		err = testTx.QueryRow(ctx, `SELECT system_price, final_price, price_override_reason FROM offline_booking_customers WHERE booking_id = $1`, b.ID).Scan(&obcSys, &obcFinal, &obcReason)
		if err != nil {
			t.Fatalf("Failed to query offline_booking_customers: %v", err)
		}
		if obcSys != 150000 {
			t.Errorf("OBC system_price expected 150000, got %v", obcSys)
		}
		if obcFinal != 150000 {
			t.Errorf("OBC final_price expected 150000, got %v", obcFinal)
		}
		if obcReason != nil {
			t.Errorf("OBC price_override_reason expected nil, got %v", *obcReason)
		}
	})

	t.Run("Case 2: Offline booking dengan override diskon", func(t *testing.T) {
		testTx, err := tx.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin test transaction: %v", err)
		}
		defer testTx.Rollback(ctx)

		params := bookings.CreateOfflineBookingParams{
			VenueID:             venueID,
			CourtID:             courtID,
			Date:                time.Now().Format("2006-01-02"),
			StartTime:           "10:00",
			EndTime:             "12:00",
			SystemPrice:         150000,
			FinalPrice:          100000,
			Status:              "PAID",
			OwnerUserID:         ownerUserID,
			CreatedByUserID:     ownerUserID,
			CustomerName:        "Test Customer",
			CustomerPhone:       func(s string) *string { return &s }("0812345678"),
			CustomerEmail:       nil,
			Note:                nil,
			PriceOverrideReason: func(s string) *string { return &s }("Promo walk-in"),
		}

		b, err := repo.InsertOfflineBookingTx(ctx, testTx, params)
		if err != nil {
			t.Fatalf("Failed to insert offline booking: %v", err)
		}

		// Verify database table bookings
		var dbOrig *float64
		var dbDisc float64
		var dbFinal *float64
		var dbTotal float64
		err = testTx.QueryRow(ctx, `SELECT original_price, discount_amount, final_price, total_price FROM bookings WHERE id = $1`, b.ID).Scan(&dbOrig, &dbDisc, &dbFinal, &dbTotal)
		if err != nil {
			t.Fatalf("Failed to query bookings: %v", err)
		}
		if dbOrig == nil || *dbOrig != 150000 {
			t.Errorf("DB original_price expected 150000, got %v", dbOrig)
		}
		if dbDisc != 50000 {
			t.Errorf("DB discount_amount expected 50000, got %v", dbDisc)
		}
		if dbFinal == nil || *dbFinal != 100000 {
			t.Errorf("DB final_price expected 100000, got %v", dbFinal)
		}
		if dbTotal != 100000 {
			t.Errorf("DB total_price expected 100000, got %v", dbTotal)
		}

		// Verify offline_booking_customers
		var obcSys float64
		var obcFinal float64
		var obcReason *string
		err = testTx.QueryRow(ctx, `SELECT system_price, final_price, price_override_reason FROM offline_booking_customers WHERE booking_id = $1`, b.ID).Scan(&obcSys, &obcFinal, &obcReason)
		if err != nil {
			t.Fatalf("Failed to query offline_booking_customers: %v", err)
		}
		if obcSys != 150000 {
			t.Errorf("OBC system_price expected 150000, got %v", obcSys)
		}
		if obcFinal != 100000 {
			t.Errorf("OBC final_price expected 100000, got %v", obcFinal)
		}
		if obcReason == nil || *obcReason != "Promo walk-in" {
			t.Errorf("OBC price_override_reason expected Promo walk-in")
		}

		// Verify owner_finance_transactions
		var finAmount float64
		var finSource, finType, finCategory string
		err = testTx.QueryRow(ctx, `SELECT amount, source, type, category FROM owner_finance_transactions WHERE booking_id = $1`, b.ID).Scan(&finAmount, &finSource, &finType, &finCategory)
		if err != nil {
			t.Fatalf("Failed to query owner_finance_transactions: %v", err)
		}
		if finAmount != 100000 {
			t.Errorf("Finance amount expected 100000, got %v", finAmount)
		}
		if finSource != "BOOKING" {
			t.Errorf("Finance source expected BOOKING, got %v", finSource)
		}
		if finType != "INCOME" {
			t.Errorf("Finance type expected INCOME, got %v", finType)
		}
		if finCategory != "BOOKING_PAYMENT" {
			t.Errorf("Finance category expected BOOKING_PAYMENT, got %v", finCategory)
		}
	})
}
