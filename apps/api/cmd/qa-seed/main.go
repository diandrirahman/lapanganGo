package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/config"
	"lapangango-api/internal/database"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	dbPool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer dbPool.Close()

	tokenService := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpiresInHours)

	fmt.Println("--- Seeding Data for Mabar E2E QA ---")

	// 1. Create Host User
	var hostID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ('QA Host', 'host_qa@example.com', 'dummyhash', 'CUSTOMER')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&hostID)
	if err != nil {
		log.Fatal("Host insert failed:", err)
	}
	hostToken, _ := tokenService.Generate(auth.UserResponse{ID: hostID, Email: "host_qa@example.com", Role: "CUSTOMER"})

	// 2. Create Participant User
	var partID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ('QA Participant', 'part_qa@example.com', 'dummyhash', 'CUSTOMER')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&partID)
	if err != nil {
		log.Fatal("Part insert failed:", err)
	}
	partToken, _ := tokenService.Generate(auth.UserResponse{ID: partID, Email: "part_qa@example.com", Role: "CUSTOMER"})

	// 2b. Create Participant 2 User
	var part2ID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ('QA Participant 2', 'part2_qa@example.com', 'dummyhash', 'CUSTOMER')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&part2ID)
	if err != nil {
		log.Fatal("Part 2 insert failed:", err)
	}
	part2Token, _ := tokenService.Generate(auth.UserResponse{ID: part2ID, Email: "part2_qa@example.com", Role: "CUSTOMER"})

	// 3. Create Owner, Profile, Venue, Court, Sport
	var sportID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO sports (name) VALUES ('QA Futsal') 
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name 
		RETURNING id::text
	`).Scan(&sportID)
	if err != nil {
		log.Fatal("Sport insert failed:", err)
	}

	var ownerID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ('QA Owner', 'owner_qa@example.com', 'dummyhash', 'OWNER')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&ownerID)
	if err != nil {
		log.Fatal("Owner insert failed:", err)
	}

	var ownerProfileID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO owner_profiles (user_id, business_name) 
		VALUES ($1, 'QA Arena Biz') 
		ON CONFLICT (user_id) DO UPDATE SET business_name = EXCLUDED.business_name
		RETURNING id::text
	`, ownerID).Scan(&ownerProfileID)
	if err != nil {
		log.Fatal("Owner Profile insert failed:", err)
	}

	var venueID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO venues (owner_profile_id, name, address, city) 
		VALUES ($1, 'QA Arena', 'Jl. Testing', 'Jakarta') 
		ON CONFLICT DO NOTHING
		RETURNING id::text
	`, ownerProfileID).Scan(&venueID)
	if err != nil {
		dbPool.QueryRow(ctx, `SELECT id::text FROM venues WHERE owner_profile_id = $1 AND name = 'QA Arena'`, ownerProfileID).Scan(&venueID)
	}

	var courtID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO courts (venue_id, sport_id, name, location_type, price_per_hour) 
		VALUES ($1, $2, 'Court QA', 'INDOOR', 100000) 
		ON CONFLICT DO NOTHING
		RETURNING id::text
	`, venueID, sportID).Scan(&courtID)
	if err != nil {
		dbPool.QueryRow(ctx, `SELECT id::text FROM courts WHERE venue_id = $1 AND name = 'Court QA'`, venueID).Scan(&courtID)
	}

	// 4. Create court_operating_hours
	dbPool.Exec(ctx, `
		INSERT INTO court_operating_hours (court_id, day_of_week, open_time, close_time) 
		VALUES ($1, 1, '10:00', '22:00'), ($1, 2, '10:00', '22:00'), ($1, 3, '10:00', '22:00'),
		       ($1, 4, '10:00', '22:00'), ($1, 5, '10:00', '22:00'), ($1, 6, '10:00', '22:00'), ($1, 0, '10:00', '22:00')
		ON CONFLICT DO NOTHING
	`, courtID)

	// 5. Create Booking & Confirm it
	tomorrow := time.Now().Add(24 * time.Hour)
	var bookingID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO bookings (customer_id, court_id, booking_date, start_time, end_time, total_price, status)
		VALUES ($1, $2, $3, '18:00', '20:00', 200000, 'CONFIRMED')
		RETURNING id::text
	`, hostID, courtID, tomorrow).Scan(&bookingID)
	if err != nil {
		log.Fatal("Booking insert failed:", err)
	}

	// 6. Create PENDING_PAYMENT Booking
	var pendingBookingID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO bookings (customer_id, court_id, booking_date, start_time, end_time, total_price, status)
		VALUES ($1, $2, $3, '20:00', '22:00', 200000, 'PENDING_PAYMENT')
		RETURNING id::text
	`, hostID, courtID, tomorrow).Scan(&pendingBookingID)
	if err != nil {
		log.Fatal("Pending Booking insert failed:", err)
	}

	fmt.Println("\n--- QA Seeding Completed ---")
	fmt.Println("Set the following environment variables:")
	fmt.Println("$env:HOST_TOKEN=\"" + hostToken + "\"")
	fmt.Println("$env:PART_TOKEN=\"" + partToken + "\"")
	fmt.Println("$env:PART2_TOKEN=\"" + part2Token + "\"")
	fmt.Println("$env:BOOKING_ID=\"" + bookingID + "\"")
	fmt.Println("$env:PENDING_BOOKING_ID=\"" + pendingBookingID + "\"")
	fmt.Println("$env:BASE_URL=\"http://localhost:8080\"")
}
