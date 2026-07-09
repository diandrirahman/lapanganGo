package audit_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"lapangango-api/internal/audit"
	"lapangango-api/internal/database"
)

func TestAuditRepository_InsertAndList(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set, skipping repository integration test")
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Setup Test Data
	ownerUserID := uuid.New().String()
	staffUserID := uuid.New().String()
	ownerProfileID := uuid.New().String()

	// Clean up after test
	defer func() {
		pool.Exec(ctx, "DELETE FROM owner_audit_logs WHERE owner_profile_id = $1", ownerProfileID)
		pool.Exec(ctx, "DELETE FROM owner_profiles WHERE id = $1", ownerProfileID)
		pool.Exec(ctx, "DELETE FROM users WHERE id IN ($1, $2)", ownerUserID, staffUserID)
	}()

	// 1. Create owner user
	ownerEmail := "audit-owner-" + ownerUserID + "@test.local"
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role)
		VALUES ($1, 'Audit Owner', $2, 'hash', 'OWNER')
	`, ownerUserID, ownerEmail)
	if err != nil {
		t.Fatalf("Failed to create test owner user: %v", err)
	}

	// 2. Create owner profile
	busName := "Audit Business " + ownerProfileID
	_, err = pool.Exec(ctx, `
		INSERT INTO owner_profiles (id, user_id, business_name, verification_status)
		VALUES ($1, $2, $3, 'APPROVED')
	`, ownerProfileID, ownerUserID, busName)
	if err != nil {
		t.Fatalf("Failed to create test owner profile: %v", err)
	}

	// 3. Create staff user
	staffEmail := "audit-staff-" + staffUserID + "@test.local"
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role)
		VALUES ($1, 'Audit Staff', $2, 'hash', 'CUSTOMER')
	`, staffUserID, staffEmail)
	if err != nil {
		t.Fatalf("Failed to create test staff user: %v", err)
	}

	repo := audit.NewRepository(pool)
	entityID := uuid.New().String()

	ip := "127.0.0.1"
	ua := "TestAgent"

	params := audit.CreateAuditLogParams{
		OwnerProfileID: ownerProfileID,
		ActorUserID:    staffUserID,
		ActorRole:      "STAFF",
		Action:         audit.ActionFinanceCreated,
		EntityType:     audit.EntityFinanceTransaction,
		EntityID:       &entityID,
		Metadata: map[string]any{
			"test_key": "test_value",
			"amount":   float64(100000), // JSON unmarshals numbers to float64
		},
		IPAddress: &ip,
		UserAgent: &ua,
	}

	err = repo.Create(ctx, params)
	if err != nil {
		t.Fatalf("Failed to create audit log: %v", err)
	}

	// Verify insertion
	logs, total, err := repo.ListByOwner(ctx, ownerProfileID, audit.AuditLogQuery{})
	if err != nil {
		t.Fatalf("Failed to list audit logs: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected 1 log, got %d", total)
	}

	if len(logs) == 0 {
		t.Fatal("Logs array is empty")
	}

	log := logs[0]
	if log.Actor.Role != "STAFF" {
		t.Errorf("Expected ActorRole STAFF, got %s", log.Actor.Role)
	}
	if log.Action != audit.ActionFinanceCreated {
		t.Errorf("Expected Action %s, got %s", audit.ActionFinanceCreated, log.Action)
	}
	if log.Metadata["test_key"] != "test_value" {
		t.Errorf("Expected metadata test_key=test_value, got %v", log.Metadata["test_key"])
	}
	if log.Metadata["amount"] != float64(100000) {
		t.Errorf("Expected metadata amount=100000, got %v", log.Metadata["amount"])
	}

	// Verify filter Action
	filteredLogs, _, err := repo.ListByOwner(ctx, ownerProfileID, audit.AuditLogQuery{
		Action: string(audit.ActionFinanceDeleted),
	})
	if err != nil {
		t.Fatalf("Failed to list filtered audit logs: %v", err)
	}
	if len(filteredLogs) != 0 {
		t.Errorf("Expected 0 filtered logs, got %d", len(filteredLogs))
	}

	// Verify filter EntityType
	entityLogs, entityTotal, err := repo.ListByOwner(ctx, ownerProfileID, audit.AuditLogQuery{
		EntityType: string(audit.EntityFinanceTransaction),
	})
	if err != nil {
		t.Fatalf("Failed to list entity audit logs: %v", err)
	}
	if entityTotal != 1 {
		t.Errorf("Expected 1 entity log, got %d", entityTotal)
	}
	if len(entityLogs) != 1 {
		t.Errorf("Expected 1 entity log array, got %d", len(entityLogs))
	}
}
