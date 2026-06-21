package venues

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
	Name           string
	Description    *string
	Address        string
	District       *string
	City           string
	Province       *string
	PostalCode     *string
	Latitude       *float64
	Longitude      *float64
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Facility struct {
	ID   string
	Name string
	Icon *string
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

type VenueParams struct {
	OwnerProfileID string
	Name           string
	Description    *string
	Address        string
	District       *string
	City           string
	Province       *string
	PostalCode     *string
	Latitude       *float64
	Longitude      *float64
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

func (r *Repository) Create(ctx context.Context, params VenueParams, facilityIDs []string) (Venue, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Venue{}, err
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO venues (
			owner_profile_id,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING
			id::text,
			owner_profile_id::text,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude,
			status::text,
			created_at,
			updated_at
	`

	var venue Venue
	err = tx.QueryRow(
		ctx,
		query,
		params.OwnerProfileID,
		params.Name,
		params.Description,
		params.Address,
		params.District,
		params.City,
		params.Province,
		params.PostalCode,
		params.Latitude,
		params.Longitude,
	).Scan(
		&venue.ID,
		&venue.OwnerProfileID,
		&venue.Name,
		&venue.Description,
		&venue.Address,
		&venue.District,
		&venue.City,
		&venue.Province,
		&venue.PostalCode,
		&venue.Latitude,
		&venue.Longitude,
		&venue.Status,
		&venue.CreatedAt,
		&venue.UpdatedAt,
	)
	if err != nil {
		return Venue{}, err
	}

	if err := replaceVenueFacilities(ctx, tx, venue.ID, facilityIDs); err != nil {
		return Venue{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func (r *Repository) ListPublicVenues(ctx context.Context, limit, offset int) ([]Venue, error) {
	query := `
		SELECT
			id::text,
			owner_profile_id::text,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude,
			status::text,
			created_at,
			updated_at
		FROM venues
		WHERE status = 'ACTIVE'
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var venues []Venue
	for rows.Next() {
		venue, err := scanVenue(rows)
		if err != nil {
			return nil, err
		}
		venues = append(venues, venue)
	}

	return venues, rows.Err()
}

func (r *Repository) FindPublicVenueByID(ctx context.Context, id string) (Venue, error) {
	query := `
		SELECT
			id::text,
			owner_profile_id::text,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude,
			status::text,
			created_at,
			updated_at
		FROM venues
		WHERE id = $1 AND status = 'ACTIVE'
		LIMIT 1
	`

	var venue Venue
	err := r.db.QueryRow(ctx, query, id).Scan(
		&venue.ID,
		&venue.OwnerProfileID,
		&venue.Name,
		&venue.Description,
		&venue.Address,
		&venue.District,
		&venue.City,
		&venue.Province,
		&venue.PostalCode,
		&venue.Latitude,
		&venue.Longitude,
		&venue.Status,
		&venue.CreatedAt,
		&venue.UpdatedAt,
	)
	if err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func (r *Repository) FindActiveCourtsByVenueID(ctx context.Context, venueID string) ([]Court, error) {
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
		JOIN sports s ON s.id = c.sport_id
		WHERE c.venue_id = $1 AND c.status = 'ACTIVE'
		ORDER BY c.created_at DESC
	`

	rows, err := r.db.Query(ctx, query, venueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courts []Court
	for rows.Next() {
		var court Court
		err := rows.Scan(
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
			return nil, err
		}
		courts = append(courts, court)
	}

	return courts, rows.Err()
}

func (r *Repository) ListByOwnerProfileID(ctx context.Context, ownerProfileID string) ([]Venue, error) {
	query := `
		SELECT
			id::text,
			owner_profile_id::text,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude,
			status::text,
			created_at,
			updated_at
		FROM venues
		WHERE owner_profile_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, ownerProfileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var venues []Venue
	for rows.Next() {
		venue, err := scanVenue(rows)
		if err != nil {
			return nil, err
		}
		venues = append(venues, venue)
	}

	return venues, rows.Err()
}

func (r *Repository) FindByIDAndOwnerProfileID(ctx context.Context, id, ownerProfileID string) (Venue, error) {
	query := `
		SELECT
			id::text,
			owner_profile_id::text,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude,
			status::text,
			created_at,
			updated_at
		FROM venues
		WHERE id = $1
			AND owner_profile_id = $2
		LIMIT 1
	`

	var venue Venue
	err := r.db.QueryRow(ctx, query, id, ownerProfileID).Scan(
		&venue.ID,
		&venue.OwnerProfileID,
		&venue.Name,
		&venue.Description,
		&venue.Address,
		&venue.District,
		&venue.City,
		&venue.Province,
		&venue.PostalCode,
		&venue.Latitude,
		&venue.Longitude,
		&venue.Status,
		&venue.CreatedAt,
		&venue.UpdatedAt,
	)
	if err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func (r *Repository) UpdateByIDAndOwnerProfileID(ctx context.Context, id string, params VenueParams, facilityIDs []string) (Venue, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Venue{}, err
	}
	defer tx.Rollback(ctx)

	query := `
		UPDATE venues
		SET name = $3,
			description = $4,
			address = $5,
			district = $6,
			city = $7,
			province = $8,
			postal_code = $9,
			latitude = $10,
			longitude = $11,
			updated_at = now()
		WHERE id = $1
			AND owner_profile_id = $2
		RETURNING
			id::text,
			owner_profile_id::text,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude,
			status::text,
			created_at,
			updated_at
	`

	var venue Venue
	err = tx.QueryRow(
		ctx,
		query,
		id,
		params.OwnerProfileID,
		params.Name,
		params.Description,
		params.Address,
		params.District,
		params.City,
		params.Province,
		params.PostalCode,
		params.Latitude,
		params.Longitude,
	).Scan(
		&venue.ID,
		&venue.OwnerProfileID,
		&venue.Name,
		&venue.Description,
		&venue.Address,
		&venue.District,
		&venue.City,
		&venue.Province,
		&venue.PostalCode,
		&venue.Latitude,
		&venue.Longitude,
		&venue.Status,
		&venue.CreatedAt,
		&venue.UpdatedAt,
	)
	if err != nil {
		return Venue{}, err
	}

	if err := replaceVenueFacilities(ctx, tx, venue.ID, facilityIDs); err != nil {
		return Venue{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func (r *Repository) UpdateStatusByIDAndOwnerProfileID(ctx context.Context, id, ownerProfileID, status string) (Venue, error) {
	query := `
		UPDATE venues
		SET status = $3,
			updated_at = now()
		WHERE id = $1
			AND owner_profile_id = $2
		RETURNING
			id::text,
			owner_profile_id::text,
			name,
			description,
			address,
			district,
			city,
			province,
			postal_code,
			latitude,
			longitude,
			status::text,
			created_at,
			updated_at
	`

	var venue Venue
	err := r.db.QueryRow(ctx, query, id, ownerProfileID, status).Scan(
		&venue.ID,
		&venue.OwnerProfileID,
		&venue.Name,
		&venue.Description,
		&venue.Address,
		&venue.District,
		&venue.City,
		&venue.Province,
		&venue.PostalCode,
		&venue.Latitude,
		&venue.Longitude,
		&venue.Status,
		&venue.CreatedAt,
		&venue.UpdatedAt,
	)
	if err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func (r *Repository) FindFacilitiesByIDs(ctx context.Context, ids []string) ([]Facility, error) {
	if len(ids) == 0 {
		return []Facility{}, nil
	}

	query := `
		SELECT id::text, name, icon
		FROM facilities
		WHERE id::text = ANY($1)
		ORDER BY name ASC
	`

	rows, err := r.db.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFacilities(rows)
}

func (r *Repository) FindFacilitiesByVenueID(ctx context.Context, venueID string) ([]Facility, error) {
	query := `
		SELECT f.id::text, f.name, f.icon
		FROM venue_facilities vf
		JOIN facilities f ON f.id = vf.facility_id
		WHERE vf.venue_id = $1
		ORDER BY f.name ASC
	`

	rows, err := r.db.Query(ctx, query, venueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFacilities(rows)
}

func (r *Repository) FindFacilitiesByVenueIDs(ctx context.Context, venueIDs []string) (map[string][]Facility, error) {
	if len(venueIDs) == 0 {
		return make(map[string][]Facility), nil
	}

	query := `
		SELECT vf.venue_id::text, f.id::text, f.name, f.icon
		FROM venue_facilities vf
		JOIN facilities f ON f.id = vf.facility_id
		WHERE vf.venue_id = ANY($1::uuid[])
		ORDER BY vf.venue_id, f.name ASC
	`

	rows, err := r.db.Query(ctx, query, venueIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	facilitiesMap := make(map[string][]Facility)
	for rows.Next() {
		var venueID string
		var facility Facility
		if err := rows.Scan(&venueID, &facility.ID, &facility.Name, &facility.Icon); err != nil {
			return nil, err
		}
		facilitiesMap[venueID] = append(facilitiesMap[venueID], facility)
	}

	return facilitiesMap, rows.Err()
}

func replaceVenueFacilities(ctx context.Context, tx pgx.Tx, venueID string, facilityIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM venue_facilities WHERE venue_id = $1`, venueID); err != nil {
		return err
	}

	if len(facilityIDs) == 0 {
		return nil
	}

	_, err := tx.Exec(
		ctx,
		`
			INSERT INTO venue_facilities (venue_id, facility_id)
			SELECT $1, id
			FROM facilities
			WHERE id::text = ANY($2)
		`,
		venueID,
		facilityIDs,
	)
	return err
}

func scanVenue(row pgx.Row) (Venue, error) {
	var venue Venue
	err := row.Scan(
		&venue.ID,
		&venue.OwnerProfileID,
		&venue.Name,
		&venue.Description,
		&venue.Address,
		&venue.District,
		&venue.City,
		&venue.Province,
		&venue.PostalCode,
		&venue.Latitude,
		&venue.Longitude,
		&venue.Status,
		&venue.CreatedAt,
		&venue.UpdatedAt,
	)
	if err != nil {
		return Venue{}, err
	}

	return venue, nil
}

func scanFacilities(rows pgx.Rows) ([]Facility, error) {
	var facilities []Facility
	for rows.Next() {
		var facility Facility
		if err := rows.Scan(&facility.ID, &facility.Name, &facility.Icon); err != nil {
			return nil, err
		}
		facilities = append(facilities, facility)
	}

	return facilities, rows.Err()
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
