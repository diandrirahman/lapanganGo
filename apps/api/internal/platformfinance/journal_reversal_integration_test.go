package platformfinance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func integrationReverseJournalParams(journalID, reason string) ReverseJournalParams {
	return ReverseJournalParams{
		JournalID:   journalID,
		Reason:      reason,
		EffectiveAt: time.Now().UTC(),
		Metadata: map[string]string{
			"reason_code": "correction",
		},
	}
}

func createCommittedSourceJournal(t *testing.T, pool *pgxpool.Pool, service JournalService, eventKey string) *PostedJournal {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	params := integrationPostJournalParams(eventKey)
	params.Entries = []PostJournalEntry{
		{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 4},
		{AccountCode: "PSP_CLEARING", Side: JournalSideDebit, AmountRupiah: 3},
		{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 7},
	}
	posted, err := service.PostJournal(ctx, tx, params)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return posted
}

func assertExactReversalRows(t *testing.T, pool *pgxpool.Pool, sourceID, reversalID string) {
	t.Helper()
	ctx := context.Background()
	var differenceCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			(
				SELECT account_code, owner_profile_id, side, amount_rupiah
				FROM platform_ledger_entries
				WHERE journal_id = $1
				EXCEPT ALL
				SELECT account_code, owner_profile_id,
				       CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END,
				       amount_rupiah
				FROM platform_ledger_entries
				WHERE journal_id = $2
			)
			UNION ALL
			(
				SELECT account_code, owner_profile_id,
				       CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END,
				       amount_rupiah
				FROM platform_ledger_entries
				WHERE journal_id = $2
				EXCEPT ALL
				SELECT account_code, owner_profile_id, side, amount_rupiah
				FROM platform_ledger_entries
				WHERE journal_id = $1
			)
		) AS difference
	`, reversalID, sourceID).Scan(&differenceCount))
	assert.Zero(t, differenceCount)
}

func TestReverseJournalCreatesExactImmutableReversal(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal:"+uuid.NewString())
	params := integrationReverseJournalParams(source.ID, "manual correction")

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	reversal, err := service.ReverseJournal(ctx, tx, params)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	require.NotNil(t, reversal.ReversesJournalID)
	assert.Equal(t, source.ID, *reversal.ReversesJournalID)
	assert.Equal(t, params.Reason, *reversal.ReversalReason)
	assert.Equal(t, "journal.reversed:"+source.ID, reversal.EventKey)
	assert.Equal(t, JournalReversalEventType, reversal.EventType)
	assert.Equal(t, source.BookingID, reversal.BookingID)
	assertExactReversalRows(t, pool, source.ID, reversal.ID)

	var sourceDescription *string
	var sourceReversalID *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT description, reverses_journal_id FROM platform_journals WHERE id = $1`, source.ID).Scan(&sourceDescription, &sourceReversalID))
	assert.Nil(t, sourceDescription)
	assert.Nil(t, sourceReversalID)
	assertJournalRowCounts(t, pool, source.ID, 1, 3)
	assertJournalRowCounts(t, pool, reversal.ID, 1, 3)
}

func TestReverseJournalExactRetryReplaysAndDifferentPayloadConflicts(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal-replay:"+uuid.NewString())
	params := integrationReverseJournalParams(source.ID, "manual correction")

	firstTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	first, err := service.ReverseJournal(ctx, firstTx, params)
	require.NoError(t, err)
	require.NoError(t, firstTx.Commit(ctx))

	retryTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	retry, err := service.ReverseJournal(ctx, retryTx, params)
	require.NoError(t, err)
	require.NoError(t, retryTx.Commit(ctx))
	assert.Equal(t, first.ID, retry.ID)
	assert.Equal(t, first.PayloadHash, retry.PayloadHash)
	assert.Equal(t, first.Entries, retry.Entries)

	different := params
	different.Reason = "different correction"
	conflictTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.ReverseJournal(ctx, conflictTx, different)
	assert.ErrorIs(t, err, ErrJournalEventKeyConflict)
	require.NoError(t, conflictTx.Rollback(ctx))

	var reversalCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, source.ID).Scan(&reversalCount))
	assert.Equal(t, 1, reversalCount)
}

