package audit_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"lapangango-api/internal/audit"
	"lapangango-api/internal/database"
)

func TestPlatformAuditService_RecordTx(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration tests. Set TEST_INTEGRATION=1 to run.")
	}
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	repo := audit.NewPlatformRepository()
	service := audit.NewPlatformService(repo)

	t.Run("Audit successfully in transaction", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        audit.ActionPlatformCommercialTermCreated,
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
			Metadata: map[string]any{
				"commission_bps": 1500,
				"label":          "Valid Term",
				"phase":          "TRIAL",
				"valid_from":     "2026-08-01T00:00:00Z",
			},
		})

		if err != nil {
			t.Fatalf("failed to record audit: %v", err)
		}

		var count int
		err = tx.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", correlationID).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("expected 1 log inside tx, got %d, err %v", count, err)
		}
	})

	t.Run("Invalid action rejected and rollback", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        "INVALID_ACTION",
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
		})

		if err == nil {
			t.Fatal("expected error for invalid action, got nil")
		}
	})

	t.Run("Invalid entity type rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        audit.ActionPlatformCommercialTermCreated,
			EntityType:    "INVALID_ENTITY",
			CorrelationID: &correlationID,
		})

		if err == nil {
			t.Fatal("expected error for invalid entity, got nil")
		}
	})

	t.Run("Secret value inside scalar rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        audit.ActionPlatformCommercialTermCreated,
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
			Metadata: map[string]any{
				"label": "Term with Authorization: Bearer secret_token",
			},
		})

		if err == nil {
			t.Fatal("expected error for secret in scalar metadata value, got nil")
		}
	})

	t.Run("Unknown metadata key rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        audit.ActionPlatformCommercialTermCreated,
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
			Metadata: map[string]any{
				"unknown_key": "123",
			},
		})

		if err == nil {
			t.Fatal("expected error for unknown metadata key, got nil")
		}
	})

	t.Run("Nested metadata rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        audit.ActionPlatformCommercialTermCreated,
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
			Metadata: map[string]any{
				"commission_bps": map[string]any{"nested": 123},
			},
		})

		if err == nil {
			t.Fatal("expected error for nested metadata, got nil")
		}
	})

	t.Run("Audit failure cancels domain write", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		domainID := uuid.New().String()

		_, err = tx.Exec(ctx, "INSERT INTO users (id, name, email, password_hash, role) VALUES ($1, 'Fail Test', $2, 'hash', 'CUSTOMER')", domainID, "failtest-"+domainID+"@test.local")
		if err != nil {
			t.Fatalf("failed domain write: %v", err)
		}

		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        "INVALID_ACTION", // this will cause error
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
		})

		if err != nil {
			tx.Rollback(ctx)
		} else {
			t.Fatal("Expected audit to fail but it succeeded")
		}

		var count int
		err = pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE id = $1", domainID).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query users after rollback: %v", err)
		}
		if count != 0 {
			t.Fatalf("Domain write was not cancelled, found %d rows", count)
		}
	})

	t.Run("Domain rollback removes successful audit", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SUPER_ADMIN",
			Action:        audit.ActionPlatformCommercialTermCreated,
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
			Metadata: map[string]any{
				"commission_bps": 1500,
				"label":          "Valid Term",
				"phase":          "STANDARD",
				"valid_from":     "2026-08-01T00:00:00Z",
			},
		})

		if err != nil {
			t.Fatalf("failed to record audit: %v", err)
		}

		var count int
		err = tx.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", correlationID).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query count in tx: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 log in tx, got %d", count)
		}

		err = tx.Rollback(ctx)
		if err != nil {
			t.Fatalf("failed to rollback tx: %v", err)
		}

		err = pool.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1", correlationID).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query count after rollback: %v", err)
		}
		if count != 0 {
			t.Fatalf("Audit survived domain rollback! count=%d", count)
		}
	})

	t.Run("Ownerless event valid", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		correlationID := uuid.New().String()
		err = service.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
			ActorRole:     "SYSTEM",
			Action:        audit.ActionPlatformCommercialTermLiveRejected,
			EntityType:    audit.EntityPlatformCommercialTerm,
			CorrelationID: &correlationID,
			Metadata: map[string]any{
				"reason": "LIVE_NOT_ALLOWED",
			},
		})

		if err != nil {
			t.Fatalf("failed to record ownerless audit: %v", err)
		}

		var count int
		err = tx.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE correlation_id = $1 AND owner_profile_id IS NULL AND actor_user_id IS NULL", correlationID).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("expected 1 ownerless log, got %d, err %v", count, err)
		}
	})

	t.Run("No update or delete paths", func(t *testing.T) {
		type updateService interface {
			Update(context.Context, audit.DBTX, string) error
		}

		_, hasUpdate := any(service).(updateService)
		if hasUpdate {
			t.Fatal("Service should not have Update method")
		}

		type deleteService interface {
			Delete(context.Context, audit.DBTX, string) error
		}

		_, hasDelete := any(service).(deleteService)
		if hasDelete {
			t.Fatal("Service should not have Delete method")
		}
	})
}
