package main

import (
	"context"
	"fmt"
	"os"

	"lapangango-api/internal/database"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "Migration startup failed: database_url_required")
		os.Exit(1)
	}

	if err := database.RunMigrations(databaseURL); err != nil {
		fmt.Fprintln(os.Stderr, "Migration startup failed: migration_failed")
		os.Exit(1)
	}

	pool, err := database.NewPostgresPool(context.Background(), databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Migration startup failed: database_setup_failed")
		os.Exit(1)
	}
	defer pool.Close()

	if err := database.EnsureBookingSchema(context.Background(), pool); err != nil {
		fmt.Fprintln(os.Stderr, "Migration startup failed: schema_setup_failed")
		os.Exit(1)
	}

	fmt.Println("Database migrations completed successfully.")
}
