package platformfinance

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ProjectionBasisHistorical = "HISTORICAL_SCENARIO"
	ProjectionBasisSnapshot   = "BOOKING_SNAPSHOT"
	ProjectionBasisMixed      = "MIXED"
	ProjectionMetricVersion   = "booking-snapshot-projection-v1"
	maxProjectionEvents       = 250_000
)

var (
	ErrMissingProjectionSnapshot   = errors.New("MISSING_PROJECTION_SNAPSHOT")
	ErrProjectionSnapshotMismatch  = errors.New("PROJECTION_SNAPSHOT_LEDGER_MISMATCH")
	ErrInvalidProjectionSource     = errors.New("INVALID_PROJECTION_SOURCE")
	ErrProjectionIntegrity         = errors.New("PROJECTION_INTEGRITY_FAILED")
	ErrMissingProjectionCutover    = errors.New("MISSING_PROJECTION_CUTOVER")
	ErrPostCutoverLegacyProjection = errors.New("POST_CUTOVER_LEGACY_PROJECTION")
	ErrProjectionTooLarge          = errors.New("PROJECTION_DATASET_TOO_LARGE")
)

type projectionEvent struct {
	At             time.Time
	BookingID      string
	OwnerProfileID string
	VenueID        string
	Amount         int64
	Commission     int64
	Source         string
}

type projectionAggregate struct {
	Gross, Refund                         int64
	CommGross, CommRefund                 int64
	BookingCount, RefundedCount           int
	LegacyCount, SnapshotCount            int
	LegacyCommGross, LegacyCommRefund     int64
	SnapshotCommGross, SnapshotCommRefund int64
	LegacyPresent, SnapshotPresent        bool
}

type projectionRow struct {
	Event     projectionEvent
	OwnerName string
	VenueName string
}

type projectionBreakdownCell struct {
	OwnerProfileID, OwnerName string
	VenueID, VenueName        string
	Gross, Refund             int64
	NetCommission             int64
	BookingCount              int
	LegacyCount               int
	SnapshotCount             int
	LegacyNetCommission       int64
	SnapshotNetCommission     int64
	LegacyPresent             bool
	SnapshotPresent           bool
}

func (a *projectionAggregate) addIncome(row projectionRow) {
	a.Gross += row.Event.Amount
	a.CommGross += row.Event.Commission
	a.BookingCount++
	if row.Event.Source == ProjectionBasisSnapshot {
		a.SnapshotPresent = true
		a.SnapshotCount++
		a.SnapshotCommGross += row.Event.Commission
	} else {
		a.LegacyPresent = true
		a.LegacyCount++
		a.LegacyCommGross += row.Event.Commission
	}
}

func (a *projectionAggregate) addRefund(row projectionRow) {
	a.Refund += row.Event.Amount
	a.CommRefund += row.Event.Commission
	a.RefundedCount++
	if row.Event.Source == ProjectionBasisSnapshot {
		a.SnapshotPresent = true
		a.SnapshotCommRefund += row.Event.Commission
	} else {
		a.LegacyPresent = true
		a.LegacyCommRefund += row.Event.Commission
	}
}

func projectionBasis(legacy, snapshot int, fallback string) string {
	switch {
	case legacy > 0 && snapshot > 0:
		return ProjectionBasisMixed
	case snapshot > 0:
		return ProjectionBasisSnapshot
	case legacy > 0:
		return ProjectionBasisHistorical
	default:
		return fallback
	}
}

func projectionBasisWithPresence(legacy, snapshot int, legacyPresent, snapshotPresent bool, fallback string) string {
	if legacyPresent && snapshotPresent {
		return ProjectionBasisMixed
	}
	if snapshotPresent {
		return ProjectionBasisSnapshot
	}
	if legacyPresent {
		return ProjectionBasisHistorical
	}
	return projectionBasis(legacy, snapshot, fallback)
}

func projectionBasisForEmptyRange(start, end, cutover time.Time) string {
	if cutover.IsZero() {
		return ProjectionBasisHistorical
	}
	if !end.After(cutover) {
		return ProjectionBasisHistorical
	}
	if !start.Before(cutover) {
		return ProjectionBasisSnapshot
	}
	return ProjectionBasisMixed
}

