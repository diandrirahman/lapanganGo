package admin_test

import (
	"context"
	"os"
	"testing"
	"time"

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
