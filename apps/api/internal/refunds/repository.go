package refunds

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lapangango-api/internal/httputil"
)

type BookingForRefund struct {
	ID         string
	CustomerID string
	OwnerID    string
	VenueID    *string
	Status     string
	Date       time.Time
	StartTime  time.Time
	TotalPrice float64
}

type Repository interface {
	FindBookingForRefundRequest(ctx context.Context, bookingID string) (BookingForRefund, error)
	CreateRefundRequest(ctx context.Context, req RefundRequestResponse) (RefundRequestResponse, error)
	GetActiveRefundRequestByBookingID(ctx context.Context, bookingID string) (*RefundRequestResponse, error)
	GetLatestRefundRequestByBookingID(ctx context.Context, bookingID string) (*RefundRequestResponse, error)
	GetRefundRequestByID(ctx context.Context, id string) (RefundRequestResponse, error)
	ListOwnerRefundRequests(ctx context.Context, ownerCtx httputil.OwnerContext, status string, venueID string, page, limit int) ([]OwnerRefundRequestListItem, int, error)

	// Transactional methods
	BeginTx(ctx context.Context) (pgx.Tx, error)
	LockRefundRequest(ctx context.Context, tx pgx.Tx, id string) (RefundRequestResponse, error)
	LockBooking(ctx context.Context, tx pgx.Tx, bookingID string) (BookingForRefund, error)
	HasBookingIncomeLedger(ctx context.Context, tx pgx.Tx, bookingID string) (bool, error)
	HasRefundLedger(ctx context.Context, tx pgx.Tx, bookingID string) (bool, error)
	UpdateBookingStatus(ctx context.Context, tx pgx.Tx, bookingID, status string) error
	InsertRefundLedger(ctx context.Context, tx pgx.Tx, ownerID, venueID, bookingID, ownerUserID string, amount float64, description string) error
	UpdateRefundRequest(ctx context.Context, tx pgx.Tx, id, status, ownerNote, reviewedBy string) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.Begin(ctx)
}

func (r *repository) FindBookingForRefundRequest(ctx context.Context, bookingID string) (BookingForRefund, error) {
	var b BookingForRefund
	query := `
		SELECT b.id, b.customer_id, op.user_id, c.venue_id, b.status, b.booking_date, b.start_time, b.total_price
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON c.venue_id = v.id
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		WHERE b.id = $1
	`
	err := r.db.QueryRow(ctx, query, bookingID).Scan(
		&b.ID, &b.CustomerID, &b.OwnerID, &b.VenueID, &b.Status, &b.Date, &b.StartTime, &b.TotalPrice,
	)
	if err != nil {
		return b, err
	}
	return b, nil
}