func loadProjectionCutover(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM platform_finance_cutovers`).Scan(&count); err != nil {
		return time.Time{}, mapRepositoryError(err)
	}
	if count > 1 {
		return time.Time{}, ErrMissingProjectionCutover
	}
	if count == 0 {
		// Before the operational cutover is activated, all legacy rows are
		// historical scenario data. A zero timestamp is an explicit
		// pre-cutover state, not a reason to break the existing read API.
		return time.Time{}, nil
	}
	var cutover time.Time
	if err := tx.QueryRow(ctx, `SELECT snapshot_cutover_at FROM platform_finance_cutovers WHERE id = 1`).Scan(&cutover); err != nil {
		return time.Time{}, mapRepositoryError(err)
	}
	return cutover.UTC(), nil
}

func projectionAmount(n pgtype.Numeric) (int64, error) {
	v, err := parseNumericExact(n)
	if err != nil {
		return 0, fmt.Errorf("%w: non-integer ledger amount", ErrProjectionIntegrity)
	}
	return v, nil
}

func classifyProjection(bookingCreatedAt, cutover time.Time, source, financeMode, channel string, termIDValid, commissionBpsValid bool, commissionBps int32, snapshotAmount, finalPrice, ledgerAmount int64) (string, int64, error) {
	if source == "" {
		if !cutover.IsZero() && !bookingCreatedAt.Before(cutover) {
			return "", 0, ErrMissingProjectionSnapshot
		}
		return ProjectionBasisHistorical, 0, nil
	}
	if source != "POLICY" && source != "LEGACY_NO_COMMISSION" {
		return "", 0, ErrInvalidProjectionSource
	}
	if (source == "POLICY") != termIDValid {
		return "", 0, ErrInvalidProjectionSource
	}
	if !commissionBpsValid || commissionBps < 0 || commissionBps > 3000 || snapshotAmount < 0 || finalPrice < 0 {
		return "", 0, ErrInvalidProjectionSource
	}
	expected, err := calculateCommissionForProjection(finalPrice, int64(commissionBps))
	if err != nil || expected != snapshotAmount {
		return "", 0, ErrProjectionSnapshotMismatch
	}
	if channel != "MARKETPLACE_ONLINE" || financeMode != "SIMULATION" {
		return "", 0, ErrInvalidProjectionSource
	}
	if finalPrice != ledgerAmount {
		return "", 0, ErrProjectionSnapshotMismatch
	}
	if source == "LEGACY_NO_COMMISSION" {
		if !cutover.IsZero() && !bookingCreatedAt.Before(cutover) {
			return "", 0, ErrPostCutoverLegacyProjection
		}
		return ProjectionBasisHistorical, snapshotAmount, nil
	}
	return ProjectionBasisSnapshot, snapshotAmount, nil
}

type projectionSnapshotRow struct {
	Source, Channel, FinanceMode string
	CommissionAmount, FinalPrice int64
	CommissionBps                int32
	CommissionBpsValid           bool
	TermIDValid                  bool
	Valid                        bool
}

func scanProjectionSnapshot(source, channel, financeMode *pgtype.Text, commission, finalPrice *pgtype.Int8, commissionBps *pgtype.Int4, termID *pgtype.UUID) projectionSnapshotRow {
	row := projectionSnapshotRow{Valid: source.Valid}
	if source.Valid {
		row.Source = source.String
	}
	if channel.Valid {
		row.Channel = channel.String
	}
	if financeMode.Valid {
		row.FinanceMode = financeMode.String
	}
	if commission.Valid {
		row.CommissionAmount = commission.Int64
	}
	if finalPrice.Valid {
		row.FinalPrice = finalPrice.Int64
	}
	if commissionBps.Valid {
		row.CommissionBps = commissionBps.Int32
		row.CommissionBpsValid = true
	}
	row.TermIDValid = termID.Valid
	return row
}

func (r *repository) loadProjectionEvents(ctx context.Context, tx pgx.Tx, start, end time.Time, ownerID, venueID string) ([]projectionRow, []projectionRow, time.Time, time.Time, error) {
	return r.loadProjectionEventsMode(ctx, tx, start, end, ownerID, venueID, true)
}

func (r *repository) validateProjectionEvents(ctx context.Context, tx pgx.Tx, start, end time.Time, ownerID, venueID string) (time.Time, time.Time, error) {
	_, _, asOf, cutover, err := r.loadProjectionEventsMode(ctx, tx, start, end, ownerID, venueID, false)
	return asOf, cutover, err
}

func (r *repository) loadProjectionEventsMode(ctx context.Context, tx pgx.Tx, start, end time.Time, ownerID, venueID string, collect bool) ([]projectionRow, []projectionRow, time.Time, time.Time, error) {
	cutover, err := loadProjectionCutover(ctx, tx)
	if err != nil {
		return nil, nil, time.Time{}, time.Time{}, err
	}
	var asOf time.Time
	if err := tx.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&asOf); err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}

	var missingPostCutover int
	if !cutover.IsZero() {
		if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM bookings b
		LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
		WHERE b.created_at >= $1 AND b.created_at < $2
		  AND b.created_at >= $3 AND s.booking_id IS NULL`, start, end, cutover).Scan(&missingPostCutover); err != nil {
			return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
		}
		if missingPostCutover > 0 {
			return nil, nil, time.Time{}, time.Time{}, ErrMissingProjectionSnapshot
		}
		var legacyPostCutover int
		if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM bookings b
		JOIN booking_fee_snapshots s ON s.booking_id=b.id
		WHERE b.created_at >= $1 AND b.created_at < $2
		  AND b.created_at >= $3 AND s.terms_source='LEGACY_NO_COMMISSION'`, start, end, cutover).Scan(&legacyPostCutover); err != nil {
			return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
		}
		if legacyPostCutover > 0 {
			return nil, nil, time.Time{}, time.Time{}, ErrPostCutoverLegacyProjection
		}
	}

	var duplicate int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT booking_id FROM owner_finance_transactions WHERE type='INCOME' AND source='BOOKING' AND booking_id IS NOT NULL GROUP BY booking_id HAVING count(*) > 1) q`).Scan(&duplicate); err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}
	if duplicate > 0 {
		return nil, nil, time.Time{}, time.Time{}, ErrDuplicateLedgerDetected
	}
	var fractional int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner_finance_transactions WHERE amount <> trunc(amount)`).Scan(&fractional); err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}
	if fractional > 0 {
		return nil, nil, time.Time{}, time.Time{}, ErrFractionalLedgerDetected
	}
	var orphanRefund int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM owner_finance_transactions t
		WHERE t.type = 'EXPENSE' AND t.source = 'REFUND'
		  AND t.created_at >= $1 AND t.created_at < $2
		  AND (t.booking_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions i
			WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING'
		))`, start, end).Scan(&orphanRefund); err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}
	if orphanRefund > 0 {
		return nil, nil, time.Time{}, time.Time{}, ErrOrphanRefundDetected
	}

	filter := ""
	args := []any{start, end}
	if ownerID != "" {
		filter += fmt.Sprintf(" AND op.id = $%d", len(args)+1)
		args = append(args, ownerID)
	}
	if venueID != "" {
		filter += fmt.Sprintf(" AND v.id = $%d", len(args)+1)
		args = append(args, venueID)
	}
	incomeSQL := `
SELECT t.booking_id::text, t.created_at, op.id::text, v.id::text, t.amount,
       b.created_at, s.terms_source, s.booking_channel, s.finance_mode,
       s.commission_amount_rupiah, s.final_booking_price_rupiah, s.commission_bps, s.commercial_term_id,
       op.business_name, v.name
FROM owner_finance_transactions t
JOIN bookings b ON b.id = t.booking_id
JOIN courts c ON c.id = b.court_id
JOIN venues v ON v.id = c.venue_id AND v.id = t.venue_id
JOIN owner_profiles op ON op.id = v.owner_profile_id AND op.user_id = t.owner_id
LEFT JOIN offline_booking_customers obc ON obc.booking_id = b.id
LEFT JOIN booking_fee_snapshots s ON s.booking_id = b.id
WHERE t.type='INCOME' AND t.source='BOOKING' AND t.booking_id IS NOT NULL
  AND obc.booking_id IS NULL AND t.created_at >= $1 AND t.created_at < $2` + filter + `
ORDER BY t.created_at, t.booking_id`

	rows, err := tx.Query(ctx, incomeSQL, args...)
	if err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}
	defer rows.Close()
	var incomes []projectionRow
	for rows.Next() {
		var id, ownerProfile, venue, ownerName, venueName string
		var eventAt, bookingCreated time.Time
		var amount pgtype.Numeric
		var source, channel, mode pgtype.Text
		var comm, final pgtype.Int8
		var commissionBps pgtype.Int4
		var termID pgtype.UUID
		if err := rows.Scan(&id, &eventAt, &ownerProfile, &venue, &amount, &bookingCreated, &source, &channel, &mode, &comm, &final, &commissionBps, &termID, &ownerName, &venueName); err != nil {
			return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
		}
		money, err := projectionAmount(amount)
		if err != nil {
			return nil, nil, time.Time{}, time.Time{}, err
		}
		snap := scanProjectionSnapshot(&source, &channel, &mode, &comm, &final, &commissionBps, &termID)
		basis, projected, err := classifyProjection(bookingCreated, cutover, snap.Source, snap.FinanceMode, snap.Channel, snap.TermIDValid, snap.CommissionBpsValid, snap.CommissionBps, snap.CommissionAmount, snap.FinalPrice, money)
		if err != nil {
			return nil, nil, time.Time{}, time.Time{}, err
		}
		if basis == ProjectionBasisHistorical {
			projected, err = calculateHistoricalCommission(money)
			if err != nil {
				return nil, nil, time.Time{}, time.Time{}, err
			}
		}
		if collect && len(incomes) >= maxProjectionEvents {
			return nil, nil, time.Time{}, time.Time{}, ErrProjectionTooLarge
		}
		if collect {
			incomes = append(incomes, projectionRow{Event: projectionEvent{At: eventAt, BookingID: id, OwnerProfileID: ownerProfile, VenueID: venue, Amount: money, Commission: projected, Source: basis}, OwnerName: ownerName, VenueName: venueName})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}

	refundSQL := `
SELECT t.booking_id::text, t.created_at, op.id::text, v.id::text, t.amount,
       b.created_at, s.terms_source, s.booking_channel, s.finance_mode,
       s.commission_amount_rupiah, s.final_booking_price_rupiah, s.commission_bps, s.commercial_term_id,
       i.amount, op.business_name, v.name
FROM owner_finance_transactions t
JOIN owner_finance_transactions i ON i.booking_id=t.booking_id AND i.type='INCOME' AND i.source='BOOKING'
JOIN bookings b ON b.id=t.booking_id
JOIN courts c ON c.id=b.court_id
JOIN venues v ON v.id=c.venue_id AND v.id=t.venue_id
JOIN owner_profiles op ON op.id=v.owner_profile_id AND op.user_id=t.owner_id
LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
LEFT JOIN booking_fee_snapshots s ON s.booking_id=b.id
WHERE t.type='EXPENSE' AND t.source='REFUND' AND obc.booking_id IS NULL
  AND t.created_at >= $1 AND t.created_at < $2` + filter + `
ORDER BY t.created_at, t.booking_id`
	rows, err = tx.Query(ctx, refundSQL, args...)
	if err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}
	defer rows.Close()
	var refunds []projectionRow
	for rows.Next() {
		var id, ownerProfile, venue, ownerName, venueName string
		var eventAt, bookingCreated time.Time
		var amount, original pgtype.Numeric
		var source, channel, mode pgtype.Text
		var comm, final pgtype.Int8
		var commissionBps pgtype.Int4
		var termID pgtype.UUID
		if err := rows.Scan(&id, &eventAt, &ownerProfile, &venue, &amount, &bookingCreated, &source, &channel, &mode, &comm, &final, &commissionBps, &termID, &original, &ownerName, &venueName); err != nil {
			return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
		}
		refundAmount, err := projectionAmount(amount)
		if err != nil {
			return nil, nil, time.Time{}, time.Time{}, err
		}
		originalAmount, err := projectionAmount(original)
		if err != nil {
			return nil, nil, time.Time{}, time.Time{}, err
		}
		if refundAmount != originalAmount {
			return nil, nil, time.Time{}, time.Time{}, ErrRefundAmountMismatch
		}
		snap := scanProjectionSnapshot(&source, &channel, &mode, &comm, &final, &commissionBps, &termID)
		basis, projected, err := classifyProjection(bookingCreated, cutover, snap.Source, snap.FinanceMode, snap.Channel, snap.TermIDValid, snap.CommissionBpsValid, snap.CommissionBps, snap.CommissionAmount, snap.FinalPrice, originalAmount)
		if err != nil {
			return nil, nil, time.Time{}, time.Time{}, err
		}
		if basis == ProjectionBasisHistorical {
			projected, err = calculateHistoricalCommission(originalAmount)
			if err != nil {
				return nil, nil, time.Time{}, time.Time{}, err
			}
		}
		if collect && len(incomes)+len(refunds) >= maxProjectionEvents {
			return nil, nil, time.Time{}, time.Time{}, ErrProjectionTooLarge
		}
		if collect {
			refunds = append(refunds, projectionRow{Event: projectionEvent{At: eventAt, BookingID: id, OwnerProfileID: ownerProfile, VenueID: venue, Amount: refundAmount, Commission: projected, Source: basis}, OwnerName: ownerName, VenueName: venueName})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, time.Time{}, time.Time{}, mapRepositoryError(err)
	}
	return incomes, refunds, asOf, cutover, nil
}

