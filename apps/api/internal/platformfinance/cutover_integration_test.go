package platformfinance

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/database"
)

var testDSN string

func fatalAfterCleanup(cleanup func() error, message string) {
	if err := cleanup(); err != nil {
		log.Fatalf("%s; disposable database cleanup also failed", message)
	}
	log.Fatal(message)
}

func TestMain(m *testing.M) {
	if os.Getenv("TEST_CUTOVER_DISPOSABLE") == "1" {
		baseDSN := os.Getenv("CUTOVER_TEST_DATABASE_URL")
		if baseDSN == "" {
			log.Fatal("TEST_CUTOVER_DISPOSABLE=1 is set, but CUTOVER_TEST_DATABASE_URL is empty")
		}

		// Create dedicated test database with UUID
		dbName := "lapangango_cutover_" + strings.ReplaceAll(uuid.New().String(), "-", "_")
		dsn, cleanup, err := createDisposableDB(baseDSN, dbName)
		if err != nil {
			log.Fatal("failed to create disposable database")
		}
		testDSN = dsn

		// Run all migrations to 21
		migInst, err := migrate.New(getMigrationsPath(), testDSN)
		if err != nil {
			fatalAfterCleanup(cleanup, "failed to create migrate instance")
		}
		err = migInst.Up()
		if err != nil && err.Error() != "no change" {
			migInst.Close()
			fatalAfterCleanup(cleanup, "failed to run migrations")
		}
		migInst.Close()

		code := m.Run()

		err = cleanup()
		if err != nil {
			log.Fatalf("failed to cleanup disposable DB: %v", err)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func createDisposableDB(baseDSN string, dbName string) (string, func() error, error) {
	u, err := url.Parse(baseDSN)
	if err != nil {
		return "", nil, err
	}

	u.Path = "/postgres"
	adminDSN := u.String()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		adminPool, err = pgxpool.New(ctx, baseDSN)
		if err != nil {
			return "", nil, err
		}
	}
	defer adminPool.Close()

	// Create database directly
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		return "", nil, err
	}

	u.Path = "/" + dbName
	tempDSN := u.String()

	cleanup := func() error {
		ctxClose, cancelClose := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelClose()

		adminPoolClose, errClose := pgxpool.New(ctxClose, adminDSN)
		if errClose != nil {
			adminPoolClose, errClose = pgxpool.New(ctxClose, baseDSN)
			if errClose != nil {
				return errClose
			}
		}
		defer adminPoolClose.Close()

		_, err = adminPoolClose.Exec(ctxClose, fmt.Sprintf(`
			SELECT pg_terminate_backend(pg_stat_activity.pid)
			FROM pg_stat_activity
			WHERE pg_stat_activity.datname = '%s'
			  AND pid <> pg_backend_pid();
		`, dbName))
		if err != nil {
			return err
		}

		_, err = adminPoolClose.Exec(ctxClose, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		if err != nil {
			return err
		}

		return nil
	}

	return tempDSN, cleanup, nil
}

func setupCutoverTestDB(t *testing.T) *pgxpool.Pool {
	if os.Getenv("TEST_CUTOVER_DISPOSABLE") != "1" {
		t.Skip("Skipping integration test; set TEST_CUTOVER_DISPOSABLE=1")
	}

	if testDSN == "" {
		t.Skip("testDSN not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, testDSN)
	require.NoError(t, err)

	return pool
}

func getMigrationsPath() string {
	paths := []string{
		"../../../../db/migrations",
		"../../../db/migrations",
		"../../db/migrations",
		"./db/migrations",
	}
	for _, p := range paths {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			return "file://" + p
		}
	}
	return ""
}

func cleanupAllTables(t *testing.T, pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		TRUNCATE TABLE
			open_match_participants,
			open_matches,
			booking_fee_snapshots,
			bookings,
			court_blocked_slots,
			court_operating_hours,
			courts,
			venues,
			owner_profiles,
			users,
			platform_finance_cutovers,
			platform_commercial_terms,
			owner_finance_transactions
		CASCADE;
	`)
	require.NoError(t, err)
}

func restoreTrigger(t *testing.T, pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION enforce_booking_snapshot_after_cutover()
		RETURNS TRIGGER
		LANGUAGE plpgsql
		SET search_path = public, pg_temp
		AS $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM booking_fee_snapshots
				WHERE booking_id = NEW.id
			) AND (
				current_setting('transaction_isolation') <> 'read committed'
				OR EXISTS (
					SELECT 1
					FROM platform_finance_cutovers
					WHERE id = 1
				)
			) THEN
				RAISE EXCEPTION USING
					ERRCODE = '23514',
					MESSAGE = 'booking snapshot is required after platform finance cutover',
					CONSTRAINT = 'booking_snapshot_required_after_cutover';
			END IF;
			RETURN NULL;
		END;
		$$;

		DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;
		CREATE CONSTRAINT TRIGGER booking_snapshot_required_after_cutover
		AFTER INSERT ON bookings
		DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW
		EXECUTE FUNCTION enforce_booking_snapshot_after_cutover();
	`)
	require.NoError(t, err)
}

func insertSuperAdminActor(t *testing.T, ctx context.Context, tx pgx.Tx, status string) string {
	userID := uuid.New().String()
	email := fmt.Sprintf("sa_%d@test.com", rand.Intn(1000000))
	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role, status, created_at, updated_at)
		VALUES ($1, 'Super Admin', $2, 'hash', 'SUPER_ADMIN', $3, now(), now())
	`, userID, email, status)
	require.NoError(t, err)
	return userID
}

func insertSuperAdminActorPool(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status string) string {
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	saID := insertSuperAdminActor(t, ctx, tx, status)
	err = tx.Commit(ctx)
	require.NoError(t, err)
	return saID
}

func insertCustomerActor(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	userID := uuid.New().String()
	email := fmt.Sprintf("cust_%d@test.com", rand.Intn(1000000))
	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role, status, created_at, updated_at)
		VALUES ($1, 'Customer User', $2, 'hash', 'CUSTOMER', 'ACTIVE', now(), now())
	`, userID, email)
	require.NoError(t, err)
	return userID
}

