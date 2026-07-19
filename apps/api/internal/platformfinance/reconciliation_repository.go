package platformfinance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// reconciliationRepository deliberately has its own read model instead of
// calling the HTTP summary repository. This keeps diagnostics independent of
// pagination and makes the single repeatable-read transaction explicit.
type reconciliationRepository struct {
	db                    *pgxpool.Pool
	transactionStarter    reconciliationTransactionStarter
	actualMetricsProvider ReconciliationActualMetricsProvider
}

type reconciliationTransactionStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

func NewReconciliationRepository(db *pgxpool.Pool, provider ReconciliationActualMetricsProvider) ReconciliationRepository {
	repo := &reconciliationRepository{db: db, actualMetricsProvider: provider}
	if db != nil {
		repo.transactionStarter = db
	}
	return repo
}

func (r *reconciliationRepository) LoadReconciliationSnapshot(ctx context.Context, utcStart, utcEndExclusive time.Time) (*ReconciliationSnapshot, error) {
	if r == nil || r.actualMetricsProvider == nil {
		return nil, ErrReconciliationActualMetricsProviderRequired
	}
	if r == nil || r.transactionStarter == nil {
		return nil, ErrReconciliationNoData
	}
	tx, err := r.transactionStarter.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var asOf time.Time
	if err := tx.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&asOf); err != nil {
		return nil, mapRepositoryError(err)
	}
	cutover, err := loadProjectionCutover(ctx, tx)
	if err != nil {
		return nil, err
	}
	actualMetrics, err := r.actualMetricsProvider.LoadActualMetrics(ctx, utcStart, utcEndExclusive)
	if err != nil {
		return nil, err
	}
	snapshot := &ReconciliationSnapshot{AsOf: asOf.UTC(), CutoverAt: cutover.UTC(), ActualMetrics: actualMetrics}
	snapshot.DataIssues, err = loadReconciliationDataIssues(ctx, tx, utcStart, utcEndExclusive, cutover)
	if err != nil {
		return nil, err
	}
	var incomes, refunds []projectionRow
	// Existing projection readers intentionally fail closed on corrupt rows.
	// The reconciliation report keeps those exceptions visible while still
	// collecting independent OPEX/source facts in the same snapshot.
	if len(snapshot.DataIssues) == 0 {
		incomes, refunds, _, _, err = (&repository{db: r.db}).loadProjectionEvents(ctx, tx, utcStart, utcEndExclusive, "", "")
		if err != nil {
			if issue, ok := reconciliationIssueForProjectionError(err); ok {
				snapshot.DataIssues = append(snapshot.DataIssues, issue)
				snapshot.ProjectionFactsIncomplete = true
			} else {
				return nil, err
			}
		}
	} else {
		snapshot.ProjectionFactsIncomplete = true
	}
	if snapshot.ProjectionFactsIncomplete {
		// Do not manufacture zero-valued projection facts from an unsafe stream.
		// The evaluator will mark projection-dependent checks BLOCKED.
		incomes, refunds = nil, nil
	}
	total, _, _ := aggregateProjection(incomes, refunds)
	snapshot.SummaryTotals = ReconciliationTotals{
		Gross:        total.Gross,
		Refund:       total.Refund,
		Net:          total.Gross - total.Refund,
		Commission:   total.CommGross - total.CommRefund,
		BookingCount: int64(total.BookingCount),
		RefundCount:  int64(total.RefundedCount),
	}
	for _, row := range incomes {
		snapshot.IncomeEvents = append(snapshot.IncomeEvents, ReconciliationEvent{
			BookingID: row.Event.BookingID, OwnerProfileID: row.Event.OwnerProfileID, VenueID: row.Event.VenueID,
			EventAt: row.Event.At, Amount: row.Event.Amount, Commission: row.Event.Commission, Source: row.Event.Source,
		})
	}
	for _, row := range refunds {
		// loadProjectionEvents only admits exact reversals. OriginalAmount and
		// expected commission therefore equal the canonical income values here.
		snapshot.RefundEvents = append(snapshot.RefundEvents, ReconciliationEvent{
			BookingID: row.Event.BookingID, OwnerProfileID: row.Event.OwnerProfileID, VenueID: row.Event.VenueID,
			EventAt: row.Event.At, Amount: row.Event.Amount, OriginalAmount: row.Event.Amount,
			Commission: row.Event.Commission, ExpectedCommission: row.Event.Commission, Exact: true, Source: row.Event.Source,
		})
	}
	if !snapshot.ProjectionFactsIncomplete {
		snapshot.LedgerBuckets, snapshot.SourceBuckets, err = loadReconciliationOnlineBuckets(ctx, tx, utcStart, utcEndExclusive)
		if err != nil {
			return nil, err
		}
		snapshot.LedgerTotals = sumBuckets(snapshot.LedgerBuckets)
		snapshot.SourceTotals = sumBuckets(snapshot.SourceBuckets)
		snapshot.OwnerBuckets, snapshot.VenueBuckets, err = loadProductionReconciliationBreakdownBuckets(ctx, tx, utcStart, utcEndExclusive)
		if err != nil {
			return nil, err
		}
		snapshot.OwnerTotals = []ReconciliationTotals{sumBuckets(snapshot.OwnerBuckets)}
		snapshot.VenueTotals = []ReconciliationTotals{sumBuckets(snapshot.VenueBuckets)}
	}

	opexEvents, _, err := loadReconciliationOPEXEvents(ctx, tx, utcStart, utcEndExclusive)
	if err != nil {
		return nil, err
	}
	snapshot.OPEXEvents = opexEvents
	// The reported summary is sourced from the expense facts and effective
	// journal links; OPEXEvents independently reads the immutable ledger entry
	// rows. Comparing these two paths is what makes a damaged journal visible.
	snapshot.OPEXSourceBuckets, err = loadReconciliationOPEXSourceBuckets(ctx, tx, utcStart, utcEndExclusive)
	if err != nil {
		return nil, err
	}
	snapshot.SummaryTotals.OperatingExpense = sumBuckets(snapshot.OPEXSourceBuckets).OperatingExpense
	snapshot.SummaryBuckets = aggregateReconciliationSummaryBuckets(snapshot.IncomeEvents, snapshot.RefundEvents, snapshot.OPEXSourceBuckets)

	trendTotals, err := loadReconciliationTrendTotals(ctx, tx, utcStart, utcEndExclusive)
	if err != nil {
		return nil, err
	}
	buckets := make(map[string]*ReconciliationTotals, len(trendTotals))
	for _, bucket := range trendTotals {
		copyTotals := bucket.Totals
		buckets[bucket.BucketDate] = &copyTotals
	}
	for _, event := range opexEvents {
		bucket := localBucket(event.EffectiveAt)
		if buckets[bucket] == nil {
			buckets[bucket] = &ReconciliationTotals{}
		}
		buckets[bucket].OperatingExpense += event.Amount
	}
	for bucket, totals := range buckets {
		totals.Net = totals.Gross - totals.Refund
		snapshot.TrendTotals = append(snapshot.TrendTotals, ReconciliationBucketTotals{BucketDate: bucket, Totals: *totals})
	}
	// Ordering is part of the evidence contract, even though the evaluator
	// sums buckets. It makes serialized reports stable for CI artifacts.
	sortReconciliationBuckets(snapshot.TrendTotals)

	snapshot.PaidBookings, err = loadReconciliationPaidBookings(ctx, tx, utcStart, utcEndExclusive)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func aggregateReconciliationSummaryBuckets(incomes, refunds []ReconciliationEvent, opex []ReconciliationBucketTotals) []ReconciliationBucketTotals {
	buckets := make(map[string]*ReconciliationTotals)
	for _, event := range incomes {
		date := localBucket(event.EventAt)
		if buckets[date] == nil {
			buckets[date] = &ReconciliationTotals{}
		}
		buckets[date].Gross += event.Amount
		buckets[date].Commission += event.Commission
		buckets[date].BookingCount++
	}
	for _, event := range refunds {
		date := localBucket(event.EventAt)
		if buckets[date] == nil {
			buckets[date] = &ReconciliationTotals{}
		}
		buckets[date].Refund += event.Amount
		buckets[date].Commission -= event.Commission
		buckets[date].RefundCount++
	}
	for _, bucket := range opex {
		if buckets[bucket.BucketDate] == nil {
			buckets[bucket.BucketDate] = &ReconciliationTotals{}
		}
		buckets[bucket.BucketDate].OperatingExpense += bucket.Totals.OperatingExpense
	}
	result := make([]ReconciliationBucketTotals, 0, len(buckets))
	for date, totals := range buckets {
		totals.Net = totals.Gross - totals.Refund
		result = append(result, ReconciliationBucketTotals{BucketDate: date, Totals: *totals})
	}
	sortReconciliationBuckets(result)
	return result
}

