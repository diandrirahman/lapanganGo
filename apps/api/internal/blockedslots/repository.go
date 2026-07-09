package blockedslots

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

type BlockedSlot struct {
	ID        string
	CourtID   string
	StartAt   time.Time
	EndAt     time.Time
	Reason    *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type BlockedSlotParams struct {
	CourtID string
	StartAt time.Time
	EndAt   time.Time
	Reason  *string
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

func (r *Repository) Create(ctx context.Context, params BlockedSlotParams) (BlockedSlot, error) {
	query := `
		INSERT INTO court_blocked_slots (
			court_id,
			start_at,
			end_at,
			reason
		)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id::text,
			court_id::text,
			start_at,
			end_at,
			reason,
			created_at,
			updated_at
	`

	return r.queryBlockedSlot(ctx, query, params.CourtID, params.StartAt, params.EndAt, params.Reason)
}

func (r *Repository) ListByCourtID(ctx context.Context, courtID string, from, to *time.Time) ([]BlockedSlot, error) {
	query := `
		SELECT
			id::text,
			court_id::text,
			start_at,
			end_at,
			reason,
			created_at,
			updated_at
		FROM court_blocked_slots
		WHERE court_id = $1
			AND ($2::timestamptz IS NULL OR end_at > $2)
			AND ($3::timestamptz IS NULL OR start_at < $3)
		ORDER BY start_at ASC
	`

	rows, err := r.db.Query(ctx, query, courtID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blockedSlots []BlockedSlot
	for rows.Next() {
		blockedSlot, err := scanBlockedSlot(rows)
		if err != nil {
			return nil, err
		}
		blockedSlots = append(blockedSlots, blockedSlot)
	}

	return blockedSlots, rows.Err()
}

func (r *Repository) FindByIDAndOwnerProfileID(ctx context.Context, blockedSlotID, ownerProfileID string) (BlockedSlot, error) {
	query := `
		SELECT
			bs.id::text,
			bs.court_id::text,
			bs.start_at,
			bs.end_at,
			bs.reason,
			bs.created_at,
			bs.updated_at
		FROM court_blocked_slots bs
		JOIN courts c ON c.id = bs.court_id
		JOIN venues v ON v.id = c.venue_id
		WHERE bs.id = $1
			AND v.owner_profile_id = $2
		LIMIT 1
	`
	return r.queryBlockedSlot(ctx, query, blockedSlotID, ownerProfileID)
}

func (r *Repository) DeleteByIDAndOwnerProfileID(ctx context.Context, blockedSlotID, ownerProfileID string) (BlockedSlot, error) {
	query := `
		DELETE FROM court_blocked_slots bs
		USING courts c, venues v
		WHERE bs.id = $1
			AND bs.court_id = c.id
			AND c.venue_id = v.id
			AND v.owner_profile_id = $2
		RETURNING
			bs.id::text,
			bs.court_id::text,
			bs.start_at,
			bs.end_at,
			bs.reason,
			bs.created_at,
			bs.updated_at
	`

	return r.queryBlockedSlot(ctx, query, blockedSlotID, ownerProfileID)
}

func (r *Repository) queryBlockedSlot(ctx context.Context, query string, args ...any) (BlockedSlot, error) {
	return scanBlockedSlot(r.db.QueryRow(ctx, query, args...))
}

func scanBlockedSlot(row pgx.Row) (BlockedSlot, error) {
	var blockedSlot BlockedSlot
	err := row.Scan(
		&blockedSlot.ID,
		&blockedSlot.CourtID,
		&blockedSlot.StartAt,
		&blockedSlot.EndAt,
		&blockedSlot.Reason,
		&blockedSlot.CreatedAt,
		&blockedSlot.UpdatedAt,
	)
	if err != nil {
		return BlockedSlot{}, err
	}

	return blockedSlot, nil
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}