func insertDependencyFixtureNoBooking(t *testing.T, ctx context.Context, tx pgx.Tx) (userID, ownerID, venueID, courtID, termID string) {
	userID = uuid.New().String()
	email := fmt.Sprintf("user_%d@test.com", rand.Intn(1000000))
	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, status, created_at, updated_at)
		VALUES ($1, 'Test User', $2, 'hash', 'ACTIVE', now(), now())
	`, userID, email)
	require.NoError(t, err)

	ownerID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO owner_profiles (id, user_id, business_name, created_at, updated_at)
		VALUES ($1, $2, 'Test Business', now(), now())
	`, ownerID, userID)
	require.NoError(t, err)

	venueID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO venues (id, owner_profile_id, name, status, created_at, updated_at, address, city)
		VALUES ($1, $2, 'Test Venue', 'ACTIVE', now(), now(), 'Test Address', 'Test City')
	`, venueID, ownerID)
	require.NoError(t, err)

	var sportID string
	err = tx.QueryRow(ctx, "SELECT id FROM sports LIMIT 1").Scan(&sportID)
	require.NoError(t, err, "Expected at least one seeded sport in database")

	courtID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, created_at, updated_at)
		VALUES ($1, $2, $3, 'Test Court', 'INDOOR', 100000, now(), now())
	`, courtID, venueID, sportID)
	require.NoError(t, err)

	termID = uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO platform_commercial_terms
		(id, owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from, valid_until, created_at)
		VALUES ($1, $2, 'Test Term', 'STANDARD', 'SIMULATION', 'NONE', 700, now(), null, now())
	`, termID, &ownerID)
	require.NoError(t, err)

	return userID, ownerID, venueID, courtID, termID
}

func TestCutover_ActivationRejectedAtMigration020(t *testing.T) {
	if os.Getenv("TEST_CUTOVER_DISPOSABLE") != "1" {
		t.Skip("Skipping migration 020 activation test; set TEST_CUTOVER_DISPOSABLE=1")
	}

	baseDSN := os.Getenv("CUTOVER_TEST_DATABASE_URL")
	require.NotEmpty(t, baseDSN)

	dbName := "lapangango_mig020_" + strings.ReplaceAll(uuid.New().String(), "-", "_")
	tempDSN, cleanup, err := createDisposableDB(baseDSN, dbName)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cleanup())
	}()

	migrationsPath := getMigrationsPath()
	require.NotEmpty(t, migrationsPath, "Migrations path must be resolved")

	m, err := migrate.New(migrationsPath, tempDSN)
	require.NoError(t, err)
	defer m.Close()

	err = m.Migrate(20)
	require.NoError(t, err)

	version, dirty, err := m.Version()
	require.NoError(t, err)
	require.Equal(t, uint(20), version)
	require.False(t, dirty)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, tempDSN)
	require.NoError(t, err)
	defer pool.Close()

	var cutoverTableExists, triggerExists bool
	err = pool.QueryRow(ctx, `
		SELECT
			to_regclass('public.platform_finance_cutovers') IS NOT NULL,
			EXISTS (
				SELECT 1
				FROM pg_trigger
				WHERE tgrelid = 'public.bookings'::regclass
				  AND tgname = 'booking_snapshot_required_after_cutover'
			)
	`).Scan(&cutoverTableExists, &triggerExists)
	require.NoError(t, err)
	require.True(t, cutoverTableExists)
	require.False(t, triggerExists)

	saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
	activator, err := NewCutoverActivator(pool)
	require.NoError(t, err)

	_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
		CalculationVersion: ActiveBookingFeeCalculationVersion,
		ReleaseReference:   "migration-020-rejection",
		ActorUserID:        saID,
		LockTimeout:        5 * time.Second,
	})
	require.ErrorIs(t, err, ErrCutoverIntegrity)

	var cutoverCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&cutoverCount)
	require.NoError(t, err)
	require.Zero(t, cutoverCount)
}

func TestCutover_MigrationVerification(t *testing.T) {
	if os.Getenv("TEST_CUTOVER_DISPOSABLE") != "1" {
		t.Skip("Skipping migration verification test; set TEST_CUTOVER_DISPOSABLE=1")
	}
	baseDSN := os.Getenv("CUTOVER_TEST_DATABASE_URL")
	require.NotEmpty(t, baseDSN)

	// Create dedicated disposable DB for migration verification
	migDBName := "lapangango_mig_verify_" + strings.ReplaceAll(uuid.New().String(), "-", "_")
	tempDSN, cleanup, err := createDisposableDB(baseDSN, migDBName)
	require.NoError(t, err)
	defer func() {
		errCleanup := cleanup()
		require.NoError(t, errCleanup)
	}()

	mPath := getMigrationsPath()
	require.NotEmpty(t, mPath, "Migrations path must be resolved")

	m, err := migrate.New(mPath, tempDSN)
	require.NoError(t, err)
	defer m.Close()

	// 1. Run all migrations Up to 21
	err = m.Up()
	require.NoError(t, err)

	// 2. DOWN 1 before activation succeeds (goes to 20)
	err = m.Steps(-1)
	require.NoError(t, err)

	// 3. UP 1 again succeeds (goes to 21)
	err = m.Steps(1)
	require.NoError(t, err)

	// Verify schema version 21, dirty false
	version, dirty, err := m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(21), version)
	assert.False(t, dirty)

	// Get temp DB pool
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, tempDSN)
	require.NoError(t, err)
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() {
		if tx != nil {
			rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer rollbackCancel()
			_ = tx.Rollback(rollbackCtx)
		}
	}()

	saID := insertSuperAdminActor(t, ctx, tx, "ACTIVE")
	_, err = tx.Exec(ctx, `
		INSERT INTO platform_finance_cutovers (snapshot_cutover_at, calculation_version, release_reference, created_by_user_id)
		VALUES (now(), 'booking-fee-v1', 'test-ref', $1)
	`, saID)
	require.NoError(t, err)
	err = tx.Commit(ctx)
	require.NoError(t, err)

	// 6. DOWN after active cutover is explicitly rejected
	err = m.Steps(-1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot remove finance cutover guard after cutover activation")

	// 7. Verify migration 020 tables remain intact (both exist with count=2)
	var tablesCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('platform_finance_cutovers', 'booking_fee_snapshots')
	`).Scan(&tablesCount)
	require.NoError(t, err)
	assert.Equal(t, 2, tablesCount)
}

