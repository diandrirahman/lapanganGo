package venues

import (
	"context"
	"errors"
	"strconv"
	"strings"
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

type VenuePhoto struct {
	ID        string
	VenueID   string
	ImageURL  string
	AltText   *string
	SortOrder int
	IsPrimary bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
type Facility struct {
	ID   string
	Name string
	Icon *string
}

type PromoSummary struct {
	ID            string
	Code          string
	Name          string
	DiscountType  string
	DiscountValue float64
	StartsAt      time.Time
	EndsAt        time.Time
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

func (r *Repository) GetSports(ctx context.Context) ([]Sport, error) {
	query := `SELECT id::text, name FROM sports ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sports []Sport
	for rows.Next() {
		var s Sport
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		sports = append(sports, s)
	}

	return sports, rows.Err()
}

func (r *Repository) GetFacilities(ctx context.Context) ([]Facility, error) {
	query := `SELECT id::text, name, icon FROM facilities ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFacilities(rows)
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

func (r *Repository) ListPublicVenues(ctx context.Context, filter ListPublicVenuesQuery, offset int) ([]Venue, int, error) {
	// 1. Build the conditions and args
	var args []interface{}
	var conditions []string

	conditions = append(conditions, "v.status = 'ACTIVE'")

	if filter.Q != "" {
		args = append(args, "%"+filter.Q+"%")
		n := strconv.Itoa(len(args))
		conditions = append(conditions, "(v.name ILIKE $"+n+" OR v.city ILIKE $"+n+" OR v.address ILIKE $"+n+")")
	}

	if filter.City != "" {
		args = append(args, "%"+filter.City+"%")
		conditions = append(conditions, "v.city ILIKE $"+strconv.Itoa(len(args)))
	}

	needsCourtJoin := filter.SportID != "" || filter.MinPrice > 0 || filter.MaxPrice > 0
	joins := ""
	if needsCourtJoin {
		joins += " JOIN courts c ON c.venue_id = v.id"
		conditions = append(conditions, "c.status = 'ACTIVE'")

		if filter.SportID != "" {
			args = append(args, filter.SportID)
			conditions = append(conditions, "c.sport_id = $"+strconv.Itoa(len(args)))
		}

		if filter.MinPrice > 0 {
			args = append(args, filter.MinPrice)
			conditions = append(conditions, "c.price_per_hour >= $"+strconv.Itoa(len(args)))
		}

		if filter.MaxPrice > 0 {
			args = append(args, filter.MaxPrice)
			conditions = append(conditions, "c.price_per_hour <= $"+strconv.Itoa(len(args)))
		}
	}

	if len(filter.FacilityIDs) > 0 {
		joins += " JOIN venue_facilities vf ON vf.venue_id = v.id"
		args = append(args, filter.FacilityIDs)
		conditions = append(conditions, "vf.facility_id = ANY($"+strconv.Itoa(len(args))+")")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 2. Count total records
	countQuery := "SELECT COUNT(DISTINCT v.id) FROM venues v" + joins + whereClause
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 3. Query the data
	query := `
		SELECT DISTINCT
			v.id::text,
			v.owner_profile_id::text,
			v.name,
			v.description,
			v.address,
			v.district,
			v.city,
			v.province,
			v.postal_code,
			v.latitude,
			v.longitude,
			v.status::text,
			v.created_at,
			v.updated_at
		FROM venues v
	` + joins + whereClause + " ORDER BY v.created_at DESC"
	dataArgs := append([]interface{}{}, args...)
	dataArgs = append(dataArgs, filter.Limit)
	query += " LIMIT $" + strconv.Itoa(len(dataArgs))

	dataArgs = append(dataArgs, offset)
	query += " OFFSET $" + strconv.Itoa(len(dataArgs))

	rows, err := r.db.Query(ctx, query, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var venues []Venue
	for rows.Next() {
		venue, err := scanVenue(rows)
		if err != nil {
			return nil, 0, err
		}
		venues = append(venues, venue)
	}

	return venues, total, nil
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
			AND status != 'SUSPENDED'
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
			AND status != 'SUSPENDED'
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

func (r *Repository) FindActivePromosByVenueIDs(ctx context.Context, venueIDs []string, playDate string) (map[string][]PromoSummary, error) {
	if len(venueIDs) == 0 {
		return make(map[string][]PromoSummary), nil
	}

	var query string
	var args []interface{}

	if playDate != "" {
		query = `
			SELECT
				v.id::text,
				op.id::text,
				op.code,
				op.name,
				op.discount_type,
				op.discount_value,
				op.starts_at,
				op.ends_at
			FROM venues v
			JOIN owner_profiles owner ON owner.id = v.owner_profile_id
			JOIN owner_promos op ON op.owner_id = owner.user_id
			WHERE v.id = ANY($1::uuid[])
				AND op.status = 'ACTIVE'
				AND DATE(op.starts_at AT TIME ZONE 'Asia/Jakarta') <= $2::date
				AND DATE(op.ends_at AT TIME ZONE 'Asia/Jakarta') >= $2::date
				AND (op.venue_id IS NULL OR op.venue_id = v.id)
			ORDER BY v.id, op.created_at ASC
		`
		args = []interface{}{venueIDs, playDate}
	} else {
		query = `
			SELECT
				v.id::text,
				op.id::text,
				op.code,
				op.name,
				op.discount_type,
				op.discount_value,
				op.starts_at,
				op.ends_at
			FROM venues v
			JOIN owner_profiles owner ON owner.id = v.owner_profile_id
			JOIN owner_promos op ON op.owner_id = owner.user_id
			WHERE v.id = ANY($1::uuid[])
				AND op.status = 'ACTIVE'
				AND DATE(op.ends_at AT TIME ZONE 'Asia/Jakarta') >= DATE(now() AT TIME ZONE 'Asia/Jakarta')
				AND (op.venue_id IS NULL OR op.venue_id = v.id)
			ORDER BY v.id, op.created_at ASC
		`
		args = []interface{}{venueIDs}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	promosMap := make(map[string][]PromoSummary)
	for rows.Next() {
		var venueID string
		var p PromoSummary
		if err := rows.Scan(
			&venueID,
			&p.ID,
			&p.Code,
			&p.Name,
			&p.DiscountType,
			&p.DiscountValue,
			&p.StartsAt,
			&p.EndsAt,
		); err != nil {
			return nil, err
		}
		promosMap[venueID] = append(promosMap[venueID], p)
	}

	return promosMap, rows.Err()
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

func (r *Repository) GetVenuePhotos(ctx context.Context, venueID string) ([]VenuePhoto, error) {
	query := `
		SELECT id::text, venue_id::text, image_url, alt_text, sort_order, is_primary, created_at, updated_at
		FROM venue_photos
		WHERE venue_id = $1
		ORDER BY sort_order ASC, created_at DESC
	`
	rows, err := r.db.Query(ctx, query, venueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []VenuePhoto
	for rows.Next() {
		var p VenuePhoto
		if err := rows.Scan(&p.ID, &p.VenueID, &p.ImageURL, &p.AltText, &p.SortOrder, &p.IsPrimary, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

func (r *Repository) FindPhotosByVenueIDs(ctx context.Context, venueIDs []string) (map[string][]VenuePhoto, error) {
	if len(venueIDs) == 0 {
		return make(map[string][]VenuePhoto), nil
	}

	query := `
		SELECT id::text, venue_id::text, image_url, alt_text, sort_order, is_primary, created_at, updated_at
		FROM venue_photos
		WHERE venue_id = ANY($1)
		ORDER BY sort_order ASC, created_at DESC
	`
	rows, err := r.db.Query(ctx, query, venueIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	photosMap := make(map[string][]VenuePhoto)
	for rows.Next() {
		var p VenuePhoto
		if err := rows.Scan(&p.ID, &p.VenueID, &p.ImageURL, &p.AltText, &p.SortOrder, &p.IsPrimary, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		photosMap[p.VenueID] = append(photosMap[p.VenueID], p)
	}
	return photosMap, rows.Err()
}

func (r *Repository) GetVenuePhotoByID(ctx context.Context, id string) (VenuePhoto, error) {
	query := `
		SELECT id::text, venue_id::text, image_url, alt_text, sort_order, is_primary, created_at, updated_at
		FROM venue_photos
		WHERE id = $1
	`
	var p VenuePhoto
	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.VenueID, &p.ImageURL, &p.AltText, &p.SortOrder, &p.IsPrimary, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func (r *Repository) AddVenuePhoto(ctx context.Context, p VenuePhoto) (VenuePhoto, error) {
	if p.IsPrimary {
		tx, err := r.db.Begin(ctx)
		if err != nil {
			return p, err
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `UPDATE venue_photos SET is_primary = false, updated_at = now() WHERE venue_id = $1 AND is_primary = true`, p.VenueID)
		if err != nil {
			return p, err
		}

		query := `
			INSERT INTO venue_photos (venue_id, image_url, alt_text, sort_order, is_primary)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id::text, created_at, updated_at
		`
		err = tx.QueryRow(ctx, query, p.VenueID, p.ImageURL, p.AltText, p.SortOrder, p.IsPrimary).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return p, err
		}

		err = tx.Commit(ctx)
		return p, err
	}

	query := `
		INSERT INTO venue_photos (venue_id, image_url, alt_text, sort_order, is_primary)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query, p.VenueID, p.ImageURL, p.AltText, p.SortOrder, p.IsPrimary).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *Repository) UpdateVenuePhoto(ctx context.Context, p VenuePhoto) error {
	if p.IsPrimary {
		tx, err := r.db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `UPDATE venue_photos SET is_primary = false, updated_at = now() WHERE venue_id = $1 AND is_primary = true AND id != $2`, p.VenueID, p.ID)
		if err != nil {
			return err
		}

		query := `
			UPDATE venue_photos
			SET alt_text = $1, sort_order = $2, is_primary = $3, updated_at = now()
			WHERE id = $4
		`
		tag, err := tx.Exec(ctx, query, p.AltText, p.SortOrder, p.IsPrimary, p.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return tx.Commit(ctx)
	}

	query := `
		UPDATE venue_photos
		SET alt_text = $1, sort_order = $2, is_primary = $3, updated_at = now()
		WHERE id = $4
	`
	tag, err := r.db.Exec(ctx, query, p.AltText, p.SortOrder, p.IsPrimary, p.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteVenuePhoto(ctx context.Context, id string) error {
	query := `DELETE FROM venue_photos WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
