package platformfinance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func integrationPostJournalParams(eventKey string) PostJournalParams {
	return PostJournalParams{
		EventKey:    eventKey,
		EventType:   "TEST_JOURNAL",
		EffectiveAt: time.Now().UTC().Add(-time.Second),
		Metadata: map[string]string{
			"source_type":      "integration_test",
			"source_reference": "post-journal",
		},
		Entries: []PostJournalEntry{
			{AccountCode: "BANK_CASH", Side: JournalSideDebit, AmountRupiah: 101},
			{AccountCode: "FUNDING_CLEARING", Side: JournalSideCredit, AmountRupiah: 101},
		},
	}
}

func TestPostJournalCommitsBalancedJournalInCallerTransaction(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	params := integrationPostJournalParams("test.journal:" + uuid.NewString())
	posted, err := service.PostJournal(ctx, tx, params)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	assert.Equal(t, params.EventKey, posted.EventKey)
	assert.Equal(t, JournalCurrencyIDR, posted.Currency)
	assert.Equal(t, JournalPayloadHashVersionV1, posted.PayloadHashVersion)
	assert.Len(t, posted.PayloadHash, 64)
	assert.Len(t, posted.Entries, 2)

	var journalCount, entryCount int
	var debitTotal, creditTotal int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE id = $1`, posted.ID).Scan(&journalCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = $1`, posted.ID).Scan(&entryCount))
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount_rupiah) FILTER (WHERE side = 'DEBIT'), 0),
			COALESCE(SUM(amount_rupiah) FILTER (WHERE side = 'CREDIT'), 0)
		FROM platform_ledger_entries
		WHERE journal_id = $1
	`, posted.ID).Scan(&debitTotal, &creditTotal))
	assert.Equal(t, 1, journalCount)
	assert.Equal(t, 2, entryCount)
	assert.Equal(t, int64(101), debitTotal)
	assert.Equal(t, debitTotal, creditTotal)
}

func TestPostJournalSupportsCatalogRequiredOwnerDimension(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	ownerID := insertOwnerFixture(t, pool)
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	params := integrationPostJournalParams("test.owner-journal:" + uuid.NewString())
	params.OwnerProfileID = &ownerID
	params.Entries = []PostJournalEntry{
		{AccountCode: "OWNER_RECEIVABLE", OwnerProfileID: &ownerID, Side: JournalSideDebit, AmountRupiah: 50},
		{AccountCode: "OWNER_PAYABLE", OwnerProfileID: &ownerID, Side: JournalSideCredit, AmountRupiah: 50},
	}
	posted, err := service.PostJournal(ctx, tx, params)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	assert.Equal(t, ownerID, *posted.OwnerProfileID)
	assert.Equal(t, ownerID, *posted.Entries[0].OwnerProfileID)
}

func TestPostJournalCallerRollbackLeavesNoRows(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	params := integrationPostJournalParams("test.rollback:" + uuid.NewString())
	posted, err := service.PostJournal(ctx, tx, params)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(ctx))

	assertJournalRowCounts(t, pool, posted.ID, 0, 0)
}

type failEntriesJournalRepository struct {
	JournalRepository
}

func (r *failEntriesJournalRepository) InsertEntries(context.Context, pgx.Tx, string, []preparedJournalEntry) ([]PostedJournalEntry, error) {
	return nil, ErrJournalPersistence
}

type failAfterFirstEntryJournalRepository struct {
	JournalRepository
}

func (r *failAfterFirstEntryJournalRepository) InsertEntries(ctx context.Context, tx pgx.Tx, journalID string, entries []preparedJournalEntry) ([]PostedJournalEntry, error) {
	if len(entries) == 0 {
		return nil, ErrJournalPersistence
	}
	if _, err := r.JournalRepository.InsertEntries(ctx, tx, journalID, entries[:1]); err != nil {
		return nil, err
	}
	return nil, ErrJournalPersistence
}

func TestPostJournalEntryFailureRollsBackInsertedHeaderAtomically(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	repository := &failEntriesJournalRepository{JournalRepository: NewJournalRepository()}
	service, err := NewJournalService(repository)
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	params := integrationPostJournalParams("test.atomic-failure:" + uuid.NewString())
	_, err = service.PostJournal(ctx, tx, params)
	assert.ErrorIs(t, err, ErrJournalPersistence)

	var headerCount int
	require.NoError(t, tx.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&headerCount))
	assert.Equal(t, 1, headerCount, "the injected failure must happen after the header insert")
	require.NoError(t, tx.Rollback(ctx))

	var journalCount, entryCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&journalCount))
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM platform_ledger_entries e
		JOIN platform_journals j ON j.id = e.journal_id
		WHERE j.event_key = $1
	`, params.EventKey).Scan(&entryCount))
	assert.Zero(t, journalCount)
	assert.Zero(t, entryCount)
}