func TestReverseJournalPreservesRequiredOwnerDimension(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	ownerID := insertOwnerFixture(t, pool)
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	sourceParams := integrationPostJournalParams("test.reversal-owner:" + uuid.NewString())
	sourceParams.OwnerProfileID = &ownerID
	sourceParams.Entries = []PostJournalEntry{
		{AccountCode: "OWNER_RECEIVABLE", OwnerProfileID: &ownerID, Side: JournalSideDebit, AmountRupiah: 50},
		{AccountCode: "OWNER_PAYABLE", OwnerProfileID: &ownerID, Side: JournalSideCredit, AmountRupiah: 50},
	}
	source, err := service.PostJournal(ctx, tx, sourceParams)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	reversalTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	reversal, err := service.ReverseJournal(ctx, reversalTx, integrationReverseJournalParams(source.ID, "owner correction"))
	require.NoError(t, err)
	require.NoError(t, reversalTx.Commit(ctx))
	assert.Equal(t, ownerID, *reversal.OwnerProfileID)
	for _, entry := range reversal.Entries {
		assert.NotNil(t, entry.OwnerProfileID)
		assert.Equal(t, ownerID, *entry.OwnerProfileID)
	}
	assertExactReversalRows(t, pool, source.ID, reversal.ID)
}

func TestReverseJournalConcurrentIdenticalRetriesCreateOneReversal(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal-concurrent:"+uuid.NewString())
	params := integrationReverseJournalParams(source.ID, "concurrent correction")

	const workers = 8
	start := make(chan struct{})
	ready := make(chan struct{}, workers)
	results := make(chan struct {
		posted *PostedJournal
		err    error
	}, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, beginErr := pool.Begin(ctx)
			if beginErr != nil {
				results <- struct {
					posted *PostedJournal
					err    error
				}{nil, beginErr}
				return
			}
			ready <- struct{}{}
			<-start
			posted, postErr := service.ReverseJournal(ctx, tx, params)
			if postErr == nil {
				postErr = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			results <- struct {
				posted *PostedJournal
				err    error
			}{posted, postErr}
		}()
	}
	for i := 0; i < workers; i++ {
		<-ready
	}
	close(start)
	wg.Wait()
	close(results)

	var winner *PostedJournal
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.posted)
		if winner == nil {
			winner = result.posted
			continue
		}
		assert.Equal(t, winner.ID, result.posted.ID)
		assert.Equal(t, winner.PayloadHash, result.posted.PayloadHash)
	}
	require.NotNil(t, winner)
	assertJournalRowCounts(t, pool, winner.ID, 1, 3)
}

func TestReverseJournalConcurrentDifferentPayloadHasOneWinnerAndConflict(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal-conflict:"+uuid.NewString())
	params := []ReverseJournalParams{
		integrationReverseJournalParams(source.ID, "correction one"),
		integrationReverseJournalParams(source.ID, "correction two"),
	}
	start := make(chan struct{})
	results := make(chan error, len(params))
	var wg sync.WaitGroup
	for _, params := range params {
		params := params
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, beginErr := pool.Begin(ctx)
			if beginErr != nil {
				results <- beginErr
				return
			}
			_, postErr := service.ReverseJournal(ctx, tx, params)
			if postErr == nil {
				postErr = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			results <- postErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var successCount, conflictCount int
	for err := range results {
		if err == nil {
			successCount++
		} else if assert.ErrorIs(t, err, ErrJournalEventKeyConflict) {
			conflictCount++
		}
	}
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, conflictCount)

	var reversalCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, source.ID).Scan(&reversalCount))
	assert.Equal(t, 1, reversalCount)
}

