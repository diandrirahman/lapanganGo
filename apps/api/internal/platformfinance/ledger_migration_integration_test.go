package platformfinance

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/database"
)

const ledgerMigrationVersion = 23

func newLedgerMigrationDatabase(t *testing.T, targetVersion uint) (string, *migrate.Migrate, *pgxpool.Pool) {
	t.Helper()

	if os.Getenv("TEST_LEDGER_DISPOSABLE") != "1" {
		t.Skip("set TEST_LEDGER_DISPOSABLE=1 to run ledger migration integration tests")
	}

	baseDSN := os.Getenv("LEDGER_TEST_DATABASE_URL")
	require.NotEmpty(t, baseDSN, "LEDGER_TEST_DATABASE_URL is required")

	dbName := "lapangango_ledger_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	testDSN, cleanup, err := createDisposableDB(baseDSN, dbName)
	require.NoError(t, err)

	migrationsPath := getMigrationsPath()
	require.NotEmpty(t, migrationsPath, "migrations path must be resolved")

	m, err := migrate.New(migrationsPath, testDSN)
	require.NoError(t, err)

	err = m.Migrate(targetVersion)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, testDSN)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
		m.Close()
		require.NoError(t, cleanup())
	})

	return testDSN, m, pool
}

func insertLedgerJournal(t *testing.T, tx pgx.Tx, id, eventKey string, ownerProfileID *string, effectiveAt time.Time, payloadHash string) {
	t.Helper()

	_, err := tx.Exec(context.Background(), `
		INSERT INTO platform_journals (
			id, event_key, event_type, payload_hash, owner_profile_id, effective_at, metadata
		) VALUES ($1, $2, 'TEST_JOURNAL', $3, $4, $5, '{"source_type":"test"}'::jsonb)
	`, id, eventKey, payloadHash, ownerProfileID, effectiveAt)
	require.NoError(t, err)
}

