package promos

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	CreatePromo(ctx context.Context, p Promo) (Promo, error)
	ListOwnerPromos(ctx context.Context, ownerID string) ([]Promo, error)
	GetPromoByIDAndOwner(ctx context.Context, id, ownerID string) (Promo, error)
	UpdatePromo(ctx context.Context, id, ownerID string, params UpdatePromoParams) (Promo, error)
	FindActivePromoByCode(ctx context.Context, ownerID, code string) (Promo, error)
	IsVenueOwnedByOwner(ctx context.Context, ownerUserID, venueID string) (bool, error)
	GetCourtValidationInfo(ctx context.Context, courtID string) (CourtValidationInfo, error)
	DeletePromo(ctx context.Context, id, ownerID string) error
}

type CourtValidationInfo struct {
	PricePerHour float64
	OwnerUserID  string
	VenueID      string
}

type Promo struct {
	ID            string
	OwnerID       string
	VenueID       *string
	Code          string
	Name          string
	Description   *string
	DiscountType  string
	DiscountValue float64
	StartsAt      time.Time
	EndsAt        time.Time
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UpdatePromoParams struct {
	Name          *string
	Description   *string
	DiscountType  *string
	DiscountValue *float64
	StartsAt      *time.Time
	EndsAt        *time.Time
	Status        *string
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) CreatePromo(ctx context.Context, p Promo) (Promo, error) {
	query := `
		INSERT INTO owner_promos (owner_id, venue_id, code, name, description, discount_type, discount_value, starts_at, ends_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, owner_id, venue_id, code, name, description, discount_type, discount_value, starts_at, ends_at, status, created_at, updated_at
	`
	var created Promo
	err := r.db.QueryRow(ctx, query,
		p.OwnerID, p.VenueID, p.Code, p.Name, p.Description, p.DiscountType, p.DiscountValue, p.StartsAt, p.EndsAt, p.Status,
	).Scan(
		&created.ID, &created.OwnerID, &created.VenueID, &created.Code, &created.Name, &created.Description,
		&created.DiscountType, &created.DiscountValue, &created.StartsAt, &created.EndsAt, &created.Status, &created.CreatedAt, &created.UpdatedAt,
	)
	if err != nil {
		return Promo{}, err
	}
	return created, nil
}

func (r *repository) ListOwnerPromos(ctx context.Context, ownerID string) ([]Promo, error) {
	query := `
		SELECT id, owner_id, venue_id, code, name, description, discount_type, discount_value, starts_at, ends_at, status, created_at, updated_at
		FROM owner_promos
		WHERE owner_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	promos := make([]Promo, 0)
	for rows.Next() {
		var p Promo
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description, &p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		promos = append(promos, p)
	}
	return promos, rows.Err()
}

func (r *repository) GetPromoByIDAndOwner(ctx context.Context, id, ownerID string) (Promo, error) {
	query := `
		SELECT id, owner_id, venue_id, code, name, description, discount_type, discount_value, starts_at, ends_at, status, created_at, updated_at
		FROM owner_promos
		WHERE id = $1 AND owner_id = $2
	`
	var p Promo
	err := r.db.QueryRow(ctx, query, id, ownerID).Scan(
		&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Promo{}, err
	}
	return p, nil
}

func (r *repository) UpdatePromo(ctx context.Context, id, ownerID string, params UpdatePromoParams) (Promo, error) {
	query := `
		UPDATE owner_promos
		SET
			name = COALESCE($1, name),
			description = COALESCE($2, description),
			discount_type = COALESCE($3, discount_type),
			discount_value = COALESCE($4, discount_value),
			starts_at = COALESCE($5, starts_at),
			ends_at = COALESCE($6, ends_at),
			status = COALESCE($7, status),
			updated_at = NOW()
		WHERE id = $8 AND owner_id = $9
		RETURNING id, owner_id, venue_id, code, name, description, discount_type, discount_value, starts_at, ends_at, status, created_at, updated_at
	`
	var p Promo
	err := r.db.QueryRow(ctx, query,
		params.Name, params.Description, params.DiscountType, params.DiscountValue, params.StartsAt, params.EndsAt, params.Status, id, ownerID,
	).Scan(
		&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Promo{}, err
	}
	return p, nil
}

func (r *repository) FindActivePromoByCode(ctx context.Context, ownerID, code string) (Promo, error) {
	query := `
		SELECT id, owner_id, venue_id, code, name, description, discount_type, discount_value, starts_at, ends_at, status, created_at, updated_at
		FROM owner_promos
		WHERE owner_id = $1 AND UPPER(code) = UPPER($2) AND status = 'ACTIVE'
	`
	var p Promo
	err := r.db.QueryRow(ctx, query, ownerID, code).Scan(
		&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Promo{}, err
	}
	return p, nil
}

func (r *repository) IsVenueOwnedByOwner(ctx context.Context, ownerUserID, venueID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM venues v
			JOIN owner_profiles op ON op.id = v.owner_profile_id
			WHERE v.id = $1 AND op.user_id = $2
		)
	`
	var exists bool
	err := r.db.QueryRow(ctx, query, venueID, ownerUserID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *repository) GetCourtValidationInfo(ctx context.Context, courtID string) (CourtValidationInfo, error) {
	query := `
		SELECT
			c.price_per_hour,
			op.user_id,
			v.id
		FROM courts c
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON op.id = v.owner_profile_id
		WHERE c.id = $1
	`
	var info CourtValidationInfo
	err := r.db.QueryRow(ctx, query, courtID).Scan(&info.PricePerHour, &info.OwnerUserID, &info.VenueID)
	if err != nil {
		return CourtValidationInfo{}, err
	}
	return info, nil
}

func (r *repository) DeletePromo(ctx context.Context, id, ownerID string) error {
	query := `
		DELETE FROM owner_promos
		WHERE id = $1 AND owner_id = $2
	`
	cmd, err := r.db.Exec(ctx, query, id, ownerID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