func TestReverseJournalRejectsReversalOfReversalAndSameCreationTransaction(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal-chain:"+uuid.NewString())
	params := integrationReverseJournalParams(source.ID, "first correction")
	firstTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	first, err := service.ReverseJournal(ctx, firstTx, params)
	require.NoError(t, err)
	require.NoError(t, firstTx.Commit(ctx))

	secondParams := integrationReverseJournalParams(first.ID, "second correction")
	secondTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.ReverseJournal(ctx, secondTx, secondParams)
	assert.ErrorIs(t, err, ErrInvalidJournalRequest)
	require.NoError(t, secondTx.Rollback(ctx))

	creationTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	newSourceParams := integrationPostJournalParams("test.same-tx-source:" + uuid.NewString())
	newSourceParams.Entries = []PostJournalEntry{
		{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 1},
		{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 1},
	}
	newSource, err := service.PostJournal(ctx, creationTx, newSourceParams)
	require.NoError(t, err)
	_, err = service.ReverseJournal(ctx, creationTx, integrationReverseJournalParams(newSource.ID, "same tx"))
	assert.ErrorIs(t, err, ErrInvalidJournalRequest)
	require.NoError(t, creationTx.Rollback(ctx))
	assertJournalRowCounts(t, pool, newSource.ID, 0, 0)
}

func TestReverseJournalEffectiveNowAndFutureRules(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal-effective:"+uuid.NewString())

	nowTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	nowParams := integrationReverseJournalParams(source.ID, "current correction")
	nowParams.EffectiveAt = time.Now().UTC()
	nowReversal, err := service.ReverseJournal(ctx, nowTx, nowParams)
	require.NoError(t, err)
	require.NoError(t, nowTx.Commit(ctx))
	assert.False(t, nowReversal.EffectiveAt.After(nowReversal.PostedAt))

	futureSource := createCommittedSourceJournal(t, pool, service, "test.reversal-future:"+uuid.NewString())
	futureTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	futureParams := integrationReverseJournalParams(futureSource.ID, "future correction")
	futureParams.EffectiveAt = time.Now().UTC().Add(time.Hour)
	_, err = service.ReverseJournal(ctx, futureTx, futureParams)
	assert.ErrorIs(t, err, ErrInvalidJournalRequest)
	require.NoError(t, futureTx.Rollback(ctx))
	var reversalCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, futureSource.ID).Scan(&reversalCount))
	assert.Zero(t, reversalCount)
}

func TestReverseJournalPartialEntryFailureCannotCommitOrSurviveRollback(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal-partial:"+uuid.NewString())
	repository := &failAfterFirstEntryJournalRepository{JournalRepository: NewJournalRepository()}
	failingService, err := NewJournalService(repository)
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = failingService.ReverseJournal(ctx, tx, integrationReverseJournalParams(source.ID, "partial correction"))
	assert.ErrorIs(t, err, ErrJournalPersistence)
	require.NoError(t, tx.Rollback(ctx))

	var reversalCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, source.ID).Scan(&reversalCount))
	assert.Zero(t, reversalCount)

	commitTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = failingService.ReverseJournal(ctx, commitTx, integrationReverseJournalParams(source.ID, "partial commit correction"))
	assert.ErrorIs(t, err, ErrJournalPersistence)
	assert.Error(t, commitTx.Commit(ctx), "deferred balance guard must reject a partial reversal")
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE reverses_journal_id = $1`, source.ID).Scan(&reversalCount))
	assert.Zero(t, reversalCount)
}

func TestReverseJournalTimeoutAfterCommitRetriesExistingReversal(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	source := createCommittedSourceJournal(t, pool, service, "test.reversal-timeout:"+uuid.NewString())
	params := integrationReverseJournalParams(source.ID, "timeout correction")

	first, lostResponseErr := reverseJournalWithCommitTimeout(ctx, pool, service, params)
	assert.ErrorIs(t, lostResponseErr, context.DeadlineExceeded)

	retryTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	retry, err := service.ReverseJournal(ctx, retryTx, params)
	require.NoError(t, err)
	require.NoError(t, retryTx.Commit(ctx))
	assert.Equal(t, first.ID, retry.ID)
	assert.Equal(t, first.PayloadHash, retry.PayloadHash)
	assertJournalRowCounts(t, pool, first.ID, 1, 3)
}

func reverseJournalWithCommitTimeout(ctx context.Context, pool *pgxpool.Pool, service JournalService, params ReverseJournalParams) (*PostedJournal, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	posted, err := service.ReverseJournal(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return posted, context.DeadlineExceeded
}
