package platformfinance

import (
	"context"
	"fmt"
	"sort"
	"time"
)

const reconciliationTimezone = "Asia/Jakarta"

func (s *reconciliationService) Reconcile(ctx context.Context, query ReconciliationQuery) (*ReconciliationReport, error) {
	// Unlike the public summary endpoint, diagnostics never use an implicit
	// month-to-date default. An explicit range makes two runs comparable.
	if query.StartDate == "" || query.EndDate == "" {
		return nil, ErrReconciliationInvalidRange
	}
	utcStart, utcEnd, err := ParseAndValidateDates(query.StartDate, query.EndDate)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReconciliationInvalidRange, err)
	}
	if s == nil || s.repo == nil {
		return nil, ErrReconciliationNoData
	}
	snapshot, err := s.repo.LoadReconciliationSnapshot(ctx, utcStart, utcEnd)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, ErrReconciliationNoData
	}

	report := &ReconciliationReport{
		Period:   Period{StartDate: query.StartDate, EndDate: query.EndDate},
		Timezone: reconciliationTimezone,
		AsOf:     snapshot.AsOf,
		Status:   ReconciliationClean,
	}
	checks := []ReconciliationCheckResult{
		evaluateOnline(snapshot),
		evaluateSnapshot(snapshot),
		evaluateOffline(snapshot),
		evaluateRefund(snapshot),
		evaluateDuplicates(snapshot),
		evaluateRollups(snapshot),
		evaluateOPEX(snapshot),
		evaluateActualMetrics(snapshot, query.StartDate),
	}
	report.Checks = checks
	report.Clean = true
	for _, check := range checks {
		if check.Status != ReconciliationPass {
			report.Clean = false
			break
		}
	}
	if !report.Clean {
		report.Status = ReconciliationExceptions
	}
	return report, nil
}

func evaluateOnline(snapshot *ReconciliationSnapshot) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckOnline)
	if snapshot.ProjectionFactsIncomplete {
		result.Status = ReconciliationBlocked
		result.Reason = "projection facts were withheld because source-to-ledger comparison was unsafe"
	} else {
		compareReconciliationBucketSeries(&result, "online", snapshot.SourceBuckets, snapshot.LedgerBuckets, bucketCompareOnline)
	}
	return applyDataIssues(result, snapshot.DataIssues, "ONLINE_LEDGER_GMV_MATCH", "DUPLICATE_INCOME", "FRACTIONAL_LEDGER", "FRACTIONAL_SOURCE", "SOURCE_LEDGER_MISMATCH", "LEDGER_WITHOUT_BOOKING", "PAID_WITHOUT_LEDGER", "SNAPSHOT_MISMATCH")
}

func evaluateSnapshot(snapshot *ReconciliationSnapshot) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckSnapshot)
	for _, booking := range snapshot.PaidBookings {
		if booking.Offline {
			continue
		}
		// A post-cutover online booking must be backed by the immutable fee
		// snapshot and exactly one ledger income for the source final price.
		if !snapshot.CutoverAt.IsZero() && !booking.CreatedAt.Before(snapshot.CutoverAt) {
			if !booking.Snapshot {
				result.addException(ReconciliationException{Metric: "paid_snapshot", BucketDate: localBucket(booking.CreatedAt), ExpectedCount: 1, ActualCount: 0, DifferenceCount: 1})
				continue
			}
		}
		if booking.Snapshot && (booking.LedgerCount != 1 || booking.LedgerAmount != booking.SnapshotFinalPrice) {
			differenceCount, err := checkedSubInt64(1, booking.LedgerCount)
			if err != nil {
				result.Status = ReconciliationBlocked
				result.Reason = "int64 overflow while calculating paid snapshot ledger count difference"
				continue
			}
			result.addException(ReconciliationException{Metric: "paid_snapshot_ledger", BucketDate: localBucket(booking.CreatedAt), ExpectedCount: 1, ActualCount: booking.LedgerCount, DifferenceCount: differenceCount, ExpectedRupiah: booking.SnapshotFinalPrice, ActualRupiah: booking.LedgerAmount, DifferenceRupiah: booking.SnapshotFinalPrice - booking.LedgerAmount})
		}
	}
	return applyDataIssues(result, snapshot.DataIssues, "PAID_SNAPSHOT_SOURCE_MATCH", "MISSING_SNAPSHOT", "SNAPSHOT_MISMATCH", "PAID_WITHOUT_LEDGER")
}

