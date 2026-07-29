package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnsafeRuntimeDatabaseRole = errors.New("unsafe runtime database role")

// ValidateRuntimeRole prevents the API process from running with migration or
// replication privileges. Migrations must be completed by the dedicated
// one-shot migration process before the API starts.
func ValidateRuntimeRole(ctx context.Context, pool *pgxpool.Pool) error {
	var (
		superuser           bool
		replication         bool
		createRole          bool
		createDatabase      bool
		bypassRLS           bool
		memberOfSuperuser   bool
		memberOfAnyRole     bool
		canCreateDatabase   bool
		canCreateInSchema   bool
		canSetReplicaMode   bool
		canAlterReplicaMode bool
		replicationMode     string
	)
	err := pool.QueryRow(ctx, `
		SELECT r.rolsuper,
		       r.rolreplication,
		       r.rolcreaterole,
		       r.rolcreatedb,
		       r.rolbypassrls,
		       EXISTS (
		           SELECT 1
		           FROM pg_roles privileged
		           WHERE privileged.rolsuper
		             AND pg_has_role(r.rolname, privileged.oid, 'MEMBER')
		       ),
		       EXISTS (
		           SELECT 1
		           FROM pg_auth_members membership
		           WHERE membership.member = r.oid
		       ),
		       has_database_privilege(r.rolname, current_database(), 'CREATE'),
		       has_schema_privilege(r.rolname, 'public', 'CREATE'),
		       has_parameter_privilege(r.rolname, 'session_replication_role', 'SET'),
		       has_parameter_privilege(r.rolname, 'session_replication_role', 'ALTER SYSTEM'),
		       current_setting('session_replication_role')
		FROM pg_roles r
		WHERE r.rolname = current_user
	`).Scan(
		&superuser,
		&replication,
		&createRole,
		&createDatabase,
		&bypassRLS,
		&memberOfSuperuser,
		&memberOfAnyRole,
		&canCreateDatabase,
		&canCreateInSchema,
		&canSetReplicaMode,
		&canAlterReplicaMode,
		&replicationMode,
	)
	if err != nil {
		return ErrUnsafeRuntimeDatabaseRole
	}
	if superuser || replication || createRole || createDatabase || bypassRLS ||
		memberOfSuperuser || memberOfAnyRole || canCreateDatabase ||
		canCreateInSchema || canSetReplicaMode || canAlterReplicaMode ||
		replicationMode != "origin" {
		return ErrUnsafeRuntimeDatabaseRole
	}

	var outboxExists bool
	if err := pool.QueryRow(ctx, `
		SELECT to_regclass('public.payment_provider_commands') IS NOT NULL
	`).Scan(&outboxExists); err != nil {
		return ErrUnsafeRuntimeDatabaseRole
	}
	if !outboxExists {
		return nil
	}

	var ownsOutbox, canTruncateOutbox bool
	if err := pool.QueryRow(ctx, `
		SELECT c.relowner = (SELECT oid FROM pg_roles WHERE rolname = current_user),
		       has_table_privilege(current_user, c.oid, 'TRUNCATE')
		FROM pg_class c
		WHERE c.oid = 'public.payment_provider_commands'::regclass
	`).Scan(&ownsOutbox, &canTruncateOutbox); err != nil {
		return ErrUnsafeRuntimeDatabaseRole
	}
	if ownsOutbox || canTruncateOutbox {
		return ErrUnsafeRuntimeDatabaseRole
	}
	return nil
}