func calculateHistoricalCommission(amount int64) (int64, error) {
	if amount < 0 {
		return 0, ErrProjectionIntegrity
	}
	// SQL and Go both use exact numeric half-up at 700 bps.
	qv := amount / 10000
	remv := amount % 10000
	// Avoid an intermediate overflow: amount is bounded by the booking schema.
	commission := qv * 700
	remProduct := remv * 700
	commission += remProduct / 10000
	if remProduct%10000 >= 5000 {
		commission++
	}
	return commission, nil
}

func calculateCommissionForProjection(amount, bps int64) (int64, error) {
	if amount < 0 || bps < 0 || bps > 3000 {
		return 0, ErrProjectionIntegrity
	}
	quotient := amount / 10000
	remainder := amount % 10000
	commission := quotient * bps
	product := remainder * bps
	commission += product / 10000
	if product%10000 >= 5000 {
		commission++
	}
	return commission, nil
}

func aggregateProjection(incomes, refunds []projectionRow) (projectionAggregate, map[string]*projectionAggregate, map[string]*projectionAggregate) {
	var total projectionAggregate
	owners := map[string]*projectionAggregate{}
	venues := map[string]*projectionAggregate{}
	for _, row := range incomes {
		total.addIncome(row)
		if owners[row.Event.OwnerProfileID] == nil {
			owners[row.Event.OwnerProfileID] = &projectionAggregate{}
		}
		owners[row.Event.OwnerProfileID].addIncome(row)
		if venues[row.Event.VenueID] == nil {
			venues[row.Event.VenueID] = &projectionAggregate{}
		}
		venues[row.Event.VenueID].addIncome(row)
	}
	for _, row := range refunds {
		total.addRefund(row)
		if owners[row.Event.OwnerProfileID] == nil {
			owners[row.Event.OwnerProfileID] = &projectionAggregate{}
		}
		owners[row.Event.OwnerProfileID].addRefund(row)
		if venues[row.Event.VenueID] == nil {
			venues[row.Event.VenueID] = &projectionAggregate{}
		}
		venues[row.Event.VenueID].addRefund(row)
	}
	return total, owners, venues
}

