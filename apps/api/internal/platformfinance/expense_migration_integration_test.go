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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/database"
)

const expenseMigrationVersion uint = 24

func newExpenseMigrationDatabase(t *testing.T, targetVersion uint) (string, *migrate.Migrate, *pgxpool.Pool) {
	t.Helper()

	if os.Getenv("TEST_EXPENSE_DISPOSABLE") != "1" {
		t.Skip("set TEST_EXPENSE_DISPOSABLE=1 to run expense migration integration tests")
	}

	baseDSN := os.Getenv("EXPENSE_TEST_DATABASE_URL")
	require.NotEmpty(t, baseDSN, "EXPENSE_TEST_DATABASE_URL is required")

	dbName := "lapangango_expense_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	testDSN, cleanup, err := createDisposableDB(baseDSN, dbName)
	require.NoError(t, err)

	migrationsPath := getMigrationsPath()
	require.NotEmpty(t, migrationsPath, "migrations path must be resolved")

	m, err := migrate.New(migrationsPath, testDSN)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(targetVersion))

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

func insertExpenseUser(t *testing.T, pool *pgxpool.Pool, role string) string {
	t.Helper()
	userID := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, name, email, password_hash, role, status)
		VALUES ($1, $2, $3, 'test-hash', $4::user_role, 'ACTIVE'::user_status)
	`, userID, "Expense Test User", "expense."+userID+"@test.local", role)
	require.NoError(t, err)
	return userID
}

func insertExpenseDraft(t *testing.T, pool *pgxpool.Pool, creatorID, vendor, externalReference string, occurredAt time.Time) string {
	t.Helper()
	expenseID := uuid.NewString()
	var vendorValue any
	if vendor != "" {
		vendorValue = vendor
	}
	var referenceValue any
	if externalReference != "" {
		referenceValue = externalReference
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO platform_expenses (
			id, category, vendor, amount_rupiah, currency, occurred_at,
			payment_account, external_reference, description, created_by_user_id
		) VALUES ($1, 'INFRASTRUCTURE', $2, 100000, 'IDR', $3,
		          'FUNDING_CLEARING', $4, 'Hosting test expense', $5)
	`, expenseID, vendorValue, occurredAt, referenceValue, creatorID)
	require.NoError(t, err)
	return expenseID
}

