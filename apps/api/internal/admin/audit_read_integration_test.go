package admin_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"lapangango-api/internal/admin"
	"lapangango-api/internal/database"
)

func TestAdminRepositoryAuditReadScopes(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("set TEST_INTEGRATION=1 to run read-only audit repository verification")
	}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, databaseURL)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	defer pool.Close()

	var ownerTotal, platformTotal int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM owner_audit_logs").Scan(&ownerTotal))
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs").Scan(&platformTotal))

	repo := admin.NewRepository(pool)
	ownerLogs, gotOwnerTotal, err := repo.GetAuditLogs(ctx, admin.AuditLogQuery{Scope: "OWNER", PaginationQuery: admin.PaginationQuery{Limit: 100}})
	require.NoError(t, err)
	require.Equal(t, ownerTotal, gotOwnerTotal)
	require.LessOrEqual(t, len(ownerLogs), ownerTotal)

	platformLogs, gotPlatformTotal, err := repo.GetAuditLogs(ctx, admin.AuditLogQuery{Scope: "PLATFORM", PaginationQuery: admin.PaginationQuery{Limit: 100}})
	require.NoError(t, err)
	require.Equal(t, platformTotal, gotPlatformTotal)
	for _, log := range platformLogs {
		require.Equal(t, "PLATFORM", log.Scope)
		require.Nil(t, log.IPAddress)
		require.Nil(t, log.UserAgent)
	}

	allLogs, gotAllTotal, err := repo.GetAuditLogs(ctx, admin.AuditLogQuery{Scope: "ALL", PaginationQuery: admin.PaginationQuery{Limit: 100}})
	require.NoError(t, err)
	require.Equal(t, ownerTotal+platformTotal, gotAllTotal)
	require.LessOrEqual(t, len(allLogs), gotAllTotal)
	for i := 1; i < len(allLogs); i++ {
		previous := allLogs[i-1]
		current := allLogs[i]
		require.False(t, current.CreatedAt.After(previous.CreatedAt), "audit rows are not ordered by created_at DESC")
		if current.CreatedAt.Equal(previous.CreatedAt) {
			require.LessOrEqual(t, current.ID, previous.ID, "audit rows are not ordered by id DESC")
		}
	}

	empty, emptyTotal, err := repo.GetAuditLogs(ctx, admin.AuditLogQuery{
		Scope:           "PLATFORM",
		Action:          "__NO_SUCH_AUDIT_ACTION__",
		EntityType:      "PLATFORM_COMMERCIAL_TERM",
		PaginationQuery: admin.PaginationQuery{Limit: 20},
	})
	require.NoError(t, err)
	require.Equal(t, 0, emptyTotal)
	require.NotNil(t, empty)
	require.Empty(t, empty)
}

func TestAdminRepositoryAuditOutOfRangePagePreservesTotal(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("set TEST_INTEGRATION=1 to run audit pagination verification")
	}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, databaseURL)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}

	firstID := uuid.NewString()
	secondID := uuid.NewString()
	action := "TASK_2C_06_PAGINATION_" + firstID
	const entityType = "TASK_2C_06_AUDIT_FIXTURE"
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, cleanupErr := pool.Exec(cleanupCtx, "DELETE FROM platform_audit_logs WHERE id IN ($1, $2)", firstID, secondID)
		if cleanupErr != nil {
			t.Errorf("failed to clean audit pagination fixtures: %v", cleanupErr)
		}
		pool.Close()
	}()

	_, err = pool.Exec(ctx, `
		INSERT INTO platform_audit_logs (id, actor_role, action, entity_type, metadata, created_at)
		VALUES ($1, 'SUPER_ADMIN', $3, $4, '{}'::jsonb, now() - interval '1 millisecond'),
		       ($2, 'SUPER_ADMIN', $3, $4, '{}'::jsonb, now())
	`, firstID, secondID, action, entityType)
	require.NoError(t, err)

	repo := admin.NewRepository(pool)
	logs, total, err := repo.GetAuditLogs(ctx, admin.AuditLogQuery{
		Scope:      "PLATFORM",
		Action:     action,
		EntityType: entityType,
		PaginationQuery: admin.PaginationQuery{
			Page:  3,
			Limit: 1,
		},
	})
	require.NoError(t, err)
	require.Empty(t, logs)
	require.Equal(t, 2, total)
}
