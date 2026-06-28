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
	ExpiresAt  *time.Time
}

type Booking struct {
	ID               string
	CustomerID       string
	CourtID          string
	Date             time.Time
	StartTime        time.Time
	EndTime          time.Time
	TotalPrice       float64
	Status           string
	PaymentReference *string
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CustomerBooking struct {
	Booking
	VenueID        string
	VenueName      string
	VenueAddress   string
	VenueCity      string
	CourtName      string
	CourtSportName string
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
	ID               string
	CustomerID       string
	CustomerName     string
	CustomerEmail    string
	CustomerPhone    *string
	VenueID          string
	VenueName        string
	CourtID          string
	CourtName        string
	Date             time.Time
	StartTime        time.Time
	EndTime          time.Time
	TotalPrice       float64
	Status           string
	PaymentReference *string
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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

func (r *Repository) ListByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]CustomerBooking, int, error) {
	// Count total
	countQuery := `
		SELECT count(*)
		FROM bookings b
		WHERE b.customer_id = $1
	`
	var total int
	if err := r.db.QueryRow(ctx, countQuery, customerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT b.id::text, b.customer_id::text, b.court_id::text, b.booking_date, b.start_time, b.end_time, b.total_price, b.status, b.payment_reference, b.expires_at, b.created_at, b.updated_at,
		       v.id::text, v.name, v.address, v.city, c.name, s.name
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE b.customer_id = $1
		ORDER BY b.created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Query(ctx, query, customerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var bookings []CustomerBooking
	for rows.Next() {
		var b CustomerBooking
		if err := rows.Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt, &b.VenueID, &b.VenueName, &b.VenueAddress, &b.VenueCity, &b.CourtName, &b.CourtSportName); err != nil {
			return nil, 0, err
		}
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

func (r *Repository) FindCustomerBookingByID(ctx context.Context, id, customerID string) (CustomerBooking, error) {
	query := `
		SELECT b.id::text, b.customer_id::text, b.court_id::text, b.booking_date, b.start_time, b.end_time, b.total_price, b.status, b.payment_reference, b.expires_at, b.created_at, b.updated_at,
		       v.id::text, v.name, v.address, v.city, c.name, s.name
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE b.id = $1 AND b.customer_id = $2
	`
	var b CustomerBooking
	err := r.db.QueryRow(ctx, query, id, customerID).Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt, &b.VenueID, &b.VenueName, &b.VenueAddress, &b.VenueCity, &b.CourtName, &b.CourtSportName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}
	return b, nil
}

func (r *Repository) FindByIDAndCustomerID(ctx context.Context, id, customerID string) (Booking, error) {
	query := `
		SELECT id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, payment_reference, expires_at, created_at, updated_at
		FROM bookings
		WHERE id = $1 AND customer_id = $2
	`
	var b Booking
	err := r.db.QueryRow(ctx, query, id, customerID).Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt)
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

func (r *Repository) ListOwnerVenueBookings(ctx context.Context, ownerProfileID, venueID, date, status, scope string, limit, offset int) ([]OwnerBooking, int, error) {
	countQuery := `
		SELECT count(*)
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		WHERE v.owner_profile_id = $1
			AND v.id = $2
			AND ($3 = '' OR b.booking_date::text = $3)
			AND ($4 = '' OR b.status = $4)
			AND ($5 = '' OR ($5 = 'upcoming' AND b.booking_date >= CURRENT_DATE AND b.status NOT IN ('CANCELLED', 'COMPLETED')))
	`
	var total int
	if err := r.db.QueryRow(ctx, countQuery, ownerProfileID, venueID, date, status, scope).Scan(&total); err != nil {
		return nil, 0, err
	}

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
			b.payment_reference,
			b.expires_at,
			b.created_at,
			b.updated_at
		FROM bookings b
		JOIN users u ON u.id = b.customer_id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		WHERE v.owner_profile_id = $1
			AND v.id = $2
			AND ($3 = '' OR b.booking_date::text = $3)
			AND ($4 = '' OR b.status = $4)
			AND ($5 = '' OR ($5 = 'upcoming' AND b.booking_date >= CURRENT_DATE AND b.status NOT IN ('CANCELLED', 'COMPLETED')))
		ORDER BY b.start_time ASC, b.created_at ASC
		LIMIT $6 OFFSET $7
	`
	rows, err := r.db.Query(ctx, query, ownerProfileID, venueID, date, status, scope, limit, offset)
	if err != nil {
		return nil, 0, err
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
			&booking.PaymentReference,
			&booking.ExpiresAt,
			&booking.CreatedAt,
			&booking.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		bookings = append(bookings, booking)
	}

	return bookings, total, rows.Err()
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
		  AND NOT (status = 'PENDING_PAYMENT' AND (expires_at IS NULL OR expires_at <= NOW()))
	`
	var count int
	err := tx.QueryRow(ctx, query, courtID, date, startTime, endTime).Scan(&count)
	return count > 0, err
}

func (r *Repository) InsertBooking(ctx context.Context, tx pgx.Tx, params CreateBookingParams) (Booking, error) {
	query := `
		INSERT INTO bookings (customer_id, court_id, booking_date, start_time, end_time, total_price, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING_PAYMENT', $7)
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, payment_reference, expires_at, created_at, updated_at
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
		params.ExpiresAt,
	).Scan(
		&b.ID,
		&b.CustomerID,
		&b.CourtID,
		&b.Date,
		&b.StartTime,
		&b.EndTime,
		&b.TotalPrice,
		&b.Status,
		&b.PaymentReference,
		&b.ExpiresAt,
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
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, payment_reference, expires_at, created_at, updated_at
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
		&b.PaymentReference,
		&b.ExpiresAt,
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
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, payment_reference, expires_at, created_at, updated_at
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
		&b.PaymentReference,
		&b.ExpiresAt,
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

func (r *Repository) UpdatePaymentReference(ctx context.Context, bookingID, customerID, reference string) (Booking, error) {
	query := `
		UPDATE bookings
		SET payment_reference = $3,
		    status = 'WAITING_VERIFICATION',
		    updated_at = now()
		WHERE id = $1 AND customer_id = $2 AND status = 'PENDING_PAYMENT'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	var b Booking
	err := r.db.QueryRow(ctx, query, bookingID, customerID, reference).Scan(
		&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime,
		&b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}
	return b, nil
}

func (r *Repository) VerifyPayment(ctx context.Context, bookingID string, isApproved bool) (Booking, error) {
	var newStatus string
	if isApproved {
		newStatus = "CONFIRMED"
	} else {
		newStatus = "PENDING_PAYMENT"
	}

	query := `
		UPDATE bookings
		SET status = $2,
		    updated_at = now()
		WHERE id = $1 AND status = 'WAITING_VERIFICATION'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	var b Booking
	err := r.db.QueryRow(ctx, query, bookingID, newStatus).Scan(
		&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime,
		&b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}
	return b, nil
}

func (r *Repository) GetBookingOwnerProfileID(ctx context.Context, bookingID string) (string, error) {
	query := `
		SELECT v.owner_profile_id::text
		FROM bookings b
		JOIN courts c ON b.court_id = c.id
		JOIN venues v ON c.venue_id = v.id
		WHERE b.id = $1
	`
	var ownerProfileID string
	err := r.db.QueryRow(ctx, query, bookingID).Scan(&ownerProfileID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", pgx.ErrNoRows
		}
		return "", err
	}
	return ownerProfileID, nil
}

type OwnerMetrics struct {
	TotalVenues          int
	UpcomingBookings     int
	PendingVerifications int
	RevenueCurrent       float64
	RevenueAllTime       float64
	OccupancyRate        float64
}

func (r *Repository) GetOwnerMetrics(ctx context.Context, ownerProfileID string, startDate string, endDate string) (OwnerMetrics, error) {
	var metrics OwnerMetrics

	// 1. Total Venues
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM venues WHERE owner_profile_id = $1`, ownerProfileID).Scan(&metrics.TotalVenues)
	if err != nil {
		return metrics, err
	}

	// 2. Upcoming Bookings
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) 
		FROM bookings b 
		JOIN courts c ON b.court_id = c.id 
		JOIN venues v ON c.venue_id = v.id 
		WHERE v.owner_profile_id = $1 
		  AND b.booking_date >= CURRENT_DATE 
		  AND b.status IN ('CONFIRMED', 'PAID', 'WAITING_VERIFICATION')
	`, ownerProfileID).Scan(&metrics.UpcomingBookings)
	if err != nil {
		return metrics, err
	}

	// 2.5 Pending Verifications
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) 
		FROM bookings b 
		JOIN courts c ON b.court_id = c.id 
		JOIN venues v ON c.venue_id = v.id 
		WHERE v.owner_profile_id = $1 
		  AND b.status = 'WAITING_VERIFICATION'
	`, ownerProfileID).Scan(&metrics.PendingVerifications)
	if err != nil {
		return metrics, err
	}

	// 3. Revenue All Time
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(b.total_price), 0)
		FROM bookings b 
		JOIN courts c ON b.court_id = c.id 
		JOIN venues v ON c.venue_id = v.id 
		WHERE v.owner_profile_id = $1 
		  AND b.status IN ('CONFIRMED', 'PAID', 'COMPLETED')
	`, ownerProfileID).Scan(&metrics.RevenueAllTime)
	if err != nil {
		return metrics, err
	}

	// 4. Revenue Current
	revenueCurrentQuery := `
		SELECT COALESCE(SUM(b.total_price), 0)
		FROM bookings b 
		JOIN courts c ON b.court_id = c.id 
		JOIN venues v ON c.venue_id = v.id 
		WHERE v.owner_profile_id = $1 
		  AND b.status IN ('CONFIRMED', 'PAID', 'COMPLETED')
	`
	var args []interface{}
	args = append(args, ownerProfileID)

	if startDate != "" && endDate != "" {
		revenueCurrentQuery += ` AND b.booking_date >= $2 AND b.booking_date <= $3`
		args = append(args, startDate, endDate)
	} else {
		revenueCurrentQuery += ` AND date_trunc('month', b.booking_date) = date_trunc('month', CURRENT_DATE)`
	}

	err = r.db.QueryRow(ctx, revenueCurrentQuery, args...).Scan(&metrics.RevenueCurrent)
	if err != nil {
		return metrics, err
	}

	// TODO: Occupancy rate calculation requires a separate batch analytics process
	// to calculate the ratio of booked slots vs total available slots.
	// Returning 0 for now as MVP.
	metrics.OccupancyRate = 0.0

	return metrics, nil
}

func (r *Repository) CancelExpiredPendingBookings(ctx context.Context) (int64, error) {
	query := `
		UPDATE bookings
		SET status = 'CANCELLED', updated_at = NOW()
		WHERE status = 'PENDING_PAYMENT'
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
	`
	cmdTag, err := r.db.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return cmdTag.RowsAffected(), nil
}
