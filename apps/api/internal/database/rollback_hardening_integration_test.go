package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

var downMigrations = map[int]string{
	19: "019_platform_audit_and_commercial_terms.down.sql",
	20: "020_booking_fee_snapshots.down.sql",
	21: "021_platform_finance_cutover_guard.down.sql",
	22: "022_platform_double_entry_ledger.down.sql",
	23: "023_platform_ledger_balance_reschedule.down.sql",
	24: "024_platform_expenses.down.sql",
}

func checkOptIn(t *testing.T) string {
	t.Helper()
	optIn := os.Getenv("TEST_ROLLBACK_HARDENING_DISPOSABLE")
	if optIn == "" || optIn == "0" || optIn == "false" {
		t.Skip("TEST_ROLLBACK_HARDENING_DISPOSABLE not enabled, skipping.")
	}
	if optIn == "1" {
		adminDSN := os.Getenv("ROLLBACK_HARDENING_TEST_DATABASE_URL")
		if adminDSN == "" {
			t.Fatal("TEST_ROLLBACK_HARDENING_DISPOSABLE is 1 but ROLLBACK_HARDENING_TEST_DATABASE_URL is not set.")
		}
		return adminDSN
	}
	t.Fatalf("TEST_ROLLBACK_HARDENING_DISPOSABLE must be one of unset, 0, false, or 1; got %q", optIn)
	return "" // unreachable; keeps the helper's return contract explicit.
}

func createDisposableDB(t *testing.T, adminDSN string) (string, func()) {
	t.Helper()
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("could not parse admin DSN: %v", err)
	}

	sourceDBName := parsed.Path
	if sourceDBName == "" || sourceDBName == "/" {
		t.Fatalf("invalid admin DSN: missing database name")
	}

	dbName := "lapangango_rollback_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	adminDB, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatalf("could not connect to admin db: %v", err)
	}

	if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
		adminDB.Close()
		t.Fatalf("could not create disposable database %s: %v", dbName, err)
	}
	adminDB.Close()

	parsed.Path = "/" + dbName
	targetDSN := parsed.String()

	cleanup := func() {
		adminDBForCleanup, err := sql.Open("postgres", adminDSN)
		if err == nil {
			defer adminDBForCleanup.Close()
			adminDBForCleanup.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, dbName)
			_, err = adminDBForCleanup.Exec("DROP DATABASE " + dbName)
			if err != nil {
				t.Errorf("failed to drop disposable database %s: %v", dbName, err)
			}
		} else {
			t.Errorf("failed to open admin db for cleanup: %v", err)
		}
	}

	return targetDSN, cleanup
}