func aggregateReconciliationBreakdown(cells []projectionBreakdownCell) ([]ReconciliationTotals, []ReconciliationTotals) {
	owners := make(map[string]*ReconciliationTotals)
	venues := make(map[string]*ReconciliationTotals)
	for _, cell := range cells {
		owner := owners[cell.OwnerProfileID]
		if owner == nil {
			owner = &ReconciliationTotals{}
			owners[cell.OwnerProfileID] = owner
		}
		venue := venues[cell.VenueID]
		if venue == nil {
			venue = &ReconciliationTotals{}
			venues[cell.VenueID] = venue
		}
		for _, total := range []*ReconciliationTotals{owner, venue} {
			total.Gross += cell.Gross
			total.Refund += cell.Refund
			total.Net = total.Gross - total.Refund
			total.Commission += cell.NetCommission
			total.BookingCount += int64(cell.BookingCount)
			total.RefundCount += int64(cell.RefundCount)
		}
	}
	ownerTotals := make([]ReconciliationTotals, 0, len(owners))
	for _, total := range owners {
		ownerTotals = append(ownerTotals, *total)
	}
	venueTotals := make([]ReconciliationTotals, 0, len(venues))
	for _, total := range venues {
		venueTotals = append(venueTotals, *total)
	}
	return ownerTotals, venueTotals
}

