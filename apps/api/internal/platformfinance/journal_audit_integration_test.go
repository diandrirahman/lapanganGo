package platformfinance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/audit"
)

type failingFinanceAuditService struct {
	err error
}

func (f failingFinanceAuditService) Record(_ context.Context, _ audit.DBTX, _ audit.CreatePlatformAuditLogParams) error {
	return f.err
}

func TestAuditedJournalReversalCommitsJournalAndOneAuditMarker(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	journalService, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	auditService := audit.NewPlatformService(audit.NewPlatformRepository())
	coordinator, err := NewAuditedJournalService(pool, journalService, auditService)
	require.NoError(t, err)

	source := createCommittedSourceJournal(t, pool, journalService, "test.audited-reversal:"+uuid.NewString())
	params := integrationReverseJournalParams(source.ID, "audited correction")
	actor := JournalAuditContext{ActorRole: "SYSTEM"}

	first, err := coordinator.ReverseJournal(ctx, params, actor)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, source.ID, *first.ReversesJournalID)
	assertExactReversalRows(t, pool, source.ID, first.ID)

	var auditCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM platform_audit_logs
		WHERE action = $1 AND correlation_id = $2
	`, audit.ActionPlatformFinanceJournalReversed, "journal.reversed:"+source.ID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)

	retry, err := coordinator.ReverseJournal(ctx, params, actor)
	require.NoError(t, err)
	assert.Equal(t, first.ID, retry.ID)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM platform_audit_logs
		WHERE action = $1 AND correlation_id = $2
	`, audit.ActionPlatformFinanceJournalReversed, "journal.reversed:"+source.ID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount, "same reversal retry must not duplicate audit marker")
}

func TestAuditedJournalReversalAuditFailureRollsBackJournal(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	journalService, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	coordinator, err := NewAuditedJournalService(pool, journalService, failingFinanceAuditService{err: errors.New("injected audit failure")})
	require.NoError(t, err)

	source := createCommittedSourceJournal(t, pool, journalService, "test.audited-reversal-failure:"+uuid.NewString())
	params := integrationReverseJournalParams(source.ID, "rollback correction")
	_, err = coordinator.ReverseJournal(ctx, params, JournalAuditContext{ActorRole: "SYSTEM"})
	require.Error(t, err)

	var reversalCount, entryCount, auditCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, source.ID).Scan(&reversalCount))
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM platform_ledger_entries e
		JOIN platform_journals j ON j.id = e.journal_id
		WHERE j.reverses_journal_id = $1
	`, source.ID).Scan(&entryCount))
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM platform_audit_logs
		WHERE action = $1 AND correlation_id = $2
	`, audit.ActionPlatformFinanceJournalReversed, "journal.reversed:"+source.ID).Scan(&auditCount))
	assert.Zero(t, reversalCount)
	assert.Zero(t, entryCount)
	assert.Zero(t, auditCount)
}

