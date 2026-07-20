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
	t.Skip("TEST_ROLLBACK_HARDENING_DISPOSABLE has invalid value, skipping.")
	return ""
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
		JWTSecret: "test-secret",
		JWTExpiresInHours: 1,
		GeneralRateLimitPerMinute: 100,
		AuthRateLimitPerMinute: 100,
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

	testCases := []struct {
		name       string
		token      string
		expected   int
	}{
		{"No JWT", "", http.StatusUnauthorized},
		{"Invalid JWT", "invalid.token.here", http.StatusUnauthorized},
		{"Inactive SUPER_ADMIN", generateToken(inactiveSuperAdminID, inactiveSuperAdminEmail, "SUPER_ADMIN"), http.StatusForbidden},
		{"Active CUSTOMER", generateToken(activeCustomerID, activeCustomerEmail, "CUSTOMER"), http.StatusForbidden},
		{"Active OWNER", generateToken(activeOwnerID, activeOwnerEmail, "OWNER"), http.StatusForbidden},
		{"Active SUPER_ADMIN", generateToken(activeSuperAdminID, activeSuperAdminEmail, "SUPER_ADMIN"), http.StatusOK},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/admin/finance/summary", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, w.Code)
			}
		})
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

	dbPool, err := pgxpool.New(context.Background(), targetDSN)
	if err != nil {
		t.Fatalf("failed to create pgxpool: %v", err)
	}
	defer dbPool.Close()

	activeSuperAdminID, activeSuperAdminEmail := seedUser(t, db, "SUPER_ADMIN", "ACTIVE")

	// Create router with admin DISABLED
	gin.SetMode(gin.TestMode)
	cfg := config.Config{
		PlatformFinanceAdminEnabled: false,
		JWTSecret: "test-secret",
		JWTExpiresInHours: 1,
		GeneralRateLimitPerMinute: 100,
		AuthRateLimitPerMinute: 100,
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

	routes := []string{
		"/admin/finance/summary",
		"/admin/finance/breakdown",
		"/admin/finance/expenses",
		"/admin/finance/journals",
	}

	for _, route := range routes {
		req, _ := http.NewRequest("GET", route, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for disabled route %s, got %d", route, w.Code)
		}
	}

	// Assert facts remain identical
	var count int
	err = db.QueryRow("SELECT count(*) FROM platform_audit_logs WHERE id = $1", auditID).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected audit fact to be preserved, count %d err %v", count, err)
	}
}