func TestCutover_ActivationAndGuards(t *testing.T) {
	pool := setupCutoverTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cleanupAllTables(t, pool)

	activator, err := NewCutoverActivator(pool)
	require.NoError(t, err)

	t.Run("Nil database pool constructor rejected", func(t *testing.T) {
		_, err := NewCutoverActivator(nil)
		assert.Error(t, err)
	})

	t.Run("Wrong role actor rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()

		custID := insertCustomerActor(t, ctx, tx)
		err = tx.Commit(ctx)
		require.NoError(t, err)

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-1",
			ActorUserID:        custID,
			LockTimeout:        2 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverActorForbidden)
	})

	t.Run("Suspended SUPER_ADMIN rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()

		saID := insertSuperAdminActor(t, ctx, tx, "SUSPENDED")
		err = tx.Commit(ctx)
		require.NoError(t, err)

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-1",
			ActorUserID:        saID,
			LockTimeout:        2 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverActorForbidden)
	})

	t.Run("Invalid calculation version rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()

		saID := insertSuperAdminActor(t, ctx, tx, "ACTIVE")
		err = tx.Commit(ctx)
		require.NoError(t, err)

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "invalid-version",
			ReleaseReference:   "ref-1",
			ActorUserID:        saID,
			LockTimeout:        2 * time.Second,
		})
		assert.ErrorIs(t, err, ErrInvalidCutoverParams)
	})

	t.Run("Empty/oversized release reference rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()

		saID := insertSuperAdminActor(t, ctx, tx, "ACTIVE")
		err = tx.Commit(ctx)
		require.NoError(t, err)

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "",
			ActorUserID:        saID,
			LockTimeout:        2 * time.Second,
		})
		assert.ErrorIs(t, err, ErrInvalidCutoverParams)

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   strings.Repeat("a", 256),
			ActorUserID:        saID,
			LockTimeout:        2 * time.Second,
		})
		assert.ErrorIs(t, err, ErrInvalidCutoverParams)
	})

	t.Run("Cutover activation creates exactly one immutable row", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()

		saID := insertSuperAdminActor(t, ctx, tx, "ACTIVE")
		err = tx.Commit(ctx)
		require.NoError(t, err)

		record, err := activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-success",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		require.NoError(t, err)
		assert.Equal(t, int16(1), record.ID)
		assert.Equal(t, "booking-fee-v1", record.CalculationVersion)
		assert.Equal(t, "ref-success", record.ReleaseReference)
		assert.Equal(t, saID, record.CreatedByUserID)

		// Repeated activation rejected
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-success-2",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverAlreadyActive)
	})

	t.Run("Concurrent activation results in exactly one row", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()
		saID := insertSuperAdminActor(t, ctx, tx, "ACTIVE")
		err = tx.Commit(ctx)
		require.NoError(t, err)

		var wg sync.WaitGroup
		results := make(chan error, 5)

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				concurrentCtx, concurrentCancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer concurrentCancel()

				_, err := activator.ActivateCutover(concurrentCtx, ActivateCutoverParams{
					CalculationVersion: "booking-fee-v1",
					ReleaseReference:   "ref-concurrent",
					ActorUserID:        saID,
					LockTimeout:        10 * time.Second,
				})
				results <- err
			}()
		}

		wg.Wait()
		close(results)

		successCount := 0
		alreadyActiveCount := 0
		for err := range results {
			if err == nil {
				successCount++
			} else if errors.Is(err, ErrCutoverAlreadyActive) {
				alreadyActiveCount++
			}
		}

		assert.Equal(t, 1, successCount)
		assert.Equal(t, 4, alreadyActiveCount)
	})

	t.Run("No active cutover test leaves zero cutover rows after rollback", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()
		_ = insertSuperAdminActor(t, ctx, tx, "ACTIVE")
		err = tx.Commit(ctx)
		require.NoError(t, err)

		// Call ActivateCutover but with an invalid actor which will fail
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-rollback",
			ActorUserID:        "00000000-0000-0000-0000-000000000000", // Invalid actor
			LockTimeout:        5 * time.Second,
		})
		assert.Error(t, err)

		// Assert zero rows in platform_finance_cutovers
		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("UPDATE/DELETE cutover remains rejected by migration 020 immutability trigger", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()
		saID := insertSuperAdminActor(t, ctx, tx, "ACTIVE")
		err = tx.Commit(ctx)
		require.NoError(t, err)

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-immutable",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		require.NoError(t, err)

		// Try updating the record
		_, err = pool.Exec(ctx, "UPDATE platform_finance_cutovers SET release_reference = 'hacked' WHERE id = 1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Updates and Deletes are strictly forbidden on immutable platform finance tables")

		// Try deleting the record
		_, err = pool.Exec(ctx, "DELETE FROM platform_finance_cutovers WHERE id = 1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Updates and Deletes are strictly forbidden on immutable platform finance tables")
	})

	t.Run("Missing trigger rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		_, err := pool.Exec(ctx, "DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;")
		require.NoError(t, err)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-missing-trigger",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverIntegrity)

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		restoreTrigger(t, pool)
	})

	t.Run("Trigger on decoy schema rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)

		_, err := pool.Exec(ctx, "DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;")
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS decoy;
			CREATE TABLE IF NOT EXISTS decoy.bookings (id uuid PRIMARY KEY);

			CREATE OR REPLACE FUNCTION decoy.enforce_booking_snapshot_after_cutover()
			RETURNS TRIGGER
			LANGUAGE plpgsql
			AS $$
			BEGIN
				RETURN NULL;
			END;
			$$;

			DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON decoy.bookings;
			CREATE CONSTRAINT TRIGGER booking_snapshot_required_after_cutover
			AFTER INSERT ON decoy.bookings
			DEFERRABLE INITIALLY DEFERRED
			FOR EACH ROW
			EXECUTE FUNCTION decoy.enforce_booking_snapshot_after_cutover();
		`)
		require.NoError(t, err)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-decoy-schema",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverIntegrity)

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		_, err = pool.Exec(ctx, "DROP SCHEMA decoy CASCADE;")
		require.NoError(t, err)

		restoreTrigger(t, pool)
	})

	t.Run("Trigger pointing to wrong function rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)

		_, err := pool.Exec(ctx, `
			CREATE OR REPLACE FUNCTION wrong_function()
			RETURNS TRIGGER
			LANGUAGE plpgsql
			AS $$
			BEGIN
				RETURN NULL;
			END;
			$$;

			DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;
			CREATE CONSTRAINT TRIGGER booking_snapshot_required_after_cutover
			AFTER INSERT ON bookings
			DEFERRABLE INITIALLY DEFERRED
			FOR EACH ROW
			EXECUTE FUNCTION wrong_function();
		`)
		require.NoError(t, err)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-wrong-func",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverIntegrity)

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		restoreTrigger(t, pool)
	})

	t.Run("Trigger with wrong event (AFTER UPDATE) rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)

		_, err := pool.Exec(ctx, `
			DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;
			CREATE CONSTRAINT TRIGGER booking_snapshot_required_after_cutover
			AFTER UPDATE ON bookings
			DEFERRABLE INITIALLY DEFERRED
			FOR EACH ROW
			EXECUTE FUNCTION enforce_booking_snapshot_after_cutover();
		`)
		require.NoError(t, err)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-wrong-event",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverIntegrity)

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		restoreTrigger(t, pool)
	})

	t.Run("Disabled trigger rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		restoreTrigger(t, pool)
		_, err := pool.Exec(ctx, "ALTER TABLE bookings DISABLE TRIGGER booking_snapshot_required_after_cutover;")
		require.NoError(t, err)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-disabled-trigger",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverIntegrity)

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		_, err = pool.Exec(ctx, "ALTER TABLE bookings ENABLE TRIGGER booking_snapshot_required_after_cutover;")
		require.NoError(t, err)
	})

	t.Run("Trigger non-deferrable rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		_, err := pool.Exec(ctx, `
			DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;
			CREATE CONSTRAINT TRIGGER booking_snapshot_required_after_cutover
			AFTER INSERT ON bookings
			NOT DEFERRABLE
			FOR EACH ROW
			EXECUTE FUNCTION enforce_booking_snapshot_after_cutover();
		`)
		require.NoError(t, err)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-non-deferrable",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverIntegrity)

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		restoreTrigger(t, pool)
	})

	t.Run("Trigger non-initially-deferred rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		_, err := pool.Exec(ctx, `
			DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;
			CREATE CONSTRAINT TRIGGER booking_snapshot_required_after_cutover
			AFTER INSERT ON bookings
			DEFERRABLE INITIALLY IMMEDIATE
			FOR EACH ROW
			EXECUTE FUNCTION enforce_booking_snapshot_after_cutover();
		`)
		require.NoError(t, err)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-non-init-deferred",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		assert.ErrorIs(t, err, ErrCutoverIntegrity)

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		restoreTrigger(t, pool)
	})
}

func TestCutover_DeferredTriggerGuards(t *testing.T) {
	pool := setupCutoverTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cleanupAllTables(t, pool)

	activator, err := NewCutoverActivator(pool)
	require.NoError(t, err)

	t.Run("Pre-cutover direct booking insert without snapshot commits successfully", func(t *testing.T) {
		cleanupAllTables(t, pool)
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx.Rollback(rollbackCtx)
			}
		}()

		userID, _, _, courtID, _ := insertDependencyFixtureNoBooking(t, ctx, tx)

		// Insert booking directly
		bookingID := uuid.New().String()
		_, err = tx.Exec(ctx, `
			INSERT INTO bookings (id, court_id, customer_id, booking_date, start_time, end_time, status, total_price, created_at, updated_at)
			VALUES ($1, $2, $3, CURRENT_DATE, '14:00:00', '15:00:00', 'PENDING_PAYMENT', 150000, now(), now())
		`, bookingID, courtID, userID)
		require.NoError(t, err)

		err = tx.Commit(ctx)
		require.NoError(t, err)
	})

	t.Run("Post-cutover booking without snapshot rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		restoreTrigger(t, pool)
		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-active",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		require.NoError(t, err)

		tx2, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx2 != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx2.Rollback(rollbackCtx)
			}
		}()

		userID, _, _, courtID, _ := insertDependencyFixtureNoBooking(t, ctx, tx2)
		bookingID := uuid.New().String()

		_, err = tx2.Exec(ctx, `
			INSERT INTO bookings (id, court_id, customer_id, booking_date, start_time, end_time, status, total_price, created_at, updated_at)
			VALUES ($1, $2, $3, CURRENT_DATE, '15:00:00', '16:00:00', 'PENDING_PAYMENT', 150000, now(), now())
		`, bookingID, courtID, userID)
		require.NoError(t, err)

		err = tx2.Commit(ctx)
		require.Error(t, err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			assert.Equal(t, "23514", pgErr.Code)
		} else {
			t.Fatalf("expected pg error, got: %v", err)
		}
	})

	t.Run("Post-cutover booking plus valid snapshot in the same transaction commits", func(t *testing.T) {
		cleanupAllTables(t, pool)
		restoreTrigger(t, pool)
		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")

		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-active",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		require.NoError(t, err)

		tx2, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() {
			if tx2 != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx2.Rollback(rollbackCtx)
			}
		}()

		userID, ownerID, venueID, courtID, termID := insertDependencyFixtureNoBooking(t, ctx, tx2)
		bookingID := uuid.New().String()

		_, err = tx2.Exec(ctx, `
			INSERT INTO bookings (id, court_id, customer_id, booking_date, start_time, end_time, status, total_price, created_at, updated_at)
			VALUES ($1, $2, $3, CURRENT_DATE, '15:00:00', '16:00:00', 'PENDING_PAYMENT', 150000, now(), now())
		`, bookingID, courtID, userID)
		require.NoError(t, err)

		_, err = tx2.Exec(ctx, `
			INSERT INTO booking_fee_snapshots (
				booking_id, owner_profile_id, venue_id, commercial_term_id, terms_source, booking_channel, finance_mode,
				original_price_rupiah, owner_price_adjustment_rupiah, price_adjustment_reason, final_booking_price_rupiah,
				customer_service_fee_rupiah, customer_charge_amount_rupiah, commission_basis_amount_rupiah,
				commission_bps, commission_amount_rupiah, owner_net_amount_rupiah, calculation_version
			) VALUES (
				$1, $2, $3, $4, 'POLICY', 'MARKETPLACE_ONLINE', 'SIMULATION',
				150000, 0, null, 150000,
				0, 150000, 150000,
				700, 10500, 139500, 'booking-fee-v1'
			)
		`, bookingID, ownerID, venueID, termID)
		require.NoError(t, err)

		err = tx2.Commit(ctx)
		require.NoError(t, err)
	})

	t.Run("Repeatable Read bypass rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		restoreTrigger(t, pool)

		tx1, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		require.NoError(t, err)
		defer func() {
			if tx1 != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx1.Rollback(rollbackCtx)
			}
		}()

		var exists bool
		err = tx1.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM platform_finance_cutovers WHERE id = 1)").Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-rr-test",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		require.NoError(t, err)

		userID, _, _, courtID, _ := insertDependencyFixtureNoBooking(t, ctx, tx1)
		bookingID := uuid.New().String()
		_, err = tx1.Exec(ctx, `
			INSERT INTO bookings (id, court_id, customer_id, booking_date, start_time, end_time, status, total_price, created_at, updated_at)
			VALUES ($1, $2, $3, CURRENT_DATE, '15:00:00', '16:00:00', 'PENDING_PAYMENT', 150000, now(), now())
		`, bookingID, courtID, userID)
		require.NoError(t, err)

		err = tx1.Commit(ctx)
		require.Error(t, err)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			assert.Equal(t, "23514", pgErr.Code)
		} else {
			t.Fatalf("expected pg error, got: %v", err)
		}
	})

	t.Run("Serializable bypass rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		restoreTrigger(t, pool)

		tx1, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		require.NoError(t, err)
		defer func() {
			if tx1 != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer rollbackCancel()
				_ = tx1.Rollback(rollbackCtx)
			}
		}()

		var exists bool
		err = tx1.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM platform_finance_cutovers WHERE id = 1)").Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists)

		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")
		_, err = activator.ActivateCutover(ctx, ActivateCutoverParams{
			CalculationVersion: "booking-fee-v1",
			ReleaseReference:   "ref-ser-test",
			ActorUserID:        saID,
			LockTimeout:        5 * time.Second,
		})
		require.NoError(t, err)

		userID, _, _, courtID, _ := insertDependencyFixtureNoBooking(t, ctx, tx1)
		bookingID := uuid.New().String()
		_, err = tx1.Exec(ctx, `
			INSERT INTO bookings (id, court_id, customer_id, booking_date, start_time, end_time, status, total_price, created_at, updated_at)
			VALUES ($1, $2, $3, CURRENT_DATE, '15:00:00', '16:00:00', 'PENDING_PAYMENT', 150000, now(), now())
		`, bookingID, courtID, userID)
		require.NoError(t, err)

		err = tx1.Commit(ctx)
		require.Error(t, err)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			assert.Equal(t, "23514", pgErr.Code)
		} else {
			t.Fatalf("expected pg error, got: %v", err)
		}
	})
}

func TestCutover_CLI(t *testing.T) {
	pool := setupCutoverTestDB(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cleanupAllTables(t, pool)
	restoreTrigger(t, pool)

	runCLI := func(args ...string) ([]byte, error) {
		cliCtx, cliCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cliCancel()

		fullArgs := append([]string{"run", "../../cmd/finance-cutover/main.go"}, args...)
		cmd := exec.CommandContext(cliCtx, "go", fullArgs...)
		cmd.Env = append(os.Environ(),
			"DATABASE_URL="+testDSN,
			"JWT_SECRET=supersecretkey",
		)
		return cmd.CombinedOutput()
	}

	t.Run("CLI preflight does not print PII or DB details on invalid actor", func(t *testing.T) {
		out, err := runCLI(
			"--calculation-version=booking-fee-v1",
			"--release-reference=rel-preflight",
			"--user-id=00000000-0000-0000-0000-000000000000",
		)
		assert.Error(t, err)
		outStr := string(out)
		assert.NotContains(t, outStr, "DATABASE_URL")
		assert.NotContains(t, outStr, "postgres://")
		assert.NotContains(t, outStr, "00000000-0000")
	})

	t.Run("default/preflight produces zero writes", func(t *testing.T) {
		cleanupAllTables(t, pool)
		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")

		out, err := runCLI(
			"--calculation-version=booking-fee-v1",
			"--release-reference=rel-preflight",
			"--user-id="+saID,
		)
		require.NoError(t, err, "preflight failed: %s", string(out))

		outStr := string(out)
		assert.Contains(t, outStr, "FINANCE CUTOVER PREFLIGHT (DRY-RUN)")
		assert.Contains(t, outStr, "Preflight status: VALID")
		assert.NotContains(t, outStr, "DATABASE_URL")

		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_finance_cutovers").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("--apply without maintenance confirmation rejected", func(t *testing.T) {
		cleanupAllTables(t, pool)
		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")

		out, err := runCLI(
			"--apply",
			"--calculation-version=booking-fee-v1",
			"--release-reference=rel-preflight",
			"--user-id="+saID,
		)
		assert.Error(t, err)
		assert.NotContains(t, string(out), "DATABASE_URL")
	})

	t.Run("apply succeeds once, second apply fails", func(t *testing.T) {
		cleanupAllTables(t, pool)
		saID := insertSuperAdminActorPool(t, ctx, pool, "ACTIVE")

		out, err := runCLI(
			"--apply",
			"--maintenance-confirmed",
			"--calculation-version=booking-fee-v1",
			"--release-reference=rel-live",
			"--user-id="+saID,
		)
		require.NoError(t, err, "apply failed: %s", string(out))

		outStr := string(out)
		assert.Contains(t, outStr, "SUCCESS: Cutover activated")
		assert.Contains(t, outStr, "Timestamp:")
		assert.NotContains(t, outStr, "DATABASE_URL")

		out2, err2 := runCLI(
			"--apply",
			"--maintenance-confirmed",
			"--calculation-version=booking-fee-v1",
			"--release-reference=rel-live-2",
			"--user-id="+saID,
		)
		assert.Error(t, err2)
		assert.NotContains(t, string(out2), "DATABASE_URL")
	})
}