func setupMigrate(t *testing.T, targetDSN string) (*sql.DB, *migrate.Migrate) {
	t.Helper()
	db, err := sql.Open("postgres", targetDSN)
	if err != nil {
		t.Fatalf("could not connect to target db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("could not ping target db: %v", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		t.Fatalf("could not create migration driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://../../../../db/migrations", "postgres", driver)
	if err != nil {
		t.Fatalf("could not create migrate instance: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("could not run up migrations: %v", err)
	}

	return db, m
}

func getDBFingerprint(ctx context.Context, db *sql.DB) (string, error) {
	var objectFingerprint string
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)::text || '-' ||
		       (SELECT count(*) FROM information_schema.table_constraints WHERE constraint_schema = 'public') || '-' ||
		       (SELECT count(*) FROM information_schema.triggers WHERE trigger_schema = 'public')
		FROM information_schema.tables WHERE table_schema = 'public'
	`).Scan(&objectFingerprint); err != nil {
		return "", err
	}

	facts, err := getTableFingerprint(ctx, db, []string{
		"platform_commercial_terms",
		"platform_audit_logs",
		"platform_finance_cutovers",
		"booking_fee_snapshots",
		"platform_journals",
		"platform_ledger_entries",
		"platform_expenses",
		"platform_expense_idempotency",
	})
	if err != nil {
		return "", err
	}
	return "objects=" + objectFingerprint + ";" + facts, nil
}

func getTableFingerprint(ctx context.Context, db *sql.DB, tables []string) (string, error) {
	parts := make([]string, 0, len(tables))
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		var tableExists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&tableExists); err != nil {
			return "", err
		}
		if !tableExists {
			parts = append(parts, table+"=missing")
			continue
		}
		var rowFingerprint sql.NullString
		query := fmt.Sprintf(`SELECT md5(COALESCE(string_agg(row_to_json(t)::text, '|' ORDER BY row_to_json(t)::text), '')) FROM public.%s t`, quoted)
		if err := db.QueryRowContext(ctx, query).Scan(&rowFingerprint); err != nil {
			return "", err
		}
		parts = append(parts, table+"="+rowFingerprint.String)
	}
	return strings.Join(parts, ";"), nil
}

func survivingFactTables(target int) []string {
	tables := []string{"platform_commercial_terms", "platform_audit_logs"}
	if target >= 20 {
		tables = append(tables, "platform_finance_cutovers", "booking_fee_snapshots")
	}
	if target >= 22 {
		tables = append(tables, "platform_journals", "platform_ledger_entries")
	}
	if target >= 24 {
		tables = append(tables, "platform_expenses", "platform_expense_idempotency")
	}
	return tables
}

func fingerprintDSN(t *testing.T, dsn string) string {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open fingerprint database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping fingerprint database: %v", err)
	}
	fingerprint, err := getDBFingerprint(context.Background(), db)
	if err != nil {
		t.Fatalf("failed to fingerprint database: %v", err)
	}
	return fingerprint
}

func TestRollbackHardening_PreFactDown(t *testing.T) {
	adminDSN := checkOptIn(t)
	sourceBefore := fingerprintDSN(t, adminDSN)
	defer func() {
		if sourceAfter := fingerprintDSN(t, adminDSN); sourceAfter != sourceBefore {
			t.Errorf("admin/source database changed during disposable rollback test")
		}
	}()
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	_, m := setupMigrate(t, targetDSN)

	err := m.Steps(-6) // From 24 down to 18
	if err != nil {
		t.Fatalf("expected pre-fact down migration to succeed, got: %v", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if dirty {
		t.Fatalf("expected dirty=false, got true")
	}
	if version != 18 {
		t.Fatalf("expected version 18, got %d", version)
	}
}

func TestRollbackHardening_PostFactRefusal(t *testing.T) {
	adminDSN := checkOptIn(t)
	sourceBefore := fingerprintDSN(t, adminDSN)
	defer func() {
		if sourceAfter := fingerprintDSN(t, adminDSN); sourceAfter != sourceBefore {
			t.Errorf("admin/source database changed during disposable rollback test")
		}
	}()

	cases := []struct {
		name                 string
		target               int
		expectedDirtyVersion int
		insertQuery          string
		insertArgs           []interface{}
	}{
		{
			name:                 "019 Additional commercial term",
			target:               19,
			expectedDirtyVersion: 18,
			insertQuery: `
				WITH new_user AS (
					INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'term-actor', 'term@test.com', 'hash') ON CONFLICT DO NOTHING RETURNING id
				),
				new_owner AS (
					INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($2, (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))), 'Business', 'APPROVED') ON CONFLICT DO NOTHING RETURNING id
				)
				INSERT INTO platform_commercial_terms (owner_profile_id, label, phase, finance_mode, collection_method, commission_bps, valid_from) 
				VALUES ((SELECT COALESCE((SELECT id FROM new_owner), (SELECT id FROM owner_profiles LIMIT 1))), 'Another Term', 'STANDARD', 'LIVE', 'NONE', 500, $3)
			`,
			insertArgs: []interface{}{uuid.New().String(), uuid.New().String(), time.Now()},
		},
		{
			name:                 "019 Mutated frozen seed",
			target:               19,
			expectedDirtyVersion: 18,
			insertQuery:          `UPDATE platform_commercial_terms SET commission_bps = 800 WHERE label = 'Global Default Term'`,
			insertArgs:           nil,
		},
		{
			name:                 "019 Mutated frozen seed valid_from",
			target:               19,
			expectedDirtyVersion: 18,
			insertQuery:          `UPDATE platform_commercial_terms SET valid_from = valid_from + interval '1 second' WHERE label = 'Global Default Term'`,
			insertArgs:           nil,
		},
		{
			name:                 "019 Mutated frozen seed created_at",
			target:               19,
			expectedDirtyVersion: 18,
			insertQuery:          `UPDATE platform_commercial_terms SET created_at = created_at + interval '1 second' WHERE label = 'Global Default Term'`,
			insertArgs:           nil,
		},
		{
			name:                 "019 Missing frozen seed",
			target:               19,
			expectedDirtyVersion: 18,
			insertQuery:          `DELETE FROM platform_commercial_terms WHERE label = 'Global Default Term'`,
			insertArgs:           nil,
		},
		{
			name:                 "019 Platform audit fact",
			target:               19,
			expectedDirtyVersion: 18,
			insertQuery:          `INSERT INTO platform_audit_logs (actor_role, action, entity_type) VALUES ($1, $2, $3)`,
			insertArgs:           []interface{}{"SYSTEM", "CREATE", "MIGRATION_TEST"},
		},
		{
			name:                 "020 Booking fee snapshot (direct)",
			target:               20,
			expectedDirtyVersion: 19,
			insertQuery: `
				WITH new_user AS (
					INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'snapshot-actor', 'snap@test.com', 'hash') ON CONFLICT DO NOTHING RETURNING id
				),
				new_owner AS (
					INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($2, (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))), 'Business', 'APPROVED') ON CONFLICT DO NOTHING RETURNING id
				),
				new_venue AS (
					INSERT INTO venues (id, owner_profile_id, name, description, address, city, status, latitude, longitude) VALUES ($3, (SELECT COALESCE((SELECT id FROM new_owner), (SELECT id FROM owner_profiles LIMIT 1))), 'V', 'D', 'A', 'City', 'ACTIVE', 0, 0) ON CONFLICT DO NOTHING RETURNING id
				),
				new_sport AS (
					INSERT INTO sports (id, name, status) VALUES (gen_random_uuid(), 'Soccer', 'ACTIVE') ON CONFLICT DO NOTHING RETURNING id
				),
				new_court AS (
					INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, status) 
					VALUES (gen_random_uuid(), (SELECT COALESCE((SELECT id FROM new_venue), (SELECT id FROM venues LIMIT 1))), (SELECT COALESCE((SELECT id FROM new_sport), (SELECT id FROM sports LIMIT 1))), 'Court 1', 'INDOOR', 100, 'ACTIVE') ON CONFLICT DO NOTHING RETURNING id
				),
				new_booking AS (
					INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, total_price, status) 
					VALUES ($4, (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))), (SELECT COALESCE((SELECT id FROM new_court), (SELECT id FROM courts LIMIT 1))), $5, '10:00:00', '11:00:00', 100, 'CONFIRMED') ON CONFLICT DO NOTHING RETURNING id
				)
				INSERT INTO booking_fee_snapshots (
					booking_id, owner_profile_id, venue_id, commercial_term_id, terms_source, booking_channel, finance_mode,
					original_price_rupiah, owner_price_adjustment_rupiah, final_booking_price_rupiah,
					customer_charge_amount_rupiah, commission_basis_amount_rupiah, commission_bps, commission_amount_rupiah, owner_net_amount_rupiah,
					calculation_version
				) VALUES (
					(SELECT COALESCE((SELECT id FROM new_booking), (SELECT id FROM bookings LIMIT 1))),
					(SELECT COALESCE((SELECT id FROM new_owner), (SELECT id FROM owner_profiles LIMIT 1))),
					(SELECT COALESCE((SELECT id FROM new_venue), (SELECT id FROM venues LIMIT 1))),
					(SELECT id FROM platform_commercial_terms LIMIT 1),
					'POLICY', 'MARKETPLACE_ONLINE', 'SIMULATION',
					10000, 0, 10000,
					10000, 10000, 700, 700, 9300,
					'V1'
				)
			`,
			insertArgs: []interface{}{uuid.New().String(), uuid.New().String(), uuid.New().String(), uuid.New().String(), time.Now()},
		},
		{
			name:                 "020 Active cutover guard (direct)",
			target:               20,
			expectedDirtyVersion: 20,
			insertQuery: `
				WITH new_user AS (
					INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'cutover-direct-actor', 'cutover-direct@test.com', 'hash') ON CONFLICT DO NOTHING RETURNING id
				)
				INSERT INTO platform_finance_cutovers (id, snapshot_cutover_at, calculation_version, release_reference, created_by_user_id)
				VALUES (1, $2, 'V1', 'R1', (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))))
			`,
			insertArgs: []interface{}{uuid.New().String(), time.Now()},
		},
		{
			name:                 "021 Active cutover guard",
			target:               21,
			expectedDirtyVersion: 20,
			insertQuery: `
				WITH new_user AS (
					INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'cutover-actor', 'cutover@test.com', 'hash') ON CONFLICT DO NOTHING RETURNING id
				)
				INSERT INTO platform_finance_cutovers (id, snapshot_cutover_at, calculation_version, release_reference, created_by_user_id) 
				VALUES (1, $2, 'V1', 'R1', (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))))
			`,
			insertArgs: []interface{}{uuid.New().String(), time.Now()},
		},
		{
			name:                 "022 Ledger facts",
			target:               22,
			expectedDirtyVersion: 22,
			insertQuery: `
				WITH new_journal AS (
					INSERT INTO platform_journals (id, event_key, event_type, payload_hash, effective_at) 
					VALUES ($1, 'journal.created:test', 'TEST', '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', $2) RETURNING id
				)
				INSERT INTO platform_ledger_entries (id, journal_id, account_code, side, amount_rupiah) 
				VALUES 
					($3, (SELECT id FROM new_journal), 'BANK_CASH', 'DEBIT', 10),
					($4, (SELECT id FROM new_journal), 'COMMISSION_REVENUE', 'CREDIT', 10)
			`,
			insertArgs: []interface{}{uuid.New().String(), time.Now(), uuid.New().String(), uuid.New().String()},
		},
		{
			name:                 "023 Ledger balance reschedule",
			target:               23,
			expectedDirtyVersion: 22,
			insertQuery: `
				WITH new_journal AS (
					INSERT INTO platform_journals (id, event_key, event_type, payload_hash, effective_at) 
					VALUES ($1, 'journal.created:test2', 'TEST', '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', $2) RETURNING id
				)
				INSERT INTO platform_ledger_entries (id, journal_id, account_code, side, amount_rupiah) 
				VALUES 
					($3, (SELECT id FROM new_journal), 'BANK_CASH', 'DEBIT', 10),
					($4, (SELECT id FROM new_journal), 'COMMISSION_REVENUE', 'CREDIT', 10)
			`,
			insertArgs: []interface{}{uuid.New().String(), time.Now(), uuid.New().String(), uuid.New().String()},
		},
		{
			name:                 "024 Expense row",
			target:               24,
			expectedDirtyVersion: 23,
			insertQuery: `
				WITH new_user AS (
					INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'exp', 'expense-actor@example.com', 'hash') ON CONFLICT DO NOTHING RETURNING id
				)
				INSERT INTO platform_expenses (id, category, amount_rupiah, currency, occurred_at, payment_account, description, created_by_user_id) 
				VALUES ($2, 'OTHER', 100, 'IDR', $3, 'ACCOUNTS_PAYABLE', 'Desc', (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))))
			`,
			insertArgs: []interface{}{uuid.New().String(), uuid.New().String(), time.Now()},
		},
		{
			name:                 "024 Expense idempotency",
			target:               24,
			expectedDirtyVersion: 23,
			insertQuery: `
				WITH new_user AS (
					INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'idemp', 'idemp-actor@example.com', 'hash') ON CONFLICT DO NOTHING RETURNING id
				),
				new_expense AS (
					INSERT INTO platform_expenses (id, category, amount_rupiah, currency, occurred_at, payment_account, description, created_by_user_id) 
					VALUES ($2, 'OTHER', 100, 'IDR', $3, 'ACCOUNTS_PAYABLE', 'Desc', (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))))
					RETURNING id
				)
				INSERT INTO platform_expense_idempotency (id, actor_user_id, action, idempotency_key, request_hash, expense_id, response_status, response_body)
				VALUES ($4, (SELECT COALESCE((SELECT id FROM new_user), (SELECT id FROM users LIMIT 1))), 'CREATE', 'key1', '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', (SELECT id FROM new_expense), 200, '{}')
			`,
			insertArgs: []interface{}{uuid.New().String(), uuid.New().String(), time.Now(), uuid.New().String()},
		},
	}

	for _, tc := range cases {
		// Run golang-migrate path
		t.Run(tc.name+"_golang_migrate", func(t *testing.T) {
			targetDSN, cleanup := createDisposableDB(t, adminDSN)
			defer cleanup()

			db, m := setupMigrate(t, targetDSN)
			defer db.Close()

			if len(tc.insertQuery) > 0 {
				_, err := db.ExecContext(context.Background(), tc.insertQuery, tc.insertArgs...)
				if err != nil {
					t.Fatalf("failed to insert fixture: %v", err)
				}
			}
			beforeFingerprint, err := getTableFingerprint(context.Background(), db, survivingFactTables(tc.target))
			if err != nil {
				t.Fatalf("failed to fingerprint facts before migration refusal: %v", err)
			}

			err = m.Steps(-1 * (24 - tc.target + 1))
			db.Exec("ROLLBACK") // clear aborted txn state
			if err == nil {
				t.Fatalf("expected down migration to fail for target %d, but it succeeded", tc.target)
			}

			// Get version using a fresh connection to avoid aborted transaction
			freshDB, err := sql.Open("postgres", targetDSN)
			if err != nil {
				t.Fatalf("failed to open fresh db: %v", err)
			}
			defer freshDB.Close()

			var version int
			var dirty bool
			errVer := freshDB.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
			if errVer != nil {
				t.Fatalf("failed to get version after error: %v", errVer)
			}
			afterFingerprint, err := getTableFingerprint(context.Background(), freshDB, survivingFactTables(tc.target))
			if err != nil {
				t.Fatalf("failed to fingerprint facts after migration refusal: %v", err)
			}
			if beforeFingerprint != afterFingerprint {
				t.Errorf("facts/schema objects mutated after refused migration\nBefore: %s\nAfter: %s", beforeFingerprint, afterFingerprint)
			}

			if !dirty {
				t.Errorf("expected dirty=true after refusal, got false")
			}

			if version != tc.expectedDirtyVersion {
				t.Errorf("expected exact dirty version %d after refused rollback, got %d", tc.expectedDirtyVersion, version)
			}
		})

		// Run Raw SQL path to verify no partial drops
		t.Run(tc.name+"_raw_sql", func(t *testing.T) {
			targetDSN, cleanup := createDisposableDB(t, adminDSN)
			defer cleanup()

			db, _ := setupMigrate(t, targetDSN)
			defer db.Close()

			if len(tc.insertQuery) > 0 {
				_, err := db.ExecContext(context.Background(), tc.insertQuery, tc.insertArgs...)
				if err != nil {
					t.Fatalf("failed to insert fixture: %v", err)
				}
			}

			// Fingerprint before running down migration
			freshDBBefore, err := sql.Open("postgres", targetDSN)
			if err != nil {
				t.Fatalf("failed to open fresh db for fingerprint: %v", err)
			}
			fpBefore, err := getDBFingerprint(context.Background(), freshDBBefore)
			freshDBBefore.Close()
			if err != nil {
				t.Fatalf("failed to get fingerprint before: %v", err)
			}

			fileName, ok := downMigrations[tc.target]
			if !ok {
				t.Fatalf("no down migration file mapped for target %d", tc.target)
			}

			scriptBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "db", "migrations", fileName))
			if err != nil {
				t.Fatalf("failed to read down script %s: %v", fileName, err)
			}

			_, err = db.ExecContext(context.Background(), string(scriptBytes))
			db.Exec("ROLLBACK") // clear aborted txn state
			if err == nil {
				t.Fatalf("expected raw sql down migration to fail, but it succeeded")
			}

			// Fingerprint after failing
			freshDB, err := sql.Open("postgres", targetDSN)
			if err != nil {
				t.Fatalf("failed to open fresh db for fingerprint: %v", err)
			}
			defer freshDB.Close()

			fpAfter, err := getDBFingerprint(context.Background(), freshDB)
			if err != nil {
				t.Fatalf("failed to get fingerprint after: %v", err)
			}

			if fpBefore != fpAfter {
				t.Errorf("database fingerprint mutated after failed rollback!\nBefore: %s\nAfter: %s", fpBefore, fpAfter)
			}
		})
	}
}