func loadProjectionQuality(ctx context.Context, tx pgx.Tx, start, end time.Time, ownerID, venueID string) (int, int, error) {
	filter := ""
	args := []any{start, end}
	if ownerID != "" {
		filter += fmt.Sprintf(" AND v.owner_profile_id = $%d", len(args)+1)
		args = append(args, ownerID)
	}
	if venueID != "" {
		filter += fmt.Sprintf(" AND v.id = $%d", len(args)+1)
		args = append(args, venueID)
	}
	var paidWithout int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM bookings b
		JOIN courts c ON c.id=b.court_id
		JOIN venues v ON v.id=c.venue_id
		LEFT JOIN offline_booking_customers obc ON obc.booking_id=b.id
		WHERE b.status IN ('PAID','COMPLETED') AND obc.booking_id IS NULL
		  AND b.created_at >= $1 AND b.created_at < $2
		  AND NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions t
			WHERE t.booking_id=b.id AND t.type='INCOME' AND t.source='BOOKING'
			  AND t.venue_id=v.id
			  AND t.owner_id=(SELECT user_id FROM owner_profiles WHERE id=v.owner_profile_id)
		  )`+filter, args...).Scan(&paidWithout); err != nil {
		return 0, 0, mapRepositoryError(err)
	}

	var ledgerWithout int
	ledgerFilter, ledgerArgs := buildFilters(ownerID, venueID, 3)
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM owner_finance_transactions t
		WHERE t.type='INCOME' AND t.source='BOOKING'
		  AND t.created_at >= $1 AND t.created_at < $2
		  AND NOT (`+canonicalLedgerBookingPredicate+`)`+ledgerFilter, append([]any{start, end}, ledgerArgs...)...).Scan(&ledgerWithout); err != nil {
		return 0, 0, mapRepositoryError(err)
	}
	return paidWithout, ledgerWithout, nil
}

