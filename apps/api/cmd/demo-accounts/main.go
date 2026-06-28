package main

import (
	"context"
	"fmt"
	"log"

	"lapangango-api/internal/config"
	"lapangango-api/internal/database"

	"golang.org/x/crypto/bcrypt"
)

type demoAccount struct {
	Name         string
	Email        string
	Phone        string
	Password     string
	Role         string
	BusinessName string
}

func main() {
	cfg := config.Load()
	ctx := context.Background()

	dbPool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer dbPool.Close()

	accounts := []demoAccount{
		{
			Name:         "Demo Owner Stable",
			Email:        "demo.owner@lapangango.test",
			Phone:        "089900000001",
			Password:     "DemoPassword123!",
			Role:         "OWNER",
			BusinessName: "Demo Owner Business",
		},
		{
			Name:     "Demo Customer Stable",
			Email:    "demo.customer@lapangango.test",
			Phone:    "089900000002",
			Password: "DemoPassword123!",
			Role:     "CUSTOMER",
		},
	}

	tx, err := dbPool.Begin(ctx)
	if err != nil {
		log.Fatal("Failed to begin transaction:", err)
	}
	defer tx.Rollback(ctx)

	fmt.Println("--- Upserting non-destructive demo accounts ---")
	for _, account := range accounts {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(account.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal("Failed to hash password:", err)
		}

		var userID string
		err = tx.QueryRow(ctx, `
			INSERT INTO users (name, email, phone, password_hash, role, status)
			VALUES ($1, $2, $3, $4, $5, 'ACTIVE')
			ON CONFLICT (email) DO UPDATE SET
				name = EXCLUDED.name,
				phone = EXCLUDED.phone,
				password_hash = EXCLUDED.password_hash,
				role = EXCLUDED.role,
				status = EXCLUDED.status,
				updated_at = now()
			RETURNING id::text
		`, account.Name, account.Email, account.Phone, string(hashedPassword), account.Role).Scan(&userID)
		if err != nil {
			log.Fatalf("Failed to upsert user %s: %v", account.Email, err)
		}

		if account.Role == "OWNER" {
			_, err = tx.Exec(ctx, `
				INSERT INTO owner_profiles (
					user_id,
					business_name,
					identity_number,
					bank_name,
					bank_account_number,
					bank_account_name,
					verification_status,
					verified_at
				)
				VALUES ($1, $2, 'DEMO-OWNER-001', 'BCA', '1234567890', $2, 'APPROVED', now())
				ON CONFLICT (user_id) DO UPDATE SET
					business_name = EXCLUDED.business_name,
					identity_number = EXCLUDED.identity_number,
					bank_name = EXCLUDED.bank_name,
					bank_account_number = EXCLUDED.bank_account_number,
					bank_account_name = EXCLUDED.bank_account_name,
					verification_status = EXCLUDED.verification_status,
					verified_at = EXCLUDED.verified_at,
					updated_at = now()
			`, userID, account.BusinessName)
			if err != nil {
				log.Fatalf("Failed to upsert owner profile for %s: %v", account.Email, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatal("Failed to commit transaction:", err)
	}

	fmt.Println("\nDemo accounts ready. Local demo only:")
	fmt.Println("Role     Email                         Password")
	for _, account := range accounts {
		fmt.Printf("%-8s %-29s %s\n", account.Role, account.Email, account.Password)
	}
	fmt.Println("\nNo venues, courts, bookings, mabar, or blocked slots were deleted or modified.")
}
