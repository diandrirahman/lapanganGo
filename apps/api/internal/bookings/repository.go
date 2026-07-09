package bookings

import (
	"context"
	"errors"
	"fmt"
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
	VenueID      string
	OwnerUserID  string
}

type OperatingHour struct {
	OpenTime  *time.Time
	CloseTime *time.Time
	IsClosed  bool
}

type CreateBookingParams struct {
	CustomerID     string
	CourtID        string
	Date           string
	StartTime      string
	EndTime        string
	OriginalPrice  *float64
	DiscountAmount float64
	FinalPrice     *float64
	PromoID        *string
	PromoCode      *string
	TotalPrice     float64
	ExpiresAt      *time.Time
}

type CreateOfflineBookingParams struct {
	VenueID             string
	CourtID             string
	Date                string
	StartTime           string
	EndTime             string
	SystemPrice         float64
	FinalPrice          float64
	Status              string
	OwnerUserID         string
	CreatedByUserID     string
	CustomerName        string
	CustomerPhone       *string
	CustomerEmail       *string
	Note                *string
	PriceOverrideReason *string
}

type Booking struct {
	ID               string
	CustomerID       string
	CourtID          string
	Date             time.Time
	StartTime        time.Time
	EndTime          time.Time
	OriginalPrice    *float64
	DiscountAmount   float64
	FinalPrice       *float64
	PromoID          *string
	PromoCode        *string
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
	OriginalPrice    *float64
	DiscountAmount   float64
	TotalPrice       float64
	PromoID          *string
	PromoCode        *string
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
		SELECT c.price_per_hour, c.status, v.status, v.id, op.user_id
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON op.id = v.owner_profile_id
		WHERE c.id = $1
		FOR UPDATE
	`
	var info CourtValidationInfo
	err := tx.QueryRow(ctx, query, courtID).Scan(&info.PricePerHour, &info.CourtStatus, &info.VenueStatus, &info.VenueID, &info.OwnerUserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return info, ErrCourtNotFound
		}
		return info, err
	}
	return info, nil
}

func (r *Repository) LockOwnerCourtValidationInfo(ctx context.Context, tx pgx.Tx, courtID, venueID, ownerProfileID string) (CourtValidationInfo, error) {
	query := `
		SELECT c.price_per_hour, c.status, v.status, op.user_id
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON op.id = v.owner_profile_id
		WHERE c.id = $1 AND v.id = $2 AND v.owner_profile_id = $3
		FOR UPDATE
	`
	var info CourtValidationInfo
	err := tx.QueryRow(ctx, query, courtID, venueID, ownerProfileID).Scan(&info.PricePerHour, &info.CourtStatus, &info.VenueStatus, &info.OwnerUserID)
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
		SELECT b.id::text, b.customer_id::text, b.court_id::text, b.booking_date, b.start_time, b.end_time, b.original_price, b.discount_amount, b.final_price, b.promo_id::text, b.promo_code, b.total_price, b.status, b.payment_reference, b.expires_at, b.created_at, b.updated_at,
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
		if err := rows.Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.OriginalPrice, &b.DiscountAmount, &b.FinalPrice, &b.PromoID, &b.PromoCode, &b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt, &b.VenueID, &b.VenueName, &b.VenueAddress, &b.VenueCity, &b.CourtName, &b.CourtSportName); err != nil {
			return nil, 0, err
		}
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

func (r *Repository) FindCustomerBookingByID(ctx context.Context, id, customerID string) (CustomerBooking, error) {
	query := `
		SELECT b.id::text, b.customer_id::text, b.court_id::text, b.booking_date, b.start_time, b.end_time, b.original_price, b.discount_amount, b.final_price, b.promo_id::text, b.promo_code, b.total_price, b.status, b.payment_reference, b.expires_at, b.created_at, b.updated_at,
		       v.id::text, v.name, v.address, v.city, c.name, s.name
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE b.id = $1 AND b.customer_id = $2
	`
	var b CustomerBooking
	err := r.db.QueryRow(ctx, query, id, customerID).Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.OriginalPrice, &b.DiscountAmount, &b.FinalPrice, &b.PromoID, &b.PromoCode, &b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt, &b.VenueID, &b.VenueName, &b.VenueAddress, &b.VenueCity, &b.CourtName, &b.CourtSportName)
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
		SELECT id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
		FROM bookings
		WHERE id = $1 AND customer_id = $2
	`
	var b Booking
	err := r.db.QueryRow(ctx, query, id, customerID).Scan(&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime, &b.OriginalPrice, &b.DiscountAmount, &b.FinalPrice, &b.PromoID, &b.PromoCode, &b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt)
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
			COALESCE(obc.name, u.name),
			COALESCE(obc.email, u.email),
			COALESCE(obc.phone, u.phone),
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
		LEFT JOIN offline_booking_customers obc ON obc.booking_id = b.id
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
		INSERT INTO bookings (customer_id, court_id, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id, promo_code, total_price, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'PENDING_PAYMENT', $12)
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	return scanBooking(tx.QueryRow(
		ctx,
		query,
		params.CustomerID,
		params.CourtID,
		params.Date,
		params.StartTime,
		params.EndTime,
		params.OriginalPrice,
		params.DiscountAmount,
		params.FinalPrice,
		params.PromoID,
		params.PromoCode,
		params.TotalPrice,
		params.ExpiresAt,
	))
}

