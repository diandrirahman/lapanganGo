package platformfinance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type reconciliationFixtureRepository struct {
	snapshot *ReconciliationSnapshot
	err      error
	start    time.Time
	end      time.Time
}

type reconciliationActualMetricsProviderStub struct {
	metrics ReconciliationActualMetrics
	err     error
}

func (s reconciliationActualMetricsProviderStub) LoadActualMetrics(context.Context, time.Time, time.Time) (ReconciliationActualMetrics, error) {
	return s.metrics, s.err
}

func (r *reconciliationFixtureRepository) LoadReconciliationSnapshot(_ context.Context, start, end time.Time) (*ReconciliationSnapshot, error) {
	r.start, r.end = start, end
	return r.snapshot, r.err
}

func TestReconciliationCleanEmptyRange(t *testing.T) {
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{AsOf: time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC), ActualMetrics: unavailableMetrics()}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.NoError(t, err)
	require.True(t, report.Clean)
	require.Equal(t, ReconciliationClean, report.Status)
	require.Equal(t, "Asia/Jakarta", report.Timezone)
	require.Equal(t, "2026-07-01", report.Period.StartDate)
	require.Len(t, report.Checks, 8)
	for _, check := range report.Checks {
		require.Equal(t, ReconciliationPass, check.Status, check.Code)
	}
	require.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, GetJakartaLocation()).UTC(), repo.start)
	require.Equal(t, time.Date(2026, 7, 2, 0, 0, 0, 0, GetJakartaLocation()).UTC(), repo.end)
}

func TestReconciliationRejectsImplicitOrInvalidRange(t *testing.T) {
	service := NewReconciliationService(&reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{ActualMetrics: unavailableMetrics()}})
	tests := []ReconciliationQuery{
		{},
		{StartDate: "2026-07-01"},
		{EndDate: "2026-07-01"},
		{StartDate: "2026-07-02", EndDate: "2026-07-01"},
		{StartDate: "2026-01-01", EndDate: "2027-01-02"},
	}
	for _, query := range tests {
		_, err := service.Reconcile(context.Background(), query)
		require.ErrorIs(t, err, ErrReconciliationInvalidRange, query)
	}
}

func TestReconciliationAcceptsMaximumJakartaRange(t *testing.T) {
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{ActualMetrics: unavailableMetrics()}}
	_, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2024-01-01", EndDate: "2024-12-31"})
	require.NoError(t, err)
	require.Equal(t, 366*24*time.Hour, repo.end.Sub(repo.start))

	_, err = NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2024-01-01", EndDate: "2025-01-01"})
	require.ErrorIs(t, err, ErrReconciliationInvalidRange)
}

func TestReconciliationReportsExactFaultBuckets(t *testing.T) {
	cutover := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	bookingAt := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{
		AsOf:          bookingAt,
		CutoverAt:     cutover,
		SummaryTotals: ReconciliationTotals{Gross: 100_000, Commission: 7_000, BookingCount: 1, OperatingExpense: 5_000},
		SourceTotals:  ReconciliationTotals{Gross: 100_000, BookingCount: 1},
		LedgerTotals:  ReconciliationTotals{Gross: 90_000, BookingCount: 1},
		OwnerTotals:   []ReconciliationTotals{{Gross: 90_000, Commission: 7_000, BookingCount: 1}},
		VenueTotals:   []ReconciliationTotals{{Gross: 90_000, Commission: 7_000, BookingCount: 1}},
		TrendTotals:   []ReconciliationBucketTotals{{BucketDate: "2026-07-01", Totals: ReconciliationTotals{Gross: 90_000, Commission: 7_000, BookingCount: 1, OperatingExpense: 4_000}}},
		PaidBookings:  []ReconciliationPaidBooking{{CreatedAt: bookingAt, Snapshot: false, SnapshotFinalPrice: 100_000, LedgerCount: 1, LedgerAmount: 100_000}},
		RefundEvents:  []ReconciliationEvent{{EventAt: bookingAt, Amount: 40_000, OriginalAmount: 50_000, Exact: false}},
		OPEXEvents:    []ReconciliationOPEXEvent{{EffectiveAt: bookingAt, ExpectedAmount: 4_000, Amount: 4_000, ExactLink: true}},
		DataIssues: []ReconciliationDataIssue{{
			Code: "DUPLICATE_INCOME", BucketDate: "2026-07-01", DifferenceCount: 2, Reason: "two income events for one booking",
		}},
		ActualMetrics: unavailableMetrics(),
	}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.NoError(t, err)
	require.False(t, report.Clean)
	require.Equal(t, ReconciliationExceptions, report.Status)
	require.Equal(t, []string{
		ReconciliationCheckOnline,
		ReconciliationCheckSnapshot,
		ReconciliationCheckOffline,
		ReconciliationCheckRefund,
		ReconciliationCheckDuplicate,
		ReconciliationCheckRollup,
		ReconciliationCheckOPEX,
		ReconciliationCheckActual,
	}, checkCodes(report))
	require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckOnline).Status)
	require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckSnapshot).Status)
	require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckRefund).Status)
	require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckDuplicate).Status)
	require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckRollup).Status)
	require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckOPEX).Status)
	require.Equal(t, "2026-07-01", checkByCode(report, ReconciliationCheckRefund).Exceptions[0].BucketDate)
}

