package platformfinance

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func TestPostJournalDuplicateEventKeyReturnsGenericConflict(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()
	service, err := NewJournalService(NewJournalRepository())
	require.NoError(t, err)
	params := integrationPostJournalParams("test.duplicate:" + uuid.NewString())

	firstTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.PostJournal(ctx, firstTx, params)
	require.NoError(t, err)
	require.NoError(t, firstTx.Commit(ctx))

	secondTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = service.PostJournal(ctx, secondTx, params)
	assert.ErrorIs(t, err, ErrJournalEventKeyConflict)
	assert.Equal(t, "JOURNAL_EVENT_KEY_CONFLICT", err.Error())
	require.NoError(t, secondTx.Rollback(ctx))

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE event_key = $1`, params.EventKey).Scan(&count))
	assert.Equal(t, 1, count)
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