func insertLedgerEntry(t *testing.T, tx pgx.Tx, journalID, accountCode, ownerProfileID, side string, amount int64) string {
	t.Helper()

	entryID := uuid.NewString()
	_, err := tx.Exec(context.Background(), `
		INSERT INTO platform_ledger_entries (id, journal_id, account_code, owner_profile_id, side, amount_rupiah)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, entryID, journalID, accountCode, nullableUUID(ownerProfileID), side, amount)
	require.NoError(t, err)
	return entryID
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func insertBalancedLedgerJournal(t *testing.T, pool *pgxpool.Pool, ownerProfileID string) (string, string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	journalID := uuid.NewString()
	eventKey := "test.journal:" + journalID
	insertLedgerJournal(t, tx, journalID, eventKey, optionalString(ownerProfileID), time.Now().UTC().Add(-time.Minute), strings.Repeat("a", 64))
	debitID := insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 100)
	insertLedgerEntry(t, tx, journalID, "FUNDING_CLEARING", "", "CREDIT", 100)
	require.NoError(t, tx.Commit(ctx))
	return journalID, debitID
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func insertOwnerFixture(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	userID := uuid.NewString()
	ownerID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role, status)
		VALUES ($1, 'Ledger Owner', $2, 'hash', 'OWNER', 'ACTIVE')
	`, userID, fmt.Sprintf("ledger-owner-%s@example.com", userID))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO owner_profiles (id, user_id, business_name)
		VALUES ($1, $2, 'Ledger Owner Business')
	`, ownerID, userID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return ownerID
}

func TestLedgerMigrationFreshUpgradeAndPreFactDown(t *testing.T) {
	_, m, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()

	version, dirty, err := m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(ledgerMigrationVersion), version)
	assert.False(t, dirty)

	var accountCount int
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_accounts").Scan(&accountCount))
	assert.Equal(t, 26, accountCount)

	_, err = pool.Exec(ctx, `
		INSERT INTO platform_audit_logs (actor_role, action, entity_type)
		VALUES ('SUPER_ADMIN', 'PLATFORM_COMMERCIAL_TERM_CREATED', 'PLATFORM_COMMERCIAL_TERM')
	`)
	require.NoError(t, err)

	require.NoError(t, m.Steps(-1))
	version, dirty, err = m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(22), version)
	assert.False(t, dirty)

	var ledgerTables int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('platform_accounts', 'platform_journals', 'platform_ledger_entries')
	`).Scan(&ledgerTables))
	assert.Equal(t, 3, ledgerTables)

	var ledgerFunctions int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc
		WHERE pronamespace = 'public'::regnamespace
		  AND proname = ANY($1::text[])
	`, []string{
		"validate_platform_journal_metadata",
		"prevent_platform_ledger_mutation",
		"prevent_platform_account_catalog_mutation",
		"stamp_platform_journal_creation",
		"validate_platform_journal_reversal_source",
		"validate_platform_ledger_entry_insert",
		"validate_platform_journal_balance",
	}).Scan(&ledgerFunctions))
	assert.Equal(t, 7, ledgerFunctions)

	var auditCount int
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_audit_logs").Scan(&auditCount))
	assert.Equal(t, 1, auditCount)

	require.NoError(t, m.Steps(-1))
	version, dirty, err = m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(21), version)
	assert.False(t, dirty)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('platform_accounts', 'platform_journals', 'platform_ledger_entries')
	`).Scan(&ledgerTables))
	assert.Zero(t, ledgerTables)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc
		WHERE pronamespace = 'public'::regnamespace
		  AND proname = ANY($1::text[])
	`, []string{
		"validate_platform_journal_metadata",
		"prevent_platform_ledger_mutation",
		"prevent_platform_account_catalog_mutation",
		"stamp_platform_journal_creation",
		"validate_platform_journal_reversal_source",
		"validate_platform_ledger_entry_insert",
		"validate_platform_journal_balance",
		"validate_platform_journal_balance_for",
		"validate_platform_ledger_entry_balance",
	}).Scan(&ledgerFunctions))
	assert.Zero(t, ledgerFunctions)
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_audit_logs").Scan(&auditCount))
	assert.Equal(t, 1, auditCount)

	require.NoError(t, m.Steps(2))
	version, dirty, err = m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(ledgerMigrationVersion), version)
	assert.False(t, dirty)
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_accounts").Scan(&accountCount))
	assert.Equal(t, 26, accountCount)
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_audit_logs").Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('platform_accounts', 'platform_journals', 'platform_ledger_entries')
	`).Scan(&ledgerTables))
	assert.Equal(t, 3, ledgerTables)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc
		WHERE pronamespace = 'public'::regnamespace
		  AND proname = ANY($1::text[])
	`, []string{
		"validate_platform_journal_metadata",
		"prevent_platform_ledger_mutation",
		"prevent_platform_account_catalog_mutation",
		"stamp_platform_journal_creation",
		"validate_platform_journal_reversal_source",
		"validate_platform_ledger_entry_insert",
		"validate_platform_journal_balance",
		"validate_platform_journal_balance_for",
		"validate_platform_ledger_entry_balance",
	}).Scan(&ledgerFunctions))
	assert.Equal(t, 9, ledgerFunctions)

	_, upgradeM, upgradePool := newLedgerMigrationDatabase(t, 21)
	_, err = upgradePool.Exec(ctx, `
		INSERT INTO platform_audit_logs (actor_role, action, entity_type)
		VALUES ('SUPER_ADMIN', 'PLATFORM_COMMERCIAL_TERM_CREATED', 'PLATFORM_COMMERCIAL_TERM')
	`)
	require.NoError(t, err)
	require.NoError(t, upgradeM.Migrate(ledgerMigrationVersion))
	require.NoError(t, upgradePool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_accounts").Scan(&accountCount))
	assert.Equal(t, 26, accountCount)
	require.NoError(t, upgradePool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_audit_logs").Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
}

func TestLedgerMigrationForwardPatchFromVersion22(t *testing.T) {
	_, m, pool := newLedgerMigrationDatabase(t, 22)
	ctx := context.Background()

	originalID, _ := insertBalancedLedgerJournal(t, pool, "")
	version, dirty, err := m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(22), version)
	assert.False(t, dirty)

	require.NoError(t, m.Steps(1))
	version, dirty, err = m.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(ledgerMigrationVersion), version)
	assert.False(t, dirty)

	var entryGuardCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_trigger
		WHERE tgname = 'platform_ledger_entry_balance_guard'
		  AND tgrelid = 'platform_ledger_entries'::regclass
	`).Scan(&entryGuardCount))
	assert.Equal(t, 1, entryGuardCount)

	var helperCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc
		WHERE pronamespace = 'public'::regnamespace
		  AND proname = ANY($1::text[])
	`, []string{"validate_platform_journal_balance_for", "validate_platform_ledger_entry_balance"}).Scan(&helperCount))
	assert.Equal(t, 2, helperCount)

	t.Run("balance guard is armed after 22 to 23 upgrade", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		journalID := uuid.NewString()
		insertLedgerJournal(t, tx, journalID, "test.journal:upgrade-balance:"+journalID, nil, time.Now().UTC().Add(-time.Minute), strings.Repeat("b", 64))
		insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 100)
		insertLedgerEntry(t, tx, journalID, "FUNDING_CLEARING", "", "CREDIT", 100)
		_, err = tx.Exec(ctx, "SET CONSTRAINTS platform_journal_balance_guard IMMEDIATE")
		require.NoError(t, err)
		insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 1)

		err = tx.Commit(ctx)
		require.Error(t, err)
		var journalCount, entryCount int
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals WHERE id = $1", journalID).Scan(&journalCount))
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = $1", journalID).Scan(&entryCount))
		assert.Zero(t, journalCount)
		assert.Zero(t, entryCount)
	})

	t.Run("exact-reversal guard is armed after 22 to 23 upgrade", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		reversalID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_journals (
				id, event_key, event_type, payload_hash, reverses_journal_id, reversal_reason, effective_at, metadata
			) VALUES ($1, $2, 'TEST_REVERSAL', $3, $4, 'upgrade reversal guard', now() - interval '1 minute', '{"source_type":"test"}'::jsonb)
		`, reversalID, "journal.reversed:"+originalID, strings.Repeat("c", 64), originalID)
		require.NoError(t, err)
		insertLedgerEntry(t, tx, reversalID, "BANK_CASH", "", "CREDIT", 100)
		insertLedgerEntry(t, tx, reversalID, "FUNDING_CLEARING", "", "DEBIT", 100)
		_, err = tx.Exec(ctx, "SET CONSTRAINTS platform_journal_balance_guard IMMEDIATE")
		require.NoError(t, err)
		insertLedgerEntry(t, tx, reversalID, "BANK_CASH", "", "DEBIT", 1)
		insertLedgerEntry(t, tx, reversalID, "FUNDING_CLEARING", "", "CREDIT", 1)

		err = tx.Commit(ctx)
		require.Error(t, err)
		var journalCount, entryCount int
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals WHERE id = $1", reversalID).Scan(&journalCount))
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = $1", reversalID).Scan(&entryCount))
		assert.Zero(t, journalCount)
		assert.Zero(t, entryCount)
	})
}

