package platformfinance

import (
	"context"
	"os"
	"testing"
	"time"

	"lapangango-api/internal/database"

	"github.com/jackc/pgx/v5"
)

type fixedReconciliationTransactionStarter struct {
	tx      pgx.Tx
	options pgx.TxOptions
	calls   int
}

type countingReconciliationTx struct {
	pgx.Tx
	queryCalls int
}

func (tx *countingReconciliationTx) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	tx.queryCalls++
	return tx.Tx.Query(ctx, query, args...)
}

func (s *fixedReconciliationTransactionStarter) BeginTx(_ context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	s.options = options
	s.calls++
	return s.tx, nil
}

func TestReconciliationRepositoryCleanReadOnlySnapshot(t *testing.T) {
	if os.Getenv("TEST_RECONCILIATION_INTEGRATION") != "1" {
		t.Skip("set TEST_RECONCILIATION_INTEGRATION=1 to run against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	start := time.Date(2000, time.January, 1, 0, 0, 0, 0, GetJakartaLocation()).UTC()
	end := time.Date(2000, time.January, 2, 0, 0, 0, 0, GetJakartaLocation()).UTC()
	actualMetricsProvider := NewProductionReconciliationActualMetricsProvider()
	reconciliationRepo := NewReconciliationRepository(pool, actualMetricsProvider)
	snapshot, err := reconciliationRepo.LoadReconciliationSnapshot(ctx, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil {
		t.Fatal("expected a read-only reconciliation snapshot")
	}
	report, err := NewReconciliationService(reconciliationRepo).Reconcile(ctx, ReconciliationQuery{StartDate: "2000-01-01", EndDate: "2000-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Checks) != 8 {
		t.Fatalf("expected eight reconciliation checks, got %d", len(report.Checks))
	}
	if !report.Clean {
		t.Fatalf("expected empty range to be clean, got status=%s checks=%v", report.Status, report.Checks)
	}

	actualContribution := "1"
	actualMetrics := unavailableMetrics()
	actualMetrics.TransactionContributionValue = &actualContribution
	actualRepo := NewReconciliationRepository(pool, reconciliationActualMetricsProviderStub{metrics: actualMetrics})
	actualReport, err := NewReconciliationService(actualRepo).Reconcile(ctx, ReconciliationQuery{StartDate: "2000-01-01", EndDate: "2000-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if check := checkByCode(actualReport, ReconciliationCheckActual); check.Status != ReconciliationFail {
		t.Fatalf("actual metrics check status=%s, want FAIL for non-null production summary value", check.Status)
	}
}

func TestReconciliationMissingSnapshotPreflightIncludesNonPaidPostCutover(t *testing.T) {
	if os.Getenv("TEST_RECONCILIATION_INTEGRATION") != "1" {
		t.Skip("set TEST_RECONCILIATION_INTEGRATION=1 to run against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tx := beginFixtureTx(t, ctx, pool)
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE booking_fee_snapshots (
			booking_id uuid PRIMARY KEY,
			terms_source text,
			final_booking_price_rupiah bigint,
			finance_mode text,
			booking_channel text,
			commission_bps integer,
			commission_amount_rupiah bigint
		)`); err != nil {
		t.Fatal(err)
	}

	location := GetJakartaLocation()
	start := time.Date(2030, time.January, 10, 0, 0, 0, 0, location).UTC()
	end := time.Date(2030, time.January, 11, 0, 0, 0, 0, location).UTC()
	cutover := start.Add(-time.Hour)
	insertBooking(t, ctx, tx, "eeeeeeee-0000-0000-0000-000000000001", "PENDING_PAYMENT", start.Add(time.Hour))

	issues, err := loadReconciliationDataIssues(ctx, tx, start, end, cutover)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		if issue.Code == "MISSING_SNAPSHOT" {
			if issue.BucketDate != "2030-01-10" || issue.DifferenceCount != 1 {
				t.Fatalf("missing snapshot issue=%#v, want Jakarta bucket 2030-01-10 count 1", issue)
			}
			return
		}
	}
	t.Fatalf("issues=%#v, want dated MISSING_SNAPSHOT for non-paid post-cutover booking", issues)
}

func TestReconciliationSourceLedgerPreflightRejectsOffsettingAndFractionalRows(t *testing.T) {
	if os.Getenv("TEST_RECONCILIATION_INTEGRATION") != "1" {
		t.Skip("set TEST_RECONCILIATION_INTEGRATION=1 to run against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tx := beginFixtureTx(t, ctx, pool)
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE booking_fee_snapshots (
			booking_id uuid PRIMARY KEY,
			terms_source text,
			final_booking_price_rupiah bigint,
			finance_mode text,
			booking_channel text,
			commission_bps integer,
			commission_amount_rupiah bigint
		)`); err != nil {
		t.Fatal(err)
	}

	location := GetJakartaLocation()
	start := time.Date(2030, time.January, 10, 0, 0, 0, 0, location).UTC()
	end := time.Date(2030, time.January, 11, 0, 0, 0, 0, location).UTC()
	fixtures := []struct {
		id           string
		sourceAmount string
		ledgerAmount string
		at           time.Time
	}{
		{id: "eeeeeeee-1000-0000-0000-000000000001", sourceAmount: "100", ledgerAmount: "90", at: start.Add(time.Hour)},
		{id: "eeeeeeee-1000-0000-0000-000000000002", sourceAmount: "200", ledgerAmount: "210", at: start.Add(2 * time.Hour)},
		{id: "eeeeeeee-1000-0000-0000-000000000003", sourceAmount: "100.50", ledgerAmount: "100", at: start.Add(3 * time.Hour)},
	}
	for _, fixture := range fixtures {
		insertBooking(t, ctx, tx, fixture.id, "COMPLETED", fixture.at)
		if _, err := tx.Exec(ctx, `UPDATE bookings SET total_price=$2 WHERE id=$1`, fixture.id, fixture.sourceAmount); err != nil {
			t.Fatal(err)
		}
		insertLedger(t, ctx, tx, fixture.id, "INCOME", "BOOKING", fixture.ledgerAmount, fixture.at)
	}

	issues, err := loadReconciliationDataIssues(ctx, tx, start, end, end.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var mismatchDeltas []int64
	var fractionalCount int64
	for _, issue := range issues {
		switch issue.Code {
		case "SOURCE_LEDGER_MISMATCH":
			if issue.BucketDate != "2030-01-10" {
				t.Fatalf("mismatch bucket=%q, want 2030-01-10", issue.BucketDate)
			}
			mismatchDeltas = append(mismatchDeltas, issue.DifferenceRupiah)
		case "FRACTIONAL_SOURCE":
			if issue.BucketDate != "2030-01-10" {
				t.Fatalf("fractional bucket=%q, want 2030-01-10", issue.BucketDate)
			}
			fractionalCount += issue.DifferenceCount
		}
	}
	if len(mismatchDeltas) != 2 || mismatchDeltas[0] != 10 || mismatchDeltas[1] != -10 {
		t.Fatalf("mismatch deltas=%v, want independent +10/-10 rows", mismatchDeltas)
	}
	if fractionalCount != 1 {
		t.Fatalf("fractional source count=%d, want 1", fractionalCount)
	}
}

func TestReconciliationMaximumRangeBreakdownUsesOneQueryAndJakartaDates(t *testing.T) {
	if os.Getenv("TEST_RECONCILIATION_INTEGRATION") != "1" {
		t.Skip("set TEST_RECONCILIATION_INTEGRATION=1 to run against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tx := beginFixtureTx(t, ctx, pool)
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE booking_fee_snapshots (
			booking_id uuid PRIMARY KEY,
			terms_source text,
			final_booking_price_rupiah bigint,
			finance_mode text,
			booking_channel text,
			commission_bps integer,
			commission_amount_rupiah bigint
		)`); err != nil {
		t.Fatal(err)
	}

	location := GetJakartaLocation()
	// 2032 is a leap year: this is the maximum accepted inclusive 366-day
	// reconciliation range, represented here as [start, endExclusive).
	start := time.Date(2032, time.January, 1, 0, 0, 0, 0, location).UTC()
	end := time.Date(2033, time.January, 1, 0, 0, 0, 0, location).UTC()
	fixtures := []struct {
		id     string
		amount string
		at     time.Time
	}{
		{id: "eeeeeeee-2000-0000-0000-000000000001", amount: "100", at: start.Add(time.Hour)},
		{id: "eeeeeeee-2000-0000-0000-000000000002", amount: "200", at: end.Add(-time.Hour)},
	}
	for _, fixture := range fixtures {
		insertBooking(t, ctx, tx, fixture.id, "COMPLETED", fixture.at)
		if _, err := tx.Exec(ctx, `UPDATE bookings SET total_price=$2 WHERE id=$1`, fixture.id, fixture.amount); err != nil {
			t.Fatal(err)
		}
		insertLedger(t, ctx, tx, fixture.id, "INCOME", "BOOKING", fixture.amount, fixture.at)
	}
	// Refund counts now come from the same grouped production query rather than
	// a second in-memory event scan; keep that parity covered on the last day.
	insertLedger(t, ctx, tx, fixtures[1].id, "EXPENSE", "REFUND", fixtures[1].amount, fixtures[1].at.Add(30*time.Minute))

	ledger, source, err := loadReconciliationOnlineBuckets(ctx, tx, start, end)
	if err != nil {
		t.Fatal(err)
	}
	countingTx := &countingReconciliationTx{Tx: tx}
	owner, venue, err := loadProductionReconciliationBreakdownBuckets(ctx, countingTx, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if countingTx.queryCalls != 1 {
		t.Fatalf("production breakdown query calls=%d, want exactly 1 for a 366-day range", countingTx.queryCalls)
	}
	for name, buckets := range map[string][]ReconciliationBucketTotals{"ledger": ledger, "source": source, "owner": owner, "venue": venue} {
		if len(buckets) != 2 || buckets[0].BucketDate != "2032-01-01" || buckets[1].BucketDate != "2032-12-31" {
			t.Fatalf("%s buckets=%#v, want exact Jakarta dates 2032-01-01/2032-12-31", name, buckets)
		}
	}
	if source[0].Totals.Gross != 100 || ledger[1].Totals.Gross != 200 || owner[0].Totals.Commission != 7 ||
		venue[1].Totals.Refund != 200 || venue[1].Totals.RefundCount != 1 || venue[1].Totals.Commission != 0 {
		t.Fatalf("unexpected daily totals: ledger=%#v source=%#v owner=%#v venue=%#v", ledger, source, owner, venue)
	}
}

func TestReconciliationFullRepositoryPathRejectsOffsettingAndFractionalSource(t *testing.T) {
	if os.Getenv("TEST_RECONCILIATION_INTEGRATION") != "1" {
		t.Skip("set TEST_RECONCILIATION_INTEGRATION=1 to run against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable"
	}
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tx := beginFixtureTx(t, ctx, pool)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE booking_fee_snapshots (
			booking_id uuid PRIMARY KEY,
			terms_source text,
			booking_channel text,
			finance_mode text,
			commission_amount_rupiah bigint,
			final_booking_price_rupiah bigint,
			commission_bps integer,
			commercial_term_id uuid
		);
		CREATE TEMP TABLE platform_finance_cutovers (id smallint PRIMARY KEY DEFAULT 1, snapshot_cutover_at timestamptz NOT NULL);
		CREATE TEMP TABLE platform_expenses (
			id uuid PRIMARY KEY,
			status text NOT NULL,
			posted_journal_id uuid,
			void_journal_id uuid,
			amount_rupiah bigint NOT NULL
		);
		CREATE TEMP TABLE platform_journals (id uuid PRIMARY KEY, effective_at timestamptz NOT NULL);
		CREATE TEMP TABLE platform_ledger_entries (
			journal_id uuid NOT NULL,
			account_code text NOT NULL,
			side text NOT NULL,
			amount_rupiah bigint NOT NULL
		)`); err != nil {
		t.Fatal(err)
	}

	location := GetJakartaLocation()
	start := time.Date(2030, time.January, 10, 0, 0, 0, 0, location).UTC()
	end := time.Date(2030, time.January, 11, 0, 0, 0, 0, location).UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO platform_finance_cutovers (snapshot_cutover_at) VALUES ($1)`, end.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		id           string
		sourceAmount string
		ledgerAmount string
		at           time.Time
	}{
		{id: "eeeeeeee-3000-0000-0000-000000000001", sourceAmount: "100", ledgerAmount: "90", at: start.Add(time.Hour)},
		{id: "eeeeeeee-3000-0000-0000-000000000002", sourceAmount: "200", ledgerAmount: "210", at: start.Add(2 * time.Hour)},
		{id: "eeeeeeee-3000-0000-0000-000000000003", sourceAmount: "100.50", ledgerAmount: "100", at: start.Add(3 * time.Hour)},
	}
	for _, fixture := range fixtures {
		insertBooking(t, ctx, tx, fixture.id, "COMPLETED", fixture.at)
		if _, err := tx.Exec(ctx, `UPDATE bookings SET total_price=$2 WHERE id=$1`, fixture.id, fixture.sourceAmount); err != nil {
			t.Fatal(err)
		}
		insertLedger(t, ctx, tx, fixture.id, "INCOME", "BOOKING", fixture.ledgerAmount, fixture.at)
	}

	starter := &fixedReconciliationTransactionStarter{tx: tx}
	repo := &reconciliationRepository{
		transactionStarter:    starter,
		actualMetricsProvider: reconciliationActualMetricsProviderStub{metrics: unavailableMetrics()},
	}
	report, err := NewReconciliationService(repo).Reconcile(ctx, ReconciliationQuery{StartDate: "2030-01-10", EndDate: "2030-01-10"})
	if err != nil {
		t.Fatalf("full repository/service reconciliation returned raw error: %v", err)
	}
	if starter.calls != 1 || starter.options.IsoLevel != pgx.RepeatableRead || starter.options.AccessMode != pgx.ReadOnly {
		t.Fatalf("transaction evidence calls=%d options=%#v, want one repeatable-read/read-only transaction", starter.calls, starter.options)
	}
	if report.Clean {
		t.Fatalf("report unexpectedly CLEAN: %#v", report)
	}
	check := checkByCode(report, ReconciliationCheckOnline)
	if check.Status != ReconciliationFail {
		t.Fatalf("online check status=%s, want FAIL", check.Status)
	}
	var mismatchDeltas []int64
	var fractionalCount int64
	for _, exception := range check.Exceptions {
		if exception.BucketDate != "2030-01-10" {
			t.Fatalf("exception bucket=%q, want 2030-01-10", exception.BucketDate)
		}
		switch exception.Metric {
		case "SOURCE_LEDGER_MISMATCH":
			mismatchDeltas = append(mismatchDeltas, exception.DifferenceRupiah)
		case "FRACTIONAL_SOURCE":
			fractionalCount += exception.DifferenceCount
		}
	}
	if len(mismatchDeltas) != 2 || mismatchDeltas[0] != 10 || mismatchDeltas[1] != -10 {
		t.Fatalf("full-path mismatch deltas=%v, want independent +10/-10", mismatchDeltas)
	}
	if fractionalCount != 1 {
		t.Fatalf("full-path fractional count=%d, want 1", fractionalCount)
	}
}