func TestReconciliationDailyComparisonsNeverEmitPeriodBucket(t *testing.T) {
	dayOne := "2026-07-01"
	dayTwo := "2026-07-02"
	summary := []ReconciliationBucketTotals{
		{BucketDate: dayOne, Totals: ReconciliationTotals{Gross: 100, Net: 100, Commission: 7, BookingCount: 1}},
		{BucketDate: dayTwo, Totals: ReconciliationTotals{Gross: 200, Net: 200, Commission: 14, BookingCount: 1, OperatingExpense: 5}},
	}
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{
		AsOf:           time.Date(2026, 7, 2, 10, 0, 0, 0, GetJakartaLocation()),
		SummaryBuckets: summary,
		SourceBuckets:  []ReconciliationBucketTotals{{BucketDate: dayOne, Totals: ReconciliationTotals{Gross: 100, Net: 100, BookingCount: 1}}},
		LedgerBuckets:  []ReconciliationBucketTotals{{BucketDate: dayOne, Totals: ReconciliationTotals{Gross: 90, Net: 90, BookingCount: 1}}},
		OwnerBuckets: []ReconciliationBucketTotals{
			{BucketDate: dayOne, Totals: summary[0].Totals},
			{BucketDate: dayTwo, Totals: ReconciliationTotals{Gross: 190, Net: 190, Commission: 14, BookingCount: 1}},
		},
		VenueBuckets: []ReconciliationBucketTotals{
			{BucketDate: dayOne, Totals: summary[0].Totals},
			{BucketDate: dayTwo, Totals: ReconciliationTotals{Gross: 200, Net: 200, Commission: 14, BookingCount: 1}},
		},
		TrendTotals: []ReconciliationBucketTotals{
			{BucketDate: dayOne, Totals: summary[0].Totals},
			{BucketDate: dayTwo, Totals: summary[1].Totals},
		},
		OPEXSourceBuckets: []ReconciliationBucketTotals{{BucketDate: dayTwo, Totals: ReconciliationTotals{OperatingExpense: 5}}},
		OPEXEvents:        []ReconciliationOPEXEvent{{EffectiveAt: time.Date(2026, 7, 2, 12, 0, 0, 0, GetJakartaLocation()), ExpectedAmount: 4, Amount: 4, ExactLink: true}},
		ActualMetrics:     unavailableMetrics(),
	}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: dayOne, EndDate: dayTwo})
	require.NoError(t, err)
	require.False(t, report.Clean)
	require.Equal(t, dayOne, checkByCode(report, ReconciliationCheckOnline).Exceptions[0].BucketDate)
	require.Equal(t, dayTwo, checkByCode(report, ReconciliationCheckRollup).Exceptions[0].BucketDate)
	require.Equal(t, dayTwo, checkByCode(report, ReconciliationCheckOPEX).Exceptions[0].BucketDate)
	for _, check := range report.Checks {
		for _, exception := range check.Exceptions {
			_, err := time.Parse("2006-01-02", exception.BucketDate)
			require.NoError(t, err, "check=%s metric=%s bucket=%q", check.Code, exception.Metric, exception.BucketDate)
			require.NotEqual(t, "period", exception.BucketDate)
			require.NotEqual(t, "unknown", exception.BucketDate)
		}
	}
}

