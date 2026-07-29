package database_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lib/pq"
	"lapangango-api/internal/database"
)

func TestRuntimeRoleValidationRejectsAdminAndAcceptsLeastPrivilege(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, m := setupMigrate(t, targetDSN)
	defer db.Close()
	defer m.Close()

	adminPool, err := pgxpool.New(context.Background(), targetDSN)
	if err != nil {
		t.Fatalf("open admin runtime-validation pool: %v", err)
	}
	if err := adminPool.Ping(context.Background()); err != nil {
		adminPool.Close()
		t.Fatalf("ping admin runtime-validation pool: %v", err)
	}
	if err := database.ValidateRuntimeRole(context.Background(), adminPool); !errors.Is(err, database.ErrUnsafeRuntimeDatabaseRole) {
		adminPool.Close()
		t.Fatalf("admin role validation = %v; want ErrUnsafeRuntimeDatabaseRole", err)
	}
	adminPool.Close()

	roleName := "lapangango_app_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pq.QuoteIdentifier(roleName)
	const rolePassword = "runtime-role-test-password"
	parsedTarget, err := url.Parse(targetDSN)
	if err != nil {
		t.Fatalf("parse target DSN: %v", err)
	}
	databaseName := strings.TrimPrefix(parsedTarget.Path, "/")
	if databaseName == "" {
		t.Fatal("target DSN has no database name")
	}

	if _, err := db.Exec(`
		CREATE ROLE ` + quotedRole + `
		LOGIN PASSWORD '` + rolePassword + `'
		NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS
	`); err != nil {
		t.Fatalf("create least-privilege test role: %v", err)
	}
	defer dropRuntimeTestRole(t, db, quotedRole)

	for _, grant := range []string{
		`GRANT CONNECT ON DATABASE ` + pq.QuoteIdentifier(databaseName) + ` TO ` + quotedRole,
		`GRANT USAGE ON SCHEMA public TO ` + quotedRole,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ` + quotedRole,
		`GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO ` + quotedRole,
	} {
		if _, err := db.Exec(grant); err != nil {
			t.Fatalf("grant least-privilege runtime access: %v", err)
		}
	}

	parsedTarget.User = url.UserPassword(roleName, rolePassword)
	runtimePool, err := pgxpool.New(context.Background(), parsedTarget.String())
	if err != nil {
		t.Fatalf("open least-privilege runtime pool: %v", err)
	}
	defer func() {
		if runtimePool != nil {
			runtimePool.Close()
		}
	}()
	if err := runtimePool.Ping(context.Background()); err != nil {
		t.Fatalf("ping least-privilege runtime pool: %v", err)
	}
	if err := database.ValidateRuntimeRole(context.Background(), runtimePool); err != nil {
		t.Fatalf("least-privilege role rejected: %v", err)
	}

	if _, err := db.Exec(`GRANT pg_read_all_data TO ` + quotedRole); err != nil {
		t.Fatalf("grant escalation test membership: %v", err)
	}
	if err := database.ValidateRuntimeRole(context.Background(), runtimePool); !errors.Is(err, database.ErrUnsafeRuntimeDatabaseRole) {
		t.Fatalf("role-membership validation = %v; want ErrUnsafeRuntimeDatabaseRole", err)
	}
	if _, err := db.Exec(`REVOKE pg_read_all_data FROM ` + quotedRole); err != nil {
		t.Fatalf("revoke escalation test membership: %v", err)
	}

	if _, err := db.Exec(`GRANT SET ON PARAMETER session_replication_role TO ` + quotedRole); err != nil {
		t.Fatalf("grant session_replication_role test privilege: %v", err)
	}
	if err := database.ValidateRuntimeRole(context.Background(), runtimePool); !errors.Is(err, database.ErrUnsafeRuntimeDatabaseRole) {
		t.Fatalf("parameter-privilege validation = %v; want ErrUnsafeRuntimeDatabaseRole", err)
	}
	if _, err := db.Exec(`REVOKE SET ON PARAMETER session_replication_role FROM ` + quotedRole); err != nil {
		t.Fatalf("revoke session_replication_role test privilege: %v", err)
	}

	if _, err := db.Exec(`GRANT ALTER SYSTEM ON PARAMETER session_replication_role TO ` + quotedRole); err != nil {
		t.Fatalf("grant ALTER SYSTEM test privilege: %v", err)
	}
	if err := database.ValidateRuntimeRole(context.Background(), runtimePool); !errors.Is(err, database.ErrUnsafeRuntimeDatabaseRole) {
		t.Fatalf("ALTER SYSTEM privilege validation = %v; want ErrUnsafeRuntimeDatabaseRole", err)
	}
	if _, err := db.Exec(`REVOKE ALTER SYSTEM ON PARAMETER session_replication_role FROM ` + quotedRole); err != nil {
		t.Fatalf("revoke ALTER SYSTEM test privilege: %v", err)
	}

	runtimePool.Close()
	runtimePool = nil
	if _, err := db.Exec(`ALTER ROLE ` + quotedRole + ` SET session_replication_role = replica`); err != nil {
		t.Fatalf("configure replica-mode test role: %v", err)
	}
	runtimePool, err = pgxpool.New(context.Background(), parsedTarget.String())
	if err != nil {
		t.Fatalf("open replica-mode runtime pool: %v", err)
	}
	if err := runtimePool.Ping(context.Background()); err != nil {
		t.Fatalf("ping replica-mode runtime pool: %v", err)
	}
	var replicationMode string
	if err := runtimePool.QueryRow(context.Background(), `SELECT current_setting('session_replication_role')`).Scan(&replicationMode); err != nil {
		t.Fatalf("read active replication mode: %v", err)
	}
	if replicationMode != "replica" {
		t.Fatalf("active replication mode = %q; want replica", replicationMode)
	}
	if err := database.ValidateRuntimeRole(context.Background(), runtimePool); !errors.Is(err, database.ErrUnsafeRuntimeDatabaseRole) {
		t.Fatalf("active replica-mode validation = %v; want ErrUnsafeRuntimeDatabaseRole", err)
	}
	if guardedPool, err := database.NewPostgresPool(context.Background(), parsedTarget.String()); !errors.Is(err, database.ErrUnsafeRuntimeDatabaseRole) {
		if guardedPool != nil {
			guardedPool.Close()
		}
		t.Fatalf("replica-mode guarded pool error = %v; want ErrUnsafeRuntimeDatabaseRole", err)
	}
	runtimePool.Close()
	runtimePool = nil
	if _, err := db.Exec(`ALTER ROLE ` + quotedRole + ` RESET session_replication_role`); err != nil {
		t.Fatalf("reset replica-mode test role: %v", err)
	}
	runtimePool, err = pgxpool.New(context.Background(), parsedTarget.String())
	if err != nil {
		t.Fatalf("reopen least-privilege runtime pool: %v", err)
	}
	if err := runtimePool.Ping(context.Background()); err != nil {
		t.Fatalf("ping reset least-privilege runtime pool: %v", err)
	}
	if err := database.ValidateRuntimeRole(context.Background(), runtimePool); err != nil {
		t.Fatalf("least-privilege role rejected after escalation cleanup: %v", err)
	}
	if _, err := runtimePool.Exec(context.Background(), `SET session_replication_role = replica`); err == nil {
		t.Fatal("least-privilege role changed session_replication_role")
	}
	if _, err := runtimePool.Exec(context.Background(), `TRUNCATE payment_provider_commands`); err == nil {
		t.Fatal("least-privilege role truncated the outbox")
	}
}

func dropRuntimeTestRole(t *testing.T, db *sql.DB, quotedRole string) {
	t.Helper()
	if _, err := db.Exec(`DROP OWNED BY ` + quotedRole); err != nil {
		t.Errorf("drop runtime test role privileges: %v", err)
		return
	}
	if _, err := db.Exec(`DROP ROLE ` + quotedRole); err != nil {
		t.Errorf("drop runtime test role: %v", err)
	}
}
