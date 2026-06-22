package availability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

type Court struct {
	ID          string
	Status      string
	VenueStatus string
}

type OperatingHour struct {
	CourtID   string
	DayOfWeek int
	OpenTime  *string
	CloseTime *string
	IsClosed  bool
}

type BlockedSlot struct {
	ID      string
	CourtID string
	StartAt time.Time
	EndAt   time.Time
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindCourtByID(ctx context.Context, courtID string) (Court, error) {
	query := `
		SELECT c.id::text, c.status::text, v.status::text
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		WHERE c.id = $1
		LIMIT 1
	`

	var court Court
	err := r.db.QueryRow(ctx, query, courtID).Scan(
		&court.ID,
		&court.Status,
		&court.VenueStatus,
	)
	if err != nil {
		return Court{}, err
	}

	return court, nil
}

func (r *Repository) FindOperatingHour(ctx context.Context, courtID string, dayOfWeek int) (OperatingHour, error) {
	query := `
		SELECT
			court_id::text,
			day_of_week,
			CASE WHEN open_time IS NULL THEN NULL ELSE to_char(open_time, 'HH24:MI') END,
			CASE WHEN close_time IS NULL THEN NULL ELSE to_char(close_time, 'HH24:MI') END,
			is_closed
		FROM court_operating_hours
		WHERE court_id = $1
			AND day_of_week = $2
		LIMIT 1
	`

	var operatingHour OperatingHour
	err := r.db.QueryRow(ctx, query, courtID, dayOfWeek).Scan(
		&operatingHour.CourtID,
		&operatingHour.DayOfWeek,
		&operatingHour.OpenTime,
		&operatingHour.CloseTime,
		&operatingHour.IsClosed,
	)
	if err != nil {
		return OperatingHour{}, err
	}

	return operatingHour, nil
}

func (r *Repository) ListBlockedSlots(ctx context.Context, courtID string, from, to time.Time) ([]BlockedSlot, error) {
	query := `
		SELECT id::text, court_id::text, start_at, end_at
		FROM court_blocked_slots
		WHERE court_id = $1
			AND end_at > $2
			AND start_at < $3
		ORDER BY start_at ASC
	`

	rows, err := r.db.Query(ctx, query, courtID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blockedSlots []BlockedSlot
	for rows.Next() {
		var blockedSlot BlockedSlot
		err := rows.Scan(
			&blockedSlot.ID,
			&blockedSlot.CourtID,
			&blockedSlot.StartAt,
			&blockedSlot.EndAt,
		)
		if err != nil {
			return nil, err
		}
		blockedSlots = append(blockedSlots, blockedSlot)
	}

	return blockedSlots, rows.Err()
}

type ActiveBooking struct {
	ID        string
	CourtID   string
	Date      time.Time
	StartTime time.Time
	EndTime   time.Time
}

func (r *Repository) ListActiveBookings(ctx context.Context, courtID string, date string) ([]ActiveBooking, error) {
	query := `
		SELECT id::text, court_id::text, booking_date, start_time, end_time
		FROM bookings
		WHERE court_id = $1
			AND booking_date = $2
			AND status != 'CANCELLED'
	`

	rows, err := r.db.Query(ctx, query, courtID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []ActiveBooking
	for rows.Next() {
		var b ActiveBooking
		err := rows.Scan(
			&b.ID,
			&b.CourtID,
			&b.Date,
			&b.StartTime,
			&b.EndTime,
		)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}

	return bookings, rows.Err()
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}