func evaluateOffline(snapshot *ReconciliationSnapshot) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckOffline)
	for _, booking := range snapshot.PaidBookings {
		if !booking.Offline {
			continue
		}
		if booking.CommissionBPS != 0 || booking.Commission != 0 {
			result.addException(ReconciliationException{Metric: "offline_commission", BucketDate: localBucket(booking.CreatedAt), ExpectedRupiah: 0, ActualRupiah: booking.Commission, DifferenceRupiah: booking.Commission})
		}
	}
	return applyDataIssues(result, snapshot.DataIssues, "OFFLINE_ZERO_COMMISSION", "OFFLINE_COMMISSION")
}

func evaluateRefund(snapshot *ReconciliationSnapshot) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckRefund)
	if snapshot.ProjectionFactsIncomplete {
		result.Status = ReconciliationBlocked
		result.Reason = "projection facts were withheld because an integrity issue made exact refund comparison unsafe"
		return applyDataIssues(result, snapshot.DataIssues, "REFUND_EXACT_REVERSAL", "REFUND_MISMATCH", "ORPHAN_REFUND")
	}
	for _, refund := range snapshot.RefundEvents {
		if !refund.Exact || refund.Amount != refund.OriginalAmount {
			result.addException(ReconciliationException{Metric: "refund_exact_reversal", BucketDate: localBucket(refund.EventAt), ExpectedRupiah: refund.OriginalAmount, ActualRupiah: refund.Amount, DifferenceRupiah: refund.OriginalAmount - refund.Amount})
		}
		if refund.Commission != refund.ExpectedCommission {
			result.addException(ReconciliationException{Metric: "refund_commission_reversal", BucketDate: localBucket(refund.EventAt), ExpectedRupiah: refund.ExpectedCommission, ActualRupiah: refund.Commission, DifferenceRupiah: refund.ExpectedCommission - refund.Commission})
		}
	}
	return applyDataIssues(result, snapshot.DataIssues, "REFUND_EXACT_REVERSAL", "REFUND_MISMATCH", "ORPHAN_REFUND")
}

func evaluateDuplicates(snapshot *ReconciliationSnapshot) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckDuplicate)
	return applyDataIssues(result, snapshot.DataIssues, "NO_DUPLICATE_EVENTS", "DUPLICATE_EVENT", "DUPLICATE_INCOME", "FRACTIONAL_LEDGER")
}

func evaluateRollups(snapshot *ReconciliationSnapshot) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckRollup)
	if snapshot.ProjectionFactsIncomplete {
		result.Status = ReconciliationBlocked
		result.Reason = "projection facts were withheld because an integrity issue made rollup comparison unsafe"
		return applyDataIssues(result, snapshot.DataIssues, "SUMMARY_BREAKDOWN_TREND_MATCH", "ROLLUP_MISMATCH", "MISSING_SNAPSHOT", "SNAPSHOT_MISMATCH", "POST_CUTOVER_LEGACY", "DUPLICATE_INCOME", "FRACTIONAL_SOURCE", "SOURCE_LEDGER_MISMATCH", "REFUND_MISMATCH", "OPEX_MISMATCH", "PAID_WITHOUT_LEDGER", "LEDGER_WITHOUT_BOOKING")
	}
	compareReconciliationBucketSeries(&result, "summary_owner", snapshot.SummaryBuckets, snapshot.OwnerBuckets, bucketCompareProjection)
	compareReconciliationBucketSeries(&result, "summary_venue", snapshot.SummaryBuckets, snapshot.VenueBuckets, bucketCompareProjection)
	compareReconciliationBucketSeries(&result, "summary_trend", snapshot.SummaryBuckets, snapshot.TrendTotals, bucketCompareProjectionAndOPEX)
	return applyDataIssues(result, snapshot.DataIssues, "SUMMARY_BREAKDOWN_TREND_MATCH", "ROLLUP_MISMATCH", "MISSING_SNAPSHOT", "SNAPSHOT_MISMATCH", "POST_CUTOVER_LEGACY", "DUPLICATE_INCOME", "FRACTIONAL_SOURCE", "SOURCE_LEDGER_MISMATCH", "REFUND_MISMATCH", "OPEX_MISMATCH", "PAID_WITHOUT_LEDGER", "LEDGER_WITHOUT_BOOKING")
}

