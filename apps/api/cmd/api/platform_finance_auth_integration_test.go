package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

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

	sourceDBName := strings.TrimPrefix(parsed.Path, "/")
	if sourceDBName == "" {
		t.Fatalf("invalid admin DSN: missing database name")
	}

	dbName := "lapangango_auth_" + strings.ReplaceAll(uuid.New().String(), "-", "")

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
		t.Fatalf("could not create postgres driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://../../../../db/migrations",
		"postgres",
		driver,
	)
	if err != nil {
		t.Fatalf("could not create migrate instance: %v", err)
	}
	return db, m
}

func seedUser(t *testing.T, db *sql.DB, role, status string) (string, string) {
	t.Helper()
	id := uuid.New().String()
	email := "test_" + id + "@example.com"
	_, err := db.Exec(`
		INSERT INTO users (id, name, email, password_hash, role, status)
		VALUES ($1, 'Test User', $2, 'hash', $3, $4)
	`, id, email, role, status)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return id, email
}

func financeTableFingerprint(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var fingerprint string
	query := "SELECT md5(COALESCE(string_agg(row_to_json(t)::text, '|' ORDER BY row_to_json(t)::text), '')) FROM " + table + " t"
	if err := db.QueryRow(query).Scan(&fingerprint); err != nil {
		t.Fatalf("failed to fingerprint finance table %s: %v", table, err)
	}
	return fingerprint
}