func TestPostJournalUnknownAccountDoesNotInsertHeader(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	params := integrationPostJournalParams("test.unknown-account:" + uuid.NewString())
	params.Entries[0].AccountCode = "UNKNOWN_ACCOUNT"
	_, err = service.PostJournal(ctx, tx, params)
	assert.ErrorIs(t, err, ErrUnknownJournalAccount)

	var count int
	require.NoError(t, tx.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&count))
	assert.Zero(t, count)
	require.NoError(t, tx.Rollback(ctx))
}

func TestPostJournalDuplicateEventKeyReplaysExistingJournal(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	params := integrationPostJournalParams("test.duplicate:" + uuid.NewString())

	firstTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	firstPosted, err := service.PostJournal(ctx, firstTx, params)
	require.NoError(t, err)
	require.NoError(t, firstTx.Commit(ctx))

	retryParams := params
	retryParams.Entries = []PostJournalEntry{params.Entries[1], params.Entries[0]}
	secondTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	secondPosted, err := service.PostJournal(ctx, secondTx, retryParams)
	require.NoError(t, err)
	assert.Equal(t, firstPosted.ID, secondPosted.ID)
	assert.Equal(t, firstPosted.PayloadHash, secondPosted.PayloadHash)
	assert.Equal(t, firstPosted.Entries, secondPosted.Entries)
	require.NoError(t, secondTx.Rollback(ctx))

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPostJournalDifferentPayloadForExistingEventKeyConflicts(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	params := integrationPostJournalParams("test.payload-conflict:" + uuid.NewString())

	firstTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	firstPosted, err := service.PostJournal(ctx, firstTx, params)
	require.NoError(t, err)
	require.NoError(t, firstTx.Commit(ctx))

	params.Entries[0].AmountRupiah = 102
	params.Entries[1].AmountRupiah = 102
	secondTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.PostJournal(ctx, secondTx, params)
	assert.ErrorIs(t, err, ErrJournalEventKeyConflict)
	require.NoError(t, secondTx.Rollback(ctx))

	assertJournalRowCounts(t, pool, firstPosted.ID, 1, 2)
	var eventCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&eventCount))
	assert.Equal(t, 1, eventCount)
}

func TestPostJournalConcurrentSamePayloadCreatesOneJournalAndReplays(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	params := integrationPostJournalParams("test.concurrent:" + uuid.NewString())

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
			posted, postErr := service.PostJournal(ctx, tx, params)
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
		assert.Equal(t, winner.Entries, result.posted.Entries)
	}
	require.NotNil(t, winner)
	assertJournalRowCounts(t, pool, winner.ID, 1, 2)
}

func TestPostJournalTimeoutAfterCommitRetriesExistingJournal(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	params := integrationPostJournalParams("test.timeout-after-commit:" + uuid.NewString())

	firstPosted, lostResponseErr := postJournalWithCommitTimeout(ctx, pool, service, params)
	assert.ErrorIs(t, lostResponseErr, context.DeadlineExceeded)

	secondTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	secondPosted, err := service.PostJournal(ctx, secondTx, params)
	require.NoError(t, err)
	require.NoError(t, secondTx.Commit(ctx))

	assert.Equal(t, firstPosted.ID, secondPosted.ID)
	assert.Equal(t, firstPosted.PayloadHash, secondPosted.PayloadHash)
	assert.Equal(t, firstPosted.Entries, secondPosted.Entries)
	assertJournalRowCounts(t, pool, firstPosted.ID, 1, 2)
}