func evaluateOPEX(snapshot *ReconciliationSnapshot) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckOPEX)
	actualByDate := make(map[string]*ReconciliationTotals)
	mismatchedDates := make(map[string]struct{})
	for _, event := range snapshot.OPEXEvents {
		date := localBucket(event.EffectiveAt)
		if event.ExpectedAmount != event.Amount {
			result.addException(ReconciliationException{
				Metric: "opex_expense_journal", BucketDate: date,
				ExpectedRupiah: event.ExpectedAmount, ActualRupiah: event.Amount,
				DifferenceRupiah: event.ExpectedAmount - event.Amount,
				Reason:           "expense amount differs from linked journal amount",
			})
			mismatchedDates[date] = struct{}{}
		}
		if !event.ExactLink {
			result.addException(ReconciliationException{
				Metric: "opex_exact_reversal", BucketDate: date,
				ExpectedCount: 1, ActualCount: 0, DifferenceCount: 1,
				Reason: "void journal does not reverse the expense posted journal",
			})
			mismatchedDates[date] = struct{}{}
		}
		if actualByDate[date] == nil {
			actualByDate[date] = &ReconciliationTotals{}
		}
		actual, err := checkedAddInt64(actualByDate[date].OperatingExpense, event.Amount)
		if err != nil {
			result.Status = ReconciliationBlocked
			result.Reason = "OPEX amount overflowed int64 during reconciliation"
			return result
		}
		actualByDate[date].OperatingExpense = actual
	}
	actualBuckets := bucketMapToSortedSlice(actualByDate)
	// Preserve the aggregate comparison for missing/extra event detection, but
	// do not duplicate exceptions for dates already diagnosed per expense.
	compareReconciliationBucketSeries(
		&result,
		"opex_posted_minus_reversal",
		filterReconciliationBuckets(snapshot.OPEXSourceBuckets, mismatchedDates),
		filterReconciliationBuckets(actualBuckets, mismatchedDates),
		bucketCompareOPEX,
	)
	return applyDataIssues(result, snapshot.DataIssues, "OPEX_POSTED_REVERSAL_MATCH", "OPEX_MISMATCH")
}

func filterReconciliationBuckets(buckets []ReconciliationBucketTotals, excluded map[string]struct{}) []ReconciliationBucketTotals {
	if len(excluded) == 0 {
		return buckets
	}
	filtered := make([]ReconciliationBucketTotals, 0, len(buckets))
	for _, bucket := range buckets {
		if _, skip := excluded[bucket.BucketDate]; !skip {
			filtered = append(filtered, bucket)
		}
	}
	return filtered
}

func evaluateActualMetrics(snapshot *ReconciliationSnapshot, fallbackDate string) ReconciliationCheckResult {
	result := newReconciliationCheck(ReconciliationCheckActual)
	if !snapshot.ActualMetrics.AllUnavailable() {
		result.Status = ReconciliationFail
		result.Reason = "actual gateway/platform metrics are available before the live reconciliation gate"
		bucket := fallbackDate
		if !snapshot.AsOf.IsZero() {
			bucket = localBucket(snapshot.AsOf)
		}
		result.addException(ReconciliationException{Metric: "actual_metrics", BucketDate: bucket, ExpectedCount: 0, ActualCount: 1, DifferenceCount: 1})
	}
	return result
}

func newReconciliationCheck(code string) ReconciliationCheckResult {
	return ReconciliationCheckResult{Code: code, Status: ReconciliationPass}
}

func (r *ReconciliationCheckResult) addException(exception ReconciliationException) {
	r.Exceptions = append(r.Exceptions, exception)
	r.Status = ReconciliationFail
	r.DifferenceCount += exception.DifferenceCount
	r.DifferenceRupiah += exception.DifferenceRupiah
}

func compareReconciliationTotals(result *ReconciliationCheckResult, metric, bucket string, expected, actual int64) {
	if expected == actual {
		return
	}
	difference, err := checkedSubInt64(expected, actual)
	if err != nil {
		result.Status = ReconciliationBlocked
		result.Reason = "int64 overflow while calculating reconciliation difference"
		return
	}
	result.addException(ReconciliationException{Metric: metric, BucketDate: bucket, ExpectedRupiah: expected, ActualRupiah: actual, DifferenceRupiah: difference})
}

func compareReconciliationCounts(result *ReconciliationCheckResult, metric, bucket string, expected, actual int64) {
	if expected == actual {
		return
	}
	difference, err := checkedSubInt64(expected, actual)
	if err != nil {
		result.Status = ReconciliationBlocked
		result.Reason = "int64 overflow while calculating reconciliation count difference"
		return
	}
	result.addException(ReconciliationException{Metric: metric, BucketDate: bucket, ExpectedCount: expected, ActualCount: actual, DifferenceCount: difference})
}

type reconciliationBucketComparison uint8

const (
	bucketCompareOnline reconciliationBucketComparison = iota
	bucketCompareProjection
	bucketCompareProjectionAndOPEX
	bucketCompareOPEX
)

