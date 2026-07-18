package platformfinance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

func TestExpenseServicePostAndVoidAreAtomicExactAndIdempotent(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	actorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	journalService, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	service := NewExpenseService(NewExpenseRepository(pool), pool, audit.NewPlatformService(audit.NewPlatformRepository()), journalService)

	created, _, err := service.CreateDraft(context.Background(), CreateExpenseRequest{
		AmountRupiah: "125000", Currency: "IDR", OccurredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		Category: "OFFICE_ADMIN", PaymentAccount: "FUNDING_CLEARING", Vendor: "Post Vendor",
		ExternalReference: "POST-" + actorID[:8], Description: "Post and void test expense",
	}, "create-post-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	_, _, err = service.ApproveDraft(context.Background(), created.ID, "approve-post-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)

	posted, replayed, err := service.PostExpense(context.Background(), created.ID, "post-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, "POSTED", posted.Status)
	require.NotNil(t, posted.PostedJournalID)
	require.NotNil(t, posted.PostedAt)

	replayedPosted, replayed, err := service.PostExpense(context.Background(), created.ID, "post-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, posted.ID, replayedPosted.ID)
	_, _, err = service.PostExpense(context.Background(), created.ID, "post-key-2-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	assert.ErrorIs(t, err, ErrExpenseConflict)

	voided, replayed, err := service.VoidExpense(context.Background(), created.ID, "supplier correction", "void-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, "VOID", voided.Status)
	require.NotNil(t, voided.VoidJournalID)
	require.NotNil(t, voided.VoidedAt)
	var reversalEffectiveAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT effective_at FROM platform_journals WHERE id = $1`, *voided.VoidJournalID).Scan(&reversalEffectiveAt))
	assert.Equal(t, voided.VoidedAt.UTC(), reversalEffectiveAt.UTC())

	replayedVoided, replayed, err := service.VoidExpense(context.Background(), created.ID, "supplier correction", "void-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, voided.ID, replayedVoided.ID)
	_, _, err = service.VoidExpense(context.Background(), created.ID, "another reason", "void-key-2-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	assert.ErrorIs(t, err, ErrExpenseConflict)

	var reversalCount, postedAuditCount, voidedAuditCount, reversalAuditCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, *posted.PostedJournalID).Scan(&reversalCount))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, created.ID, audit.ActionPlatformExpensePosted).Scan(&postedAuditCount))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_audit_logs WHERE entity_id = $1 AND action = $2`, created.ID, audit.ActionPlatformExpenseVoided).Scan(&voidedAuditCount))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_audit_logs WHERE action = $1 AND correlation_id = $2`, audit.ActionPlatformFinanceJournalReversed, "journal.reversed:"+*posted.PostedJournalID).Scan(&reversalAuditCount))
	assert.Equal(t, 1, reversalCount)
	assert.Equal(t, 1, postedAuditCount)
	assert.Equal(t, 1, voidedAuditCount)
	assert.Equal(t, 1, reversalAuditCount)

	var mismatchCount int
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM (
			SELECT account_code, owner_profile_id, side, amount_rupiah FROM platform_ledger_entries WHERE journal_id = $1
			EXCEPT ALL
			SELECT account_code, owner_profile_id, CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END, amount_rupiah FROM platform_ledger_entries WHERE journal_id = $2
			UNION ALL
			SELECT account_code, owner_profile_id, CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END, amount_rupiah FROM platform_ledger_entries WHERE journal_id = $1
			EXCEPT ALL
			SELECT account_code, owner_profile_id, side, amount_rupiah FROM platform_ledger_entries WHERE journal_id = $2
		) differences`, *posted.PostedJournalID, *voided.VoidJournalID).Scan(&mismatchCount))
	assert.Equal(t, 0, mismatchCount)
}