func execExpenseInsert(
	pool *pgxpool.Pool,
	creatorID, category string,
	amount int64,
	currency string,
	occurredAt time.Time,
	paymentAccount, vendor, externalReference, description, status string,
) error {
	return func() error {
		_, err := pool.Exec(context.Background(), `
			INSERT INTO platform_expenses (
				id, category, vendor, amount_rupiah, currency, occurred_at,
				payment_account, external_reference, description, status, created_by_user_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, uuid.NewString(), category, nullableExpenseText(vendor), amount, currency,
			occurredAt, paymentAccount, nullableExpenseText(externalReference), description, status, creatorID)
		return err
	}()
}

func execExpenseInsertWithNullField(pool *pgxpool.Pool, creatorID, nullField string, occurredAt time.Time) error {
	args := []any{
		uuid.NewString(),
		"INFRASTRUCTURE",
		"Vendor",
		int64(100000),
		"IDR",
		occurredAt,
		"FUNDING_CLEARING",
		"NULL-REF",
		"Description",
		"DRAFT",
		creatorID,
	}

	indices := map[string]int{
		"id":                 0,
		"category":           1,
		"amount_rupiah":      3,
		"currency":           4,
		"occurred_at":        5,
		"payment_account":    6,
		"description":        8,
		"status":             9,
		"created_by_user_id": 10,
	}
	index, ok := indices[nullField]
	if !ok {
		return fmt.Errorf("unsupported nullable test field %q", nullField)
	}
	args[index] = nil

	_, err := pool.Exec(context.Background(), `
		INSERT INTO platform_expenses (
			id, category, vendor, amount_rupiah, currency, occurred_at,
			payment_account, external_reference, description, status, created_by_user_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, args...)
	return err
}

func nullableExpenseText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func TestExpenseMigrationFreshSchemaAndTemporalContract(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	ctx := context.Background()
	creatorID := insertExpenseUser(t, pool, "SUPER_ADMIN")

	var tableCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('platform_expenses', 'platform_expense_idempotency')
	`).Scan(&tableCount))
	assert.Equal(t, 2, tableCount)

	var functionCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc
		WHERE pronamespace = 'public'::regnamespace
		  AND proname = ANY($1::text[])
	`, []string{
		"platform_expense_occurred_at_is_allowed",
		"validate_platform_expense_write",
		"prevent_platform_expense_idempotency_mutation",
	}).Scan(&functionCount))
	assert.Equal(t, 3, functionCount)

	var uniqueIndexCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = 'public'
		  AND indexname = 'uq_platform_expenses_vendor_external_reference'
	`).Scan(&uniqueIndexCount))
	assert.Equal(t, 1, uniqueIndexCount)

	var journalConstraintCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint
		WHERE conrelid = 'platform_expenses'::regclass
		  AND conname = ANY($1::text[])
	`, []string{
		"uq_platform_expenses_posted_journal",
		"uq_platform_expenses_void_journal",
		"chk_platform_expense_distinct_journals",
	}).Scan(&journalConstraintCount))
	assert.Equal(t, 3, journalConstraintCount)

	var restrictForeignKeyCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint
		WHERE conrelid = 'platform_expenses'::regclass
		  AND confrelid = 'platform_journals'::regclass
		  AND contype = 'f'
		  AND confdeltype = 'r'
	`).Scan(&restrictForeignKeyCount))
	assert.Equal(t, 2, restrictForeignKeyCount)

	ref := time.Now().UTC().Add(-time.Minute)
	insertExpenseDraft(t, pool, creatorID, "Acme Hosting", "INV-001", ref)

	var allowed bool
	referenceAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT platform_expense_occurred_at_is_allowed($1, $2)",
		referenceAt.Add(-90*24*time.Hour), referenceAt,
	).Scan(&allowed))
	assert.True(t, allowed)
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT platform_expense_occurred_at_is_allowed($1, $2)",
		referenceAt.Add(-90*24*time.Hour-time.Microsecond), referenceAt,
	).Scan(&allowed))
	assert.False(t, allowed)

	assert.Error(t, execExpenseInsert(pool, creatorID, "INFRASTRUCTURE", 100000, "IDR", time.Now().UTC().Add(time.Minute), "FUNDING_CLEARING", "Vendor Future", "INV-FUTURE", "Future expense", "DRAFT"))
	assert.Error(t, execExpenseInsert(pool, creatorID, "INFRASTRUCTURE", 100000, "IDR", time.Now().UTC().Add(-91*24*time.Hour), "FUNDING_CLEARING", "Vendor Old", "INV-OLD", "Old expense", "DRAFT"))
}

func TestExpenseMigrationFieldAndReferenceConstraints(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	creatorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	occurredAt := time.Now().UTC().Add(-time.Minute)

	cases := []struct {
		name           string
		category       string
		amount         int64
		currency       string
		paymentAccount string
		vendor         string
		externalRef    string
		description    string
		status         string
	}{
		{"unknown category", "UNKNOWN", 100000, "IDR", "FUNDING_CLEARING", "Vendor", "CAT-1", "Description", "DRAFT"},
		{"zero amount", "INFRASTRUCTURE", 0, "IDR", "FUNDING_CLEARING", "Vendor", "AMT-0", "Description", "DRAFT"},
		{"negative amount", "INFRASTRUCTURE", -1, "IDR", "FUNDING_CLEARING", "Vendor", "AMT-NEG", "Description", "DRAFT"},
		{"amount over cap", "INFRASTRUCTURE", 1000000001, "IDR", "FUNDING_CLEARING", "Vendor", "AMT-HIGH", "Description", "DRAFT"},
		{"invalid currency", "INFRASTRUCTURE", 100000, "USD", "FUNDING_CLEARING", "Vendor", "CUR-1", "Description", "DRAFT"},
		{"invalid payment account", "INFRASTRUCTURE", 100000, "IDR", "BANK_CASH", "Vendor", "ACC-1", "Description", "DRAFT"},
		{"empty vendor", "INFRASTRUCTURE", 100000, "IDR", "FUNDING_CLEARING", " ", "V-EMPTY", "Description", "DRAFT"},
		{"empty reference", "INFRASTRUCTURE", 100000, "IDR", "FUNDING_CLEARING", "Vendor", " ", "Description", "DRAFT"},
		{"reference without vendor", "INFRASTRUCTURE", 100000, "IDR", "FUNDING_CLEARING", "", "REF-NO-VENDOR", "Description", "DRAFT"},
		{"description with leading whitespace", "INFRASTRUCTURE", 100000, "IDR", "FUNDING_CLEARING", "Vendor", "DESC-1", " Description", "DRAFT"},
		{"non-draft insert", "INFRASTRUCTURE", 100000, "IDR", "FUNDING_CLEARING", "Vendor", "STATE-1", "Description", "APPROVED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execExpenseInsert(pool, creatorID, tc.category, tc.amount, tc.currency, occurredAt,
				tc.paymentAccount, tc.vendor, tc.externalRef, tc.description, tc.status)
			require.Error(t, err)
		})
	}

	for _, field := range []string{
		"id",
		"category",
		"amount_rupiah",
		"currency",
		"occurred_at",
		"payment_account",
		"description",
		"status",
		"created_by_user_id",
	} {
		t.Run("NULL "+field, func(t *testing.T) {
			require.Error(t, execExpenseInsertWithNullField(pool, creatorID, field, occurredAt))
		})
	}

	insertExpenseDraft(t, pool, creatorID, "Acme", "INV-DUP", occurredAt)
	require.Error(t, execExpenseInsert(pool, creatorID, "INFRASTRUCTURE", 100000, "IDR", occurredAt,
		"FUNDING_CLEARING", "acme", "inv-dup", "Duplicate invoice", "DRAFT"))
}

func TestExpenseMigrationStateGuardAndActorDelete(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	ctx := context.Background()
	creatorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	approverID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	posterID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	voiderID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	expenseID := insertExpenseDraft(t, pool, creatorID, "State Vendor", "STATE-1", time.Now().UTC().Add(-time.Minute))

	_, err := pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp(), posted_journal_id = $2, posted_by_user_id = $3
		WHERE id = $1
	`, expenseID, uuid.NewString(), posterID)
	require.Error(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'APPROVED', approved_at = clock_timestamp(), approved_by_user_id = $2
		WHERE id = $1
	`, expenseID, approverID)
	require.NoError(t, err)

	journalID, _ := insertBalancedLedgerJournal(t, pool, "")
	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp(), posted_journal_id = $2,
		    posted_by_user_id = $3, approved_by_user_id = $4
		WHERE id = $1
	`, expenseID, journalID, posterID, posterID)
	require.Error(t, err, "a transition cannot rewrite the historical approval actor")

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp(), posted_journal_id = $2,
		    posted_by_user_id = $3, approved_at = clock_timestamp()
		WHERE id = $1
	`, expenseID, journalID, posterID)
	require.Error(t, err, "a transition cannot rewrite the historical approval timestamp")

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp() + interval '1 hour',
		    posted_journal_id = $2, posted_by_user_id = $3
		WHERE id = $1
	`, expenseID, journalID, posterID)
	require.Error(t, err, "a transition timestamp cannot be future-dated")

	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, approverID)
	require.NoError(t, err)
	var approvedActor *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT approved_by_user_id::text FROM platform_expenses WHERE id = $1`, expenseID).Scan(&approvedActor))
	assert.Nil(t, approvedActor)

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'CANCELLED', cancelled_at = clock_timestamp(), cancelled_by_user_id = $2, cancel_reason = 'wrong draft'
		WHERE id = $1
	`, expenseID, creatorID)
	require.Error(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM platform_expenses WHERE id = $1`, expenseID)
	require.Error(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp(), posted_journal_id = $2, posted_by_user_id = $3
		WHERE id = $1
	`, expenseID, journalID, posterID)
	require.NoError(t, err)

	voidJournalID, _ := insertBalancedLedgerJournal(t, pool, "")
	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'VOID', voided_at = clock_timestamp(), void_journal_id = $2,
		    voided_by_user_id = $3, void_reason = 'void test', posted_by_user_id = $3,
		    posted_journal_id = $2
		WHERE id = $1
	`, expenseID, voidJournalID, voiderID)
	require.Error(t, err, "a void transition cannot rewrite the historical posting actor or journal")

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'VOID', voided_at = clock_timestamp(), void_journal_id = $2,
		    voided_by_user_id = $3, void_reason = 'void test', posted_at = clock_timestamp()
		WHERE id = $1
	`, expenseID, voidJournalID, voiderID)
	require.Error(t, err, "a void transition cannot rewrite the historical posting timestamp")

	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, posterID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'VOID', voided_at = clock_timestamp(), void_journal_id = $2,
		    voided_by_user_id = $3, void_reason = 'void test'
		WHERE id = $1
	`, expenseID, voidJournalID, voiderID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, creatorID)
	require.Error(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, voiderID)
	require.NoError(t, err)
}

func TestExpenseMigrationJournalLinkConstraints(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	ctx := context.Background()
	creatorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	actorID := insertExpenseUser(t, pool, "SUPER_ADMIN")

	firstID := insertExpenseDraft(t, pool, creatorID, "Journal Vendor One", "JOURNAL-1", time.Now().UTC().Add(-time.Minute))
	firstPostedJournalID, _ := insertBalancedLedgerJournal(t, pool, "")
	_, err := pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'APPROVED', approved_at = clock_timestamp(), approved_by_user_id = $2
		WHERE id = $1
	`, firstID, actorID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp(), posted_journal_id = $2, posted_by_user_id = $3
		WHERE id = $1
	`, firstID, firstPostedJournalID, actorID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM platform_journals WHERE id = $1`, firstPostedJournalID)
	require.Error(t, err, "referenced posted journals must be protected by FK RESTRICT")

	secondID := insertExpenseDraft(t, pool, creatorID, "Journal Vendor Two", "JOURNAL-2", time.Now().UTC().Add(-time.Minute))
	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'APPROVED', approved_at = clock_timestamp(), approved_by_user_id = $2
		WHERE id = $1
	`, secondID, actorID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp(), posted_journal_id = $2, posted_by_user_id = $3
		WHERE id = $1
	`, secondID, firstPostedJournalID, actorID)
	require.Error(t, err, "a posted journal can belong to only one expense")

	firstVoidJournalID, _ := insertBalancedLedgerJournal(t, pool, "")
	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'VOID', voided_at = clock_timestamp(), void_journal_id = $2,
		    voided_by_user_id = $3, void_reason = 'void test'
		WHERE id = $1
	`, firstID, firstVoidJournalID, actorID)
	require.NoError(t, err)

	secondPostedJournalID, _ := insertBalancedLedgerJournal(t, pool, "")
	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'POSTED', posted_at = clock_timestamp(), posted_journal_id = $2, posted_by_user_id = $3
		WHERE id = $1
	`, secondID, secondPostedJournalID, actorID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'VOID', voided_at = clock_timestamp(), void_journal_id = $2,
		    voided_by_user_id = $3, void_reason = 'duplicate void journal'
		WHERE id = $1
	`, secondID, firstVoidJournalID, actorID)
	require.Error(t, err, "a void journal can belong to only one expense")

	_, err = pool.Exec(ctx, `
		UPDATE platform_expenses
		SET status = 'VOID', voided_at = clock_timestamp(), void_journal_id = $2,
		    voided_by_user_id = $3, void_reason = 'same journal'
		WHERE id = $1
	`, secondID, secondPostedJournalID, actorID)
	require.Error(t, err, "posted and void journal links must be distinct")
}

func TestExpenseMigrationUpgradeAndDownSafety(t *testing.T) {
	_, migration, pool := newExpenseMigrationDatabase(t, 23)
	ctx := context.Background()
	creatorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	ledgerID, _ := insertBalancedLedgerJournal(t, pool, "")

	require.NoError(t, migration.Steps(1))
	var version uint
	var dirty bool
	version, dirty, err := migration.Version()
	require.NoError(t, err)
	assert.Equal(t, expenseMigrationVersion, version)
	assert.False(t, dirty)

	var preserved int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_journals WHERE id = $1`, ledgerID).Scan(&preserved))
	assert.Equal(t, 1, preserved)

	expenseID := insertExpenseDraft(t, pool, creatorID, "Down Vendor", "DOWN-1", time.Now().UTC().Add(-time.Minute))
	_, err = pool.Exec(ctx, `
		INSERT INTO platform_expense_idempotency (
			actor_user_id, action, idempotency_key, request_hash, expense_id,
			response_status, response_body
		) VALUES ($1, 'CREATE', 'down-key', repeat('a', 64), $2, 201, '{"id":"x"}'::jsonb)
	`, creatorID, expenseID)
	require.NoError(t, err)

	pool.Close()
	err = migration.Steps(-1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot remove platform expense migration after an expense fact exists")
	migration.Close()

	_, cleanMigration, cleanPool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	cleanPool.Close()
	require.NoError(t, cleanMigration.Steps(-1))
	version, dirty, err = cleanMigration.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(23), version)
	assert.False(t, dirty)
	cleanMigration.Close()
}