func TestReconciliationOffsettingSourceLedgerIssuesRemainFailures(t *testing.T) {
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{
		AsOf:                      time.Date(2030, 1, 10, 12, 0, 0, 0, GetJakartaLocation()),
		ProjectionFactsIncomplete: true,
		DataIssues: []ReconciliationDataIssue{
			{Code: "SOURCE_LEDGER_MISMATCH", BucketDate: "2030-01-10", DifferenceCount: 1, DifferenceRupiah: 10},
			{Code: "SOURCE_LEDGER_MISMATCH", BucketDate: "2030-01-10", DifferenceCount: 1, DifferenceRupiah: -10},
		},
		ActualMetrics: unavailableMetrics(),
	}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2030-01-10", EndDate: "2030-01-10"})
	require.NoError(t, err)
	check := checkByCode(report, ReconciliationCheckOnline)
	require.Equal(t, ReconciliationFail, check.Status)
	require.Len(t, check.Exceptions, 2)
	require.Equal(t, int64(2), check.DifferenceCount)
	require.Equal(t, int64(0), check.DifferenceRupiah, "net zero must not turn two unexplained row differences into PASS")
	require.False(t, report.Clean)
}

func TestReconciliationActualMetricsMustRemainUnavailableBeforeLiveGate(t *testing.T) {
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{ActualMetrics: ReconciliationActualMetrics{ActualCommissionRevenue: "AVAILABLE"}}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.NoError(t, err)
	require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckActual).Status)
	require.Contains(t, checkByCode(report, ReconciliationCheckActual).Reason, "live reconciliation gate")
}

func TestReconciliationActualMetricsRejectsUnavailableLabelWithActualValue(t *testing.T) {
	actual := "0"
	metrics := unavailableMetrics()
	metrics.ActualCommissionRevenueValue = &actual
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{ActualMetrics: metrics}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.NoError(t, err)
	check := checkByCode(report, ReconciliationCheckActual)
	require.Equal(t, ReconciliationFail, check.Status)
	require.Contains(t, check.Reason, "actual gateway/platform metrics")
}

func TestReconciliationActualMetricsRejectsEveryNullableActualField(t *testing.T) {
	tests := []struct {
		name string
		set  func(*ReconciliationActualMetrics)
	}{
		{name: "gateway captured gmv", set: func(metrics *ReconciliationActualMetrics) { value := "1"; metrics.GatewayCapturedGMVValue = &value }},
		{name: "actual commission revenue", set: func(metrics *ReconciliationActualMetrics) {
			value := "1"
			metrics.ActualCommissionRevenueValue = &value
		}},
		{name: "payment processing expense", set: func(metrics *ReconciliationActualMetrics) {
			value := "1"
			metrics.PaymentProcessingExpenseValue = &value
		}},
		{name: "owner payable", set: func(metrics *ReconciliationActualMetrics) { value := "1"; metrics.OwnerPayableValue = &value }},
		{name: "platform revenue", set: func(metrics *ReconciliationActualMetrics) { value := "1"; metrics.PlatformRevenueValue = &value }},
		{name: "transaction contribution", set: func(metrics *ReconciliationActualMetrics) {
			value := "1"
			metrics.TransactionContributionValue = &value
		}},
		{name: "operating result", set: func(metrics *ReconciliationActualMetrics) { value := "1"; metrics.OperatingResultValue = &value }},
		{name: "gross take rate", set: func(metrics *ReconciliationActualMetrics) { value := 1; metrics.GrossTakeRateBPSValue = &value }},
		{name: "net take rate", set: func(metrics *ReconciliationActualMetrics) { value := 1; metrics.NetTakeRateBPSValue = &value }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metrics := unavailableMetrics()
			tc.set(&metrics)
			repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{ActualMetrics: metrics}}
			report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
			require.NoError(t, err)
			require.Equal(t, ReconciliationFail, checkByCode(report, ReconciliationCheckActual).Status)
		})
	}
}

