package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"lapangango-api/internal/config"
	"lapangango-api/internal/database"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	email := os.Getenv("SUPERADMIN_EMAIL")
	password := os.Getenv("SUPERADMIN_PASSWORD")
	name := os.Getenv("SUPERADMIN_NAME")
	if name == "" {
		name = "Super Admin"
	}

	if email == "" || password == "" {
		log.Fatal("SUPERADMIN_EMAIL and SUPERADMIN_PASSWORD must be provided via environment variables")
	}

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx := context.Background()

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Check if user exists
	var existingID, existingRole string
	err = db.QueryRow(ctx, "SELECT id, role FROM users WHERE email = $1", email).Scan(&existingID, &existingRole)
	
	if err != nil {
		// User does not exist, insert
		if err.Error() == "no rows in result set" {
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				log.Fatalf("Failed to hash password: %v", err)
			}

			_, err = db.Exec(ctx, 
				"INSERT INTO users (name, email, password_hash, role, status) VALUES ($1, $2, $3, 'SUPER_ADMIN', 'ACTIVE')",
				name, email, string(hashedPassword))
			
			if err != nil {
				log.Fatalf("Failed to create superadmin: %v", err)
			}
			fmt.Println("Successfully created superadmin user.")
			return
		}
		log.Fatalf("Database error: %v", err)
	}

	// User exists
	if existingRole == "SUPER_ADMIN" {
		fmt.Println("Superadmin user already exists. Skipping.")
		return
	} else {
		log.Fatalf("User with email %s already exists but role is %s, not SUPER_ADMIN", email, existingRole)
	}
}