func TestPlatformFinanceAuth_AuthMatrix(t *testing.T) {
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

	// Seed test users
	inactiveSuperAdminID, inactiveSuperAdminEmail := seedUser(t, db, "SUPER_ADMIN", "INACTIVE")
	activeCustomerID, activeCustomerEmail := seedUser(t, db, "CUSTOMER", "ACTIVE")
	activeOwnerID, activeOwnerEmail := seedUser(t, db, "OWNER", "ACTIVE")
	activeSuperAdminID, activeSuperAdminEmail := seedUser(t, db, "SUPER_ADMIN", "ACTIVE")

	// Create router with real middlewares
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		PlatformFinanceAdminEnabled: true,
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

	generateToken := func(id, email, role string) string {
		token, _ := tokenService.Generate(auth.UserResponse{
			ID:    id,
			Email: email,
			Role:  role,
		})
		return token
	}

	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"summary", http.MethodGet, "/admin/finance/summary?start_date=2026-01-01&end_date=2026-01-01", ""},
		{"breakdown", http.MethodGet, "/admin/finance/breakdown?start_date=2026-01-01&end_date=2026-01-01&dimension=owner", ""},
		{"list expenses", http.MethodGet, "/admin/finance/expenses?page=1&limit=20", ""},
		{"create expense", http.MethodPost, "/admin/finance/expenses", `{}`},
		{"cancel expense", http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/cancel", `{"reason":"auth matrix"}`},
		{"approve expense", http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/approve", `{}`},
		{"post expense", http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/post", `{}`},
		{"void expense", http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/void", `{"reason":"auth matrix"}`},
		{"journals", http.MethodGet, "/admin/finance/journals?start_date=2026-01-01&end_date=2026-01-01", ""},
	}
	authCases := []struct {
		name  string
		token string
		want  int
	}{
		{"No JWT", "", http.StatusUnauthorized},
		{"Invalid JWT", "invalid.token.here", http.StatusUnauthorized},
		{"Inactive SUPER_ADMIN", generateToken(inactiveSuperAdminID, inactiveSuperAdminEmail, "SUPER_ADMIN"), http.StatusForbidden},
		{"Active CUSTOMER", generateToken(activeCustomerID, activeCustomerEmail, "CUSTOMER"), http.StatusForbidden},
		{"Active OWNER", generateToken(activeOwnerID, activeOwnerEmail, "OWNER"), http.StatusForbidden},
		{"Active SUPER_ADMIN", generateToken(activeSuperAdminID, activeSuperAdminEmail, "SUPER_ADMIN"), 0},
	}

	for _, route := range routes {
		for _, tc := range authCases {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				req, _ := http.NewRequest(route.method, route.path, strings.NewReader(route.body))
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				if route.method == http.MethodPost {
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Idempotency-Key", uuid.NewString())
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				if tc.want != 0 && w.Code != tc.want {
					t.Errorf("expected %d, got %d", tc.want, w.Code)
				}
				if tc.want == 0 && (w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden) {
					t.Errorf("active SUPER_ADMIN did not pass production auth chain: got %d", w.Code)
				}
			})
		}
	}
}

func TestPlatformFinanceAuth_DisabledPreservation(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Seed exact facts to ensure persistence is preserved.
	// We'll just insert an audit fact to prove DB works and isn't mutated
	auditID := uuid.New().String()
	_, err := db.Exec(`
		INSERT INTO platform_audit_logs (id, actor_role, action, entity_type, entity_id, metadata, ip_address, user_agent)
		VALUES ($1, 'SUPER_ADMIN', 'TEST', 'TEST', $1, '{}', '127.0.0.1', 'UA')
	`, auditID)
	if err != nil {
		t.Fatalf("failed to insert audit fact: %v", err)
	}
	activeSuperAdminID, activeSuperAdminEmail := seedUser(t, db, "SUPER_ADMIN", "ACTIVE")
	journalID := uuid.NewString()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin journal fixture transaction: %v", err)
	}
	if _, err = tx.Exec(`
		INSERT INTO platform_journals (id, event_key, event_type, payload_hash, effective_at, created_by_user_id, description)
		VALUES ($1, 'journal.created:disabled-preservation', 'TEST', '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', $2, $3, 'disabled route preservation')
	`, journalID, time.Now().Add(-time.Minute), activeSuperAdminID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("failed to insert journal fact: %v", err)
	}
	if _, err = tx.Exec(`
		INSERT INTO platform_ledger_entries (journal_id, account_code, side, amount_rupiah)
		VALUES ($1, 'BANK_CASH', 'DEBIT', 100), ($1, 'COMMISSION_REVENUE', 'CREDIT', 100)
	`, journalID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("failed to insert ledger facts: %v", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("failed to commit journal and ledger facts: %v", err)
	}
	expenseID := uuid.NewString()
	if _, err = db.Exec(`
		INSERT INTO platform_expenses (id, category, vendor, amount_rupiah, currency, occurred_at, payment_account, description, created_by_user_id)
		VALUES ($1, 'OTHER', 'Preservation Fixture', 100, 'IDR', $2, 'ACCOUNTS_PAYABLE', 'disabled route preservation', $3)
	`, expenseID, time.Now().Add(-time.Hour), activeSuperAdminID); err != nil {
		t.Fatalf("failed to insert expense fact: %v", err)
	}
	if _, err = db.Exec(`
		INSERT INTO platform_expense_idempotency (actor_user_id, action, idempotency_key, request_hash, expense_id, response_status, response_body)
		VALUES ($1, 'CREATE', 'disabled-preservation-key', '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', $2, 201, '{}')
	`, activeSuperAdminID, expenseID); err != nil {
		t.Fatalf("failed to insert expense idempotency fact: %v", err)
	}
	beforeFingerprints := make(map[string]string)
	financeTables := []string{"platform_audit_logs", "platform_commercial_terms", "booking_fee_snapshots", "platform_finance_cutovers", "platform_journals", "platform_ledger_entries", "platform_expenses", "platform_expense_idempotency"}
	for _, table := range financeTables {
		beforeFingerprints[table] = financeTableFingerprint(t, db, table)
	}

	dbPool, err := pgxpool.New(context.Background(), targetDSN)
	if err != nil {
		t.Fatalf("failed to create pgxpool: %v", err)
	}
	defer dbPool.Close()

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
		ID:    activeSuperAdminID,
		Email: activeSuperAdminEmail,
		Role:  "SUPER_ADMIN",
	})

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/admin/finance/summary?start_date=2026-01-01&end_date=2026-01-01", ""},
		{http.MethodGet, "/admin/finance/breakdown?start_date=2026-01-01&end_date=2026-01-01&dimension=owner", ""},
		{http.MethodGet, "/admin/finance/expenses?page=1&limit=20", ""},
		{http.MethodPost, "/admin/finance/expenses", `{}`},
		{http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/cancel", `{"reason":"disabled"}`},
		{http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/approve", `{}`},
		{http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/post", `{}`},
		{http.MethodPost, "/admin/finance/expenses/00000000-0000-0000-0000-000000000000/void", `{"reason":"disabled"}`},
		{http.MethodGet, "/admin/finance/journals?start_date=2026-01-01&end_date=2026-01-01", ""},
	}

	for _, route := range routes {
		req, _ := http.NewRequest(route.method, route.path, strings.NewReader(route.body))
		req.Header.Set("Authorization", "Bearer "+token)
		if route.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", uuid.NewString())
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for disabled route %s %s, got %d", route.method, route.path, w.Code)
		}
	}

	// Assert every finance fact remains unchanged.
	for _, table := range financeTables {
		afterFingerprint := financeTableFingerprint(t, db, table)
		if afterFingerprint != beforeFingerprints[table] {
			t.Errorf("table %s changed while admin routes were disabled: before=%s after=%s", table, beforeFingerprints[table], afterFingerprint)
		}
	}
	var auditCount int
	if err = db.QueryRow("SELECT count(*) FROM platform_audit_logs WHERE id = $1", auditID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Errorf("expected audit fact to be preserved, count %d err %v", auditCount, err)
	}
}