func (r *repository) CreateRefundRequest(ctx context.Context, req RefundRequestResponse) (RefundRequestResponse, error) {
	query := `
		INSERT INTO booking_refund_requests (booking_id, customer_id, owner_id, venue_id, reason, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, requested_at, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query, req.BookingID, req.CustomerID, req.OwnerID, req.VenueID, req.Reason, req.Status).Scan(
		&req.ID, &req.RequestedAt, &req.CreatedAt, &req.UpdatedAt,
	)
	return req, err
}

func (r *repository) GetActiveRefundRequestByBookingID(ctx context.Context, bookingID string) (*RefundRequestResponse, error) {
	var req RefundRequestResponse
	query := `
		SELECT id, booking_id, customer_id, owner_id, venue_id, reason, status, owner_note, requested_at, reviewed_at, reviewed_by_user_id, created_at, updated_at
		FROM booking_refund_requests
		WHERE booking_id = $1 AND status = 'PENDING'
	`
	err := r.db.QueryRow(ctx, query, bookingID).Scan(
		&req.ID, &req.BookingID, &req.CustomerID, &req.OwnerID, &req.VenueID, &req.Reason, &req.Status,
		&req.OwnerNote, &req.RequestedAt, &req.ReviewedAt, &req.ReviewedByUserID, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

func (r *repository) GetLatestRefundRequestByBookingID(ctx context.Context, bookingID string) (*RefundRequestResponse, error) {
	var req RefundRequestResponse
	query := `
		SELECT id, booking_id, customer_id, owner_id, venue_id, reason, status, owner_note, requested_at, reviewed_at, reviewed_by_user_id, created_at, updated_at
		FROM booking_refund_requests
		WHERE booking_id = $1
		ORDER BY requested_at DESC
		LIMIT 1
	`
	err := r.db.QueryRow(ctx, query, bookingID).Scan(
		&req.ID, &req.BookingID, &req.CustomerID, &req.OwnerID, &req.VenueID, &req.Reason, &req.Status,
		&req.OwnerNote, &req.RequestedAt, &req.ReviewedAt, &req.ReviewedByUserID, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

func (r *repository) GetRefundRequestByID(ctx context.Context, id string) (RefundRequestResponse, error) {
	var req RefundRequestResponse
	query := `
		SELECT id, booking_id, customer_id, owner_id, venue_id, reason, status, owner_note, requested_at, reviewed_at, reviewed_by_user_id, created_at, updated_at
		FROM booking_refund_requests
		WHERE id = $1
	`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&req.ID, &req.BookingID, &req.CustomerID, &req.OwnerID, &req.VenueID, &req.Reason, &req.Status,
		&req.OwnerNote, &req.RequestedAt, &req.ReviewedAt, &req.ReviewedByUserID, &req.CreatedAt, &req.UpdatedAt,
	)
	return req, err
}

func (r *repository) ListOwnerRefundRequests(ctx context.Context, ownerCtx httputil.OwnerContext, status string, venueID string, page, limit int) ([]OwnerRefundRequestListItem, int, error) {
	offset := (page - 1) * limit

	var args []interface{}
	args = append(args, ownerCtx.EffectiveOwnerUserID, status, venueID)
	argIdx := 4

	venueFilter := ""
	if len(ownerCtx.AllowedVenueIDs) > 0 {
		venueFilter = fmt.Sprintf(" AND br.venue_id = ANY($%d::uuid[])", argIdx)
		args = append(args, ownerCtx.AllowedVenueIDs)
		argIdx++
	}

	countQuery := `
		SELECT COUNT(*)
		FROM booking_refund_requests br
		WHERE br.owner_id = $1
		AND ($2 = '' OR br.status = $2)
		AND ($3 = '' OR br.venue_id::text = $3)
	` + venueFilter

	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT 
			br.id, br.booking_id, 
			u.name as customer_name, u.email as customer_email,
			v.name as venue_name, c.name as court_name,
			b.booking_date::text, b.start_time::text, b.end_time::text,
			b.total_price as amount,
			br.reason, br.status, br.requested_at
		FROM booking_refund_requests br
		JOIN users u ON u.id = br.customer_id
		JOIN bookings b ON b.id = br.booking_id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = br.venue_id
		WHERE br.owner_id = $1
		AND ($2 = '' OR br.status = $2)
		AND ($3 = '' OR br.venue_id::text = $3)
		` + venueFilter + fmt.Sprintf(" ORDER BY br.requested_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []OwnerRefundRequestListItem
	for rows.Next() {
		var item OwnerRefundRequestListItem
		err := rows.Scan(
			&item.ID, &item.BookingID,
			&item.CustomerName, &item.CustomerEmail,
			&item.VenueName, &item.CourtName,
			&item.BookingDate, &item.StartTime, &item.EndTime,
			&item.Amount,
			&item.Reason, &item.Status, &item.RequestedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	return items, total, nil
}

func (r *repository) LockRefundRequest(ctx context.Context, tx pgx.Tx, id string) (RefundRequestResponse, error) {
	var req RefundRequestResponse
	query := `
		SELECT id, booking_id, customer_id, owner_id, venue_id, reason, status, owner_note, requested_at, reviewed_at, reviewed_by_user_id, created_at, updated_at
		FROM booking_refund_requests
		WHERE id = $1
		FOR UPDATE
	`
	err := tx.QueryRow(ctx, query, id).Scan(
		&req.ID, &req.BookingID, &req.CustomerID, &req.OwnerID, &req.VenueID, &req.Reason, &req.Status,
		&req.OwnerNote, &req.RequestedAt, &req.ReviewedAt, &req.ReviewedByUserID, &req.CreatedAt, &req.UpdatedAt,
	)
	return req, err
}

func (r *repository) LockBooking(ctx context.Context, tx pgx.Tx, bookingID string) (BookingForRefund, error) {
	var b BookingForRefund
	query := `
		SELECT b.id, b.customer_id, op.user_id, c.venue_id, b.status, b.booking_date, b.start_time, b.total_price
		FROM bookings b
		JOIN courts c ON b.court_id = c.id
		JOIN venues v ON c.venue_id = v.id
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		WHERE b.id = $1 FOR UPDATE
	`
	err := tx.QueryRow(ctx, query, bookingID).Scan(
		&b.ID, &b.CustomerID, &b.OwnerID, &b.VenueID, &b.Status, &b.Date, &b.StartTime, &b.TotalPrice,
	)
	return b, err
}

func (r *repository) HasBookingIncomeLedger(ctx context.Context, tx pgx.Tx, bookingID string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM owner_finance_transactions 
			WHERE booking_id = $1 AND type = 'INCOME' AND source = 'BOOKING'
		)
	`
	err := tx.QueryRow(ctx, query, bookingID).Scan(&exists)
	return exists, err
}

func (r *repository) HasRefundLedger(ctx context.Context, tx pgx.Tx, bookingID string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM owner_finance_transactions 
			WHERE booking_id = $1 AND type = 'EXPENSE' AND source = 'REFUND'
		)
	`
	err := tx.QueryRow(ctx, query, bookingID).Scan(&exists)
	return exists, err
}

func (r *repository) UpdateBookingStatus(ctx context.Context, tx pgx.Tx, bookingID, status string) error {
	query := `UPDATE bookings SET status = $1, updated_at = now() WHERE id = $2`
	_, err := tx.Exec(ctx, query, status, bookingID)
	return err
}

func (r *repository) InsertRefundLedger(ctx context.Context, tx pgx.Tx, ownerID, venueID, bookingID, ownerUserID string, amount float64, description string) error {
	query := `
		INSERT INTO owner_finance_transactions
		  (owner_id, venue_id, booking_id, created_by_user_id, type, source, category, amount, transaction_date, description)
		VALUES
		  ($1, $2, $3, $4, 'EXPENSE', 'REFUND', 'BOOKING_REFUND', $5, CURRENT_DATE, $6)
	`
	var vID *string
	if venueID != "" {
		vID = &venueID
	}
	_, err := tx.Exec(ctx, query, ownerID, vID, bookingID, ownerUserID, amount, description)
	return err
}

func (r *repository) UpdateRefundRequest(ctx context.Context, tx pgx.Tx, id, status, ownerNote, reviewedBy string) error {
	query := `
		UPDATE booking_refund_requests
		SET status = $1, owner_note = $2, reviewed_by_user_id = $3, reviewed_at = now()
		WHERE id = $4
	`
	var note *string
	if ownerNote != "" {
		note = &ownerNote
	}
	_, err := tx.Exec(ctx, query, status, note, reviewedBy, id)
	return err
}
