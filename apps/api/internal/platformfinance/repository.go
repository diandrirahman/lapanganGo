package platformfinance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateLedgerDetected  = errors.New("DUPLICATE_LEDGER_DETECTED")
	ErrFractionalLedgerDetected = errors.New("FRACTIONAL_LEDGER_DETECTED")
	ErrOrphanRefundDetected     = errors.New("ORPHAN_REFUND_DETECTED")
	ErrRefundAmountMismatch     = errors.New("REFUND_AMOUNT_MISMATCH")
	ErrOverflowDetected         = errors.New("OVERFLOW_DETECTED")
)

const canonicalLedgerBookingPredicate = `EXISTS (
	SELECT 1
	FROM bookings b
	JOIN courts c ON c.id = b.court_id
	JOIN venues v ON v.id = c.venue_id
	JOIN owner_profiles op ON op.id = v.owner_profile_id
	WHERE b.id = t.booking_id
	  AND v.id = t.venue_id
	  AND op.user_id = t.owner_id
)`

type Repository interface {
	OwnerMatchesVenue(ctx context.Context, ownerProfileID, venueID string) (bool, error)
	GetSummaryData(ctx context.Context, utcStart, utcEndExclusive time.Time, ownerID, venueID string) (*SummaryDataResult, error)
	GetPaginatedBreakdown(ctx context.Context, utcStart, utcEndExclusive time.Time, ownerID, venueID, dimension string, page, limit int) (*BreakdownResult, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

type SummaryDataResult struct {
	AsOf                  time.Time
	Gross                 int64
	RealizedBookingCount  int
	ProjectedCommGross    int64
	RefundPrincipal       int64
	RefundedBookingCount  int
	ProjectedCommRefunded int64

	PaidWithoutLedgerCount    int
	LedgerWithoutBookingCount int

	IncomeBuckets []BucketResult
	RefundBuckets []BucketResult

	TopOwners []BreakdownRow
	TopVenues []BreakdownRow
}

type BucketResult struct {
	Bucket time.Time
	Amount int64
	Comm   int64
}

type BreakdownRow struct {
	ID             string
	Name           string
	OwnerProfileID string // only used for venue dimension
	Gross          int64
	Refund         int64
	Net            int64
	BookingCount   int
	NetComm        int64
}

type BreakdownResult struct {
	AsOf       time.Time
	TotalItems int
	Rows       []BreakdownRow
}

func buildFilters(ownerProfileID, venueID string, argOffset int) (string, []any) {
	var clauses []string
	var args []any
	if ownerProfileID != "" {
		// Finance transactions reference users.id; the public filter references
		// owner_profiles.id. Resolve it in SQL so every query applies the same scope.
		clauses = append(clauses, fmt.Sprintf("t.owner_id = (SELECT user_id FROM owner_profiles WHERE id = $%d)", argOffset))
		args = append(args, ownerProfileID)
		argOffset++
	}
	if venueID != "" {
		clauses = append(clauses, fmt.Sprintf("t.venue_id = $%d", argOffset))
		args = append(args, venueID)
		argOffset++
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func buildBookingFilters(ownerProfileID, venueID string, argOffset int) (string, []any) {
	var clauses []string
	var args []any
	if ownerProfileID != "" {
		clauses = append(clauses, fmt.Sprintf("v.owner_profile_id = $%d", argOffset))
		args = append(args, ownerProfileID)
		argOffset++
	}
	if venueID != "" {
		clauses = append(clauses, fmt.Sprintf("v.id = $%d", argOffset))
		args = append(args, venueID)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func (r *repository) OwnerMatchesVenue(ctx context.Context, ownerProfileID, venueID string) (bool, error) {
	var matches bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM venues WHERE id = $1 AND owner_profile_id = $2
		)
	`, venueID, ownerProfileID).Scan(&matches)
	return matches, mapRepositoryError(err)
}

func mapRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "22003" {
		return ErrOverflowDetected
	}
	return err
}

func (r *repository) GetSummaryData(ctx context.Context, utcStart, utcEndExclusive time.Time, ownerID, venueID string) (*SummaryDataResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	return getSummaryDataInTx(ctx, tx, utcStart, utcEndExclusive, ownerID, venueID)
}

func getSummaryDataInTx(ctx context.Context, tx pgx.Tx, utcStart, utcEndExclusive time.Time, ownerID, venueID string) (*SummaryDataResult, error) {
	var err error
	var asOf time.Time
	if err := tx.QueryRow(ctx, "SELECT CURRENT_TIMESTAMP").Scan(&asOf); err != nil {
		return nil, err
	}

	// 1. Duplicate check (Fail closed)
	var dupCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT booking_id 
			FROM owner_finance_transactions 
			WHERE type = 'INCOME' AND source = 'BOOKING' 
			GROUP BY booking_id 
			HAVING count(*) > 1
		) d
	`).Scan(&dupCount)
	if err != nil {
		return nil, err
	}
	if dupCount > 0 {
		// Log gracefully (server-side log is handled by caller or here)
		return nil, ErrDuplicateLedgerDetected
	}

	// 1a. Fractional ledger check (Fail closed)
	var fracCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*) 
		FROM owner_finance_transactions 
		WHERE amount != TRUNC(amount)
	`).Scan(&fracCount)
	if err != nil {
		return nil, err
	}
	if fracCount > 0 {
		return nil, ErrFractionalLedgerDetected
	}

	// A refund without a valid canonical booking and its matching original
	// income has no deterministic commission reversal.
	var orphanRefundCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM owner_finance_transactions t
		WHERE t.type = 'EXPENSE' AND t.source = 'REFUND'
		  AND (
			t.booking_id IS NULL
			OR NOT (`+canonicalLedgerBookingPredicate+`)
			OR NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions i
			WHERE i.booking_id = t.booking_id
			  AND i.type = 'INCOME' AND i.source = 'BOOKING'
			  AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id
			)
		  )
	`).Scan(&orphanRefundCount)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if orphanRefundCount > 0 {
		return nil, ErrOrphanRefundDetected
	}

	// Phase 1A supports full refunds only. Reversing the full original
	// commission for a partial/malformed refund would misstate the projection.
	var refundAmountMismatchCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM owner_finance_transactions t
		WHERE t.type = 'EXPENSE' AND t.source = 'REFUND'
		  AND `+canonicalLedgerBookingPredicate+`
		  AND NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions i
			WHERE i.booking_id = t.booking_id
			  AND i.type = 'INCOME' AND i.source = 'BOOKING'
			  AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id
			  AND i.amount = t.amount
		  )
	`).Scan(&refundAmountMismatchCount)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if refundAmountMismatchCount > 0 {
		return nil, ErrRefundAmountMismatch
	}

	filterSQL, filterArgs := buildFilters(ownerID, venueID, 3)

	res := &SummaryDataResult{AsOf: asOf}

	// 2. Gross GMV & Comm
	qGross := `
		SELECT 
			CAST(COALESCE(SUM(amount), 0) AS bigint),
			count(DISTINCT booking_id),
			CAST(COALESCE(SUM(CAST(ROUND(amount * 700.0 / 10000.0) AS bigint)), 0) AS bigint)
		FROM owner_finance_transactions t
		WHERE type = 'INCOME' AND source = 'BOOKING'
		  AND created_at >= $1 AND created_at < $2
		  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
		  AND ` + canonicalLedgerBookingPredicate + filterSQL
	argsGross := append([]any{utcStart, utcEndExclusive}, filterArgs...)
	err = tx.QueryRow(ctx, qGross, argsGross...).Scan(&res.Gross, &res.RealizedBookingCount, &res.ProjectedCommGross)
	if err != nil {
		return nil, mapRepositoryError(err)
	}

	// 3. Refund & Exact Reversal Comm
	qRefund := `
		SELECT 
			CAST(COALESCE(SUM(amount), 0) AS bigint),
			count(DISTINCT booking_id),
			CAST(COALESCE(SUM(
				COALESCE((
					SELECT CAST(ROUND(i.amount * 700.0 / 10000.0) AS bigint)
					FROM owner_finance_transactions i
					WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING'
					  AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount
					LIMIT 1
				), 0)
			), 0) AS bigint)
		FROM owner_finance_transactions t
		WHERE type = 'EXPENSE' AND source = 'REFUND' AND EXISTS (SELECT 1 FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount)
		  AND created_at >= $1 AND created_at < $2
		  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
		  AND ` + canonicalLedgerBookingPredicate + filterSQL
	err = tx.QueryRow(ctx, qRefund, argsGross...).Scan(&res.RefundPrincipal, &res.RefundedBookingCount, &res.ProjectedCommRefunded)
	if err != nil {
		return nil, mapRepositoryError(err)
	}

	// 4. Data Quality. Apply the same owner/venue scope as the summary.
	bookingFilterSQL, bookingFilterArgs := buildBookingFilters(ownerID, venueID, 3)
	dataQualityArgs := append([]any{utcStart, utcEndExclusive}, bookingFilterArgs...)
	err = tx.QueryRow(ctx, `
		SELECT 
			(SELECT count(*)
			 FROM bookings b
			 JOIN courts c ON c.id = b.court_id
			 JOIN venues v ON v.id = c.venue_id
			 JOIN owner_profiles op ON op.id = v.owner_profile_id
			 WHERE b.status IN ('PAID', 'COMPLETED')
			   AND b.created_at >= $1 AND b.created_at < $2
			   AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = b.id)
			   AND NOT EXISTS (
				 SELECT 1 FROM owner_finance_transactions oft
				 WHERE oft.booking_id = b.id AND oft.type = 'INCOME' AND oft.source = 'BOOKING'
				   AND oft.owner_id = op.user_id AND oft.venue_id = v.id
			   )`+bookingFilterSQL+`),
			(SELECT count(*)
			 FROM owner_finance_transactions t
			 WHERE t.type = 'INCOME' AND t.source = 'BOOKING'
			   AND t.created_at >= $1 AND t.created_at < $2
			   AND NOT (`+canonicalLedgerBookingPredicate+`)`+filterSQL+`)
	`, dataQualityArgs...).Scan(&res.PaidWithoutLedgerCount, &res.LedgerWithoutBookingCount)
	if err != nil {
		return nil, mapRepositoryError(err)
	}

	// 5. Buckets (Day granularity for SQL, Go will aggregate further if needed to Week/Month to ensure timezone safety)
	// We pull day buckets using timezone AT TIME ZONE 'Asia/Jakarta'
	qIncomeBucket := `
		SELECT 
			date_trunc('day', created_at AT TIME ZONE 'Asia/Jakarta') AS bucket,
			CAST(COALESCE(SUM(amount), 0) AS bigint),
			CAST(COALESCE(SUM(CAST(ROUND(amount * 700.0 / 10000.0) AS bigint)), 0) AS bigint)
		FROM owner_finance_transactions t
		WHERE type = 'INCOME' AND source = 'BOOKING'
		  AND created_at >= $1 AND created_at < $2
		  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
		  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
		GROUP BY bucket
	`
	rows, err := tx.Query(ctx, qIncomeBucket, argsGross...)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var b BucketResult
		if err := rows.Scan(&b.Bucket, &b.Amount, &b.Comm); err != nil {
			return nil, mapRepositoryError(err)
		}
		res.IncomeBuckets = append(res.IncomeBuckets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}

	qRefundBucket := `
		SELECT 
			date_trunc('day', created_at AT TIME ZONE 'Asia/Jakarta') AS bucket,
			CAST(COALESCE(SUM(amount), 0) AS bigint),
			CAST(COALESCE(SUM(COALESCE((SELECT CAST(ROUND(i.amount * 700.0 / 10000.0) AS bigint) FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount), 0)), 0) AS bigint)
		FROM owner_finance_transactions t
		WHERE type = 'EXPENSE' AND source = 'REFUND' AND EXISTS (SELECT 1 FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount)
		  AND created_at >= $1 AND created_at < $2
		  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
		  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
		GROUP BY bucket
	`
	rows2, err := tx.Query(ctx, qRefundBucket, argsGross...)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var b BucketResult
		if err := rows2.Scan(&b.Bucket, &b.Amount, &b.Comm); err != nil {
			return nil, mapRepositoryError(err)
		}
		res.RefundBuckets = append(res.RefundBuckets, b)
	}
	if err := rows2.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}

	// 6. Top 10 Owners
	qTopOwners := `
		WITH income_stats AS (
			SELECT owner_id, CAST(COALESCE(SUM(amount), 0) AS bigint) AS gross, count(DISTINCT booking_id) AS booking_cnt, CAST(COALESCE(SUM(CAST(ROUND(amount * 700.0 / 10000.0) AS bigint)), 0) AS bigint) AS comm
			FROM owner_finance_transactions t
			WHERE type = 'INCOME' AND source = 'BOOKING' AND created_at >= $1 AND created_at < $2
			  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
			  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
			GROUP BY owner_id
		),
		refund_stats AS (
			SELECT owner_id, CAST(COALESCE(SUM(amount), 0) AS bigint) AS refund, 
			  CAST(COALESCE(SUM(COALESCE((SELECT CAST(ROUND(i.amount * 700.0 / 10000.0) AS bigint) FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount), 0)), 0) AS bigint) AS refund_comm
			FROM owner_finance_transactions t
			WHERE type = 'EXPENSE' AND source = 'REFUND' AND EXISTS (SELECT 1 FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount) AND created_at >= $1 AND created_at < $2
			  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
			  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
			GROUP BY owner_id
		)
		SELECT op.id, op.business_name,
			COALESCE(i.gross, 0), COALESCE(r.refund, 0),
			COALESCE(i.gross, 0) - COALESCE(r.refund, 0) AS net,
			COALESCE(i.booking_cnt, 0),
			COALESCE(i.comm, 0) - COALESCE(r.refund_comm, 0)
		FROM income_stats i
		FULL OUTER JOIN refund_stats r ON i.owner_id = r.owner_id
		JOIN owner_profiles op ON op.user_id = COALESCE(i.owner_id, r.owner_id)
		ORDER BY net DESC, op.id ASC
		LIMIT 10
	`
	rows3, err := tx.Query(ctx, qTopOwners, argsGross...)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows3.Close()
	for rows3.Next() {
		var row BreakdownRow
		if err := rows3.Scan(&row.ID, &row.Name, &row.Gross, &row.Refund, &row.Net, &row.BookingCount, &row.NetComm); err != nil {
			return nil, mapRepositoryError(err)
		}
		res.TopOwners = append(res.TopOwners, row)
	}
	if err := rows3.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}

	// 7. Top 10 Venues
	qTopVenues := `
		WITH income_stats AS (
			SELECT venue_id, CAST(COALESCE(SUM(amount), 0) AS bigint) AS gross, count(DISTINCT booking_id) AS booking_cnt, CAST(COALESCE(SUM(CAST(ROUND(amount * 700.0 / 10000.0) AS bigint)), 0) AS bigint) AS comm
			FROM owner_finance_transactions t
			WHERE type = 'INCOME' AND source = 'BOOKING' AND created_at >= $1 AND created_at < $2
			  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
			  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
			GROUP BY venue_id
		),
		refund_stats AS (
			SELECT venue_id, CAST(COALESCE(SUM(amount), 0) AS bigint) AS refund, 
			  CAST(COALESCE(SUM(COALESCE((SELECT CAST(ROUND(i.amount * 700.0 / 10000.0) AS bigint) FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount), 0)), 0) AS bigint) AS refund_comm
			FROM owner_finance_transactions t
			WHERE type = 'EXPENSE' AND source = 'REFUND' AND EXISTS (SELECT 1 FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount) AND created_at >= $1 AND created_at < $2
			  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
			  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
			GROUP BY venue_id
		)
		SELECT 
			COALESCE(i.venue_id, r.venue_id), v.name, v.owner_profile_id,
			COALESCE(i.gross, 0), COALESCE(r.refund, 0),
			COALESCE(i.gross, 0) - COALESCE(r.refund, 0) AS net,
			COALESCE(i.booking_cnt, 0),
			COALESCE(i.comm, 0) - COALESCE(r.refund_comm, 0)
		FROM income_stats i
		FULL OUTER JOIN refund_stats r ON i.venue_id = r.venue_id
		JOIN venues v ON v.id = COALESCE(i.venue_id, r.venue_id)
		ORDER BY net DESC, COALESCE(i.venue_id, r.venue_id) ASC
		LIMIT 10
	`
	rows4, err := tx.Query(ctx, qTopVenues, argsGross...)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows4.Close()
	for rows4.Next() {
		var row BreakdownRow
		if err := rows4.Scan(&row.ID, &row.Name, &row.OwnerProfileID, &row.Gross, &row.Refund, &row.Net, &row.BookingCount, &row.NetComm); err != nil {
			return nil, mapRepositoryError(err)
		}
		res.TopVenues = append(res.TopVenues, row)
	}
	if err := rows4.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}

	return res, nil
}

func (r *repository) GetPaginatedBreakdown(ctx context.Context, utcStart, utcEndExclusive time.Time, ownerID, venueID, dimension string, page, limit int) (*BreakdownResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var asOf time.Time
	if err := tx.QueryRow(ctx, "SELECT CURRENT_TIMESTAMP").Scan(&asOf); err != nil {
		return nil, err
	}

	// 1. Duplicate check (Fail closed)
	var dupCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT booking_id 
			FROM owner_finance_transactions 
			WHERE type = 'INCOME' AND source = 'BOOKING' 
			GROUP BY booking_id 
			HAVING count(*) > 1
		) d
	`).Scan(&dupCount)
	if err != nil {
		return nil, err
	}
	if dupCount > 0 {
		return nil, ErrDuplicateLedgerDetected
	}

	// 1a. Fractional ledger check (Fail closed)
	var fracCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*) 
		FROM owner_finance_transactions 
		WHERE amount != TRUNC(amount)
	`).Scan(&fracCount)
	if err != nil {
		return nil, err
	}
	if fracCount > 0 {
		return nil, ErrFractionalLedgerDetected
	}

	var orphanRefundCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM owner_finance_transactions t
		WHERE t.type = 'EXPENSE' AND t.source = 'REFUND'
		  AND (
			t.booking_id IS NULL
			OR NOT (`+canonicalLedgerBookingPredicate+`)
			OR NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions i
			WHERE i.booking_id = t.booking_id
			  AND i.type = 'INCOME' AND i.source = 'BOOKING'
			  AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id
			)
		  )
	`).Scan(&orphanRefundCount)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if orphanRefundCount > 0 {
		return nil, ErrOrphanRefundDetected
	}

	var refundAmountMismatchCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM owner_finance_transactions t
		WHERE t.type = 'EXPENSE' AND t.source = 'REFUND'
		  AND `+canonicalLedgerBookingPredicate+`
		  AND NOT EXISTS (
			SELECT 1 FROM owner_finance_transactions i
			WHERE i.booking_id = t.booking_id
			  AND i.type = 'INCOME' AND i.source = 'BOOKING'
			  AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id
			  AND i.amount = t.amount
		  )
	`).Scan(&refundAmountMismatchCount)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if refundAmountMismatchCount > 0 {
		return nil, ErrRefundAmountMismatch
	}

	filterSQL, filterArgs := buildFilters(ownerID, venueID, 3)

	dimCol := "owner_id"
	joinTable := "owner_profiles"
	nameCol := "business_name"
	joinCol := "user_id"
	if dimension == "venue" {
		dimCol = "venue_id"
		joinTable = "venues"
		nameCol = "name"
		joinCol = "id"
	}

	qStats := `
		WITH income_stats AS (
			SELECT ` + dimCol + ` AS dim_id, CAST(COALESCE(SUM(amount), 0) AS bigint) AS gross, count(DISTINCT booking_id) AS booking_cnt, CAST(COALESCE(SUM(CAST(ROUND(amount * 700.0 / 10000.0) AS bigint)), 0) AS bigint) AS comm
			FROM owner_finance_transactions t
			WHERE type = 'INCOME' AND source = 'BOOKING' AND created_at >= $1 AND created_at < $2
			  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
			  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
			GROUP BY ` + dimCol + `
		),
		refund_stats AS (
			SELECT ` + dimCol + ` AS dim_id, CAST(COALESCE(SUM(amount), 0) AS bigint) AS refund, 
			  CAST(COALESCE(SUM(COALESCE((SELECT CAST(ROUND(i.amount * 700.0 / 10000.0) AS bigint) FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount), 0)), 0) AS bigint) AS refund_comm
			FROM owner_finance_transactions t
			WHERE type = 'EXPENSE' AND source = 'REFUND' AND EXISTS (SELECT 1 FROM owner_finance_transactions i WHERE i.booking_id = t.booking_id AND i.type = 'INCOME' AND i.source = 'BOOKING' AND i.owner_id = t.owner_id AND i.venue_id = t.venue_id AND i.amount = t.amount) AND created_at >= $1 AND created_at < $2
			  AND NOT EXISTS (SELECT 1 FROM offline_booking_customers obc WHERE obc.booking_id = t.booking_id)
			  AND ` + canonicalLedgerBookingPredicate + filterSQL + `
			GROUP BY ` + dimCol + `
		),
		combined AS (
			SELECT
				COALESCE(i.dim_id, r.dim_id) AS dim_id,
				COALESCE(i.gross, 0) AS gross, COALESCE(r.refund, 0) AS refund,
				COALESCE(i.gross, 0) - COALESCE(r.refund, 0) AS net,
				COALESCE(i.booking_cnt, 0) AS booking_cnt,
				COALESCE(i.comm, 0) - COALESCE(r.refund_comm, 0) AS net_comm
			FROM income_stats i
			FULL OUTER JOIN refund_stats r ON i.dim_id = r.dim_id
		)
	`

	args := append([]any{utcStart, utcEndExclusive}, filterArgs...)

	var totalItems int
	err = tx.QueryRow(ctx, qStats+` SELECT count(*) FROM combined JOIN `+joinTable+` j ON j.`+joinCol+` = combined.dim_id`, args...).Scan(&totalItems)
	if err != nil {
		return nil, mapRepositoryError(err)
	}

	offset := (page - 1) * limit

	ownerProfileSelect := "''"
	if dimension == "venue" {
		ownerProfileSelect = "j.owner_profile_id"
	}

	qRows := qStats + `
		SELECT j.id, j.` + nameCol + `, ` + ownerProfileSelect + `, combined.gross, combined.refund, combined.net, combined.booking_cnt, combined.net_comm
		FROM combined
		JOIN ` + joinTable + ` j ON j.` + joinCol + ` = combined.dim_id
		ORDER BY net DESC, j.id ASC
		LIMIT $` + fmt.Sprintf("%d", len(args)+1) + ` OFFSET $` + fmt.Sprintf("%d", len(args)+2)

	argsRows := append(args, limit, offset)

	res := &BreakdownResult{AsOf: asOf, TotalItems: totalItems}
	rows, err := tx.Query(ctx, qRows, argsRows...)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var row BreakdownRow
		if err := rows.Scan(&row.ID, &row.Name, &row.OwnerProfileID, &row.Gross, &row.Refund, &row.Net, &row.BookingCount, &row.NetComm); err != nil {
			return nil, mapRepositoryError(err)
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, mapRepositoryError(err)
	}

	return res, nil
}