func (r *Repository) InsertOfflineBookingTx(ctx context.Context, tx pgx.Tx, params CreateOfflineBookingParams) (Booking, error) {
	// 1. Calculate discount
	discountAmount := 0.0
	if params.FinalPrice < params.SystemPrice {
		discountAmount = params.SystemPrice - params.FinalPrice
	}

	// 2. Insert booking
	queryBooking := `
		INSERT INTO bookings (customer_id, court_id, booking_date, start_time, end_time, original_price, discount_amount, final_price, total_price, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err := scanBooking(tx.QueryRow(
		ctx, queryBooking,
		params.OwnerUserID, params.CourtID, params.Date, params.StartTime, params.EndTime, params.SystemPrice, discountAmount, params.FinalPrice, params.FinalPrice, params.Status,
	))
	if err != nil {
		return b, err
	}

	// 2. Insert offline_booking_customers
	queryOBC := `
		INSERT INTO offline_booking_customers (booking_id, name, phone, email, notes, system_price, final_price, price_override_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err = tx.Exec(ctx, queryOBC, b.ID, params.CustomerName, params.CustomerPhone, params.CustomerEmail, params.Note, params.SystemPrice, params.FinalPrice, params.PriceOverrideReason)
	if err != nil {
		return b, err
	}

	// 3. Insert into owner_finance_transactions
	queryLedger := `
		INSERT INTO owner_finance_transactions (owner_id, venue_id, booking_id, created_by_user_id, type, source, category, amount, transaction_date, description)
		VALUES ($1, $2, $3, $4, 'INCOME', 'BOOKING', 'BOOKING_PAYMENT', $5, CURRENT_DATE, $6)
	`
	desc := "Offline booking payment"
	if params.PriceOverrideReason != nil && *params.PriceOverrideReason != "" {
		desc = desc + ". Price adjusted from Rp" + fmt.Sprintf("%.2f", params.SystemPrice) + " to Rp" + fmt.Sprintf("%.2f", params.FinalPrice) + ": " + *params.PriceOverrideReason
	}
	_, err = tx.Exec(ctx, queryLedger, params.OwnerUserID, params.VenueID, b.ID, params.CreatedByUserID, params.FinalPrice, desc)
	if err != nil {
		return b, err
	}

	return b, nil
}

func (r *Repository) CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error) {
	query := `
		UPDATE bookings
		SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1 AND customer_id = $2 AND status = 'PENDING_PAYMENT'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err := scanBooking(r.db.QueryRow(ctx, query, bookingID, customerID))
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
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err := scanBooking(r.db.QueryRow(ctx, query, bookingID, customerID))
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
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err := scanBooking(r.db.QueryRow(ctx, query, bookingID, customerID, reference))
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}
	return b, nil
}

func (r *Repository) VerifyPayment(ctx context.Context, ownerUserID string, bookingID string, isApproved bool) (Booking, error) {
	var newStatus string
	if isApproved {
		newStatus = "PAID"
	} else {
		newStatus = "PENDING_PAYMENT"
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback(ctx)

	query := `
		UPDATE bookings
		SET status = $2,
		    updated_at = now()
		WHERE id = $1 AND status = 'WAITING_VERIFICATION'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err := scanBooking(tx.QueryRow(ctx, query, bookingID, newStatus))
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}

	if isApproved {
		// Insert or Upsert to owner_finance_transactions
		financeQuery := `
			INSERT INTO owner_finance_transactions 
				(owner_id, venue_id, booking_id, created_by_user_id, type, source, category, amount, transaction_date, description)
			SELECT 
				op.user_id,
				v.id,
				b.id,
				$2,
				'INCOME',
				'BOOKING',
				'BOOKING_PAYMENT',
				b.total_price,
				CURRENT_DATE,
				'Pembayaran booking ' || b.id
			FROM bookings b
			JOIN courts c ON c.id = b.court_id
			JOIN venues v ON v.id = c.venue_id
			JOIN owner_profiles op ON v.owner_profile_id = op.id
			WHERE b.id = $1
			ON CONFLICT (booking_id) WHERE source = 'BOOKING' AND booking_id IS NOT NULL DO NOTHING
		`
		_, err = tx.Exec(ctx, financeQuery, bookingID, ownerUserID)
		if err != nil {
			return b, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return b, err
	}

	return b, nil
}

