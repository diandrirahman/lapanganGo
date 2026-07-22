package platformfinance

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// TestReconciliationBoundarySuiteDisposable owns the complete database
// lifecycle so the required fixture suite never depends on a shared/local
// schema being at migration 24.
func TestReconciliationBoundarySuiteDisposable(t *testing.T) {
	if os.Getenv("TEST_RECONCILIATION_DISPOSABLE") != "1" {
		t.Skip("set TEST_RECONCILIATION_DISPOSABLE=1 with RECONCILIATION_ADMIN_DATABASE_URL")
	}
	adminDSN := os.Getenv("RECONCILIATION_ADMIN_DATABASE_URL")
	if adminDSN == "" {
		t.Fatal("RECONCILIATION_ADMIN_DATABASE_URL is required for disposable reconciliation regression")
	}

	parsed, err := url.Parse(adminDSN)
	if err != nil || parsed.Path == "" || parsed.Path == "/" {
		t.Fatal("RECONCILIATION_ADMIN_DATABASE_URL must contain an admin database name")
	}
	dbName := "lapangango_reconciliation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatal("open reconciliation admin database")
	}
	defer admin.Close()
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create disposable reconciliation database: %v", err)
	}
	defer func() {
		_, _ = admin.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, dbName)
		if _, err := admin.Exec("DROP DATABASE " + dbName); err != nil {
			t.Errorf("drop disposable reconciliation database: %v", err)
		}
	}()

	parsed.Path = "/" + dbName
	targetDSN := parsed.String()
	target, err := sql.Open("postgres", targetDSN)
	if err != nil {
		t.Fatal("open disposable reconciliation database")
	}
	driver, err := postgres.WithInstance(target, &postgres.Config{})
	if err != nil {
		t.Fatalf("create migration driver: %v", err)
	}
	migrator, err := migrate.NewWithDatabaseInstance("file://../../../../db/migrations", "postgres", driver)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate disposable reconciliation database: %v", err)
	}
	if sourceErr, databaseErr := migrator.Close(); sourceErr != nil || databaseErr != nil {
		t.Fatalf("close migrator: source=%v database=%v", sourceErr, databaseErr)
	}

	t.Setenv("TEST_INTEGRATION", "1")
	t.Setenv("TEST_DATABASE_URL", targetDSN)
	TestReconciliationBoundarySuite(t)
}
