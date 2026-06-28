package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureBookingSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var bookingsExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.bookings') IS NOT NULL`).Scan(&bookingsExists); err != nil {
		return err
	}
	if !bookingsExists {
		return nil
	}

	statements := []string{
		`ALTER TABLE bookings ADD COLUMN IF NOT EXISTS payment_reference VARCHAR(255)`,
		`ALTER TABLE bookings ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`,
		`ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_status_check`,
		`ALTER TABLE bookings ADD CONSTRAINT bookings_status_check CHECK (status IN ('PENDING_PAYMENT', 'WAITING_VERIFICATION', 'CONFIRMED', 'PAID', 'CANCELLED', 'COMPLETED'))`,
		`UPDATE bookings SET expires_at = created_at + interval '30 minutes' WHERE status = 'PENDING_PAYMENT' AND expires_at IS NULL`,
	}

	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return err
		}
	}

	return nil
}

func RunMigrations(dbURL string) error {
	// Try to find the migrations directory
	paths := []string{
		"../../db/migrations",
		"./db/migrations",
		"db/migrations",
	}

	var migrationsPath string
	for _, p := range paths {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			migrationsPath = "file://" + p
			break
		}
	}

	if migrationsPath == "" {
		return errors.New("migrations directory not found")
	}

	log.Printf("Running migrations from %s", migrationsPath)

	m, err := migrate.New(migrationsPath, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