func TestLedgerMigrationDatabaseInvariants(t *testing.T) {
	_, _, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	ctx := context.Background()

	validJournalID, validEntryID := insertBalancedLedgerJournal(t, pool, "")
	assertImmutableMutation := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		assert.Equal(t, "55000", pgErr.Code)
	}

	_, err := pool.Exec(ctx, "UPDATE platform_journals SET description = 'mutated' WHERE id = $1", validJournalID)
	assertImmutableMutation(t, err)
	_, err = pool.Exec(ctx, "DELETE FROM platform_journals WHERE id = $1", validJournalID)
	assertImmutableMutation(t, err)
	_, err = pool.Exec(ctx, "UPDATE platform_ledger_entries SET amount_rupiah = amount_rupiah + 1 WHERE id = $1", validEntryID)
	assertImmutableMutation(t, err)
	_, err = pool.Exec(ctx, "DELETE FROM platform_ledger_entries WHERE id = $1", validEntryID)
	assertImmutableMutation(t, err)
	_, err = pool.Exec(ctx, "UPDATE platform_accounts SET code = code WHERE code = 'BANK_CASH'")
	assertImmutableMutation(t, err)
	_, err = pool.Exec(ctx, "INSERT INTO platform_accounts (code, account_type, normal_side, owner_dimension) VALUES ('NOT_AN_ACCOUNT', 'ASSET', 'DEBIT', 'FORBIDDEN')")
	assertImmutableMutation(t, err)
	_, err = pool.Exec(ctx, "DELETE FROM platform_accounts WHERE code = 'OPEX_OTHER'")
	assertImmutableMutation(t, err)

	var journalDescription *string
	var entryAmount int64
	var accountCount int
	require.NoError(t, pool.QueryRow(ctx, "SELECT description FROM platform_journals WHERE id = $1", validJournalID).Scan(&journalDescription))
	require.NoError(t, pool.QueryRow(ctx, "SELECT amount_rupiah FROM platform_ledger_entries WHERE id = $1", validEntryID).Scan(&entryAmount))
	require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_accounts").Scan(&accountCount))
	assert.Nil(t, journalDescription, "failed journal mutations must leave the original row unchanged")
	assert.Equal(t, int64(100), entryAmount, "failed entry mutations must leave the original row unchanged")
	assert.Equal(t, 26, accountCount, "failed catalog mutations must leave the seeded catalog unchanged")

	t.Run("append after journal transaction is rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_ledger_entries (journal_id, account_code, side, amount_rupiah)
			VALUES ($1, 'BANK_CASH', 'DEBIT', 1)
		`, validJournalID)
		require.Error(t, err)
		require.NoError(t, tx.Rollback(ctx))
	})

	t.Run("unbalanced and minimum-entry checks fail at commit", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			debit  int64
			credit int64
		}{
			{name: "rp1 imbalance", debit: 100, credit: 99},
			{name: "one entry", debit: 100, credit: 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tx, err := pool.Begin(ctx)
				require.NoError(t, err)
				journalID := uuid.NewString()
				insertLedgerJournal(t, tx, journalID, "test.journal:"+journalID, nil, time.Now().UTC().Add(-time.Minute), strings.Repeat("b", 64))
				insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", tc.debit)
				if tc.credit > 0 {
					insertLedgerEntry(t, tx, journalID, "FUNDING_CLEARING", "", "CREDIT", tc.credit)
				}
				assert.Error(t, tx.Commit(ctx))
			})
		}
	})

	t.Run("balance guard remains armed after early constraint evaluation", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		journalID := uuid.NewString()
		insertLedgerJournal(t, tx, journalID, "test.journal:early-balance:"+journalID, nil, time.Now().UTC().Add(-time.Minute), strings.Repeat("b", 64))
		insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 100)
		insertLedgerEntry(t, tx, journalID, "FUNDING_CLEARING", "", "CREDIT", 100)
		_, err = tx.Exec(ctx, "SET CONSTRAINTS platform_journal_balance_guard IMMEDIATE")
		require.NoError(t, err)
		insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 1)

		err = tx.Commit(ctx)
		require.Error(t, err)

		var journalCount, entryCount int
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals WHERE id = $1", journalID).Scan(&journalCount))
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = $1", journalID).Scan(&entryCount))
		assert.Zero(t, journalCount)
		assert.Zero(t, entryCount)
	})

	t.Run("current effective time remains valid after transaction starts", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, "SELECT pg_sleep(0.05)")
		require.NoError(t, err)

		journalID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_journals (id, event_key, event_type, payload_hash, effective_at, metadata)
			VALUES ($1, $2, 'TEST_CURRENT_EVENT', $3, clock_timestamp(), '{"source_type":"test"}'::jsonb)
		`, journalID, "test.journal:delayed-current", strings.Repeat("b", 64))
		require.NoError(t, err)
		insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 100)
		insertLedgerEntry(t, tx, journalID, "FUNDING_CLEARING", "", "CREDIT", 100)
		require.NoError(t, tx.Commit(ctx))
	})

	t.Run("invalid amount and account fail immediately", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			accountCode string
			amount      any
		}{
			{name: "zero", accountCode: "BANK_CASH", amount: 0},
			{name: "negative", accountCode: "BANK_CASH", amount: -1},
			{name: "null", accountCode: "BANK_CASH", amount: nil},
			{name: "unknown account", accountCode: "NOT_AN_ACCOUNT", amount: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tx, err := pool.Begin(ctx)
				require.NoError(t, err)
				journalID := uuid.NewString()
				insertLedgerJournal(t, tx, journalID, "test.journal:"+journalID, nil, time.Now().UTC().Add(-time.Minute), strings.Repeat("c", 64))
				_, err = tx.Exec(ctx, `
					INSERT INTO platform_ledger_entries (journal_id, account_code, side, amount_rupiah)
					VALUES ($1, $2, 'DEBIT', $3)
				`, journalID, tc.accountCode, tc.amount)
				assert.Error(t, err)
				assert.NoError(t, tx.Rollback(ctx))
			})
		}
	})

	t.Run("event key, hash, metadata, and effective time checks fail", func(t *testing.T) {
		cases := []struct {
			name string
			sql  string
			args []any
		}{
			{name: "uppercase event key", sql: `INSERT INTO platform_journals (event_key, event_type, payload_hash, effective_at) VALUES ('Test.journal:x', 'TEST_JOURNAL', $1, now())`, args: []any{strings.Repeat("a", 64)}},
			{name: "invalid hash", sql: `INSERT INTO platform_journals (event_key, event_type, payload_hash, effective_at) VALUES ('test.journal:x', 'TEST_JOURNAL', $1, now())`, args: []any{"ABC"}},
			{name: "nested metadata", sql: `INSERT INTO platform_journals (event_key, event_type, payload_hash, effective_at, metadata) VALUES ('test.journal:x', 'TEST_JOURNAL', $1, now(), '{"source_type":{"nested":true}}'::jsonb)`, args: []any{strings.Repeat("a", 64)}},
			{name: "future effective time", sql: `INSERT INTO platform_journals (event_key, event_type, payload_hash, effective_at) VALUES ('test.journal:x', 'TEST_JOURNAL', $1, now() + interval '1 hour')`, args: []any{strings.Repeat("a", 64)}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := pool.Exec(ctx, tc.sql, tc.args...)
				assert.Error(t, err)
			})
		}
	})

	t.Run("event key is unique", func(t *testing.T) {
		var existingEventKey string
		require.NoError(t, pool.QueryRow(ctx, "SELECT event_key FROM platform_journals WHERE id = $1", validJournalID).Scan(&existingEventKey))
		_, err := pool.Exec(ctx, `
			INSERT INTO platform_journals (id, event_key, event_type, payload_hash, effective_at)
			VALUES ($1, $2, 'TEST_JOURNAL', $3, now() - interval '1 minute')
		`, uuid.NewString(), existingEventKey, strings.Repeat("e", 64))
		assert.Error(t, err)
	})

	t.Run("owner dimension is required and scoped", func(t *testing.T) {
		ownerID := insertOwnerFixture(t, pool)
		journalID := uuid.NewString()
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		insertLedgerJournal(t, tx, journalID, "test.journal:"+journalID, &ownerID, time.Now().UTC().Add(-time.Minute), strings.Repeat("d", 64))
		insertLedgerEntry(t, tx, journalID, "OWNER_PAYABLE", ownerID, "CREDIT", 100)
		insertLedgerEntry(t, tx, journalID, "BANK_CASH", "", "DEBIT", 100)
		require.NoError(t, tx.Commit(ctx))

		for _, tc := range []struct {
			name       string
			account    string
			entryOwner string
			journalOwn string
		}{
			{name: "required owner missing", account: "OWNER_PAYABLE", entryOwner: "", journalOwn: ownerID},
			{name: "forbidden owner supplied", account: "BANK_CASH", entryOwner: ownerID, journalOwn: ""},
			{name: "required owner mismatched", account: "OWNER_PAYABLE", entryOwner: uuid.NewString(), journalOwn: ownerID},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tx, err := pool.Begin(ctx)
				require.NoError(t, err)
				jid := uuid.NewString()
				insertLedgerJournal(t, tx, jid, "test.journal:"+jid, optionalString(tc.journalOwn), time.Now().UTC().Add(-time.Minute), strings.Repeat("e", 64))
				_, err = tx.Exec(ctx, `
					INSERT INTO platform_ledger_entries (journal_id, account_code, owner_profile_id, side, amount_rupiah)
					VALUES ($1, $2, $3, 'DEBIT', 1)
				`, jid, tc.account, nullableUUID(tc.entryOwner))
				assert.Error(t, err)
				assert.NoError(t, tx.Rollback(ctx))
			})
		}
	})

	t.Run("exact reversal and single reversal link are enforced", func(t *testing.T) {
		originalID, _ := insertBalancedLedgerJournal(t, pool, "")
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		reversalID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_journals (
				id, event_key, event_type, payload_hash, reverses_journal_id, reversal_reason, effective_at, metadata
			) VALUES ($1, $2, 'TEST_REVERSAL', $3, $4, 'test reversal', $5, '{"source_type":"test"}'::jsonb)
		`, reversalID, "journal.reversed:"+originalID, strings.Repeat("f", 64), originalID, time.Now().UTC().Add(-time.Minute))
		require.NoError(t, err)
		insertLedgerEntry(t, tx, reversalID, "BANK_CASH", "", "CREDIT", 100)
		insertLedgerEntry(t, tx, reversalID, "FUNDING_CLEARING", "", "DEBIT", 100)
		require.NoError(t, tx.Commit(ctx))

		_, err = pool.Exec(ctx, "UPDATE platform_journals SET reversal_reason = 'wrong mutation' WHERE id = $1", reversalID)
		assert.Error(t, err)

		tx, err = pool.Begin(ctx)
		require.NoError(t, err)
		secondID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_journals (id, event_key, event_type, payload_hash, reverses_journal_id, reversal_reason, effective_at)
			VALUES ($1, $2, 'TEST_REVERSAL', $3, $4, 'second reversal', now() - interval '1 minute')
		`, secondID, "journal.reversed:"+originalID, strings.Repeat("1", 64), originalID)
		assert.Error(t, err)
		assert.NoError(t, tx.Rollback(ctx))

		tx, err = pool.Begin(ctx)
		require.NoError(t, err)
		reversalOfReversalID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_journals (id, event_key, event_type, payload_hash, reverses_journal_id, reversal_reason, effective_at)
			VALUES ($1, $2, 'TEST_REVERSAL', $3, $4, 'reverse reversal', now() - interval '1 minute')
		`, reversalOfReversalID, "journal.reversed:"+reversalID, strings.Repeat("2", 64), reversalID)
		assert.Error(t, err)
		assert.NoError(t, tx.Rollback(ctx))
	})

	t.Run("balanced but non-exact reversal is rejected atomically", func(t *testing.T) {
		originalID, _ := insertBalancedLedgerJournal(t, pool, "")
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		wrongReversalID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_journals (
				id, event_key, event_type, payload_hash, reverses_journal_id, reversal_reason, effective_at, metadata
			) VALUES ($1, $2, 'TEST_REVERSAL', $3, $4, 'wrong account reversal', now() - interval '1 minute', '{"source_type":"test"}'::jsonb)
		`, wrongReversalID, "journal.reversed:"+originalID, strings.Repeat("3", 64), originalID)
		require.NoError(t, err)
		insertLedgerEntry(t, tx, wrongReversalID, "BANK_CASH", "", "CREDIT", 100)
		insertLedgerEntry(t, tx, wrongReversalID, "PSP_CLEARING", "", "DEBIT", 100)

		err = tx.Commit(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reversal entries must exactly invert the source journal")

		var journalCount, entryCount int
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals WHERE id = $1", wrongReversalID).Scan(&journalCount))
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = $1", wrongReversalID).Scan(&entryCount))
		assert.Zero(t, journalCount)
		assert.Zero(t, entryCount)
	})

	t.Run("exact-reversal guard remains armed after early constraint evaluation", func(t *testing.T) {
		originalID, _ := insertBalancedLedgerJournal(t, pool, "")
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		reversalID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO platform_journals (
				id, event_key, event_type, payload_hash, reverses_journal_id, reversal_reason, effective_at, metadata
			) VALUES ($1, $2, 'TEST_REVERSAL', $3, $4, 'early reversal guard', now() - interval '1 minute', '{"source_type":"test"}'::jsonb)
		`, reversalID, "journal.reversed:"+originalID, strings.Repeat("4", 64), originalID)
		require.NoError(t, err)
		insertLedgerEntry(t, tx, reversalID, "BANK_CASH", "", "CREDIT", 100)
		insertLedgerEntry(t, tx, reversalID, "FUNDING_CLEARING", "", "DEBIT", 100)
		_, err = tx.Exec(ctx, "SET CONSTRAINTS platform_journal_balance_guard IMMEDIATE")
		require.NoError(t, err)
		insertLedgerEntry(t, tx, reversalID, "BANK_CASH", "", "DEBIT", 1)
		insertLedgerEntry(t, tx, reversalID, "FUNDING_CLEARING", "", "CREDIT", 1)

		err = tx.Commit(ctx)
		require.Error(t, err)

		var journalCount, entryCount int
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals WHERE id = $1", reversalID).Scan(&journalCount))
		require.NoError(t, pool.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries WHERE journal_id = $1", reversalID).Scan(&entryCount))
		assert.Zero(t, journalCount)
		assert.Zero(t, entryCount)
	})
}

func TestLedgerMigrationPostFactDownRefuses(t *testing.T) {
	testDSN, m, pool := newLedgerMigrationDatabase(t, ledgerMigrationVersion)
	insertBalancedLedgerJournal(t, pool, "")
	pool.Close()

	err := m.Steps(-1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot remove platform ledger balance reschedule after a financial fact exists")

	// golang-migrate leaves the connection's transaction aborted after an
	// expected migration refusal. Close that handle to release its advisory
	// lock, then verify the dirty marker through a fresh handle before
	// discarding this disposable database.
	_, _ = m.Close()
	verificationM, verificationErr := migrate.New(getMigrationsPath(), testDSN)
	require.NoError(t, verificationErr)
	defer verificationM.Close()
	version, dirty, versionErr := verificationM.Version()
	require.NoError(t, versionErr)
	assert.Equal(t, uint(ledgerMigrationVersion-1), version)
	assert.True(t, dirty, "golang-migrate marks an intentionally refused down as dirty; disposable DB is discarded")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	poolAfterRefusal, err := database.NewPostgresPool(ctx, testDSN)
	require.NoError(t, err)
	defer poolAfterRefusal.Close()
	var journalCount, entryCount int
	require.NoError(t, poolAfterRefusal.QueryRow(ctx, "SELECT COUNT(*) FROM platform_journals").Scan(&journalCount))
	require.NoError(t, poolAfterRefusal.QueryRow(ctx, "SELECT COUNT(*) FROM platform_ledger_entries").Scan(&entryCount))
	assert.Equal(t, 1, journalCount)
	assert.Equal(t, 2, entryCount)

	var ledgerTables int
	require.NoError(t, poolAfterRefusal.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('platform_accounts', 'platform_journals', 'platform_ledger_entries')
	`).Scan(&ledgerTables))
	assert.Equal(t, 3, ledgerTables, "a refused down must not remove ledger tables")

	var ledgerFunctions int
	require.NoError(t, poolAfterRefusal.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc
		WHERE pronamespace = 'public'::regnamespace
		  AND proname = ANY($1::text[])
	`, []string{
		"validate_platform_journal_metadata",
		"prevent_platform_ledger_mutation",
		"prevent_platform_account_catalog_mutation",
		"stamp_platform_journal_creation",
		"validate_platform_journal_reversal_source",
		"validate_platform_ledger_entry_insert",
		"validate_platform_journal_balance",
		"validate_platform_journal_balance_for",
		"validate_platform_ledger_entry_balance",
	}).Scan(&ledgerFunctions))
	assert.Equal(t, 9, ledgerFunctions, "a refused down must keep ledger guard functions installed")
}