func TestReconciliationActualMetricsSummaryAdapterPreservesNullableContract(t *testing.T) {
	metrics := Metrics{}
	availability := DataAvailability{
		ActualPlatformRevenue:    "UNAVAILABLE_UNTIL_LIVE",
		PaymentProcessingExpense: "UNAVAILABLE_UNTIL_GATEWAY",
		OwnerPayable:             "UNAVAILABLE_UNTIL_PLATFORM_COLLECTED",
	}
	adapted := NewReconciliationActualMetricsFromSummary(metrics, availability)
	require.True(t, adapted.AllUnavailable())

	actualContribution := "2"
	metrics.TransactionContribution = &actualContribution
	adapted = NewReconciliationActualMetricsFromSummary(metrics, availability)
	require.False(t, adapted.AllUnavailable())
}

func TestProductionReconciliationActualMetricsProviderUsesSharedSummaryContract(t *testing.T) {
	provider := NewProductionReconciliationActualMetricsProvider()
	metrics, err := provider.LoadActualMetrics(context.Background(), time.Time{}, time.Time{})
	require.NoError(t, err)
	expectedMetrics, expectedAvailability := currentPlatformActualMetricsContract()
	require.Equal(t, NewReconciliationActualMetricsFromSummary(expectedMetrics, expectedAvailability), metrics)
	require.True(t, metrics.AllUnavailable())
}

func TestReconciliationRepositoryRequiresExplicitActualMetricsProvider(t *testing.T) {
	repo := NewReconciliationRepository(nil, nil)
	_, err := repo.LoadReconciliationSnapshot(context.Background(), time.Time{}, time.Time{})
	require.ErrorIs(t, err, ErrReconciliationActualMetricsProviderRequired)
}

func TestReconciliationRepositoryRejectsMissingDatabase(t *testing.T) {
	repo := NewReconciliationRepository(nil, NewProductionReconciliationActualMetricsProvider())
	_, err := repo.LoadReconciliationSnapshot(context.Background(), time.Time{}, time.Time{})
	require.ErrorIs(t, err, ErrReconciliationNoData)
}

func TestReconciliationBlocksProjectionRollupsWhenFactsAreIncomplete(t *testing.T) {
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{
		DataIssues:                []ReconciliationDataIssue{{Code: "OFFLINE_COMMISSION", BucketDate: "2026-07-01", DifferenceCount: 1}},
		ProjectionFactsIncomplete: true,
		ActualMetrics:             unavailableMetrics(),
	}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.NoError(t, err)
	require.Equal(t, ReconciliationBlocked, checkByCode(report, ReconciliationCheckRollup).Status)
	require.Equal(t, ReconciliationPass, checkByCode(report, ReconciliationCheckOPEX).Status)
}

func TestReconciliationRefundExactReversalPassesOnlyWhenBothAmountsAndCommissionMatch(t *testing.T) {
	when := time.Date(2026, 7, 1, 17, 30, 0, 0, GetJakartaLocation())
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{
		RefundEvents:  []ReconciliationEvent{{EventAt: when, Amount: 100_000, OriginalAmount: 100_000, Commission: 7_000, ExpectedCommission: 7_000, Exact: true}},
		ActualMetrics: unavailableMetrics(),
	}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.NoError(t, err)
	require.Equal(t, ReconciliationPass, checkByCode(report, ReconciliationCheckRefund).Status)

	repo.snapshot.RefundEvents[0].Commission = 6_999
	report, err = NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.NoError(t, err)
	check := checkByCode(report, ReconciliationCheckRefund)
	require.Equal(t, ReconciliationFail, check.Status)
	var commissionException *ReconciliationException
	for i := range check.Exceptions {
		if check.Exceptions[i].Metric == "refund_commission_reversal" {
			commissionException = &check.Exceptions[i]
			break
		}
	}
	require.NotNil(t, commissionException)
	require.Equal(t, int64(7_000), commissionException.ExpectedRupiah)
	require.Equal(t, int64(6_999), commissionException.ActualRupiah)
	require.Equal(t, int64(1), commissionException.DifferenceRupiah)
}