func (r *repository) getProjectionSummary(ctx context.Context, start, end time.Time, ownerID, venueID string) (*SummaryDataResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	incomes, refunds, asOf, cutover, err := r.loadProjectionEvents(ctx, tx, start, end, ownerID, venueID)
	if err != nil {
		return nil, err
	}
	total, owners, venues := aggregateProjection(incomes, refunds)
	paidWithout, ledgerWithout, err := loadProjectionQuality(ctx, tx, start, end, ownerID, venueID)
	if err != nil {
		return nil, err
	}
	legacy, snapshot := total.LegacyCount, total.SnapshotCount
	result := &SummaryDataResult{AsOf: asOf, CutoverAt: cutover, Gross: total.Gross, RefundPrincipal: total.Refund, ProjectedCommGross: total.CommGross, ProjectedCommRefunded: total.CommRefund, RealizedBookingCount: total.BookingCount, RefundedBookingCount: total.RefundedCount, PaidWithoutLedgerCount: paidWithout, LedgerWithoutBookingCount: ledgerWithout, LegacyScenarioCount: legacy, SnapshotProjectionCount: snapshot, LegacyProjectionAmount: total.LegacyCommGross - total.LegacyCommRefund, SnapshotProjectionAmount: total.SnapshotCommGross - total.SnapshotCommRefund, LegacyProjectionPresent: total.LegacyPresent, SnapshotProjectionPresent: total.SnapshotPresent, ProjectionBasis: projectionBasisWithPresence(legacy, snapshot, total.LegacyPresent, total.SnapshotPresent, projectionBasisForEmptyRange(start, end, cutover))}
	result.LegacyGross = 0
	result.SnapshotGross = 0
	result.LegacyRefund = 0
	result.SnapshotRefund = 0
	for _, row := range incomes {
		if row.Event.Source == ProjectionBasisSnapshot {
			result.SnapshotGross += row.Event.Amount
		} else {
			result.LegacyGross += row.Event.Amount
		}
	}
	for _, row := range refunds {
		if row.Event.Source == ProjectionBasisSnapshot {
			result.SnapshotRefund += row.Event.Amount
		} else {
			result.LegacyRefund += row.Event.Amount
		}
	}
	for _, row := range incomes {
		result.IncomeBuckets = append(result.IncomeBuckets, BucketResult{Bucket: row.Event.At, Amount: row.Event.Amount, Comm: row.Event.Commission, Source: row.Event.Source, BookingCount: 1, CommissionAmount: row.Event.Commission})
	}
	for _, row := range refunds {
		result.RefundBuckets = append(result.RefundBuckets, BucketResult{Bucket: row.Event.At, Amount: row.Event.Amount, Comm: row.Event.Commission, Source: row.Event.Source, BookingCount: 1, CommissionAmount: row.Event.Commission})
	}
	for id, a := range owners {
		result.TopOwners = append(result.TopOwners, BreakdownRow{ID: id, Gross: a.Gross, Refund: a.Refund, Net: a.Gross - a.Refund, BookingCount: a.BookingCount, NetComm: a.CommGross - a.CommRefund, LegacyScenarioCount: a.LegacyCount, SnapshotProjectionCount: a.SnapshotCount, NonBillableProjectionAmount: a.LegacyCommGross - a.LegacyCommRefund, SnapshotProjectionAmount: a.SnapshotCommGross - a.SnapshotCommRefund, LegacyProjectionPresent: a.LegacyPresent, SnapshotProjectionPresent: a.SnapshotPresent, ProjectionBasis: projectionBasisWithPresence(a.LegacyCount, a.SnapshotCount, a.LegacyPresent, a.SnapshotPresent, ProjectionBasisHistorical)})
	}
	for id, a := range venues {
		result.TopVenues = append(result.TopVenues, BreakdownRow{ID: id, Gross: a.Gross, Refund: a.Refund, Net: a.Gross - a.Refund, BookingCount: a.BookingCount, NetComm: a.CommGross - a.CommRefund, LegacyScenarioCount: a.LegacyCount, SnapshotProjectionCount: a.SnapshotCount, NonBillableProjectionAmount: a.LegacyCommGross - a.LegacyCommRefund, SnapshotProjectionAmount: a.SnapshotCommGross - a.SnapshotCommRefund, LegacyProjectionPresent: a.LegacyPresent, SnapshotProjectionPresent: a.SnapshotPresent, ProjectionBasis: projectionBasisWithPresence(a.LegacyCount, a.SnapshotCount, a.LegacyPresent, a.SnapshotPresent, ProjectionBasisHistorical)})
	}
	ownerNames := make(map[string]string)
	venueNames := make(map[string]string)
	for _, row := range append(append([]projectionRow{}, incomes...), refunds...) {
		if row.OwnerName != "" {
			ownerNames[row.Event.OwnerProfileID] = row.OwnerName
		}
		if row.VenueName != "" {
			venueNames[row.Event.VenueID] = row.VenueName
		}
	}
	for i := range result.TopOwners {
		result.TopOwners[i].Name = ownerNames[result.TopOwners[i].ID]
	}
	for i := range result.TopVenues {
		result.TopVenues[i].Name = venueNames[result.TopVenues[i].ID]
		result.TopVenues[i].OwnerProfileID = ownerProfileForVenue(incomes, refunds, result.TopVenues[i].ID)
	}
	sort.Slice(result.TopOwners, func(i, j int) bool {
		if result.TopOwners[i].Net != result.TopOwners[j].Net {
			return result.TopOwners[i].Net > result.TopOwners[j].Net
		}
		return result.TopOwners[i].ID < result.TopOwners[j].ID
	})
	sort.Slice(result.TopVenues, func(i, j int) bool {
		if result.TopVenues[i].Net != result.TopVenues[j].Net {
			return result.TopVenues[i].Net > result.TopVenues[j].Net
		}
		return result.TopVenues[i].ID < result.TopVenues[j].ID
	})
	if len(result.TopOwners) > 10 {
		result.TopOwners = result.TopOwners[:10]
	}
	if len(result.TopVenues) > 10 {
		result.TopVenues = result.TopVenues[:10]
	}
	return result, nil
}