func compareReconciliationBucketSeries(result *ReconciliationCheckResult, prefix string, expected, actual []ReconciliationBucketTotals, comparison reconciliationBucketComparison) {
	expectedByDate := reconciliationBucketMap(expected)
	actualByDate := reconciliationBucketMap(actual)
	dates := make([]string, 0, len(expectedByDate)+len(actualByDate))
	seen := make(map[string]bool, len(expectedByDate)+len(actualByDate))
	for date := range expectedByDate {
		seen[date] = true
		dates = append(dates, date)
	}
	for date := range actualByDate {
		if !seen[date] {
			dates = append(dates, date)
		}
	}
	sort.Strings(dates)
	for _, date := range dates {
		expectedTotals := expectedByDate[date]
		actualTotals := actualByDate[date]
		switch comparison {
		case bucketCompareOnline:
			compareReconciliationTotals(result, prefix+"_gmv_gross", date, expectedTotals.Gross, actualTotals.Gross)
			compareReconciliationCounts(result, prefix+"_booking_count", date, expectedTotals.BookingCount, actualTotals.BookingCount)
		case bucketCompareProjection, bucketCompareProjectionAndOPEX:
			compareReconciliationTotals(result, prefix+"_gross", date, expectedTotals.Gross, actualTotals.Gross)
			compareReconciliationTotals(result, prefix+"_refund", date, expectedTotals.Refund, actualTotals.Refund)
			compareReconciliationTotals(result, prefix+"_net", date, expectedTotals.Net, actualTotals.Net)
			compareReconciliationTotals(result, prefix+"_commission", date, expectedTotals.Commission, actualTotals.Commission)
			compareReconciliationCounts(result, prefix+"_booking_count", date, expectedTotals.BookingCount, actualTotals.BookingCount)
			compareReconciliationCounts(result, prefix+"_refund_count", date, expectedTotals.RefundCount, actualTotals.RefundCount)
			if comparison == bucketCompareProjectionAndOPEX {
				compareReconciliationTotals(result, prefix+"_opex", date, expectedTotals.OperatingExpense, actualTotals.OperatingExpense)
			}
		case bucketCompareOPEX:
			compareReconciliationTotals(result, prefix, date, expectedTotals.OperatingExpense, actualTotals.OperatingExpense)
		}
	}
}

func reconciliationBucketMap(values []ReconciliationBucketTotals) map[string]ReconciliationTotals {
	result := make(map[string]ReconciliationTotals, len(values))
	for _, bucket := range values {
		total := result[bucket.BucketDate]
		total.Gross += bucket.Totals.Gross
		total.Refund += bucket.Totals.Refund
		total.Net += bucket.Totals.Net
		total.Commission += bucket.Totals.Commission
		total.BookingCount += bucket.Totals.BookingCount
		total.RefundCount += bucket.Totals.RefundCount
		total.OperatingExpense += bucket.Totals.OperatingExpense
		result[bucket.BucketDate] = total
	}
	return result
}

func bucketMapToSortedSlice(values map[string]*ReconciliationTotals) []ReconciliationBucketTotals {
	result := make([]ReconciliationBucketTotals, 0, len(values))
	for date, totals := range values {
		if totals != nil {
			result = append(result, ReconciliationBucketTotals{BucketDate: date, Totals: *totals})
		}
	}
	sortReconciliationBuckets(result)
	return result
}

func applyDataIssues(result ReconciliationCheckResult, issues []ReconciliationDataIssue, code string, aliases ...string) ReconciliationCheckResult {
	for _, issue := range issues {
		if issue.Code != code && !containsString(aliases, issue.Code) {
			continue
		}
		result.addException(ReconciliationException{Metric: issue.Code, BucketDate: issue.BucketDate, DifferenceCount: issue.DifferenceCount, DifferenceRupiah: issue.DifferenceRupiah, Reason: issue.Reason})
	}
	return result
}

func sumBuckets(values []ReconciliationBucketTotals) ReconciliationTotals {
	total := make([]ReconciliationBucketTotals, len(values))
	copy(total, values)
	sort.SliceStable(total, func(i, j int) bool { return total[i].BucketDate < total[j].BucketDate })
	return sumTotalsBuckets(total)
}

func sumTotalsBuckets(values []ReconciliationBucketTotals) ReconciliationTotals {
	var total ReconciliationTotals
	for _, value := range values {
		total.Gross += value.Totals.Gross
		total.Refund += value.Totals.Refund
		total.Net += value.Totals.Net
		total.Commission += value.Totals.Commission
		total.BookingCount += value.Totals.BookingCount
		total.RefundCount += value.Totals.RefundCount
		total.OperatingExpense += value.Totals.OperatingExpense
	}
	return total
}

func localBucket(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.In(GetJakartaLocation()).Format("2006-01-02")
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