func TestReconciliationPaidSnapshotLedgerCountDeltaUsesExpectedMinusActual(t *testing.T) {
	when := time.Date(2026, 7, 1, 17, 30, 0, 0, GetJakartaLocation())
	for _, tc := range []struct {
		name      string
		ledger    int64
		wantDelta int64
	}{
		{name: "missing ledger", ledger: 0, wantDelta: 1},
		{name: "duplicate ledger", ledger: 2, wantDelta: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{
				CutoverAt:     when.Add(-time.Hour),
				PaidBookings:  []ReconciliationPaidBooking{{CreatedAt: when, Snapshot: true, SnapshotFinalPrice: 100_000, LedgerCount: tc.ledger, LedgerAmount: 100_000}},
				ActualMetrics: unavailableMetrics(),
			}}
			report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
			require.NoError(t, err)
			check := checkByCode(report, ReconciliationCheckSnapshot)
			require.Equal(t, ReconciliationFail, check.Status)
			require.Len(t, check.Exceptions, 1)
			require.Equal(t, tc.wantDelta, check.Exceptions[0].DifferenceCount)
		})
	}
}

func TestReconciliationProjectionErrorPreservesJakartaEventBucket(t *testing.T) {
	eventAt := time.Date(2026, 7, 1, 17, 0, 0, 0, time.UTC)
	issue, ok := reconciliationIssueForProjectionError(&reconciliationProjectionRowError{At: eventAt, Err: ErrInvalidProjectionSource})
	require.True(t, ok)
	require.Equal(t, "2026-07-02", issue.BucketDate)

	_, ok = reconciliationIssueForProjectionError(ErrInvalidProjectionSource)
	require.False(t, ok, "an undated projection error must not be emitted as a reconciliation exception")
}

func TestReconciliationExceptionBucketsUseJakartaHalfOpenDayBoundary(t *testing.T) {
	repo := &reconciliationFixtureRepository{snapshot: &ReconciliationSnapshot{
		RefundEvents: []ReconciliationEvent{
			{EventAt: time.Date(2026, 7, 1, 16, 59, 59, 0, time.UTC), Amount: 1, OriginalAmount: 2},
			{EventAt: time.Date(2026, 7, 1, 17, 0, 0, 0, time.UTC), Amount: 1, OriginalAmount: 2},
		},
		ActualMetrics: unavailableMetrics(),
	}}
	report, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-02"})
	require.NoError(t, err)
	exceptions := checkByCode(report, ReconciliationCheckRefund).Exceptions
	require.Len(t, exceptions, 2)
	require.Equal(t, "2026-07-01", exceptions[0].BucketDate)
	require.Equal(t, "2026-07-02", exceptions[1].BucketDate)
}

func TestReconciliationRepositoryErrorsAreNotPresentedAsClean(t *testing.T) {
	repo := &reconciliationFixtureRepository{err: errors.New("database unavailable")}
	_, err := NewReconciliationService(repo).Reconcile(context.Background(), ReconciliationQuery{StartDate: "2026-07-01", EndDate: "2026-07-01"})
	require.EqualError(t, err, "database unavailable")
}

func checkCodes(report *ReconciliationReport) []string {
	codes := make([]string, 0, len(report.Checks))
	for _, check := range report.Checks {
		codes = append(codes, check.Code)
	}
	return codes
}

func unavailableMetrics() ReconciliationActualMetrics {
	return ReconciliationActualMetrics{
		GatewayCapturedGMV:       "UNAVAILABLE_UNTIL_GATEWAY",
		ActualCommissionRevenue:  "UNAVAILABLE_UNTIL_LIVE",
		PaymentProcessingExpense: "UNAVAILABLE_UNTIL_GATEWAY",
		OwnerPayable:             "UNAVAILABLE_UNTIL_PLATFORM_COLLECTED",
		PlatformRevenue:          "UNAVAILABLE_UNTIL_LIVE",
		TransactionContribution:  "UNAVAILABLE_UNTIL_LIVE",
		OperatingResult:          "UNAVAILABLE_UNTIL_LIVE",
		GrossTakeRateBPS:         "UNAVAILABLE_UNTIL_LIVE",
		NetTakeRateBPS:           "UNAVAILABLE_UNTIL_LIVE",
	}
}

func checkByCode(report *ReconciliationReport, code string) ReconciliationCheckResult {
	for _, check := range report.Checks {
		if check.Code == code {
			return check
		}
	}
	panic("missing reconciliation check: " + code)
}
