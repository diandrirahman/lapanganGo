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
	ID                    string
	OwnerID               string
	VenueID               *string
	Code                  string
	Name                  string
	Description           *string
	DiscountType          string
	DiscountValue         float64
	StartsAt              time.Time
	EndsAt                time.Time
	Status                string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	UsageCount            int
	TotalDiscountAmount   float64
	TotalFinalRevenue     float64
	BookingReferenceCount int
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
	created.UsageCount = 0
	created.TotalDiscountAmount = 0
	created.TotalFinalRevenue = 0
	created.BookingReferenceCount = 0
	if err != nil {
		return Promo{}, err
	}
	return created, nil
}

func (r *repository) ListOwnerPromos(ctx context.Context, ownerID string) ([]Promo, error) {
	query := `
		SELECT 
			p.id, p.owner_id, p.venue_id, p.code, p.name, p.description, p.discount_type, p.discount_value, p.starts_at, p.ends_at, p.status, p.created_at, p.updated_at,
			COUNT(b.id) FILTER (WHERE b.status <> 'CANCELLED') AS usage_count,
			COALESCE(SUM(b.discount_amount) FILTER (WHERE b.status <> 'CANCELLED'), 0) AS total_discount_amount,
			COALESCE(SUM(b.total_price) FILTER (WHERE b.status IN ('PAID', 'COMPLETED', 'CONFIRMED')), 0) AS total_final_revenue,
			COUNT(b.id) AS booking_reference_count
		FROM owner_promos p
		LEFT JOIN bookings b ON b.promo_id = p.id
		WHERE p.owner_id = $1
		GROUP BY p.id
		ORDER BY p.created_at DESC
	`
	rows, err := r.db.Query(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	promos := make([]Promo, 0)
	for rows.Next() {
		var p Promo
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description, &p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.UsageCount, &p.TotalDiscountAmount, &p.TotalFinalRevenue, &p.BookingReferenceCount); err != nil {
			return nil, err
		}
		promos = append(promos, p)
	}
	return promos, rows.Err()
}

func (r *repository) GetPromoByIDAndOwner(ctx context.Context, id, ownerID string) (Promo, error) {
	query := `
		SELECT 
			p.id, p.owner_id, p.venue_id, p.code, p.name, p.description, p.discount_type, p.discount_value, p.starts_at, p.ends_at, p.status, p.created_at, p.updated_at,
			COUNT(b.id) FILTER (WHERE b.status <> 'CANCELLED') AS usage_count,
			COALESCE(SUM(b.discount_amount) FILTER (WHERE b.status <> 'CANCELLED'), 0) AS total_discount_amount,
			COALESCE(SUM(b.total_price) FILTER (WHERE b.status IN ('PAID', 'COMPLETED', 'CONFIRMED')), 0) AS total_final_revenue,
			COUNT(b.id) AS booking_reference_count
		FROM owner_promos p
		LEFT JOIN bookings b ON b.promo_id = p.id
		WHERE p.id = $1 AND p.owner_id = $2
		GROUP BY p.id
	`
	var p Promo
	err := r.db.QueryRow(ctx, query, id, ownerID).Scan(
		&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt,
		&p.UsageCount, &p.TotalDiscountAmount, &p.TotalFinalRevenue, &p.BookingReferenceCount,
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
		  AND (venue_id IS NULL OR EXISTS (
		      SELECT 1 FROM venues v WHERE v.id = owner_promos.venue_id AND v.status != 'SUSPENDED'
		  ))
		RETURNING id, owner_id, venue_id, code, name, description, discount_type, discount_value, starts_at, ends_at, status, created_at, updated_at
	`
	var p Promo
	err := r.db.QueryRow(ctx, query,
		params.Name, params.Description, params.DiscountType, params.DiscountValue, params.StartsAt, params.EndsAt, params.Status, id, ownerID,
	).Scan(
		&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	p.UsageCount = 0
	p.TotalDiscountAmount = 0
	p.TotalFinalRevenue = 0
	p.BookingReferenceCount = 0
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
		  AND (venue_id IS NULL OR EXISTS (
		      SELECT 1 FROM venues v WHERE v.id = owner_promos.venue_id AND v.status != 'SUSPENDED'
		  ))
	`
	var p Promo
	err := r.db.QueryRow(ctx, query, ownerID, code).Scan(
		&p.ID, &p.OwnerID, &p.VenueID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.StartsAt, &p.EndsAt, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	p.UsageCount = 0
	p.TotalDiscountAmount = 0
	p.TotalFinalRevenue = 0
	p.BookingReferenceCount = 0
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
			WHERE v.id = $1
			  AND op.user_id = $2
			  AND v.status != 'SUSPENDED'
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
		  AND v.status != 'SUSPENDED'
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
		  AND (venue_id IS NULL OR EXISTS (
		      SELECT 1 FROM venues v WHERE v.id = owner_promos.venue_id AND v.status != 'SUSPENDED'
		  ))
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