func postJournalWithCommitTimeout(ctx context.Context, pool *pgxpool.Pool, service JournalService, params PostJournalParams) (*PostedJournal, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	posted, err := service.PostJournal(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	// Model the ambiguous outcome where PostgreSQL committed successfully but
	// the caller observed a transport timeout before receiving the acknowledgement.
	return posted, context.DeadlineExceeded
}

func TestPostJournalDuplicateIntegrityFailureDoesNotReplayTamperedFact(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	params := integrationPostJournalParams("test.integrity-failure:" + uuid.NewString())
	normalized, _, err := validateAndNormalizeJournal(params)
	require.NoError(t, err)
	payloadHash, err := hashJournalPayloadV1(normalized)
	require.NoError(t, err)

	journalID := uuid.NewString()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO platform_journals (
			id, event_key, event_type, payload_hash, effective_at, metadata
		) VALUES ($1, $2, $3, $4, $5, '{"source_type":"tampered"}'::jsonb)
	`, journalID, params.EventKey, params.EventType, payloadHash, params.EffectiveAt)
	require.NoError(t, err)
	insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 101)
	insertLedgerEntry(t, tx, journalID, "FUNDING_CLEARING", "", "CREDIT", 101)
	require.NoError(t, tx.Commit(ctx))

	retryTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.PostJournal(ctx, retryTx, params)
	assert.ErrorIs(t, err, ErrJournalIntegrity)
	require.NoError(t, retryTx.Rollback(ctx))
	assertJournalRowCounts(t, pool, journalID, 1, 2)
}

func TestPostJournalPartialEntryFailureCannotCommitOrSurviveRollback(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	repository := &failAfterFirstEntryJournalRepository{JournalRepository: NewJournalRepository()}
	service, err := NewJournalService(repository)
	require.NoError(t, err)

	params := integrationPostJournalParams("test.partial-entry:" + uuid.NewString())
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.PostJournal(ctx, tx, params)
	assert.ErrorIs(t, err, ErrJournalPersistence)
	var headerCount, entryCount int
	require.NoError(t, tx.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&headerCount))
	require.NoError(t, tx.QueryRow(ctx, `SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = (SELECT id FROM platform_journals WHERE event_key = $1)`, params.EventKey).Scan(&entryCount))
	assert.Equal(t, 1, headerCount)
	assert.Equal(t, 1, entryCount)
	require.NoError(t, tx.Rollback(ctx))

	var persisted int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&persisted))
	assert.Zero(t, persisted)

	params.EventKey = "test.partial-entry-commit:" + uuid.NewString()
	commitTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.PostJournal(ctx, commitTx, params)
	assert.ErrorIs(t, err, ErrJournalPersistence)
	assert.Error(t, commitTx.Commit(ctx), "deferred balance guard must reject a partial entry set")
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&persisted))
	assert.Zero(t, persisted)
}

func TestPostJournalForeignKeyFailureReturnsGenericReferenceError(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	params := integrationPostJournalParams("test.invalid-reference:" + uuid.NewString())
	missingBookingID := uuid.NewString()
	params.BookingID = &missingBookingID

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.PostJournal(ctx, tx, params)
	assert.ErrorIs(t, err, ErrInvalidJournalReference)
	assert.Equal(t, "INVALID_JOURNAL_REFERENCE", err.Error())
	require.NoError(t, tx.Rollback(ctx))

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&count))
	assert.Zero(t, count)
}

type journalRowCounter interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func assertJournalRowCounts(t *testing.T, db journalRowCounter, journalID string, expectedJournals, expectedEntries int) {
	t.Helper()
	ctx := context.Background()
	var journalCount, entryCount int
	require.NoError(t, db.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE id = $1`, journalID).Scan(&journalCount))
	require.NoError(t, db.QueryRow(ctx, `SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = $1`, journalID).Scan(&entryCount))
	assert.Equal(t, expectedJournals, journalCount)
	assert.Equal(t, expectedEntries, entryCount)
}
