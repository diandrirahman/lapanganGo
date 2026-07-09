package schedules

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

type OwnerProfile struct {
	ID                 string
	UserID             string
	VerificationStatus string
}

type Court struct {
	ID             string
	OwnerProfileID string
	VenueID        string
}

type OperatingHour struct {
	ID        string
	CourtID   string
	DayOfWeek int
	OpenTime  *string
	CloseTime *string
	IsClosed  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OperatingHourParams struct {
	DayOfWeek int
	OpenTime  *string
	CloseTime *string
	IsClosed  bool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindOwnerProfileByUserID(ctx context.Context, userID string) (OwnerProfile, error) {
	query := `
		SELECT id::text, user_id::text, verification_status::text
		FROM owner_profiles
		WHERE user_id = $1
		LIMIT 1
	`

	var profile OwnerProfile
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.VerificationStatus,
	)
	if err != nil {
		return OwnerProfile{}, err
	}

	return profile, nil
}

func (r *Repository) FindCourtByIDAndOwnerProfileID(ctx context.Context, courtID, ownerProfileID string) (Court, error) {
	query := `
		SELECT c.id::text, v.owner_profile_id::text, c.venue_id::text
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		WHERE c.id = $1
			AND v.owner_profile_id = $2
		LIMIT 1
	`

	var court Court
	err := r.db.QueryRow(ctx, query, courtID, ownerProfileID).Scan(
		&court.ID,
		&court.OwnerProfileID,
		&court.VenueID,
	)
	if err != nil {
		return Court{}, err
	}

	return court, nil
}

func (r *Repository) ListOperatingHoursByCourtID(ctx context.Context, courtID string) ([]OperatingHour, error) {
	query := `
		SELECT
			id::text,
			court_id::text,
			day_of_week,
			CASE WHEN open_time IS NULL THEN NULL ELSE to_char(open_time, 'HH24:MI') END,
			CASE WHEN close_time IS NULL THEN NULL ELSE to_char(close_time, 'HH24:MI') END,
			is_closed,
			created_at,
			updated_at
		FROM court_operating_hours
		WHERE court_id = $1
		ORDER BY day_of_week ASC
	`

	rows, err := r.db.Query(ctx, query, courtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanOperatingHours(rows)
}

func (r *Repository) ReplaceOperatingHours(ctx context.Context, courtID string, params []OperatingHourParams) ([]OperatingHour, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO court_operating_hours (
			court_id,
			day_of_week,
			open_time,
			close_time,
			is_closed
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (court_id, day_of_week)
		DO UPDATE SET
			open_time = EXCLUDED.open_time,
			close_time = EXCLUDED.close_time,
			is_closed = EXCLUDED.is_closed,
			updated_at = now()
	`

	for _, param := range params {
		_, err := tx.Exec(
			ctx,
			query,
			courtID,
			param.DayOfWeek,
			param.OpenTime,
			param.CloseTime,
			param.IsClosed,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return r.ListOperatingHoursByCourtID(ctx, courtID)
}

func scanOperatingHours(rows pgx.Rows) ([]OperatingHour, error) {
	var operatingHours []OperatingHour
	for rows.Next() {
		var operatingHour OperatingHour
		err := rows.Scan(
			&operatingHour.ID,
			&operatingHour.CourtID,
			&operatingHour.DayOfWeek,
			&operatingHour.OpenTime,
			&operatingHour.CloseTime,
			&operatingHour.IsClosed,
			&operatingHour.CreatedAt,
			&operatingHour.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		operatingHours = append(operatingHours, operatingHour)
	}

	return operatingHours, rows.Err()
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}
