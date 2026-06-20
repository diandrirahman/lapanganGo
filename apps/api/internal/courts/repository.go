package courts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

type Venue struct {
	ID             string
	OwnerProfileID string
}

type Sport struct {
	ID   string
	Name string
}

type Court struct {
	ID           string
	VenueID      string
	Sport        Sport
	Name         string
	Description  *string
	LocationType string
	SurfaceType  *string
	PricePerHour float64
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CourtParams struct {
	VenueID      string
	SportID      string
	Name         string
	Description  *string
	LocationType string
	SurfaceType  *string
	PricePerHour float64
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

func (r *Repository) FindVenueByIDAndOwnerProfileID(ctx context.Context, venueID, ownerProfileID string) (Venue, error) {
	query := `
		SELECT id::text, owner_profile_id::text
		FROM venues
		WHERE id = $1
			AND owner_profile_id = $2
		LIMIT 1
	`

	var venue Venue
	err := r.db.QueryRow(ctx, query, venueID, ownerProfileID).Scan(
		&venue.ID,
		&venue.OwnerProfileID,
	)
	if err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func (r *Repository) FindSportByID(ctx context.Context, sportID string) (Sport, error) {
	query := `
		SELECT id::text, name
		FROM sports
		WHERE id = $1
		LIMIT 1
	`

	var sport Sport
	err := r.db.QueryRow(ctx, query, sportID).Scan(&sport.ID, &sport.Name)
	if err != nil {
		return Sport{}, err
	}

	return sport, nil
}

func (r *Repository) Create(ctx context.Context, params CourtParams) (Court, error) {
	query := `
		WITH inserted AS (
			INSERT INTO courts (
				venue_id,
				sport_id,
				name,
				description,
				location_type,
				surface_type,
				price_per_hour
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING
				id,
				venue_id,
				sport_id,
				name,
				description,
				location_type,
				surface_type,
				price_per_hour,
				status,
				created_at,
				updated_at
		)
		SELECT
			inserted.id::text,
			inserted.venue_id::text,
			inserted.sport_id::text,
			s.name,
			inserted.name,
			inserted.description,
			inserted.location_type::text,
			inserted.surface_type,
			inserted.price_per_hour,
			inserted.status::text,
			inserted.created_at,
			inserted.updated_at
		FROM inserted
		JOIN sports s ON s.id = inserted.sport_id
	`

	return r.queryCourt(ctx, query,
		params.VenueID,
		params.SportID,
		params.Name,
		params.Description,
		params.LocationType,
		params.SurfaceType,
		params.PricePerHour,
	)
}

func (r *Repository) ListByVenueIDAndOwnerProfileID(ctx context.Context, venueID, ownerProfileID string) ([]Court, error) {
	query := `
		SELECT
			c.id::text,
			c.venue_id::text,
			c.sport_id::text,
			s.name,
			c.name,
			c.description,
			c.location_type::text,
			c.surface_type,
			c.price_per_hour,
			c.status::text,
			c.created_at,
			c.updated_at
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE c.venue_id = $1
			AND v.owner_profile_id = $2
		ORDER BY c.created_at DESC
	`

	rows, err := r.db.Query(ctx, query, venueID, ownerProfileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courts []Court
	for rows.Next() {
		court, err := scanCourt(rows)
		if err != nil {
			return nil, err
		}
		courts = append(courts, court)
	}

	return courts, rows.Err()
}

func (r *Repository) FindByIDAndOwnerProfileID(ctx context.Context, courtID, ownerProfileID string) (Court, error) {
	query := `
		SELECT
			c.id::text,
			c.venue_id::text,
			c.sport_id::text,
			s.name,
			c.name,
			c.description,
			c.location_type::text,
			c.surface_type,
			c.price_per_hour,
			c.status::text,
			c.created_at,
			c.updated_at
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		JOIN sports s ON s.id = c.sport_id
		WHERE c.id = $1
			AND v.owner_profile_id = $2
		LIMIT 1
	`

	return r.queryCourt(ctx, query, courtID, ownerProfileID)
}

func (r *Repository) UpdateByIDAndOwnerProfileID(ctx context.Context, courtID, ownerProfileID string, params CourtParams) (Court, error) {
	query := `
		WITH updated AS (
			UPDATE courts c
			SET sport_id = $3,
				name = $4,
				description = $5,
				location_type = $6,
				surface_type = $7,
				price_per_hour = $8,
				updated_at = now()
			FROM venues v
			WHERE c.id = $1
				AND c.venue_id = v.id
				AND v.owner_profile_id = $2
			RETURNING
				c.id,
				c.venue_id,
				c.sport_id,
				c.name,
				c.description,
				c.location_type,
				c.surface_type,
				c.price_per_hour,
				c.status,
				c.created_at,
				c.updated_at
		)
		SELECT
			updated.id::text,
			updated.venue_id::text,
			updated.sport_id::text,
			s.name,
			updated.name,
			updated.description,
			updated.location_type::text,
			updated.surface_type,
			updated.price_per_hour,
			updated.status::text,
			updated.created_at,
			updated.updated_at
		FROM updated
		JOIN sports s ON s.id = updated.sport_id
	`

	return r.queryCourt(ctx, query,
		courtID,
		ownerProfileID,
		params.SportID,
		params.Name,
		params.Description,
		params.LocationType,
		params.SurfaceType,
		params.PricePerHour,
	)
}

func (r *Repository) UpdateStatusByIDAndOwnerProfileID(ctx context.Context, courtID, ownerProfileID, status string) (Court, error) {
	query := `
		WITH updated AS (
			UPDATE courts c
			SET status = $3,
				updated_at = now()
			FROM venues v
			WHERE c.id = $1
				AND c.venue_id = v.id
				AND v.owner_profile_id = $2
			RETURNING
				c.id,
				c.venue_id,
				c.sport_id,
				c.name,
				c.description,
				c.location_type,
				c.surface_type,
				c.price_per_hour,
				c.status,
				c.created_at,
				c.updated_at
		)
		SELECT
			updated.id::text,
			updated.venue_id::text,
			updated.sport_id::text,
			s.name,
			updated.name,
			updated.description,
			updated.location_type::text,
			updated.surface_type,
			updated.price_per_hour,
			updated.status::text,
			updated.created_at,
			updated.updated_at
		FROM updated
		JOIN sports s ON s.id = updated.sport_id
	`

	return r.queryCourt(ctx, query, courtID, ownerProfileID, status)
}

func (r *Repository) queryCourt(ctx context.Context, query string, args ...any) (Court, error) {
	return scanCourt(r.db.QueryRow(ctx, query, args...))
}

func scanCourt(row pgx.Row) (Court, error) {
	var court Court
	err := row.Scan(
		&court.ID,
		&court.VenueID,
		&court.Sport.ID,
		&court.Sport.Name,
		&court.Name,
		&court.Description,
		&court.LocationType,
		&court.SurfaceType,
		&court.PricePerHour,
		&court.Status,
		&court.CreatedAt,
		&court.UpdatedAt,
	)
	if err != nil {
		return Court{}, err
	}

	return court, nil
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