func ownerProfileForVenue(incomes, refunds []projectionRow, venueID string) string {
	for _, row := range append(append([]projectionRow{}, incomes...), refunds...) {
		if row.Event.VenueID == venueID {
			return row.Event.OwnerProfileID
		}
	}
	return ""
}

func (r *repository) loadProjectionBreakdownCells(ctx context.Context, tx pgx.Tx, start, end time.Time, ownerID, venueID string) ([]projectionBreakdownCell, error) {
	filter := ""
	args := []any{start, end}
	if ownerID != "" {
		filter += fmt.Sprintf(" AND op.id = $%d", len(args)+1)
		args = append(args, ownerID)
	}
	if venueID != "" {
		filter += fmt.Sprintf(" AND v.id = $%d", len(args)+1)
		args = append(args, venueID)
	}

	query := `
WITH income_base AS (
    SELECT t.booking_id::text AS booking_id,
           t.owner_id,
           t.venue_id AS ledger_venue_id,
           t.created_at,
           t.amount,
           op.id::text AS owner_profile_id,
           op.business_name AS owner_name,
           v.id::text AS venue_id,
           v.name AS venue_name,
           CASE
             WHEN s.booking_id IS NULL OR s.terms_source = 'LEGACY_NO_COMMISSION'
               THEN 'HISTORICAL_SCENARIO'
             ELSE 'BOOKING_SNAPSHOT'
           END AS projection_source,
           CASE
             WHEN s.booking_id IS NULL OR s.terms_source = 'LEGACY_NO_COMMISSION'
               THEN CAST(ROUND(t.amount * 700::numeric / 10000::numeric) AS bigint)
             ELSE s.commission_amount_rupiah
           END AS commission
    FROM owner_finance_transactions t
    JOIN bookings b ON b.id = t.booking_id
    JOIN courts c ON c.id = b.court_id
    JOIN venues v ON v.id = c.venue_id AND v.id = t.venue_id
    JOIN owner_profiles op ON op.id = v.owner_profile_id AND op.user_id = t.owner_id
    LEFT JOIN offline_booking_customers obc ON obc.booking_id = b.id
    LEFT JOIN booking_fee_snapshots s ON s.booking_id = b.id
    WHERE t.type = 'INCOME' AND t.source = 'BOOKING'
      AND t.booking_id IS NOT NULL
      AND obc.booking_id IS NULL` + filter + `
), income_rows AS (
    SELECT *
    FROM income_base
    WHERE created_at >= $1 AND created_at < $2
), refund_rows AS (
    SELECT t.booking_id::text AS booking_id,
           i.owner_profile_id,
           i.owner_name,
           i.venue_id,
           i.venue_name,
           t.amount,
           i.projection_source,
           i.commission
    FROM owner_finance_transactions t
    JOIN income_base i
      ON i.booking_id = t.booking_id::text
     AND i.owner_id = t.owner_id
     AND i.ledger_venue_id = t.venue_id
     AND i.amount = t.amount
    WHERE t.type = 'EXPENSE' AND t.source = 'REFUND'
      AND t.created_at >= $1 AND t.created_at < $2
), events AS (
    SELECT owner_profile_id, owner_name, venue_id, venue_name,
           amount, commission, projection_source, 'INCOME' AS event_kind
    FROM income_rows
    UNION ALL
    SELECT owner_profile_id, owner_name, venue_id, venue_name,
           amount, commission, projection_source, 'REFUND' AS event_kind
    FROM refund_rows
)
SELECT owner_profile_id,
       owner_name,
       venue_id,
       venue_name,
       CAST(COALESCE(SUM(amount) FILTER (WHERE event_kind = 'INCOME'), 0) AS bigint) AS gross,
       CAST(COALESCE(SUM(amount) FILTER (WHERE event_kind = 'REFUND'), 0) AS bigint) AS refund,
       CAST(COUNT(*) FILTER (WHERE event_kind = 'INCOME') AS integer) AS booking_count,
       CAST(COALESCE(SUM(commission) FILTER (WHERE event_kind = 'INCOME'), 0) - COALESCE(SUM(commission) FILTER (WHERE event_kind = 'REFUND'), 0) AS bigint) AS net_commission,
       CAST(COUNT(*) FILTER (WHERE event_kind = 'INCOME' AND projection_source = 'HISTORICAL_SCENARIO') AS integer) AS legacy_count,
       CAST(COUNT(*) FILTER (WHERE event_kind = 'INCOME' AND projection_source = 'BOOKING_SNAPSHOT') AS integer) AS snapshot_count,
       CAST(COALESCE(SUM(commission) FILTER (WHERE projection_source = 'HISTORICAL_SCENARIO' AND event_kind = 'INCOME'), 0) - COALESCE(SUM(commission) FILTER (WHERE projection_source = 'HISTORICAL_SCENARIO' AND event_kind = 'REFUND'), 0) AS bigint) AS legacy_net_commission,
       CAST(COALESCE(SUM(commission) FILTER (WHERE projection_source = 'BOOKING_SNAPSHOT' AND event_kind = 'INCOME'), 0) - COALESCE(SUM(commission) FILTER (WHERE projection_source = 'BOOKING_SNAPSHOT' AND event_kind = 'REFUND'), 0) AS bigint) AS snapshot_net_commission,
       COALESCE(BOOL_OR(projection_source = 'HISTORICAL_SCENARIO'), false) AS legacy_present,
       COALESCE(BOOL_OR(projection_source = 'BOOKING_SNAPSHOT'), false) AS snapshot_present
FROM events
GROUP BY owner_profile_id, owner_name, venue_id, venue_name`

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()
	var cells []projectionBreakdownCell
	for rows.Next() {
		var cell projectionBreakdownCell
		if err := rows.Scan(
			&cell.OwnerProfileID, &cell.OwnerName, &cell.VenueID, &cell.VenueName,
			&cell.Gross, &cell.Refund, &cell.BookingCount, &cell.NetCommission,
			&cell.LegacyCount, &cell.SnapshotCount, &cell.LegacyNetCommission,
			&cell.SnapshotNetCommission, &cell.LegacyPresent, &cell.SnapshotPresent,
		); err != nil {
			return nil, mapRepositoryError(err)
		}
		cells = append(cells, cell)
	}
	if err := rows.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}
	return cells, nil
}