func (r *Repository) MarkBookingPaid(ctx context.Context, ownerUserID string, bookingID string) (Booking, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback(ctx)

	query := `
		UPDATE bookings
		SET status = 'PAID',
		    updated_at = now()
		WHERE id = $1 AND status = 'CONFIRMED'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err := scanBooking(tx.QueryRow(ctx, query, bookingID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return b, pgx.ErrNoRows
		}
		return b, err
	}

	financeQuery := `
		INSERT INTO owner_finance_transactions 
			(owner_id, venue_id, booking_id, created_by_user_id, type, source, category, amount, transaction_date, description)
		SELECT 
			op.user_id,
			v.id,
			b.id,
			$2,
			'INCOME',
			'BOOKING',
			'BOOKING_PAYMENT',
			b.total_price,
			CURRENT_DATE,
			'Pembayaran booking ' || b.id
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		WHERE b.id = $1
		ON CONFLICT (booking_id) WHERE source = 'BOOKING' AND booking_id IS NOT NULL DO NOTHING
	`
	_, err = tx.Exec(ctx, financeQuery, bookingID, ownerUserID)
	if err != nil {
		return b, err
	}

	if err := tx.Commit(ctx); err != nil {
		return b, err
	}

	return b, nil
}

func (r *Repository) CompleteBooking(ctx context.Context, bookingID string) (Booking, error) {
	query := `
		UPDATE bookings
		SET status = 'COMPLETED', updated_at = now()
		WHERE id = $1
		  AND status = 'PAID'
		  AND (
		    booking_date < (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE
		    OR (
		      booking_date = (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE 
		      AND end_time <= (NOW() AT TIME ZONE 'Asia/Jakarta')::TIME
		    )
		  )
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err := scanBooking(r.db.QueryRow(ctx, query, bookingID))
	if err != nil {
		return b, err
	}
	return b, nil
}

func (r *Repository) CancelPaidBookingWithRefund(ctx context.Context, ownerUserID string, actorUserID string, bookingID string, reason string) (Booking, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback(ctx)

	// 1. Lock the target booking row
	lockQuery := `
		SELECT
			b.id::text,
			b.customer_id::text,
			b.court_id::text,
			b.booking_date,
			b.start_time,
			b.end_time,
			b.total_price,
			b.status,
			b.payment_reference,
			b.expires_at,
			b.created_at,
			b.updated_at,
			v.id::text AS venue_id,
			op.user_id::text AS owner_user_id
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON op.id = v.owner_profile_id
		WHERE b.id = $1
		FOR UPDATE
	`
	var b Booking
	var venueID string
	var dbOwnerUserID string
	err = tx.QueryRow(ctx, lockQuery, bookingID).Scan(
		&b.ID, &b.CustomerID, &b.CourtID, &b.Date, &b.StartTime, &b.EndTime,
		&b.TotalPrice, &b.Status, &b.PaymentReference, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt,
		&venueID, &dbOwnerUserID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b, pgx.ErrNoRows
		}
		return b, err
	}

	// 2. Validate ownership
	if dbOwnerUserID != ownerUserID {
		return b, ErrForbidden
	}

	// 3. Validate status
	if b.Status != "PAID" {
		return b, ErrBookingCannotBeRefunded
	}

	// 4. Ensure original booking income ledger exists
	var hasIncome bool
	incomeQuery := `
		SELECT EXISTS (
			SELECT 1
			FROM owner_finance_transactions
			WHERE booking_id = $1
			  AND source = 'BOOKING'
			  AND type = 'INCOME'
		)
	`
	err = tx.QueryRow(ctx, incomeQuery, bookingID).Scan(&hasIncome)
	if err != nil {
		return b, err
	}
	if !hasIncome {
		return b, ErrBookingIncomeLedgerNotFound
	}

	// 5. Prevent duplicate refund ledger
	var hasRefund bool
	refundQuery := `
		SELECT EXISTS (
			SELECT 1
			FROM owner_finance_transactions
			WHERE booking_id = $1
			  AND source = 'REFUND'
			  AND type = 'EXPENSE'
		)
	`
	err = tx.QueryRow(ctx, refundQuery, bookingID).Scan(&hasRefund)
	if err != nil {
		return b, err
	}
	if hasRefund {
		return b, ErrBookingRefundAlreadyExists
	}

	// 6. Update booking
	updateQuery := `
		UPDATE bookings
		SET status = 'CANCELLED',
		    updated_at = now()
		WHERE id = $1
		  AND status = 'PAID'
		RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, original_price, discount_amount, final_price, promo_id::text, promo_code, total_price, status, payment_reference, expires_at, created_at, updated_at
	`
	b, err = scanBooking(tx.QueryRow(ctx, updateQuery, bookingID))
	if err != nil {
		return b, err
	}

	// 7. Insert refund ledger
	description := "Refund booking " + bookingID
	if reason != "" {
		description += ": " + reason
	}

	insertRefundQuery := `
		INSERT INTO owner_finance_transactions
		  (owner_id, venue_id, booking_id, created_by_user_id, type, source, category, amount, transaction_date, description)
		VALUES
		  ($1, $2, $3, $4, 'EXPENSE', 'REFUND', 'BOOKING_REFUND', $5, CURRENT_DATE, $6)
	`
	_, err = tx.Exec(ctx, insertRefundQuery, ownerUserID, venueID, bookingID, actorUserID, b.TotalPrice, description)
	if err != nil {
		return b, err
	}

	// 8. Commit
	if err := tx.Commit(ctx); err != nil {
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
	var id string
	err := r.db.QueryRow(ctx, query, bookingID).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", err
		}
		return "", err
	}
	return id, nil
}