func loadReconciliationDataIssues(ctx context.Context, tx pgx.Tx, start, end, cutover time.Time) ([]ReconciliationDataIssue, error) {
	issues := make([]ReconciliationDataIssue, 0)
	addCountIssue := func(code, query string, args ...any) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return mapRepositoryError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var bucket time.Time
			var count int64
			if err := rows.Scan(&bucket, &count); err != nil {
				return mapRepositoryError(err)
			}
			if count > 0 {
				issues = append(issues, ReconciliationDataIssue{Code: code, BucketDate: bucket.In(GetJakartaLocation()).Format("2006-01-02"), DifferenceCount: count})
			}
		}
		return mapRepositoryError(rows.Err())
	}
	addMoneyIssue := func(code, reason, query string, args ...any) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return mapRepositoryError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var bucket time.Time
			var difference int64
			if err := rows.Scan(&bucket, &difference); err != nil {
				return mapRepositoryError(err)
			}
			issues = append(issues, ReconciliationDataIssue{
				Code: code, BucketDate: bucket.In(GetJakartaLocation()).Format("2006-01-02"),
				DifferenceCount: 1, DifferenceRupiah: difference, Reason: reason,
			})
		}
		return mapRepositoryError(rows.Err())
	}
	if err := addCountIssue("DUPLICATE_INCOME", `
		SELECT date_trunc('day', MIN(created_at) AT TIME ZONE 'Asia/Jakarta'), COUNT(*) - 1
		FROM owner_finance_transactions
		WHERE type='INCOME' AND source='BOOKING' AND booking_id IS NOT NULL
		GROUP BY booking_id HAVING COUNT(*) > 1`); err != nil {
		return nil, err
	}
	if err := addCountIssue("DUPLICATE_EVENT", `
		SELECT date_trunc('day', MIN(created_at) AT TIME ZONE 'Asia/Jakarta'), COUNT(*) - 1
		FROM owner_finance_transactions
		WHERE type='EXPENSE' AND source='REFUND' AND booking_id IS NOT NULL
		GROUP BY booking_id HAVING COUNT(*) > 1`); err != nil {
		return nil, err
	}
	if err := addCountIssue("FRACTIONAL_LEDGER", `
		SELECT date_trunc('day', MIN(created_at) AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM owner_finance_transactions WHERE amount <> trunc(amount)
		GROUP BY date_trunc('day', created_at AT TIME ZONE 'Asia/Jakarta')`); err != nil {
		return nil, err
	}
	if !cutover.IsZero() {
		if err := addCountIssue("MISSING_SNAPSHOT", `
			SELECT date_trunc('day', b.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
			FROM bookings b LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
			WHERE b.created_at >= $1 AND b.created_at < $2
			  AND b.created_at >= $3 AND s.booking_id IS NULL
			GROUP BY 1`, start, end, cutover); err != nil {
			return nil, err
		}
		if err := addCountIssue("POST_CUTOVER_LEGACY", `
			SELECT date_trunc('day', b.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
			FROM bookings b
			JOIN booking_fee_snapshots s ON s.booking_id=b.id
			WHERE b.created_at >= $1 AND b.created_at < $2
			  AND b.created_at >= $3 AND s.terms_source='LEGACY_NO_COMMISSION'
			GROUP BY 1`, start, end, cutover); err != nil {
			return nil, err
		}
	}
	if err := addCountIssue("SNAPSHOT_MISMATCH", `
		SELECT date_trunc('day', t.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM bookings b
		JOIN courts c ON c.id=b.court_id
		JOIN venues v ON v.id=c.venue_id
		JOIN owner_profiles op ON op.id=v.owner_profile_id
		JOIN booking_fee_snapshots s ON s.booking_id=b.id
		JOIN owner_finance_transactions t ON t.booking_id=b.id
		  AND t.type='INCOME' AND t.source='BOOKING'
		  AND t.venue_id=v.id AND t.owner_id=op.user_id
		WHERE t.created_at >= $1 AND t.created_at < $2
		  AND t.amount <> s.final_booking_price_rupiah
		GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	// Source reconciliation is row-based before period aggregation. Otherwise
	// opposite booking differences (for example +Rp10 and -Rp10) can cancel and
	// incorrectly make ONLINE_LEDGER_GMV_MATCH pass. Historical rows use the
	// immutable booking total here; snapshot-backed rows are already covered by
	// the SNAPSHOT_MISMATCH check above.
	if err := addCountIssue("FRACTIONAL_SOURCE", `
		WITH canonical AS (
			SELECT t.created_at,
			       CASE WHEN s.booking_id IS NOT NULL
			            THEN s.final_booking_price_rupiah::numeric ELSE b.total_price END AS source_amount
			FROM owner_finance_transactions t
			JOIN bookings b ON b.id=t.booking_id
			JOIN courts c ON c.id=b.court_id
			JOIN venues v ON v.id=c.venue_id AND v.id=t.venue_id
			JOIN owner_profiles op ON op.id=v.owner_profile_id AND op.user_id=t.owner_id
			LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
			LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
			WHERE t.type='INCOME' AND t.source='BOOKING'
			  AND t.created_at >= $1 AND t.created_at < $2
			  AND obc.booking_id IS NULL
			  AND s.booking_id IS NULL
		)
		SELECT date_trunc('day', created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM canonical WHERE source_amount <> trunc(source_amount)
		GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	if err := addMoneyIssue("SOURCE_LEDGER_MISMATCH", "booking source amount differs from canonical ledger income", `
		WITH canonical AS (
			SELECT t.created_at, t.amount,
			       CASE WHEN s.booking_id IS NOT NULL
			            THEN s.final_booking_price_rupiah::numeric ELSE b.total_price END AS source_amount
			FROM owner_finance_transactions t
			JOIN bookings b ON b.id=t.booking_id
			JOIN courts c ON c.id=b.court_id
			JOIN venues v ON v.id=c.venue_id AND v.id=t.venue_id
			JOIN owner_profiles op ON op.id=v.owner_profile_id AND op.user_id=t.owner_id
			LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
			LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
			WHERE t.type='INCOME' AND t.source='BOOKING'
			  AND t.created_at >= $1 AND t.created_at < $2
			  AND obc.booking_id IS NULL
			  AND s.booking_id IS NULL
		)
		SELECT date_trunc('day', created_at AT TIME ZONE 'Asia/Jakarta'),
		       (source_amount - amount)::bigint
		FROM canonical
		WHERE source_amount = trunc(source_amount) AND amount = trunc(amount)
		  AND source_amount <> amount
		ORDER BY created_at`, start, end); err != nil {
		return nil, err
	}
	// classifyProjection rejects live-mode and owner-walk-in snapshots from
	// the online projection stream. Both values are legal schema states, so
	// detect them before row collection and retain the ledger event's day.
	if err := addCountIssue("SNAPSHOT_MISMATCH", `
		SELECT date_trunc('day', t.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM owner_finance_transactions t
		JOIN bookings b ON b.id=t.booking_id
		JOIN courts c ON c.id=b.court_id
		JOIN venues v ON v.id=c.venue_id AND v.id=t.venue_id
		JOIN owner_profiles op ON op.id=v.owner_profile_id AND op.user_id=t.owner_id
		LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
		JOIN booking_fee_snapshots s ON s.booking_id=b.id
		WHERE t.type='INCOME' AND t.source='BOOKING'
		  AND t.created_at >= $1 AND t.created_at < $2
		  AND obc.booking_id IS NULL
		  AND (s.finance_mode <> 'SIMULATION' OR s.booking_channel <> 'MARKETPLACE_ONLINE')
		GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	if err := addCountIssue("PAID_WITHOUT_LEDGER", `
		SELECT date_trunc('day', b.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM bookings b
		JOIN courts c ON c.id=b.court_id
		JOIN venues v ON v.id=c.venue_id
		LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
		WHERE b.status IN ('PAID','COMPLETED') AND obc.booking_id IS NULL
		  AND b.created_at >= $1 AND b.created_at < $2
		  AND NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions t
			WHERE t.booking_id=b.id AND t.type='INCOME' AND t.source='BOOKING'
			  AND t.venue_id=v.id AND t.owner_id=(SELECT user_id FROM owner_profiles WHERE id=v.owner_profile_id)
		  )
		GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	if err := addCountIssue("LEDGER_WITHOUT_BOOKING", `
		SELECT date_trunc('day', t.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM owner_finance_transactions t
		WHERE t.type='INCOME' AND t.source='BOOKING' AND t.created_at >= $1 AND t.created_at < $2
		  AND NOT (`+canonicalLedgerBookingPredicate+`)
		GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	if err := addCountIssue("ORPHAN_REFUND", `
		SELECT date_trunc('day', created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM owner_finance_transactions t
		WHERE t.type='EXPENSE' AND t.source='REFUND' AND t.created_at >= $1 AND t.created_at < $2
		  AND (t.booking_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions i WHERE i.booking_id=t.booking_id AND i.type='INCOME' AND i.source='BOOKING'))
		GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	if err := addCountIssue("REFUND_MISMATCH", `
		SELECT date_trunc('day', t.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM owner_finance_transactions t
		JOIN owner_finance_transactions i ON i.booking_id=t.booking_id AND i.type='INCOME' AND i.source='BOOKING'
		WHERE t.type='EXPENSE' AND t.source='REFUND' AND t.created_at >= $1 AND t.created_at < $2
		  AND t.amount <> i.amount GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	if err := addCountIssue("OFFLINE_COMMISSION", `
		SELECT date_trunc('day', b.created_at AT TIME ZONE 'Asia/Jakarta'), COUNT(*)
		FROM bookings b JOIN booking_fee_snapshots s ON s.booking_id=b.id
		JOIN offline_booking_customers obc ON obc.booking_id=b.id
		WHERE b.status IN ('PAID','COMPLETED') AND b.created_at >= $1 AND b.created_at < $2
		  AND (s.commission_bps <> 0 OR s.commission_amount_rupiah <> 0) GROUP BY 1`, start, end); err != nil {
		return nil, err
	}
	return issues, nil
}

func reconciliationIssueForProjectionError(err error) (ReconciliationDataIssue, bool) {
	var rowErr *reconciliationProjectionRowError
	if !errors.As(err, &rowErr) || rowErr == nil || rowErr.At.IsZero() {
		// All known preflight failures are expected to be dated by
		// loadReconciliationDataIssues. Refuse to manufacture an undated
		// exception if a new row-level failure bypasses that preflight.
		return ReconciliationDataIssue{}, false
	}
	issue := ReconciliationDataIssue{BucketDate: localBucket(rowErr.At), DifferenceCount: 1, Reason: err.Error()}
	switch {
	case errors.Is(err, ErrMissingProjectionSnapshot), errors.Is(err, ErrPostCutoverLegacyProjection), errors.Is(err, ErrInvalidProjectionSource), errors.Is(err, ErrProjectionSnapshotMismatch):
		issue.Code = "SNAPSHOT_MISMATCH"
	case errors.Is(err, ErrDuplicateLedgerDetected):
		issue.Code = "DUPLICATE_INCOME"
	case errors.Is(err, ErrFractionalLedgerDetected), errors.Is(err, ErrProjectionIntegrity):
		issue.Code = "FRACTIONAL_LEDGER"
	case errors.Is(err, ErrOrphanRefundDetected):
		issue.Code = "ORPHAN_REFUND"
	case errors.Is(err, ErrRefundAmountMismatch):
		issue.Code = "REFUND_MISMATCH"
	default:
		return ReconciliationDataIssue{}, false
	}
	return issue, true
}

func loadReconciliationOnlineBuckets(ctx context.Context, tx pgx.Tx, start, end time.Time) ([]ReconciliationBucketTotals, []ReconciliationBucketTotals, error) {
	rows, err := tx.Query(ctx, `
		SELECT date_trunc('day', t.created_at AT TIME ZONE 'Asia/Jakarta') AS bucket,
		       SUM(t.amount)::bigint AS ledger_gross,
		       SUM(CASE WHEN s.booking_id IS NOT NULL
		                THEN s.final_booking_price_rupiah::numeric ELSE b.total_price END)::bigint AS source_gross,
		       COUNT(*)::bigint
		FROM owner_finance_transactions t
		JOIN bookings b ON b.id=t.booking_id
		JOIN courts c ON c.id=b.court_id
		JOIN venues v ON v.id=c.venue_id AND v.id=t.venue_id
		JOIN owner_profiles op ON op.id=v.owner_profile_id AND op.user_id=t.owner_id
		LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
		LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
		WHERE t.type='INCOME' AND t.source='BOOKING'
		  AND t.created_at >= $1 AND t.created_at < $2
		  AND obc.booking_id IS NULL
		GROUP BY 1 ORDER BY 1`, start, end)
	if err != nil {
		return nil, nil, mapRepositoryError(err)
	}
	defer rows.Close()
	ledger := make([]ReconciliationBucketTotals, 0)
	source := make([]ReconciliationBucketTotals, 0)
	for rows.Next() {
		var bucket time.Time
		var ledgerGross, sourceGross, count int64
		if err := rows.Scan(&bucket, &ledgerGross, &sourceGross, &count); err != nil {
			return nil, nil, mapRepositoryError(err)
		}
		date := bucket.In(GetJakartaLocation()).Format("2006-01-02")
		ledger = append(ledger, ReconciliationBucketTotals{BucketDate: date, Totals: ReconciliationTotals{Gross: ledgerGross, Net: ledgerGross, BookingCount: count}})
		source = append(source, ReconciliationBucketTotals{BucketDate: date, Totals: ReconciliationTotals{Gross: sourceGross, Net: sourceGross, BookingCount: count}})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, mapRepositoryError(err)
	}
	return ledger, source, nil
}

// loadProductionReconciliationBreakdownBuckets uses the same joins,
// predicates, and formulas as the production breakdown endpoint. Its daily
// mode performs the Jakarta bucketing in PostgreSQL, so a maximum 366-day
// reconciliation remains one breakdown query instead of 366 queries.
func loadProductionReconciliationBreakdownBuckets(ctx context.Context, tx pgx.Tx, start, end time.Time) ([]ReconciliationBucketTotals, []ReconciliationBucketTotals, error) {
	cells, err := (&repository{}).loadProjectionBreakdownCellsByDay(ctx, tx, start, end, "", "")
	if err != nil {
		return nil, nil, err
	}
	cellsByDate := make(map[string][]projectionBreakdownCell)
	for _, cell := range cells {
		cellsByDate[cell.BucketDate] = append(cellsByDate[cell.BucketDate], cell)
	}
	ownerBuckets := make([]ReconciliationBucketTotals, 0, len(cellsByDate))
	venueBuckets := make([]ReconciliationBucketTotals, 0, len(cellsByDate))
	for date, dailyCells := range cellsByDate {
		owners, venues := aggregateReconciliationBreakdown(dailyCells)
		if len(owners) > 0 {
			ownerBuckets = append(ownerBuckets, ReconciliationBucketTotals{BucketDate: date, Totals: sumReconciliationTotals(owners)})
		}
		if len(venues) > 0 {
			venueBuckets = append(venueBuckets, ReconciliationBucketTotals{BucketDate: date, Totals: sumReconciliationTotals(venues)})
		}
	}
	sortReconciliationBuckets(ownerBuckets)
	sortReconciliationBuckets(venueBuckets)
	return ownerBuckets, venueBuckets, nil
}

func sumReconciliationTotals(values []ReconciliationTotals) ReconciliationTotals {
	var total ReconciliationTotals
	for _, value := range values {
		total.Gross += value.Gross
		total.Refund += value.Refund
		total.Net += value.Net
		total.Commission += value.Commission
		total.BookingCount += value.BookingCount
		total.RefundCount += value.RefundCount
		total.OperatingExpense += value.OperatingExpense
	}
	return total
}

func loadReconciliationTrendTotals(ctx context.Context, tx pgx.Tx, start, end time.Time) ([]ReconciliationBucketTotals, error) {
	rows, err := tx.Query(ctx, `
		WITH income_base AS (
			SELECT t.booking_id::text AS booking_id, t.owner_id, t.venue_id,
			       t.created_at AS event_at, t.amount,
			       CASE
					WHEN s.booking_id IS NULL OR s.terms_source='LEGACY_NO_COMMISSION'
					  THEN CAST(ROUND(t.amount * 700::numeric / 10000::numeric) AS bigint)
					ELSE s.commission_amount_rupiah
				END AS commission
			FROM owner_finance_transactions t
			JOIN bookings b ON b.id=t.booking_id
			JOIN courts c ON c.id=b.court_id
			JOIN venues v ON v.id=c.venue_id AND v.id=t.venue_id
			JOIN owner_profiles op ON op.id=v.owner_profile_id AND op.user_id=t.owner_id
			LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
			LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
			WHERE t.type='INCOME' AND t.source='BOOKING' AND obc.booking_id IS NULL
		), income_rows AS (
			SELECT * FROM income_base WHERE event_at >= $1 AND event_at < $2
		), refund_rows AS (
			SELECT t.created_at AS event_at, t.amount, i.commission
			FROM owner_finance_transactions t
			JOIN income_base i ON i.booking_id=t.booking_id::text
			  AND i.owner_id=t.owner_id AND i.venue_id=t.venue_id AND i.amount=t.amount
			WHERE t.type='EXPENSE' AND t.source='REFUND'
			  AND t.created_at >= $1 AND t.created_at < $2
		), events AS (
			SELECT event_at, amount, commission, 'INCOME' AS event_kind FROM income_rows
			UNION ALL
			SELECT event_at, amount, commission, 'REFUND' AS event_kind FROM refund_rows
		)
		SELECT date_trunc('day', event_at AT TIME ZONE 'Asia/Jakarta') AS bucket,
		       COALESCE(SUM(amount) FILTER (WHERE event_kind='INCOME'),0)::bigint AS gross,
		       COALESCE(SUM(amount) FILTER (WHERE event_kind='REFUND'),0)::bigint AS refund,
		       (COALESCE(SUM(commission) FILTER (WHERE event_kind='INCOME'),0)
		        - COALESCE(SUM(commission) FILTER (WHERE event_kind='REFUND'),0))::bigint AS commission,
		       COUNT(*) FILTER (WHERE event_kind='INCOME')::bigint AS booking_count,
		       COUNT(*) FILTER (WHERE event_kind='REFUND')::bigint AS refund_count
		FROM events GROUP BY 1 ORDER BY 1`, start, end)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()
	buckets := make([]ReconciliationBucketTotals, 0)
	for rows.Next() {
		var bucket time.Time
		var totals ReconciliationTotals
		if err := rows.Scan(&bucket, &totals.Gross, &totals.Refund, &totals.Commission, &totals.BookingCount, &totals.RefundCount); err != nil {
			return nil, mapRepositoryError(err)
		}
		totals.Net = totals.Gross - totals.Refund
		buckets = append(buckets, ReconciliationBucketTotals{BucketDate: bucket.In(GetJakartaLocation()).Format("2006-01-02"), Totals: totals})
	}
	if err := rows.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}
	return buckets, nil
}

func loadReconciliationPaidBookings(ctx context.Context, tx pgx.Tx, start, end time.Time) ([]ReconciliationPaidBooking, error) {
	rows, err := tx.Query(ctx, `
		SELECT b.id::text, b.created_at, v.owner_profile_id::text, v.id::text,
		       (obc.booking_id IS NOT NULL), (s.booking_id IS NOT NULL),
		       s.terms_source, s.commission_bps, s.commission_amount_rupiah,
		       s.final_booking_price_rupiah, COUNT(t.id), COALESCE(SUM(t.amount),0)::bigint
		FROM bookings b
		JOIN courts c ON c.id=b.court_id
		JOIN venues v ON v.id=c.venue_id
		LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
		LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
		LEFT JOIN owner_finance_transactions t ON t.booking_id=b.id AND t.type='INCOME' AND t.source='BOOKING'
		  AND t.venue_id=v.id AND t.owner_id=(SELECT user_id FROM owner_profiles WHERE id=v.owner_profile_id)
		WHERE b.status IN ('PAID','COMPLETED') AND b.created_at >= $1 AND b.created_at < $2
		GROUP BY b.id, b.created_at, v.owner_profile_id, v.id, obc.booking_id, s.booking_id,
		         s.terms_source, s.commission_bps, s.commission_amount_rupiah, s.final_booking_price_rupiah
		ORDER BY b.created_at, b.id`, start, end)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()
	bookings := make([]ReconciliationPaidBooking, 0)
	for rows.Next() {
		var booking ReconciliationPaidBooking
		var source pgtype.Text
		var bps pgtype.Int4
		var commission, finalPrice pgtype.Int8
		if err := rows.Scan(&booking.BookingID, &booking.CreatedAt, &booking.OwnerProfileID, &booking.VenueID, &booking.Offline, &booking.Snapshot, &source, &bps, &commission, &finalPrice, &booking.LedgerCount, &booking.LedgerAmount); err != nil {
			return nil, mapRepositoryError(err)
		}
		if source.Valid {
			booking.SnapshotSource = source.String
		}
		if bps.Valid {
			booking.CommissionBPS = bps.Int32
		}
		if commission.Valid {
			booking.Commission = commission.Int64
		}
		if finalPrice.Valid {
			booking.SnapshotFinalPrice = finalPrice.Int64
		}
		bookings = append(bookings, booking)
	}
	return bookings, mapRepositoryError(rows.Err())
}

func loadReconciliationOPEXEvents(ctx context.Context, tx pgx.Tx, start, end time.Time) ([]ReconciliationOPEXEvent, int64, error) {
	rows, err := tx.Query(ctx, `
		WITH events AS (
			SELECT expense.id::text AS expense_id, posted.id::text AS journal_id, posted.effective_at,
			       SUM(entry.amount_rupiah)::bigint AS amount
			FROM platform_expenses expense
			JOIN platform_journals posted ON posted.id=expense.posted_journal_id
			JOIN platform_ledger_entries entry ON entry.journal_id=posted.id
			WHERE expense.status IN ('POSTED','VOID') AND entry.account_code LIKE 'OPEX_%' AND entry.side='DEBIT'
			GROUP BY expense.id, posted.id, posted.effective_at
			UNION ALL
			SELECT expense.id::text, reversal.id::text, reversal.effective_at,
			       -SUM(entry.amount_rupiah)::bigint
			FROM platform_expenses expense
			JOIN platform_journals reversal ON reversal.id=expense.void_journal_id
			JOIN platform_ledger_entries entry ON entry.journal_id=reversal.id
			WHERE expense.status='VOID' AND entry.account_code LIKE 'OPEX_%' AND entry.side='CREDIT'
			GROUP BY expense.id, reversal.id, reversal.effective_at
		)
		SELECT expense_id, journal_id, effective_at, amount FROM events
		WHERE effective_at >= $1 AND effective_at < $2 ORDER BY effective_at, expense_id`, start, end)
	if err != nil {
		return nil, 0, mapRepositoryError(err)
	}
	defer rows.Close()
	events := make([]ReconciliationOPEXEvent, 0)
	var total int64
	for rows.Next() {
		var event ReconciliationOPEXEvent
		if err := rows.Scan(&event.ExpenseID, &event.JournalID, &event.EffectiveAt, &event.Amount); err != nil {
			return nil, 0, mapRepositoryError(err)
		}
		var addErr error
		total, addErr = checkedAddInt64(total, event.Amount)
		if addErr != nil {
			return nil, 0, addErr
		}
		events = append(events, event)
	}
	return events, total, mapRepositoryError(rows.Err())
}

func loadReconciliationOPEXSourceBuckets(ctx context.Context, tx pgx.Tx, start, end time.Time) ([]ReconciliationBucketTotals, error) {
	rows, err := tx.Query(ctx, `
		WITH events AS (
			SELECT posted.effective_at, expense.amount_rupiah AS amount
			FROM platform_expenses expense
			JOIN platform_journals posted ON posted.id=expense.posted_journal_id
			WHERE expense.status IN ('POSTED','VOID')
			UNION ALL
			SELECT reversal.effective_at, -expense.amount_rupiah
			FROM platform_expenses expense
			JOIN platform_journals reversal ON reversal.id=expense.void_journal_id
			WHERE expense.status='VOID'
		)
		SELECT date_trunc('day', effective_at AT TIME ZONE 'Asia/Jakarta'), SUM(amount)::bigint
		FROM events WHERE effective_at >= $1 AND effective_at < $2
		GROUP BY 1 ORDER BY 1`, start, end)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()
	buckets := make([]ReconciliationBucketTotals, 0)
	for rows.Next() {
		var bucket time.Time
		var amount int64
		if err := rows.Scan(&bucket, &amount); err != nil {
			return nil, mapRepositoryError(err)
		}
		buckets = append(buckets, ReconciliationBucketTotals{
			BucketDate: bucket.In(GetJakartaLocation()).Format("2006-01-02"),
			Totals:     ReconciliationTotals{OperatingExpense: amount},
		})
	}
	return buckets, mapRepositoryError(rows.Err())
}

func sortReconciliationBuckets(values []ReconciliationBucketTotals) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j].BucketDate < values[j-1].BucketDate; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
