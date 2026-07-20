package platformfinance

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"lapangango-api/internal/database"
)

type expectedException struct {
	Metric           string
	BucketDate       string
	ExpectedCount    int64
	ActualCount      int64
	DifferenceCount  int64
	ExpectedRupiah   int64
	ActualRupiah     int64
	DifferenceRupiah int64
}

type expectedCheckOutcome struct {
	Status     ReconciliationCheckStatus
	Exceptions []expectedException
}

type expectedReportOutcome struct {
	Clean  bool
	Status ReconciliationStatus
	Checks map[string]expectedCheckOutcome
}

func assertReportOutcome(t *testing.T, report *ReconciliationReport, expected expectedReportOutcome) {
	t.Helper()
	if report.Clean != expected.Clean {
		t.Errorf("report.Clean = %v, want %v", report.Clean, expected.Clean)
	}
	if report.Status != expected.Status {
		t.Errorf("report.Status = %v, want %v", report.Status, expected.Status)
	}
	if len(report.Checks) != 8 {
		t.Errorf("report has %d checks, want 8", len(report.Checks))
	}
	if len(expected.Checks) != 8 {
		t.Fatalf("expected matrix must define all 8 checks, got %d", len(expected.Checks))
	}

	actualChecks := make(map[string]ReconciliationCheckResult)
	for _, check := range report.Checks {
		if _, ok := actualChecks[check.Code]; ok {
			t.Errorf("duplicate check code in report: %s", check.Code)
		}
		actualChecks[check.Code] = check
	}

	for code, want := range expected.Checks {
		got, ok := actualChecks[code]
		if !ok {
			t.Errorf("missing check code in report: %s", code)
			continue
		}
		if got.Status != want.Status {
			t.Errorf("check %s Status = %v, want %v", code, got.Status, want.Status)
		}
		if len(got.Exceptions) != len(want.Exceptions) {
			t.Errorf("check %s Exceptions count = %d, want %d", code, len(got.Exceptions), len(want.Exceptions))
		}

		sort.Slice(got.Exceptions, func(i, j int) bool {
			if got.Exceptions[i].Metric != got.Exceptions[j].Metric {
				return got.Exceptions[i].Metric < got.Exceptions[j].Metric
			}
			if got.Exceptions[i].BucketDate != got.Exceptions[j].BucketDate {
				return got.Exceptions[i].BucketDate < got.Exceptions[j].BucketDate
			}
			return got.Exceptions[i].DifferenceRupiah < got.Exceptions[j].DifferenceRupiah
		})

		wantExceptions := make([]expectedException, len(want.Exceptions))
		copy(wantExceptions, want.Exceptions)
		sort.Slice(wantExceptions, func(i, j int) bool {
			if wantExceptions[i].Metric != wantExceptions[j].Metric {
				return wantExceptions[i].Metric < wantExceptions[j].Metric
			}
			if wantExceptions[i].BucketDate != wantExceptions[j].BucketDate {
				return wantExceptions[i].BucketDate < wantExceptions[j].BucketDate
			}
			return wantExceptions[i].DifferenceRupiah < wantExceptions[j].DifferenceRupiah
		})

		for i := 0; i < len(got.Exceptions) && i < len(wantExceptions); i++ {
			gotExc := got.Exceptions[i]
			wantExc := wantExceptions[i]
			if gotExc.Metric != wantExc.Metric || gotExc.BucketDate != wantExc.BucketDate ||
				gotExc.ExpectedCount != wantExc.ExpectedCount || gotExc.ActualCount != wantExc.ActualCount || gotExc.DifferenceCount != wantExc.DifferenceCount ||
				gotExc.ExpectedRupiah != wantExc.ExpectedRupiah || gotExc.ActualRupiah != wantExc.ActualRupiah || gotExc.DifferenceRupiah != wantExc.DifferenceRupiah {
				t.Errorf("check %s Exception[%d] = %+v, want %+v", code, i, gotExc, wantExc)
			}
		}
	}
}