func TestExpenseMigrationIdempotencyStorageConstraints(t *testing.T) {
	_, _, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	creatorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	expenseID := insertExpenseDraft(t, pool, creatorID, "Idempotency Vendor", "IDEM-1", time.Now().UTC().Add(-time.Minute))

	_, err := pool.Exec(context.Background(), `
		INSERT INTO platform_expense_idempotency (
			actor_user_id, action, idempotency_key, request_hash, expense_id,
			response_status, response_body
		) VALUES ($1, 'CREATE', 'same-key', repeat('b', 64), $2, 201, '{"id":"x"}'::jsonb)
	`, creatorID, expenseID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO platform_expense_idempotency (
			actor_user_id, action, idempotency_key, request_hash, expense_id,
			response_status, response_body
		) VALUES ($1, 'CREATE', 'same-key', repeat('c', 64), $2, 201, '{"id":"y"}'::jsonb)
	`, creatorID, expenseID)
	require.Error(t, err)

	_, err = pool.Exec(context.Background(), `
		UPDATE platform_expense_idempotency SET request_hash = repeat('d', 64)
		WHERE actor_user_id = $1 AND action = 'CREATE' AND idempotency_key = 'same-key'
	`, creatorID)
	require.Error(t, err)
}

func TestExpenseMigrationPostFactDownRefusesWithoutDroppingTables(t *testing.T) {
	testDSN, migration, pool := newExpenseMigrationDatabase(t, expenseMigrationVersion)
	creatorID := insertExpenseUser(t, pool, "SUPER_ADMIN")
	insertExpenseDraft(t, pool, creatorID, "Post Fact Vendor", "POST-FACT-1", time.Now().UTC().Add(-time.Minute))

	pool.Close()
	require.Error(t, migration.Steps(-1))
	migration.Close()

	verificationMigration, err := migrate.New(getMigrationsPath(), testDSN)
	require.NoError(t, err)
	defer verificationMigration.Close()
	version, dirty, err := verificationMigration.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(23), version)
	assert.True(t, dirty)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	verificationPool, err := database.NewPostgresPool(ctx, testDSN)
	require.NoError(t, err)
	defer verificationPool.Close()
	var expenseCount int
	require.NoError(t, verificationPool.QueryRow(ctx, `SELECT COUNT(*) FROM platform_expenses`).Scan(&expenseCount))
	assert.Equal(t, 1, expenseCount)
}
