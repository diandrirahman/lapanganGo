package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetBookingsTrend(ctx context.Context, params AnalyticsParams) ([]BookingTrendItem, error)
	GetRevenueTrend(ctx context.Context, params AnalyticsParams) ([]RevenueTrendItem, error)
	GetRevenueByVenue(ctx context.Context, params AnalyticsParams) ([]RevenueVenueItem, error)
	GetStatusBreakdown(ctx context.Context, params AnalyticsParams) ([]StatusBreakdownItem, error)
	GetExpenseByCategory(ctx context.Context, params AnalyticsParams) ([]ExpenseCategoryItem, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) buildDateFilter(startDate *time.Time, endDate *time.Time, dateColumn string) (string, []interface{}) {
	var filter string
	var args []interface{}

	if startDate != nil && endDate != nil {
		filter = fmt.Sprintf(" AND %s >= $2 AND %s <= $3", dateColumn, dateColumn)
		args = append(args, *startDate, *endDate)
	} else if startDate != nil {
		filter = fmt.Sprintf(" AND %s >= $2", dateColumn)
		args = append(args, *startDate)
	} else if endDate != nil {
		filter = fmt.Sprintf(" AND %s <= $2", dateColumn)
		args = append(args, *endDate)
	}
	return filter, args
}

func (r *repository) buildVenueFilter(params AnalyticsParams, venueColumn string, argIdx int) (string, []interface{}) {
	var filter string
	var args []interface{}

	if params.VenueID != nil {
		filter = fmt.Sprintf(" AND %s = $%d", venueColumn, argIdx)
		args = append(args, *params.VenueID)
	} else if len(params.AllowedVenueIDs) > 0 {
		filter = fmt.Sprintf(" AND %s = ANY($%d::uuid[])", venueColumn, argIdx)
		args = append(args, params.AllowedVenueIDs)
	}
	return filter, args
}

func (r *repository) GetBookingsTrend(ctx context.Context, params AnalyticsParams) ([]BookingTrendItem, error) {
	dateFilter, dateArgs := r.buildDateFilter(params.StartDate, params.EndDate, "b.booking_date")
	venueFilter, venueArgs := r.buildVenueFilter(params, "v.id", len(dateArgs)+2)
	dateArgs = append(dateArgs, venueArgs...)

	query := `
		SELECT 
			to_char(b.booking_date, 'YYYY-MM-DD') as date,
			COUNT(b.id) as booking_count
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		WHERE op.user_id = $1 ` + dateFilter + venueFilter + `
		AND b.status IN ('WAITING_VERIFICATION', 'CONFIRMED', 'PAID', 'COMPLETED')
		GROUP BY date
		ORDER BY date ASC
	`

	args := append([]interface{}{params.OwnerID}, dateArgs...)

	var items []BookingTrendItem
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item BookingTrendItem
		if err := rows.Scan(&item.Date, &item.BookingCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		return []BookingTrendItem{}, nil
	}
	return items, nil
}

func (r *repository) GetRevenueTrend(ctx context.Context, params AnalyticsParams) ([]RevenueTrendItem, error) {
	dateFilter, dateArgs := r.buildDateFilter(params.StartDate, params.EndDate, "t.transaction_date")
	venueFilter, venueArgs := r.buildVenueFilter(params, "t.venue_id", len(dateArgs)+2)
	dateArgs = append(dateArgs, venueArgs...)

	query := `
		SELECT 
			to_char(t.transaction_date, 'YYYY-MM-DD') as date,
			COALESCE(SUM(t.amount), 0) as revenue
		FROM owner_finance_transactions t
		WHERE t.owner_id = $1
		  AND t.type = 'INCOME'
		  AND t.source = 'BOOKING' ` + dateFilter + venueFilter + `
		GROUP BY date
		ORDER BY date ASC
	`

	args := append([]interface{}{params.OwnerID}, dateArgs...)

	var items []RevenueTrendItem
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item RevenueTrendItem
		if err := rows.Scan(&item.Date, &item.Revenue); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		return []RevenueTrendItem{}, nil
	}
	return items, nil
}

func (r *repository) GetRevenueByVenue(ctx context.Context, params AnalyticsParams) ([]RevenueVenueItem, error) {
	dateFilter, dateArgs := r.buildDateFilter(params.StartDate, params.EndDate, "t.transaction_date")
	venueFilter, venueArgs := r.buildVenueFilter(params, "t.venue_id", len(dateArgs)+2)
	dateArgs = append(dateArgs, venueArgs...)

	query := `
		SELECT 
			v.id as venue_id,
			v.name as venue_name,
			COALESCE(SUM(t.amount), 0) as revenue
		FROM venues v
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		LEFT JOIN owner_finance_transactions t ON t.venue_id = v.id
			AND t.owner_id = op.user_id
			AND t.type = 'INCOME'
			AND t.source = 'BOOKING' ` + dateFilter + `
		WHERE op.user_id = $1 ` + venueFilter + `
		GROUP BY v.id, v.name
		ORDER BY revenue DESC
	`

	args := append([]interface{}{params.OwnerID}, dateArgs...)

	var items []RevenueVenueItem
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item RevenueVenueItem
		if err := rows.Scan(&item.VenueID, &item.VenueName, &item.Revenue); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		return []RevenueVenueItem{}, nil
	}
	return items, nil
}

func (r *repository) GetStatusBreakdown(ctx context.Context, params AnalyticsParams) ([]StatusBreakdownItem, error) {
	dateFilter, dateArgs := r.buildDateFilter(params.StartDate, params.EndDate, "b.created_at")
	venueFilter, venueArgs := r.buildVenueFilter(params, "v.id", len(dateArgs)+2)
	dateArgs = append(dateArgs, venueArgs...)

	query := `
		SELECT 
			b.status,
			COUNT(b.id) as booking_count,
			COALESCE(SUM(b.total_price), 0) as amount
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		WHERE op.user_id = $1 ` + dateFilter + venueFilter + `
		GROUP BY b.status
		ORDER BY booking_count DESC
	`

	args := append([]interface{}{params.OwnerID}, dateArgs...)

	var items []StatusBreakdownItem
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item StatusBreakdownItem
		if err := rows.Scan(&item.Status, &item.BookingCount, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		return []StatusBreakdownItem{}, nil
	}
	return items, nil
}

func (r *repository) GetExpenseByCategory(ctx context.Context, params AnalyticsParams) ([]ExpenseCategoryItem, error) {
	dateFilter, dateArgs := r.buildDateFilter(params.StartDate, params.EndDate, "transaction_date")
	venueFilter, venueArgs := r.buildVenueFilter(params, "venue_id", len(dateArgs)+2)
	dateArgs = append(dateArgs, venueArgs...)

	query := `
		SELECT 
			category,
			SUM(amount) as amount
		FROM owner_finance_transactions
		WHERE owner_id = $1 AND type = 'EXPENSE' ` + dateFilter + venueFilter + `
		GROUP BY category
		ORDER BY amount DESC
	`

	args := append([]interface{}{params.OwnerID}, dateArgs...)

	var items []ExpenseCategoryItem
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item ExpenseCategoryItem
		if err := rows.Scan(&item.Category, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		return []ExpenseCategoryItem{}, nil
	}
	return items, nil
}