func createReconciliationShadowSchema(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	_, err := tx.Exec(ctx, `
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

		CREATE TEMP TABLE platform_finance_cutovers (
			id smallint PRIMARY KEY DEFAULT 1,
			snapshot_cutover_at timestamptz NOT NULL
		);

		CREATE TEMP TABLE platform_expenses (
			id uuid PRIMARY KEY,
			status text NOT NULL,
			posted_journal_id uuid,
			void_journal_id uuid,
			amount_rupiah bigint NOT NULL
		);

		CREATE TEMP TABLE platform_journals (
			id uuid PRIMARY KEY,
			effective_at timestamptz NOT NULL
		);

		CREATE TEMP TABLE platform_ledger_entries (
			journal_id uuid NOT NULL,
			account_code text NOT NULL,
			side text NOT NULL,
			amount_rupiah bigint NOT NULL
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func reconcileInFixtureTx(t *testing.T, ctx context.Context, tx pgx.Tx, startDate, endDate string) *ReconciliationReport {
	t.Helper()
	starter := &fixedReconciliationTransactionStarter{tx: tx}
	repo := &reconciliationRepository{
		transactionStarter: starter,
		actualMetricsProvider: reconciliationActualMetricsProviderStub{
			metrics: unavailableMetrics(),
		},
	}
	report, err := NewReconciliationService(repo).Reconcile(ctx, ReconciliationQuery{
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if starter.calls != 1 {
		t.Fatalf("transaction calls=%d, want 1", starter.calls)
	}
	if starter.options.IsoLevel != pgx.RepeatableRead {
		t.Fatalf("isolation=%v, want RepeatableRead", starter.options.IsoLevel)
	}
	if starter.options.AccessMode != pgx.ReadOnly {
		t.Fatalf("access mode=%v, want ReadOnly", starter.options.AccessMode)
	}
	return report
}

func insertSnapshot(t *testing.T, ctx context.Context, tx pgx.Tx, bookingID, termsSource, bookingChannel, financeMode string, commBPS int, commAmount, finalPrice int64) {
	t.Helper()
	termID := "11111111-1111-1111-1111-111111111111"
	if termsSource == "LEGACY_NO_COMMISSION" {
		_, err := tx.Exec(ctx, `
			INSERT INTO booking_fee_snapshots (booking_id, terms_source, booking_channel, finance_mode, commission_amount_rupiah, final_booking_price_rupiah, commission_bps)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, bookingID, termsSource, bookingChannel, financeMode, commAmount, finalPrice, commBPS)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		_, err := tx.Exec(ctx, `
			INSERT INTO booking_fee_snapshots (booking_id, terms_source, booking_channel, finance_mode, commission_amount_rupiah, final_booking_price_rupiah, commission_bps, commercial_term_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, bookingID, termsSource, bookingChannel, financeMode, commAmount, finalPrice, commBPS, termID)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func insertOfflineMarker(t *testing.T, ctx context.Context, tx pgx.Tx, bookingID string) {
	t.Helper()
	_, err := tx.Exec(ctx, `INSERT INTO offline_booking_customers (booking_id) VALUES ($1)`, bookingID)
	if err != nil {
		t.Fatal(err)
	}
}

func insertCutover(t *testing.T, ctx context.Context, tx pgx.Tx, cutoverAt time.Time) {
	t.Helper()
	_, err := tx.Exec(ctx, `INSERT INTO platform_finance_cutovers (snapshot_cutover_at) VALUES ($1)`, cutoverAt)
	if err != nil {
		t.Fatal(err)
	}
}

func insertOPEXExpense(t *testing.T, ctx context.Context, tx pgx.Tx, expenseID, journalID, status string, amountRupiah int64, effectiveAt time.Time) {
	t.Helper()
	_, err := tx.Exec(ctx, `INSERT INTO platform_journals (id, effective_at) VALUES ($1, $2)`, journalID, effectiveAt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_ledger_entries (journal_id, account_code, side, amount_rupiah) VALUES ($1, 'OPEX_123', 'DEBIT', $2)`, journalID, amountRupiah)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_expenses (id, status, posted_journal_id, amount_rupiah) VALUES ($1, $2, $3, $4)`, expenseID, status, journalID, amountRupiah)
	if err != nil {
		t.Fatal(err)
	}
}

func insertOPEXVoidExpense(t *testing.T, ctx context.Context, tx pgx.Tx, expenseID, postedJournalID, voidJournalID, status string, amountRupiah int64, postedAt, voidAt time.Time) {
	t.Helper()
	_, err := tx.Exec(ctx, `INSERT INTO platform_journals (id, effective_at) VALUES ($1, $2)`, postedJournalID, postedAt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_ledger_entries (journal_id, account_code, side, amount_rupiah) VALUES ($1, 'OPEX_123', 'DEBIT', $2)`, postedJournalID, amountRupiah)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_journals (id, effective_at) VALUES ($1, $2)`, voidJournalID, voidAt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_ledger_entries (journal_id, account_code, side, amount_rupiah) VALUES ($1, 'OPEX_123', 'CREDIT', $2)`, voidJournalID, amountRupiah)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_expenses (id, status, posted_journal_id, void_journal_id, amount_rupiah) VALUES ($1, $2, $3, $4, $5)`, expenseID, status, postedJournalID, voidJournalID, amountRupiah)
	if err != nil {
		t.Fatal(err)
	}
}

func updateBookingPrice(t *testing.T, ctx context.Context, tx pgx.Tx, id, price string) {
	t.Helper()
	_, err := tx.Exec(ctx, `UPDATE bookings SET total_price=$2 WHERE id=$1`, id, price)
	if err != nil {
		t.Fatal(err)
	}
}

func emptyPassCheck() expectedCheckOutcome {
	return expectedCheckOutcome{Status: ReconciliationPass}
}

func emptyBlockedCheck() expectedCheckOutcome {
	return expectedCheckOutcome{Status: ReconciliationBlocked}
}

func cleanMatrix() map[string]expectedCheckOutcome {
	return map[string]expectedCheckOutcome{
		ReconciliationCheckOnline:    emptyPassCheck(),
		ReconciliationCheckSnapshot:  emptyPassCheck(),
		ReconciliationCheckOffline:   emptyPassCheck(),
		ReconciliationCheckRefund:    emptyPassCheck(),
		ReconciliationCheckDuplicate: emptyPassCheck(),
		ReconciliationCheckRollup:    emptyPassCheck(),
		ReconciliationCheckOPEX:      emptyPassCheck(),
		ReconciliationCheckActual:    emptyPassCheck(),
	}
}

func requireIntegrationEnv(t *testing.T) string {
	t.Helper()
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("set TEST_INTEGRATION=1 to run against PostgreSQL")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL must be provided when TEST_INTEGRATION=1 for safety")
	}
	return dsn
}

func TestReconciliationBoundarySuite(t *testing.T) {
	dsn := requireIntegrationEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	location := GetJakartaLocation()
	start := time.Date(2030, time.January, 10, 0, 0, 0, 0, location).UTC()
	endExclusive := time.Date(2030, time.January, 11, 0, 0, 0, 0, location).UTC()
	defaultBucket := "2030-01-10"

	t.Run("clean commission promo offline matrix", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000001"
		insertBooking(t, ctx, tx, b1, "COMPLETED", start.Add(time.Hour))
		insertOfflineMarker(t, ctx, tx, b1)
		insertSnapshot(t, ctx, tx, b1, "POLICY", "OWNER_WALK_IN", "SIMULATION", 0, 0, 100000)
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))

		b2 := "a0000000-0000-0000-0000-000000000002"
		insertBooking(t, ctx, tx, b2, "COMPLETED", start.Add(2*time.Hour))
		insertSnapshot(t, ctx, tx, b2, "POLICY", "MARKETPLACE_ONLINE", "SIMULATION", 500, 4000, 80000)
		updateBookingPrice(t, ctx, tx, b2, "100000")
		insertLedger(t, ctx, tx, b2, "INCOME", "BOOKING", "80000", start.Add(2*time.Hour))

		b3 := "a0000000-0000-0000-0000-000000000003"
		insertBooking(t, ctx, tx, b3, "COMPLETED", start.Add(3*time.Hour))
		insertSnapshot(t, ctx, tx, b3, "POLICY", "MARKETPLACE_ONLINE", "SIMULATION", 700, 7000, 100000)
		updateBookingPrice(t, ctx, tx, b3, "100000")
		insertLedger(t, ctx, tx, b3, "INCOME", "BOOKING", "100000", start.Add(3*time.Hour))

		b4 := "a0000000-0000-0000-0000-000000000004"
		insertBooking(t, ctx, tx, b4, "COMPLETED", start.Add(4*time.Hour))
		insertSnapshot(t, ctx, tx, b4, "LEGACY_NO_COMMISSION", "MARKETPLACE_ONLINE", "SIMULATION", 0, 0, 100000)
		updateBookingPrice(t, ctx, tx, b4, "100000")
		insertLedger(t, ctx, tx, b4, "INCOME", "BOOKING", "100000", start.Add(4*time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  true,
			Status: ReconciliationClean,
			Checks: cleanMatrix(),
		})
	})

	t.Run("clean exact refund reversal", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "b0000000-0000-0000-0000-000000000001"
		insertBooking(t, ctx, tx, b1, "CANCELLED", start.Add(time.Hour))
		insertSnapshot(t, ctx, tx, b1, "POLICY", "MARKETPLACE_ONLINE", "SIMULATION", 700, 7000, 100000)
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))
		insertLedger(t, ctx, tx, b1, "EXPENSE", "REFUND", "100000", start.Add(2*time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  true,
			Status: ReconciliationClean,
			Checks: cleanMatrix(),
		})
	})

	t.Run("clean OPEX post and void across Jakarta month boundary", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		e1 := "c0000000-0000-0000-0000-000000000001"
		p1 := "d0000000-0000-0000-0000-000000000001"
		v1 := "e0000000-0000-0000-0000-000000000001"
		postAt := time.Date(2030, time.August, 31, 23, 59, 59, 0, location).UTC()
		voidAt := time.Date(2030, time.September, 1, 0, 0, 0, 0, location).UTC()

		insertOPEXVoidExpense(t, ctx, tx, e1, p1, v1, "VOID", 100000, postAt, voidAt)

		report := reconcileInFixtureTx(t, ctx, tx, "2030-08-31", "2030-09-01")
		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  true,
			Status: ReconciliationClean,
			Checks: cleanMatrix(),
		})
	})

	t.Run("paid booking without ledger", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000001"
		insertBooking(t, ctx, tx, b1, "PAID", start.Add(time.Hour))
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertSnapshot(t, ctx, tx, b1, "POLICY", "MARKETPLACE_ONLINE", "SIMULATION", 700, 7000, 100000)

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "PAID_WITHOUT_LEDGER", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}
		matrix[ReconciliationCheckSnapshot] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "PAID_WITHOUT_LEDGER", BucketDate: defaultBucket, DifferenceCount: 1},
				{Metric: "paid_snapshot_ledger", BucketDate: defaultBucket, ExpectedCount: 1, DifferenceCount: 1, ExpectedRupiah: 100000, ActualRupiah: 0, DifferenceRupiah: 100000},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "PAID_WITHOUT_LEDGER", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("ledger without canonical booking", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000002"
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "LEDGER_WITHOUT_BOOKING", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "LEDGER_WITHOUT_BOOKING", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("duplicate income", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000003"
		insertBooking(t, ctx, tx, b1, "COMPLETED", start.Add(time.Hour))
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "DUPLICATE_INCOME", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckDuplicate] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "DUPLICATE_INCOME", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "DUPLICATE_INCOME", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("missing post-cutover snapshot", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, start.Add(-time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000004"
		insertBooking(t, ctx, tx, b1, "COMPLETED", start.Add(time.Hour))
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = emptyBlockedCheck()
		matrix[ReconciliationCheckSnapshot] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "paid_snapshot", BucketDate: defaultBucket, ExpectedCount: 1, ActualCount: 0, DifferenceCount: 1},
				{Metric: "MISSING_SNAPSHOT", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "MISSING_SNAPSHOT", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("post-cutover legacy snapshot", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, start.Add(-time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000005"
		insertBooking(t, ctx, tx, b1, "COMPLETED", start.Add(time.Hour))
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertSnapshot(t, ctx, tx, b1, "LEGACY_NO_COMMISSION", "MARKETPLACE_ONLINE", "SIMULATION", 0, 0, 100000)
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = emptyBlockedCheck()
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "POST_CUTOVER_LEGACY", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("source ledger mismatch", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000006"
		insertBooking(t, ctx, tx, b1, "COMPLETED", start.Add(time.Hour))
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "90000", start.Add(time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: defaultBucket, DifferenceCount: 1, DifferenceRupiah: 10000},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: defaultBucket, DifferenceCount: 1, DifferenceRupiah: 10000},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("refund amount mismatch", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000007"
		insertBooking(t, ctx, tx, b1, "CANCELLED", start.Add(time.Hour))
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))
		insertLedger(t, ctx, tx, b1, "EXPENSE", "REFUND", "99999", start.Add(2*time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = emptyBlockedCheck()
		matrix[ReconciliationCheckRefund] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "REFUND_MISMATCH", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "REFUND_MISMATCH", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("orphan refund", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		insertLedger(t, ctx, tx, "", "EXPENSE", "REFUND", "100000", start.Add(2*time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = emptyBlockedCheck()
		matrix[ReconciliationCheckRefund] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "ORPHAN_REFUND", BucketDate: defaultBucket, DifferenceCount: 1},
			},
		}
		matrix[ReconciliationCheckRollup] = emptyBlockedCheck()

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("offline nonzero commission", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000008"
		insertBooking(t, ctx, tx, b1, "COMPLETED", start.Add(time.Hour))
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertOfflineMarker(t, ctx, tx, b1)
		insertSnapshot(t, ctx, tx, b1, "POLICY", "OWNER_WALK_IN", "SIMULATION", 700, 7000, 100000)
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "100000", start.Add(time.Hour))

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = emptyBlockedCheck()
		matrix[ReconciliationCheckOffline] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "OFFLINE_COMMISSION", BucketDate: defaultBucket, DifferenceCount: 1},
				{Metric: "offline_commission", BucketDate: defaultBucket, ActualRupiah: 7000, DifferenceRupiah: 7000},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = emptyBlockedCheck()

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("OPEX source-journal mismatch", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		e1 := "c0000000-0000-0000-0000-000000000002"
		p1 := "d0000000-0000-0000-0000-000000000002"
		insertOPEXExpense(t, ctx, tx, e1, p1, "POSTED", 90000, start.Add(time.Hour))
		_, err := tx.Exec(ctx, `UPDATE platform_expenses SET amount_rupiah=100000 WHERE id=$1`, e1)
		if err != nil {
			t.Fatal(err)
		}

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "summary_trend_opex", BucketDate: defaultBucket, ExpectedRupiah: 100000, ActualRupiah: 90000, DifferenceRupiah: 10000},
			},
		}
		matrix[ReconciliationCheckOPEX] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "opex_posted_minus_reversal", BucketDate: defaultBucket, ExpectedRupiah: 100000, ActualRupiah: 90000, DifferenceRupiah: 10000},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("Jakarta midnight boundary", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000009"
		at1 := time.Date(2030, time.January, 9, 23, 59, 59, 0, location).UTC()
		insertBooking(t, ctx, tx, b1, "COMPLETED", at1)
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "95000", at1)

		b2 := "a0000000-0000-0000-0000-000000000010"
		at2 := time.Date(2030, time.January, 10, 0, 0, 0, 0, location).UTC()
		insertBooking(t, ctx, tx, b2, "COMPLETED", at2)
		updateBookingPrice(t, ctx, tx, b2, "100000")
		insertLedger(t, ctx, tx, b2, "INCOME", "BOOKING", "103000", at2)

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-09", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: "2030-01-09", DifferenceCount: 1, DifferenceRupiah: 5000},
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: "2030-01-10", DifferenceCount: 1, DifferenceRupiah: -3000},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: "2030-01-09", DifferenceCount: 1, DifferenceRupiah: 5000},
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: "2030-01-10", DifferenceCount: 1, DifferenceRupiah: -3000},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("half-open end boundary exclusion", func(t *testing.T) {
		tx := beginFixtureTx(t, ctx, pool)
		defer tx.Rollback(ctx)
		createReconciliationShadowSchema(t, ctx, tx)
		insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))

		b1 := "a0000000-0000-0000-0000-000000000011"
		at1 := endExclusive.Add(-time.Microsecond)
		insertBooking(t, ctx, tx, b1, "COMPLETED", at1)
		updateBookingPrice(t, ctx, tx, b1, "100000")
		insertLedger(t, ctx, tx, b1, "INCOME", "BOOKING", "92000", at1)

		b2 := "a0000000-0000-0000-0000-000000000012"
		at2 := endExclusive
		insertBooking(t, ctx, tx, b2, "COMPLETED", at2)
		updateBookingPrice(t, ctx, tx, b2, "100000")
		insertLedger(t, ctx, tx, b2, "INCOME", "BOOKING", "93000", at2)

		report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
		matrix := cleanMatrix()
		matrix[ReconciliationCheckOnline] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: defaultBucket, DifferenceCount: 1, DifferenceRupiah: 8000},
			},
		}
		matrix[ReconciliationCheckRefund] = emptyBlockedCheck()
		matrix[ReconciliationCheckRollup] = expectedCheckOutcome{
			Status: ReconciliationFail,
			Exceptions: []expectedException{
				{Metric: "SOURCE_LEDGER_MISMATCH", BucketDate: defaultBucket, DifferenceCount: 1, DifferenceRupiah: 8000},
			},
		}

		assertReportOutcome(t, report, expectedReportOutcome{
			Clean:  false,
			Status: ReconciliationExceptions,
			Checks: matrix,
		})
	})

	t.Run("transaction rollback isolation", func(t *testing.T) {
		scenarioBooking := "f0000000-0000-0000-0000-000000000016"
		scenarioExpense := "e0000000-0000-0000-0000-000000000001"
		scenarioJournal := "a0000000-0000-0000-0000-000000000001"

		checks := []struct {
			name        string
			query       string
			args        []any
			expectedPre int
		}{
			{
				name:        "bookings",
				query:       `SELECT count(*) FROM bookings WHERE id=$1`,
				args:        []any{scenarioBooking},
				expectedPre: 1,
			},
			{
				name:        "owner_finance_transactions",
				query:       `SELECT count(*) FROM owner_finance_transactions WHERE booking_id=$1`,
				args:        []any{scenarioBooking},
				expectedPre: 1,
			},
			{
				name:        "booking_fee_snapshots",
				query:       `SELECT count(*) FROM booking_fee_snapshots WHERE booking_id=$1`,
				args:        []any{scenarioBooking},
				expectedPre: 1,
			},
			{
				name:        "platform_expenses",
				query:       `SELECT count(*) FROM platform_expenses WHERE id=$1`,
				args:        []any{scenarioExpense},
				expectedPre: 1,
			},
			{
				name:        "platform_journals",
				query:       `SELECT count(*) FROM platform_journals WHERE id=$1`,
				args:        []any{scenarioJournal},
				expectedPre: 1,
			},
			{
				name:        "platform_ledger_entries",
				query:       `SELECT count(DISTINCT journal_id) FROM platform_ledger_entries WHERE journal_id=$1`,
				args:        []any{scenarioJournal},
				expectedPre: 1,
			},
		}

		func() {
			tx := beginFixtureTx(t, ctx, pool)
			defer tx.Rollback(ctx)
			createReconciliationShadowSchema(t, ctx, tx)
			insertCutover(t, ctx, tx, endExclusive.Add(time.Hour))
			insertBooking(t, ctx, tx, scenarioBooking, "COMPLETED", start.Add(time.Hour))
			updateBookingPrice(t, ctx, tx, scenarioBooking, "100000")
			insertLedger(t, ctx, tx, scenarioBooking, "INCOME", "BOOKING", "100000", start.Add(time.Hour))
			insertSnapshot(t, ctx, tx, scenarioBooking, "POLICY", "MARKETPLACE_ONLINE", "SIMULATION", 700, 7000, 100000)
			insertOPEXExpense(t, ctx, tx, scenarioExpense, scenarioJournal, "POSTED", 50000, start.Add(time.Hour))

			// Precondition check before Reconcile
			for _, check := range checks {
				var c int
				if err := tx.QueryRow(ctx, check.query, check.args...).Scan(&c); err != nil {
					t.Fatalf("precondition checking %s: %v", check.name, err)
				}
				if c != check.expectedPre {
					t.Fatalf("precondition %s failed: expected %d, got %d", check.name, check.expectedPre, c)
				}
			}

			report := reconcileInFixtureTx(t, ctx, tx, "2030-01-10", "2030-01-10")
			assertReportOutcome(t, report, expectedReportOutcome{
				Clean:  true,
				Status: ReconciliationClean,
				Checks: cleanMatrix(),
			})
		}()

		tx2, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx2.Rollback(ctx)

		// Postcondition check after rollback
		for _, check := range checks {
			var c int
			if err := tx2.QueryRow(ctx, check.query, check.args...).Scan(&c); err != nil {
				t.Fatalf("postcondition checking %s residue: %v", check.name, err)
			}
			if c != 0 {
				t.Fatalf("postcondition failed: found %d %s leaked from fixture", c, check.name)
			}
		}
	})
}
