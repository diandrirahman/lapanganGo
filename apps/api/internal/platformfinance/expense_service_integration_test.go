package platformfinance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/audit"
)

func TestExpenseServiceCreateDraftUsesAuditAndIdempotency(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	actorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	service := NewExpenseService(NewExpenseRepository(pool), pool, audit.NewPlatformService(audit.NewPlatformRepository()))
	reference := "INV-" + actorID[:8]
	req := CreateExpenseRequest{
		AmountRupiah: "250000", Currency: "IDR", OccurredAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
		Category: "INFRASTRUCTURE", PaymentAccount: "FUNDING_CLEARING", Vendor: "Cloud Vendor", ExternalReference: reference, Description: "Hosting",
	}

	first, replayed, err := service.CreateDraft(context.Background(), req, "expense-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, "DRAFT", first.Status)
	assert.Equal(t, "250000", first.AmountRupiah)

	second, replayed, err := service.CreateDraft(context.Background(), req, "expense-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, first.ID, second.ID)

	conflicting := req
	conflicting.Description = "different payload"
	_, _, err = service.CreateDraft(context.Background(), conflicting, "expense-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	assert.ErrorIs(t, err, ErrExpenseConflict)

	var expenseCount, auditCount, idempotencyCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_expenses WHERE id = $1`, first.ID).Scan(&expenseCount))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, first.ID, audit.ActionPlatformExpenseCreated).Scan(&auditCount))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_expense_idempotency WHERE expense_id = $1`, first.ID).Scan(&idempotencyCount))
	assert.Equal(t, 1, expenseCount)
	assert.Equal(t, 1, auditCount)
	assert.Equal(t, 1, idempotencyCount)

	page, err := service.ListExpenses(context.Background(), ExpenseListQuery{Status: "DRAFT", Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, 1, page.TotalItems)
	assert.Len(t, page.Items, 1)

	_, _, err = service.CreateDraft(context.Background(), CreateExpenseRequest{AmountRupiah: "0"}, "invalid", actorID, "SUPER_ADMIN", "", "")
	var validationErr *ExpenseValidationError
	assert.True(t, errors.As(err, &validationErr))
}

func TestExpenseServiceCancelAndApproveAreAtomicAndIdempotent(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	actorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	service := NewExpenseService(NewExpenseRepository(pool), pool, audit.NewPlatformService(audit.NewPlatformRepository()))
	baseRequest := func(reference string) CreateExpenseRequest {
		return CreateExpenseRequest{
			AmountRupiah: "125000", Currency: "IDR", OccurredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
			Category: "OFFICE_ADMIN", PaymentAccount: "FUNDING_CLEARING", Vendor: "Action Vendor", ExternalReference: reference, Description: "Action test expense",
		}
	}

	cancelled, _, err := service.CreateDraft(context.Background(), baseRequest("CANCEL-"+actorID[:8]), "create-cancel-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	firstCancel, replayed, err := service.CancelDraft(context.Background(), cancelled.ID, "duplicate invoice", "cancel-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, "CANCELLED", firstCancel.Status)
	assert.Equal(t, "duplicate invoice", *firstCancel.CancelReason)
	secondCancel, replayed, err := service.CancelDraft(context.Background(), cancelled.ID, "duplicate invoice", "cancel-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, firstCancel.ID, secondCancel.ID)
	_, _, err = service.CancelDraft(context.Background(), cancelled.ID, "different reason", "cancel-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	assert.ErrorIs(t, err, ErrExpenseConflict)

	approved, _, err := service.CreateDraft(context.Background(), baseRequest("APPROVE-"+actorID[:8]), "create-approve-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	firstApprove, replayed, err := service.ApproveDraft(context.Background(), approved.ID, "approve-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, "APPROVED", firstApprove.Status)
	assert.NotNil(t, firstApprove.ApprovedAt)
	secondApprove, replayed, err := service.ApproveDraft(context.Background(), approved.ID, "approve-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, firstApprove.ID, secondApprove.ID)
	_, _, err = service.CancelDraft(context.Background(), approved.ID, "too late", "cancel-after-approve-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	assert.ErrorIs(t, err, ErrExpenseConflict)

	var cancelledAudit, approvedAudit, idempotencyCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, cancelled.ID, audit.ActionPlatformExpenseCancelled).Scan(&cancelledAudit))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, approved.ID, audit.ActionPlatformExpenseApproved).Scan(&approvedAudit))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_expense_idempotency WHERE expense_id = $1`, cancelled.ID).Scan(&idempotencyCount))
	assert.Equal(t, 1, cancelledAudit)
	assert.Equal(t, 1, approvedAudit)
	assert.Equal(t, 2, idempotencyCount)
}