func TestExpenseServicePostAndVoidTimeoutAfterCommitReplay(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	actorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	journalService, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	service := NewExpenseService(NewExpenseRepository(pool), pool, audit.NewPlatformService(audit.NewPlatformRepository()), journalService)
	created, _, err := service.CreateDraft(context.Background(), CreateExpenseRequest{
		AmountRupiah: "88000", Currency: "IDR", OccurredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		Category: "OTHER", PaymentAccount: "FUNDING_CLEARING", Vendor: "Timeout Vendor",
		ExternalReference: "TIMEOUT-" + actorID[:8], Description: "Timeout replay expense",
	}, "create-timeout-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	_, _, err = service.ApproveDraft(context.Background(), created.ID, "approve-timeout-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)

	posted, lostResponseErr := runExpenseActionWithCommitTimeout(func() (*PlatformExpense, bool, error) {
		return service.PostExpense(context.Background(), created.ID, "post-timeout-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	})
	assert.ErrorIs(t, lostResponseErr, context.DeadlineExceeded)
	replayedPost, replayed, err := service.PostExpense(context.Background(), created.ID, "post-timeout-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, posted.ID, replayedPost.ID)
	assert.Equal(t, 1, countExpenseJournals(t, pool, *posted.PostedJournalID))

	voided, lostResponseErr := runExpenseActionWithCommitTimeout(func() (*PlatformExpense, bool, error) {
		return service.VoidExpense(context.Background(), created.ID, "timeout correction", "void-timeout-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	})
	assert.ErrorIs(t, lostResponseErr, context.DeadlineExceeded)
	replayedVoid, replayed, err := service.VoidExpense(context.Background(), created.ID, "timeout correction", "void-timeout-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, voided.ID, replayedVoid.ID)
	assert.Equal(t, 1, countExpenseJournals(t, pool, *voided.VoidJournalID))
}

func runExpenseActionWithCommitTimeout(action func() (*PlatformExpense, bool, error)) (*PlatformExpense, error) {
	item, replayed, err := action()
	if err != nil {
		return nil, err
	}
	if replayed {
		return nil, errors.New("expected first action call to commit a new transition")
	}
	// Model the ambiguous outcome where the database commit completed but the
	// transport acknowledgement was lost before the client received it.
	return item, context.DeadlineExceeded
}

func TestExpenseServicePostAndVoidConcurrentSameKeyAreSingleTransitions(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	actorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	journalService, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	service := NewExpenseService(NewExpenseRepository(pool), pool, audit.NewPlatformService(audit.NewPlatformRepository()), journalService)
	created, _, err := service.CreateDraft(context.Background(), CreateExpenseRequest{
		AmountRupiah: "99000", Currency: "IDR", OccurredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		Category: "OTHER", PaymentAccount: "ACCOUNTS_PAYABLE", Vendor: "Concurrent Vendor",
		ExternalReference: "CONCURRENT-" + actorID[:8], Description: "Concurrent post and void",
	}, "create-concurrent-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)
	_, _, err = service.ApproveDraft(context.Background(), created.ID, "approve-concurrent-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	require.NoError(t, err)

	type actionResult struct {
		item     *PlatformExpense
		replayed bool
		err      error
	}
	runConcurrent := func(run func() (*PlatformExpense, bool, error)) []actionResult {
		results := make([]actionResult, 2)
		var group sync.WaitGroup
		group.Add(2)
		for index := range results {
			go func(index int) {
				defer group.Done()
				results[index].item, results[index].replayed, results[index].err = run()
			}(index)
		}
		group.Wait()
		return results
	}

	postResults := runConcurrent(func() (*PlatformExpense, bool, error) {
		return service.PostExpense(context.Background(), created.ID, "concurrent-post-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	})
	var posted *PlatformExpense
	for _, result := range postResults {
		require.NoError(t, result.err)
		require.NotNil(t, result.item)
		if posted == nil {
			posted = result.item
		} else {
			assert.Equal(t, posted.ID, result.item.ID)
		}
	}
	require.NotNil(t, posted)
	require.NotNil(t, posted.PostedJournalID)
	assert.Equal(t, 1, countExpenseJournals(t, pool, *posted.PostedJournalID))

	voidResults := runConcurrent(func() (*PlatformExpense, bool, error) {
		return service.VoidExpense(context.Background(), created.ID, "concurrent correction", "concurrent-void-key-"+actorID, actorID, "SUPER_ADMIN", "127.0.0.1", "integration-test")
	})
	for _, result := range voidResults {
		require.NoError(t, result.err)
		require.NotNil(t, result.item)
		assert.Equal(t, "VOID", result.item.Status)
	}
	var voidJournalID string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT void_journal_id::text FROM platform_expenses WHERE id = $1`, created.ID).Scan(&voidJournalID))
	assert.Equal(t, 1, countExpenseJournals(t, pool, voidJournalID))
}

func countExpenseJournals(t *testing.T, pool *pgxpool.Pool, journalID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM platform_journals WHERE id = $1`, journalID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