func (r *Repository) GetBookingOwnerProfileAndVenueID(ctx context.Context, bookingID string) (string, string, error) {
	query := `
		SELECT v.owner_profile_id::text, v.id::text
		FROM bookings b
		JOIN courts c ON b.court_id = c.id
		JOIN venues v ON c.venue_id = v.id
		WHERE b.id = $1
	`
	var ownerProfileID, venueID string
	err := r.db.QueryRow(ctx, query, bookingID).Scan(&ownerProfileID, &venueID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", err
		}
		return "", "", err
	}
	return ownerProfileID, venueID, nil
}

type OwnerMetrics struct {
	TotalVenues           int
	UpcomingBookings      int
	PendingVerifications  int
	RevenueCurrent        float64
	BookingRevenueCurrent float64
	RefundCurrent         float64
	NetRevenueCurrent     float64
	RevenueAllTime        float64
	OccupancyRate         float64
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
		SELECT COALESCE(SUM(t.amount), 0)
		FROM owner_profiles op
		LEFT JOIN owner_finance_transactions t ON t.owner_id = op.user_id
		  AND t.type = 'INCOME'
		  AND t.source = 'BOOKING'
		WHERE op.id = $1
	`, ownerProfileID).Scan(&metrics.RevenueAllTime)
	if err != nil {
		return metrics, err
	}

	// 4. Revenue Current (Booking Income)
	bookingRevenueQuery := `
		SELECT COALESCE(SUM(t.amount), 0)
		FROM owner_profiles op
		LEFT JOIN owner_finance_transactions t ON t.owner_id = op.user_id
		  AND t.type = 'INCOME'
		  AND t.source = 'BOOKING'
	`
	// 5. Refund Current
	refundQuery := `
		SELECT COALESCE(SUM(t.amount), 0)
		FROM owner_profiles op
		LEFT JOIN owner_finance_transactions t ON t.owner_id = op.user_id
		  AND t.type = 'EXPENSE'
		  AND t.source = 'REFUND'
	`

	var args []interface{}
	args = append(args, ownerProfileID)

	condition := ` WHERE op.id = $1`
	if startDate != "" && endDate != "" {
		condition += ` AND t.transaction_date >= $2 AND t.transaction_date <= $3`
		args = append(args, startDate, endDate)
	} else {
		condition += ` AND date_trunc('month', t.transaction_date) = date_trunc('month', CURRENT_DATE)`
	}

	err = r.db.QueryRow(ctx, bookingRevenueQuery+condition, args...).Scan(&metrics.BookingRevenueCurrent)
	if err != nil {
		return metrics, err
	}
	metrics.RevenueCurrent = metrics.BookingRevenueCurrent

	err = r.db.QueryRow(ctx, refundQuery+condition, args...).Scan(&metrics.RefundCurrent)
	if err != nil {
		return metrics, err
	}

	metrics.NetRevenueCurrent = metrics.BookingRevenueCurrent - metrics.RefundCurrent

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

func (r *Repository) ListOwnerBookings(ctx context.Context, ownerProfileID string, query OwnerBookingsQuery, limit, offset int) ([]OwnerBooking, int, error) {
	countQuery := `
		SELECT count(*)
		FROM bookings b
		JOIN users u ON u.id = b.customer_id
		LEFT JOIN offline_booking_customers obc ON obc.booking_id = b.id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		WHERE v.owner_profile_id = $1
			AND ($2 = '' OR v.id::text = $2)
			AND ($3 = '' OR b.status = $3)
			AND ($4 = '' OR ($4 = 'upcoming' AND b.booking_date >= CURRENT_DATE AND b.status NOT IN ('CANCELLED', 'COMPLETED')))
			AND ($5 = '' OR b.booking_date >= NULLIF($5, '')::date)
			AND ($6 = '' OR b.booking_date <= NULLIF($6, '')::date)
			AND ($7 = '' OR (
				u.name ILIKE '%' || $7 || '%' OR
				obc.name ILIKE '%' || $7 || '%' OR
				u.email ILIKE '%' || $7 || '%' OR
				obc.email ILIKE '%' || $7 || '%' OR
				v.name ILIKE '%' || $7 || '%' OR
				c.name ILIKE '%' || $7 || '%' OR
				b.id::text ILIKE '%' || $7 || '%'
			))
			AND ($8::text[] IS NULL OR v.id::text = ANY($8::text[]))
	`
	var total int
	var allowedVenuesParam interface{}
	if query.AllowedVenueIDs != nil {
		allowedVenuesParam = query.AllowedVenueIDs
	}
	if err := r.db.QueryRow(ctx, countQuery, ownerProfileID, query.VenueID, query.Status, query.Scope, query.StartDate, query.EndDate, query.Q, allowedVenuesParam).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderClause := "ORDER BY b.created_at DESC"
	switch query.Sort {
	case "oldest":
		orderClause = "ORDER BY b.created_at ASC"
	case "date_asc":
		orderClause = "ORDER BY b.booking_date ASC, b.start_time ASC"
	case "date_desc":
		orderClause = "ORDER BY b.booking_date DESC, b.start_time DESC"
	case "newest":
		fallthrough
	default:
		orderClause = "ORDER BY b.created_at DESC"
	}

	sqlQuery := `
		SELECT
			b.id::text,
			u.id::text,
			COALESCE(obc.name, u.name),
			COALESCE(obc.email, u.email),
			COALESCE(obc.phone, u.phone),
			v.id::text,
			v.name,
			c.id::text,
			c.name,
			b.booking_date,
			b.start_time,
			b.end_time,
			b.original_price,
			b.discount_amount,
			b.total_price,
			b.promo_id::text,
			b.promo_code,
			b.status,
			b.payment_reference,
			b.expires_at,
			b.created_at,
			b.updated_at
		FROM bookings b
		JOIN users u ON u.id = b.customer_id
		LEFT JOIN offline_booking_customers obc ON obc.booking_id = b.id
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		WHERE v.owner_profile_id = $1
			AND ($2 = '' OR v.id::text = $2)
			AND ($3 = '' OR b.status = $3)
			AND ($4 = '' OR ($4 = 'upcoming' AND b.booking_date >= CURRENT_DATE AND b.status NOT IN ('CANCELLED', 'COMPLETED')))
			AND ($5 = '' OR b.booking_date >= NULLIF($5, '')::date)
			AND ($6 = '' OR b.booking_date <= NULLIF($6, '')::date)
			AND ($7 = '' OR (
				u.name ILIKE '%' || $7 || '%' OR
				obc.name ILIKE '%' || $7 || '%' OR
				u.email ILIKE '%' || $7 || '%' OR
				obc.email ILIKE '%' || $7 || '%' OR
				v.name ILIKE '%' || $7 || '%' OR
				c.name ILIKE '%' || $7 || '%' OR
				b.id::text ILIKE '%' || $7 || '%'
			))
			AND ($8::text[] IS NULL OR v.id::text = ANY($8::text[]))
		` + orderClause + `
		LIMIT $9 OFFSET $10
	`

	rows, err := r.db.Query(ctx, sqlQuery, ownerProfileID, query.VenueID, query.Status, query.Scope, query.StartDate, query.EndDate, query.Q, allowedVenuesParam, limit, offset)
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
			&booking.OriginalPrice,
			&booking.DiscountAmount,
			&booking.TotalPrice,
			&booking.PromoID,
			&booking.PromoCode,
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

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return bookings, total, nil
}

func (r *Repository) AutoCompleteFinishedBookings(ctx context.Context) ([]Booking, error) {
	sqlQuery := `
		UPDATE bookings
		SET status = 'COMPLETED',
			updated_at = NOW()
		WHERE status = 'PAID'
		  AND (
			booking_date < (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE
			OR (
				booking_date = (NOW() AT TIME ZONE 'Asia/Jakarta')::DATE
				AND end_time <= (NOW() AT TIME ZONE 'Asia/Jakarta')::TIME
			)
		  )
		RETURNING id, customer_id
	`
	rows, err := r.db.Query(ctx, sqlQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var completed []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.CustomerID); err != nil {
			return nil, err
		}
		completed = append(completed, b)
	}
	return completed, rows.Err()
}

func (r *Repository) GetBookingsExpiringSoon(ctx context.Context, cutoff time.Time) ([]Booking, error) {
	query := `
		SELECT id, customer_id, expires_at
		FROM bookings
		WHERE status = 'PENDING_PAYMENT' 
		  AND expires_at IS NOT NULL
		  AND expires_at > NOW() 
		  AND expires_at <= $1
	`
	rows, err := r.db.Query(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.CustomerID, &b.ExpiresAt); err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	return bookings, rows.Err()
}

func (r *Repository) GetOwnerUserIDByCourtID(ctx context.Context, courtID string) (string, error) {
	var userID string
	query := `
		SELECT op.user_id 
		FROM courts c
		JOIN venues v ON c.venue_id = v.id
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		WHERE c.id = $1
	`
	err := r.db.QueryRow(ctx, query, courtID).Scan(&userID)
	return userID, err
}

func (r *Repository) GetOwnerUserIDByBookingID(ctx context.Context, bookingID string) (string, error) {
	var userID string
	query := `
		SELECT op.user_id 
		FROM bookings b
		JOIN courts c ON b.court_id = c.id
		JOIN venues v ON c.venue_id = v.id
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		WHERE b.id = $1
	`
	err := r.db.QueryRow(ctx, query, bookingID).Scan(&userID)
	return userID, err
}

func scanBooking(row pgx.Row) (Booking, error) {
	var b Booking
	err := row.Scan(
		&b.ID,
		&b.CustomerID,
		&b.CourtID,
		&b.Date,
		&b.StartTime,
		&b.EndTime,
		&b.OriginalPrice,
		&b.DiscountAmount,
		&b.FinalPrice,
		&b.PromoID,
		&b.PromoCode,
		&b.TotalPrice,
		&b.Status,
		&b.PaymentReference,
		&b.ExpiresAt,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	return b, err
}