func (r *repository) getProjectionBreakdown(ctx context.Context, start, end time.Time, ownerID, venueID, dimension string, page, limit int) (*BreakdownResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	asOf, cutover, err := r.validateProjectionEvents(ctx, tx, start, end, ownerID, venueID)
	if err != nil {
		return nil, err
	}
	cells, err := r.loadProjectionBreakdownCells(ctx, tx, start, end, ownerID, venueID)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]*BreakdownRow)
	var total BreakdownRow
	for _, cell := range cells {
		id, name := cell.OwnerProfileID, cell.OwnerName
		if dimension == "venue" {
			id, name = cell.VenueID, cell.VenueName
		}
		row := groups[id]
		if row == nil {
			row = &BreakdownRow{ID: id, Name: name}
			if dimension == "venue" {
				row.OwnerProfileID = cell.OwnerProfileID
			}
			groups[id] = row
		}
		row.Gross += cell.Gross
		row.Refund += cell.Refund
		row.BookingCount += cell.BookingCount
		row.NetComm += cell.NetCommission
		row.LegacyScenarioCount += cell.LegacyCount
		row.SnapshotProjectionCount += cell.SnapshotCount
		row.NonBillableProjectionAmount += cell.LegacyNetCommission
		row.SnapshotProjectionAmount += cell.SnapshotNetCommission
		row.LegacyProjectionPresent = row.LegacyProjectionPresent || cell.LegacyPresent
		row.SnapshotProjectionPresent = row.SnapshotProjectionPresent || cell.SnapshotPresent
		row.Net = row.Gross - row.Refund
		row.ProjectionBasis = projectionBasisWithPresence(row.LegacyScenarioCount, row.SnapshotProjectionCount, row.LegacyProjectionPresent, row.SnapshotProjectionPresent, ProjectionBasisHistorical)
		total.Gross += cell.Gross
		total.Refund += cell.Refund
		total.BookingCount += cell.BookingCount
		total.NetComm += cell.NetCommission
		total.LegacyScenarioCount += cell.LegacyCount
		total.SnapshotProjectionCount += cell.SnapshotCount
		total.NonBillableProjectionAmount += cell.LegacyNetCommission
		total.SnapshotProjectionAmount += cell.SnapshotNetCommission
		total.LegacyProjectionPresent = total.LegacyProjectionPresent || cell.LegacyPresent
		total.SnapshotProjectionPresent = total.SnapshotProjectionPresent || cell.SnapshotPresent
	}
	rows := make([]BreakdownRow, 0, len(groups))
	for _, row := range groups {
		rows = append(rows, *row)
	}
	total.Net = total.Gross - total.Refund
	total.ProjectionBasis = projectionBasisWithPresence(total.LegacyScenarioCount, total.SnapshotProjectionCount, total.LegacyProjectionPresent, total.SnapshotProjectionPresent, projectionBasisForEmptyRange(start, end, cutover))
	// Deterministic order: net descending then UUID ascending. The SQL query has
	// already reduced the event stream to dimension cells before this sort.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Net != rows[j].Net {
			return rows[i].Net > rows[j].Net
		}
		return rows[i].ID < rows[j].ID
	})
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	startIdx := (page - 1) * limit
	if startIdx > len(rows) {
		startIdx = len(rows)
	}
	endIdx := startIdx + limit
	if endIdx > len(rows) {
		endIdx = len(rows)
	}
	return &BreakdownResult{AsOf: asOf, CutoverAt: cutover, TotalItems: len(rows), Rows: rows[startIdx:endIdx], ProjectionBasis: total.ProjectionBasis, LegacyScenarioCount: total.LegacyScenarioCount, SnapshotProjectionCount: total.SnapshotProjectionCount, NonBillableProjectionAmount: total.NonBillableProjectionAmount, SnapshotProjectionAmount: total.SnapshotProjectionAmount, LegacyProjectionPresent: total.LegacyProjectionPresent, SnapshotProjectionPresent: total.SnapshotProjectionPresent}, nil
}
