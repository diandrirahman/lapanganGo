package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/config"
	"lapangango-api/internal/database"

	"github.com/jackc/pgx/v5"
)

func cleanupDemoData(ctx context.Context, tx pgx.Tx) error {
	queries := []string{
		`DELETE FROM open_match_participants WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test')`,
		`DELETE FROM open_matches WHERE booking_id IN (SELECT id FROM bookings WHERE customer_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test')) OR booking_id IN (SELECT id FROM bookings WHERE court_id IN (SELECT id FROM courts WHERE venue_id IN (SELECT id FROM venues WHERE owner_profile_id IN (SELECT id FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test')))))`,
		`DELETE FROM bookings WHERE customer_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test') OR court_id IN (SELECT id FROM courts WHERE venue_id IN (SELECT id FROM venues WHERE owner_profile_id IN (SELECT id FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test'))))`,
		`DELETE FROM court_blocked_slots WHERE court_id IN (SELECT id FROM courts WHERE venue_id IN (SELECT id FROM venues WHERE owner_profile_id IN (SELECT id FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test'))))`,
		`DELETE FROM court_operating_hours WHERE court_id IN (SELECT id FROM courts WHERE venue_id IN (SELECT id FROM venues WHERE owner_profile_id IN (SELECT id FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test'))))`,
		`DELETE FROM courts WHERE venue_id IN (SELECT id FROM venues WHERE owner_profile_id IN (SELECT id FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test')))`,
		`DELETE FROM venue_photos WHERE venue_id IN (SELECT id FROM venues WHERE owner_profile_id IN (SELECT id FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test')))`,
		`DELETE FROM venues WHERE owner_profile_id IN (SELECT id FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test'))`,
		`DELETE FROM owner_profiles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'demo.%@lapangango.test')`,
		`DELETE FROM users WHERE email LIKE 'demo.%@lapangango.test'`,
	}
	for _, q := range queries {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	cleanupFlag := flag.Bool("cleanup", false, "Remove all demo data and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("invalid application configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dbPool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Error: failed to establish database connection")
	}
	defer dbPool.Close()

	// 1. Cutover preflight check
	var cutoverActive bool
	err = dbPool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM platform_finance_cutovers WHERE id = 1)").Scan(&cutoverActive)
	if err != nil {
		log.Fatal("Error: unable to verify whether seeding is permitted")
	}
	if cutoverActive {
		log.Fatal("Error: Seeding is not allowed because platform finance cutover is active")
	}

	tokenService := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpiresInHours)

	// Mulai Transaksi
	var cancelSeed context.CancelFunc
	ctx, cancelSeed = context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelSeed()

	tx, err := dbPool.Begin(ctx)
	if err != nil {
		log.Fatal("Failed to begin transaction:", err)
	}
	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	fmt.Println("--- Starting Demo Cleanup ---")
	if err := cleanupDemoData(ctx, tx); err != nil {
		log.Fatal("Failed to clean up demo data:", err)
	}

	if *cleanupFlag {
		if err := tx.Commit(ctx); err != nil {
			log.Fatal("Failed to commit cleanup:", err)
		}
		fmt.Println("Cleanup completed successfully.")
		os.Exit(0)
	}

	fmt.Println("--- Starting Demo Seed ---")

	// Set deterministic seed
	rand.Seed(42)

	// 1. Sports
	sportsList := []string{"Futsal", "Badminton", "Mini Soccer", "Basket", "Tenis", "Voli"}
	sportIDs := make([]string, len(sportsList))
	for i, sport := range sportsList {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO sports (name) VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text
		`, sport).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed sport:", err)
		}
		sportIDs[i] = id
	}

	// 2. Users (2 Owners, 10 Hosts, 20 Participants)
	// We use the hash for "password123" so that users can actually login via UI
	// hash generated from bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	hashedPassword := "$2a$10$9pgMLWYnzz4nCP/Xa5BUyuwy1Zu1Q2xGQKqcx8LLWt3RCv5Cd0GVy" // Hash for 'password123'

	ownerIDs := []string{}
	for i := 1; i <= 2; i++ {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO users (name, email, password_hash, role)
			VALUES ($1, $2, $3, 'OWNER')
			RETURNING id::text
		`, fmt.Sprintf("Demo Owner %d", i), fmt.Sprintf("demo.owner%02d@lapangango.test", i), hashedPassword).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed owner:", err)
		}
		ownerIDs = append(ownerIDs, id)
	}

	hostIDs := []string{}
	for i := 1; i <= 10; i++ {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO users (name, email, password_hash, role)
			VALUES ($1, $2, $3, 'CUSTOMER')
			RETURNING id::text
		`, fmt.Sprintf("Demo Host %d", i), fmt.Sprintf("demo.host%02d@lapangango.test", i), hashedPassword).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed host:", err)
		}
		hostIDs = append(hostIDs, id)
	}

	partIDs := []string{}
	for i := 1; i <= 20; i++ {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO users (name, email, password_hash, role)
			VALUES ($1, $2, $3, 'CUSTOMER')
			RETURNING id::text
		`, fmt.Sprintf("Demo Participant %d", i), fmt.Sprintf("demo.customer%02d@lapangango.test", i), hashedPassword).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed participant:", err)
		}
		partIDs = append(partIDs, id)
	}

	// 3. Owner Profiles
	profileNames := []string{"Demo Arena Group", "Demo Sport Hub"}
	profileIDs := []string{}
	for i, ownerID := range ownerIDs {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO owner_profiles (user_id, business_name)
			VALUES ($1, $2)
			RETURNING id::text
		`, ownerID, profileNames[i]).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed owner profile:", err)
		}
		// Try to update verification_status if it exists, ignore if not
		tx.Exec(ctx, `SAVEPOINT sp_owner_profile`)
		_, err = tx.Exec(ctx, `UPDATE owner_profiles SET verification_status = 'APPROVED' WHERE id = $1`, id)
		if err != nil {
			tx.Exec(ctx, `ROLLBACK TO SAVEPOINT sp_owner_profile`)
		} else {
			tx.Exec(ctx, `RELEASE SAVEPOINT sp_owner_profile`)
		}
		profileIDs = append(profileIDs, id)
	}

	// 4. Venues
	venueNames := []string{
		"Demo GBK Alpha Field", "Demo Smash Arena Bintaro", "Demo Kuningan Court",
		"Demo Senayan Futsal Center", "Demo BSD Sport Hall", "Demo Depok Badminton House",
		"Demo Bekasi Mini Soccer Park", "Demo Tebet Tennis Court", "Demo Kemang Sports Club",
		"Demo Kelapa Gading Arena", "Demo Pluit Sport Hub",
	}
	cities := []string{"Jakarta Selatan", "Jakarta Pusat", "Tangerang Selatan", "Depok", "Bekasi", "Jakarta Utara"}
	venueIDs := []string{}

	for i, vName := range venueNames {
		var id string
		ownerProfileID := profileIDs[i%len(profileIDs)]
		city := cities[i%len(cities)]
		err = tx.QueryRow(ctx, `
			INSERT INTO venues (owner_profile_id, name, description, address, district, city, province, status)
			VALUES ($1, $2, 'Fasilitas demo premium terjangkau', $3, 'Kecamatan Demo', $4, 'Provinsi Demo', 'ACTIVE')
			RETURNING id::text
		`, ownerProfileID, vName, fmt.Sprintf("Jalan Raya Demo No. %d", i+1), city).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed venue:", err)
		}
		venueIDs = append(venueIDs, id)

		// Seed Venue Photos (2 photos per venue)
		photoURLs := []string{
			"https://images.unsplash.com/photo-1518605368461-1ee790bbd105?q=80&w=1200&auto=format&fit=crop",
			"https://images.unsplash.com/photo-1595435934249-5df7ed86e1c0?q=80&w=1200&auto=format&fit=crop",
		}
		for pIdx, pURL := range photoURLs {
			_, err = tx.Exec(ctx, `
				INSERT INTO venue_photos (venue_id, image_url, alt_text, sort_order, is_primary)
				VALUES ($1, $2, $3, $4, $5)
			`, id, pURL, fmt.Sprintf("Foto %d - %s", pIdx+1, vName), pIdx, pIdx == 0)
			if err != nil {
				// Don't fail the whole seeding if photo fails (just in case migration isn't there yet, though it should be)
				log.Printf("Warning: Failed to seed venue photo for %s: %v\n", id, err)
			}
		}
	}

	// 5. Courts & 6. Operating Hours
	courtIDs := []string{}
	prices := []int{75000, 100000, 150000, 200000, 300000}
	locationTypes := []string{"INDOOR", "OUTDOOR"}

	for _, vID := range venueIDs {
		numCourts := rand.Intn(4) + 2 // 2 to 5
		for c := 1; c <= numCourts; c++ {
			var id string
			sportID := sportIDs[rand.Intn(len(sportIDs))]
			price := prices[rand.Intn(len(prices))]
			locType := locationTypes[rand.Intn(len(locationTypes))]
			err = tx.QueryRow(ctx, `
				INSERT INTO courts (venue_id, sport_id, name, location_type, price_per_hour, status)
				VALUES ($1, $2, $3, $4, $5, 'ACTIVE')
				RETURNING id::text
			`, vID, sportID, fmt.Sprintf("Court Demo %d", c), locType, price).Scan(&id)
			if err != nil {
				log.Fatal("Failed to seed court:", err)
			}
			courtIDs = append(courtIDs, id)

			// Operating Hours
			for day := 0; day <= 6; day++ {
				openTime, closeTime := "08:00", "22:00"
				isClosed := false
				if rand.Float32() < 0.1 && (day == 1 || day == 2) {
					isClosed = true
				} else if day == 0 || day == 6 {
					openTime, closeTime = "07:00", "23:00"
				}

				tx.Exec(ctx, `SAVEPOINT sp_operating_hours`)
				_, err = tx.Exec(ctx, `
					INSERT INTO court_operating_hours (court_id, day_of_week, open_time, close_time, is_closed)
					VALUES ($1, $2, $3, $4, $5)
				`, id, day, openTime, closeTime, isClosed)
				if err != nil {
					tx.Exec(ctx, `ROLLBACK TO SAVEPOINT sp_operating_hours`)
					// Fallback if is_closed doesn't exist
					_, err = tx.Exec(ctx, `
						INSERT INTO court_operating_hours (court_id, day_of_week, open_time, close_time)
						VALUES ($1, $2, $3, $4)
					`, id, day, openTime, closeTime)
					if err != nil {
						log.Fatal("Failed to seed operating hours:", err)
					}
				} else {
					tx.Exec(ctx, `RELEASE SAVEPOINT sp_operating_hours`)
				}
			}
		}
	}

	// 7. Blocked Slots
	numBlocked := rand.Intn(11) + 15 // 15 to 25
	for i := 0; i < numBlocked; i++ {
		courtID := courtIDs[rand.Intn(len(courtIDs))]
		dayOffset := rand.Intn(7)      // 0 to 6 days ahead
		startHour := rand.Intn(6) + 10 // 10 to 15

		now := time.Now()
		startAt := time.Date(now.Year(), now.Month(), now.Day()+dayOffset, startHour, 0, 0, 0, now.Location())
		endAt := startAt.Add(time.Duration(rand.Intn(2)+1) * time.Hour) // 1 to 2 hours

		reasons := []string{"Maintenance", "Private event", "Cleaning", "Tournament prep"}
		_, err = tx.Exec(ctx, `
			INSERT INTO court_blocked_slots (court_id, start_at, end_at, reason)
			VALUES ($1, $2, $3, $4)
		`, courtID, startAt, endAt, reasons[rand.Intn(len(reasons))])
		if err != nil {
			log.Fatal("Failed to seed blocked slots:", err)
		}
	}

	// 8. Bookings
	numBookings := rand.Intn(41) + 60 // 60 to 100
	bookingStatuses := []string{"CONFIRMED", "PENDING_PAYMENT", "CANCELLED"}
	bookingWeights := []int{55, 30, 15}

	type BookingInfo struct {
		id         string
		customerID string
	}
	bookingIDs := []BookingInfo{}

	for i := 0; i < numBookings; i++ {
		customerID := hostIDs[rand.Intn(len(hostIDs))]
		courtID := courtIDs[rand.Intn(len(courtIDs))]

		// Status selection based on weights
		r := rand.Intn(100)
		var status string
		if r < bookingWeights[0] {
			status = bookingStatuses[0]
		} else if r < bookingWeights[0]+bookingWeights[1] {
			status = bookingStatuses[1]
		} else {
			status = bookingStatuses[2]
		}

		dayOffset := rand.Intn(14)
		if rand.Float32() < 0.1 {
			dayOffset = -rand.Intn(3) // history
		}

		startHour := rand.Intn(10) + 9 // 9 to 18
		dur := rand.Intn(2) + 1        // 1 to 2 hours

		now := time.Now()
		bookDate := time.Date(now.Year(), now.Month(), now.Day()+dayOffset, 0, 0, 0, 0, now.Location())
		startStr := fmt.Sprintf("%02d:00", startHour)
		endStr := fmt.Sprintf("%02d:00", startHour+dur)
		totalPrice := 150000 * dur // rough estimate

		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO bookings (customer_id, court_id, booking_date, start_time, end_time, total_price, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id::text
		`, customerID, courtID, bookDate, startStr, endStr, totalPrice, status).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed booking:", err)
		}
		if status == "CONFIRMED" {
			bookingIDs = append(bookingIDs, struct {
				id         string
				customerID string
			}{id, customerID})
		}
	}

	// 9. Open Matches & 10. Participants
	numMatches := rand.Intn(11) + 20 // 20 to 30
	if numMatches > len(bookingIDs) {
		numMatches = len(bookingIDs)
	}

	titles := []string{
		"Demo FC Jakarta Casuals", "Demo Smash Yuk", "Demo Hoops Weekend",
		"Demo Futsal Santai Senayan", "Demo Badminton After Office", "Demo Mini Soccer Fun Match",
		"Demo Basket Sunday Run", "Demo Tennis Beginner Club",
	}
	levels := []string{"Beginner / Fun", "Intermediate", "Advanced", "All Levels"}
	pricesPP := []int{20000, 35000, 45000, 50000, 75000, 100000}

	totalJoined := 0
	totalParticipantRecords := 0

	statusCounts := map[string]int{"OPEN": 0, "FULL": 0, "CANCELLED": 0}

	for i := 0; i < numMatches; i++ {
		bookingInfo := bookingIDs[i]
		bookingID := bookingInfo.id
		hostID := bookingInfo.customerID
		title := titles[rand.Intn(len(titles))]
		level := levels[rand.Intn(len(levels))]
		pricePP := pricesPP[rand.Intn(len(pricesPP))]
		maxPlayers := rand.Intn(6) + 6 // 6 to 11

		if title == "Demo Smash Yuk" || title == "Demo Badminton After Office" || title == "Demo Tennis Beginner Club" {
			maxPlayers = rand.Intn(3) + 2
		} else if title == "Demo Mini Soccer Fun Match" {
			maxPlayers = rand.Intn(7) + 10
		}

		status := "OPEN"
		rStatus := rand.Float32()
		if rStatus < 0.2 {
			status = "FULL"
		} else if rStatus < 0.3 {
			status = "CANCELLED"
		}

		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO open_matches (booking_id, host_user_id, title, description, level, max_players, price_per_player, status)
			VALUES ($1, $2, $3, 'Ayo join mabar demo yang seru ini!', $4, $5, $6, $7)
			RETURNING id::text
		`, bookingID, hostID, title, level, maxPlayers, pricePP, status).Scan(&id)
		if err != nil {
			log.Fatal("Failed to seed open match:", err)
		}

		statusCounts[status]++

		// Insert Participants for this match
		targetRecords := 0
		if status == "FULL" {
			targetRecords = maxPlayers
		} else if status == "OPEN" {
			targetRecords = rand.Intn(maxPlayers) // 0 to max_players - 1
			// boost to hit 60 records minimum across all matches
			recordsNeeded := 60 - totalParticipantRecords
			remainingMatches := numMatches - i - 1
			if recordsNeeded > 0 && remainingMatches > 0 {
				avgNeeded := recordsNeeded / remainingMatches
				if avgNeeded > targetRecords {
					targetRecords = avgNeeded
				}
				if targetRecords >= maxPlayers {
					targetRecords = maxPlayers - 1
				}
			}
		} else {
			targetRecords = rand.Intn(maxPlayers)
		}

		// Force enough records on the very last match if we are still short
		if i == numMatches-1 && totalParticipantRecords+targetRecords < 60 {
			targetRecords = 60 - totalParticipantRecords
			if targetRecords > len(partIDs) {
				targetRecords = len(partIDs)
			}
			if targetRecords >= maxPlayers && status == "OPEN" {
				// Must not violate OPEN condition
				targetRecords = maxPlayers - 1
			}
		}

		shuffledParts := make([]string, len(partIDs))
		copy(shuffledParts, partIDs)
		rand.Shuffle(len(shuffledParts), func(i, j int) {
			shuffledParts[i], shuffledParts[j] = shuffledParts[j], shuffledParts[i]
		})

		joinedCountForMatch := 0
		for j := 0; j < targetRecords && j < len(shuffledParts); j++ {
			partID := shuffledParts[j]
			partStatus := "JOINED"

			if status == "FULL" {
				partStatus = "JOINED"
			} else if status == "CANCELLED" {
				if rand.Float32() < 0.5 {
					partStatus = "CANCELLED"
				}
			} else {
				// OPEN
				// If we already hit max_players-1 JOINED, force CANCELLED to not become FULL
				if joinedCountForMatch >= maxPlayers-1 {
					partStatus = "CANCELLED"
				} else {
					if rand.Float32() < 0.1 {
						partStatus = "CANCELLED"
					}
				}
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO open_match_participants (open_match_id, user_id, status)
				VALUES ($1, $2, $3)
			`, id, partID, partStatus)
			if err != nil {
				log.Fatal("Failed to seed participant:", err)
			}

			totalParticipantRecords++
			if partStatus == "JOINED" {
				joinedCountForMatch++
				totalJoined++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatal("Failed to commit transaction:", err)
	}

	// Generate Tokens
	demoOwnerToken, _ := tokenService.Generate(auth.UserResponse{ID: ownerIDs[0], Email: "demo.owner01@lapangango.test", Role: "OWNER"})
	demoHostToken, _ := tokenService.Generate(auth.UserResponse{ID: hostIDs[0], Email: "demo.host01@lapangango.test", Role: "CUSTOMER"})
	demoCustomerToken, _ := tokenService.Generate(auth.UserResponse{ID: partIDs[0], Email: "demo.customer01@lapangango.test", Role: "CUSTOMER"})

	fmt.Println("\nDemo seed completed")
	fmt.Printf("Owners: %d\n", len(ownerIDs))
	fmt.Printf("Customers: %d\n", len(hostIDs)+len(partIDs))
	fmt.Printf("Venues: %d\n", len(venueIDs))
	fmt.Printf("Courts: %d\n", len(courtIDs))
	fmt.Printf("Blocked slots: %d\n", numBlocked)
	fmt.Printf("Bookings: %d\n", numBookings)
	fmt.Printf("Open matches by status:\n")
	fmt.Printf("  OPEN: %d\n", statusCounts["OPEN"])
	fmt.Printf("  FULL: %d\n", statusCounts["FULL"])
	fmt.Printf("  CANCELLED: %d\n", statusCounts["CANCELLED"])
	fmt.Printf("Participant records: %d\n", totalParticipantRecords)
	fmt.Printf("Joined participants: %d\n", totalJoined)

	fmt.Println("\n--- DEMO TOKENS ---")
	fmt.Printf("DEMO_OWNER_TOKEN=%s\n", demoOwnerToken)
	fmt.Printf("DEMO_HOST_TOKEN=%s\n", demoHostToken)
	fmt.Printf("DEMO_CUSTOMER_TOKEN=%s\n", demoCustomerToken)
	fmt.Println("-------------------")
}
