package bookings

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

type CourtValidationInfo struct {
	PricePerHour float64
	CourtStatus  string
	VenueStatus  string
}

type OperatingHour struct {
	OpenTime  *time.Time
	CloseTime *time.Time
	IsClosed  bool
}

type CreateBookingParams struct {
	CustomerID string
	CourtID    string
	Date       string
	StartTime  string
	EndTime    string
	TotalPrice float64
}

type Booking struct {
	ID         string
	CustomerID string
	CourtID    string
	Date       time.Time
	StartTime  time.Time
	EndTime    time.Time
	TotalPrice float64
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type OwnerProfile struct {
	ID     string
	UserID string
}

type OwnerVenue struct {
	ID   string
	Name string
}

type OwnerBooking struct {
	ID            string
	CustomerID    string
	CustomerName  string
	CustomerEmail string
	CustomerPhone *string
	VenueID       string
	VenueName     string
	CourtID       string
	CourtName     string
	Date          time.Time
	StartTime     time.Time
	EndTime       time.Time
	TotalPrice    float64
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

var ErrCourtNotFound = errors.New("court not found")

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) LockCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID string) (CourtValidationInfo, error) {
	query := `
		SELECT c.price_per_hour, c.status, v.status
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		WHERE c.id = $1
		FOR UPDATE
	`
	var info CourtValidationInfo
	err := tx.QueryRow(ctx, query, courtID).Scan(&info.PricePerHour, &info.CourtStatus, &info.VenueStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return info, ErrCourtNotFound
		}
		return info, err
	}
	return info, nil
}

func (r *Repository) FindOperatingHours(ctx context.Context, tx pgx.Tx, courtID string, dayOfWeek int) (OperatingHour, error) {
	query := `
		SELECT open_time, close_time, is_closed
		FROM court_operating_hours
		WHERE court_id = $1 AND day_of_week = $2
	`
	var oh OperatingHour
	err := tx.QueryRow(ctx, query, courtID, dayOfWeek).Scan(&oh.OpenTime, &oh.CloseTime, &oh.IsClosed)
	if err != nil {
		if err == pgx.ErrNoRows {
			// If not explicitly set, consider it closed or handle gracefully
			return OperatingHour{IsClosed: true}, nil
		}
		return oh, err
	}
	return oh, nil
}

func (r *Repository) ListByCustomerID(ctx context.Context, customerID string) ([]Booking, error) {
	query := `
		SELECT id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, created_at, updated_at
		FROM bookings
		WHERE customer_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.TotalPrice, &b.Status, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	return bookings, rows.Err()
}

func (r *Repository) FindByIDAndCustomerID(ctx context.Context, id, customerID string) (Booking, error) {
	query := `
		SELECT id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, created_at, updated_at
		FROM bookings
		WHERE id = $1 AND customer_id = $2
	`
	var b Booking
	err := r.db.QueryRow(ctx, query, id, customerID).Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.TotalPrice, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}
	return b, nil
}

func (r *Repository) FindOwnerProfileByUserID(ctx context.Context, userID string) (OwnerProfile, error) {
	query := `
		SELECT id::text, user_id::text
		FROM owner_profiles
		WHERE user_id = $1
		LIMIT 1
	`
	var profile OwnerProfile
	err := r.db.QueryRow(ctx, query, userID).Scan(&profile.ID, &profile.UserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return profile, pgx.ErrNoRows
		}
		return profile, err
	}
	return profile, nil
}

func (r *Repository) FindVenueByIDAndOwnerProfileID(ctx context.Context, venueID, ownerProfileID string) (OwnerVenue, error) {
	query := `
		SELECT id::text, name
		FROM venues
		WHERE id = $1 AND owner_profile_id = $2
		LIMIT 1
	`
	var venue OwnerVenue
	err := r.db.QueryRow(ctx, query, venueID, ownerProfileID).Scan(&venue.ID, &venue.Name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return venue, pgx.ErrNoRows
		}
		return venue, err
	}
	return venue, nil
}

func (r *Repository) ListOwnerVenueBookings(ctx context.Context, ownerProfileID, venueID, date, status string, limit, offset int) ([]OwnerBooking, error) {
	query := `
		SELECT
			b.id::text,
			u.id::text,
			u.name,
			u.email,
			u.phone,
			v.id::text,
			v.name,
			c.id::text,
			c.name,
			b.booking_date,
			b.start_time,
			b.end_time,
			b.total_price,
			b.status,
			b.created_at,
			b.updated_at
		FROM bookings b
		JOIN users u ON u.id = b.customer_id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		WHERE v.owner_profile_id = $1
			AND v.id = $2
			AND b.booking_date = $3
			AND ($4 = '' OR b.status = $4)
		ORDER BY b.start_time ASC, b.created_at ASC
		LIMIT $5 OFFSET $6
	`
	rows, err := r.db.Query(ctx, query, ownerProfileID, venueID, date, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []OwnerBooking
	for rows.Next() {
		var booking OwnerBooking
		if err := rows.Scan(
			&booking.ID,
			&booking.CustomerID,
			&booking.CustomerName,
			&booking.CustomerEmail,
			&booking.CustomerPhone,
			&booking.VenueID,
			&booking.VenueName,
			&booking.CourtID,
			&booking.CourtName,
			&booking.Date,
			&booking.StartTime,
			&booking.EndTime,
			&booking.TotalPrice,
			&booking.Status,
			&booking.CreatedAt,
			&booking.UpdatedAt,
		); err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	return bookings, rows.Err()
}

func (r *Repository) ExecuteBookingTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) CheckBlockedSlots(ctx context.Context, tx pgx.Tx, courtID string, startTz, endTz time.Time) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM court_blocked_slots
		WHERE court_id = $1
		  AND start_at < $3
		  AND end_at > $2
	`
	var count int
	err := tx.QueryRow(ctx, query, courtID, startTz, endTz).Scan(&count)
	return count > 0, err
}