func TestPlatformAuditRollbackRemovesJournalAndAuditTogether(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	journalService, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	auditService := audit.NewPlatformService(audit.NewPlatformRepository())
	source := createCommittedSourceJournal(t, pool, journalService, "test.domain-rollback:"+uuid.NewString())

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	reversal, err := journalService.ReverseJournal(ctx, tx, integrationReverseJournalParams(source.ID, "manual rollback"))
	require.NoError(t, err)
	correlation := "journal.reversed:" + source.ID
	require.NoError(t, auditService.Record(ctx, tx, audit.CreatePlatformAuditLogParams{
		ActorRole:     "SYSTEM",
		Action:        audit.ActionPlatformFinanceJournalReversed,
		EntityType:    audit.EntityPlatformFinanceJournal,
		EntityID:      &reversal.ID,
		CorrelationID: &correlation,
		Metadata: map[string]any{
			"source_journal_id": source.ID,
			"effective_at":      reversal.EffectiveAt.UTC().Format(time.RFC3339Nano),
		},
	}))
	require.NoError(t, tx.Rollback(ctx))

	var journalCount, auditCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, source.ID).Scan(&journalCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_audit_logs WHERE correlation_id = $1`, correlation).Scan(&auditCount))
	assert.Zero(t, journalCount)
	assert.Zero(t, auditCount)
}

func TestLiveWriteGuardRejectsWithoutCreatingJournalAndIsIdempotent(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	auditService := audit.NewPlatformService(audit.NewPlatformRepository())
	guard, err := NewLiveWriteGuard(pool, auditService)
	require.NoError(t, err)
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	attempt := LiveWriteAttempt{
		ActorRole:          "SYSTEM",
		CorrelationID:      "live.guard:" + uuid.NewString(),
		RequestFingerprint: fingerprint,
		WriteKind:          "JOURNAL",
	}

	var journalsBefore, entriesBefore int
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals").Scan(&journalsBefore))
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries").Scan(&entriesBefore))

	err = guard.RejectPrematureLiveWrite(ctx, attempt)
	require.ErrorIs(t, err, ErrPlatformFinanceLiveWriteRejected)

	var journalsAfter, entriesAfter, auditCount int
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals").Scan(&journalsAfter))
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries").Scan(&entriesAfter))
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_audit_logs WHERE action = $1 AND correlation_id = $2`, audit.ActionPlatformFinanceLiveWriteRejected, attempt.CorrelationID).Scan(&auditCount))
	assert.Equal(t, journalsBefore, journalsAfter)
	assert.Equal(t, entriesBefore, entriesAfter)
	assert.Equal(t, 1, auditCount)

	err = guard.RejectPrematureLiveWrite(ctx, attempt)
	require.ErrorIs(t, err, ErrPlatformFinanceLiveWriteRejected)
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_audit_logs WHERE action = $1 AND correlation_id = $2`, audit.ActionPlatformFinanceLiveWriteRejected, attempt.CorrelationID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)

	attempt.RequestFingerprint = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	err = guard.RejectPrematureLiveWrite(ctx, attempt)
	require.ErrorIs(t, err, ErrPlatformFinanceAuditConflict)
}

func TestLiveWriteGuardAuditFailureDoesNotLeaveMarker(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	guard, err := NewLiveWriteGuard(pool, failingFinanceAuditService{err: errors.New("injected LIVE audit failure")})
	require.NoError(t, err)
	attempt := LiveWriteAttempt{
		ActorRole:          "SYSTEM",
		CorrelationID:      "live.guard.failure:" + uuid.NewString(),
		RequestFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		WriteKind:          "JOURNAL",
	}

	require.Error(t, guard.RejectPrematureLiveWrite(ctx, attempt))
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_audit_logs WHERE correlation_id = $1`, attempt.CorrelationID).Scan(&auditCount))
	assert.Zero(t, auditCount)
}

func TestLiveWriteGuardReplayScopeMismatchConflicts(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	guard, err := NewLiveWriteGuard(pool, audit.NewPlatformService(audit.NewPlatformRepository()))
	require.NoError(t, err)
	attempt := LiveWriteAttempt{
		ActorRole:          "SYSTEM",
		CorrelationID:      "live.guard.scope:" + uuid.NewString(),
		RequestFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		WriteKind:          "JOURNAL",
	}

	require.ErrorIs(t, guard.RejectPrematureLiveWrite(ctx, attempt), ErrPlatformFinanceLiveWriteRejected)

	ownerID := uuid.NewString()
	attempt.OwnerProfileID = &ownerID
	require.ErrorIs(t, guard.RejectPrematureLiveWrite(ctx, attempt), ErrPlatformFinanceAuditConflict)

	attempt.OwnerProfileID = nil
	venueID := uuid.NewString()
	attempt.VenueID = &venueID
	require.ErrorIs(t, guard.RejectPrematureLiveWrite(ctx, attempt), ErrPlatformFinanceAuditConflict)

	var auditCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_audit_logs WHERE action = $1 AND correlation_id = $2`, audit.ActionPlatformFinanceLiveWriteRejected, attempt.CorrelationID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
}

func TestLiveWriteGuardConcurrentSameAttemptCreatesOneMarker(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	guard, err := NewLiveWriteGuard(pool, audit.NewPlatformService(audit.NewPlatformRepository()))
	require.NoError(t, err)
	attempt := LiveWriteAttempt{
		ActorRole:          "SYSTEM",
		CorrelationID:      "live.guard.concurrent:" + uuid.NewString(),
		RequestFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		WriteKind:          "JOURNAL",
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- guard.RejectPrematureLiveWrite(ctx, attempt)
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		require.ErrorIs(t, result, ErrPlatformFinanceLiveWriteRejected)
	}

	var auditCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_audit_logs WHERE action = $1 AND correlation_id = $2`, audit.ActionPlatformFinanceLiveWriteRejected, attempt.CorrelationID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
}