func (r *Repository) CheckExistingBookings(ctx context.Context, tx pgx.Tx, courtID, date, startTime, endTime string) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM bookings
		WHERE court_id = $1
		  AND booking_date = $2
		  AND start_time < $4
		  AND end_time > $3
		  AND status != 'CANCELLED'
	`
	var count int
	err := tx.QueryRow(ctx, query, courtID, date, startTime, endTime).Scan(&count)
	return count > 0, err
}

func (r *Repository) InsertBooking(ctx context.Context, tx pgx.Tx, params CreateBookingParams) (Booking, error) {
	query := `
		INSERT INTO bookings (customer_id, court_id, booking_date, start_time, end_time, total_price, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING_PAYMENT')
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, created_at, updated_at
	`
	var b Booking
	err := tx.QueryRow(
		ctx,
		query,
		params.CustomerID,
		params.CourtID,
		params.Date,
		params.StartTime,
		params.EndTime,
		params.TotalPrice,
	).Scan(
		&b.ID,
		&b.CustomerID,
		&b.CourtID,
		&b.Date,
		&b.StartTime,
		&b.EndTime,
		&b.TotalPrice,
		&b.Status,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	return b, err
}

func (r *Repository) CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error) {
	query := `
		UPDATE bookings
		SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1 AND customer_id = $2 AND status = 'PENDING_PAYMENT'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, created_at, updated_at
	`
	var b Booking
	err := r.db.QueryRow(ctx, query, bookingID, customerID).Scan(
		&b.ID,
		&b.CustomerID,
		&b.CourtID,
		&b.Date,
		&b.StartTime,
		&b.EndTime,
		&b.TotalPrice,
		&b.Status,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}
	return b, nil
}

func (r *Repository) ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error) {
	query := `
		UPDATE bookings
		SET status = 'CONFIRMED',
		    updated_at = now()
		WHERE id = $1
		  AND customer_id = $2
		  AND status = 'PENDING_PAYMENT'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, created_at, updated_at
	`
	var b Booking
	err := r.db.QueryRow(ctx, query, bookingID, customerID).Scan(
		&b.ID,
		&b.CustomerID,
		&b.CourtID,
		&b.Date,
		&b.StartTime,
		&b.EndTime,
		&b.TotalPrice,
		&b.Status,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}
	return b, nil
}
