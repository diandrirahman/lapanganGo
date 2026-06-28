package owners

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

type Profile struct {
	ID                 string
	UserID             string
	BusinessName       string
	IdentityNumber     *string
	BankName           *string
	BankAccountNumber  *string
	BankAccountName    *string
	VerificationStatus string
	VerifiedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ProfileParams struct {
	UserID            string
	BusinessName      string
	IdentityNumber    *string
	BankName          *string
	BankAccountNumber *string
	BankAccountName   *string
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, params ProfileParams) (Profile, error) {
	query := `
		INSERT INTO owner_profiles (
			user_id,
			business_name,
			identity_number,
			bank_name,
			bank_account_number,
			bank_account_name
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING
			id::text,
			user_id::text,
			business_name,
			identity_number,
			bank_name,
			bank_account_number,
			bank_account_name,
			verification_status::text,
			verified_at,
			created_at,
			updated_at
	`

	var profile Profile
	err := r.db.QueryRow(
		ctx,
		query,
		params.UserID,
		params.BusinessName,
		params.IdentityNumber,
		params.BankName,
		params.BankAccountNumber,
		params.BankAccountName,
	).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.BusinessName,
		&profile.IdentityNumber,
		&profile.BankName,
		&profile.BankAccountNumber,
		&profile.BankAccountName,
		&profile.VerificationStatus,
		&profile.VerifiedAt,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		return Profile{}, err
	}

	return profile, nil
}

func (r *Repository) FindByUserID(ctx context.Context, userID string) (Profile, error) {
	query := `
		SELECT
			id::text,
			user_id::text,
			business_name,
			identity_number,
			bank_name,
			bank_account_number,
			bank_account_name,
			verification_status::text,
			verified_at,
			created_at,
			updated_at
		FROM owner_profiles
		WHERE user_id = $1
		LIMIT 1
	`

	var profile Profile
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.BusinessName,
		&profile.IdentityNumber,
		&profile.BankName,
		&profile.BankAccountNumber,
		&profile.BankAccountName,
		&profile.VerificationStatus,
		&profile.VerifiedAt,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		return Profile{}, err
	}

	return profile, nil
}

func (r *Repository) UpdateByUserID(ctx context.Context, params ProfileParams) (Profile, error) {
	query := `
		UPDATE owner_profiles
		SET business_name = $2,
			identity_number = $3,
			bank_name = $4,
			bank_account_number = $5,
			bank_account_name = $6,
			updated_at = now()
		WHERE user_id = $1
		RETURNING
			id::text,
			user_id::text,
			business_name,
			identity_number,
			bank_name,
			bank_account_number,
			bank_account_name,
			verification_status::text,
			verified_at,
			created_at,
			updated_at
	`

	var profile Profile
	err := r.db.QueryRow(
		ctx,
		query,
		params.UserID,
		params.BusinessName,
		params.IdentityNumber,
		params.BankName,
		params.BankAccountNumber,
		params.BankAccountName,
	).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.BusinessName,
		&profile.IdentityNumber,
		&profile.BankName,
		&profile.BankAccountNumber,
		&profile.BankAccountName,
		&profile.VerificationStatus,
		&profile.VerifiedAt,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		return Profile{}, err
	}

	return profile, nil
}

func (r *Repository) GetMetrics(ctx context.Context, userID string) (int, int, float64, error) {
	// 1. Get Owner Profile ID
	queryProfile := `SELECT id FROM owner_profiles WHERE user_id = $1 LIMIT 1`
	var ownerProfileID string
	err := r.db.QueryRow(ctx, queryProfile, userID).Scan(&ownerProfileID)
	if err != nil {
		if err == pgx.ErrNoRows {
			// If no profile, metrics are 0
			return 0, 0, 0, nil
		}
		return 0, 0, 0, err
	}

	var totalVenues int
	var activeBookings int
	var totalRevenue float64

	// 2. Count Venues
	queryVenues := `SELECT count(*) FROM venues WHERE owner_profile_id = $1`
	err = r.db.QueryRow(ctx, queryVenues, ownerProfileID).Scan(&totalVenues)
	if err != nil {
		return 0, 0, 0, err
	}

	// 3. Count Active Bookings
	queryActiveBookings := `
		SELECT count(*)
		FROM bookings b
		JOIN courts c ON b.court_id = c.id
		JOIN venues v ON c.venue_id = v.id
		WHERE v.owner_profile_id = $1 AND b.status IN ('PENDING_PAYMENT', 'CONFIRMED')
	`
	err = r.db.QueryRow(ctx, queryActiveBookings, ownerProfileID).Scan(&activeBookings)
	if err != nil {
		return 0, 0, 0, err
	}

	// 4. Calculate Total Revenue
	queryRevenue := `
		SELECT COALESCE(sum(total_price), 0)
		FROM bookings b
		JOIN courts c ON b.court_id = c.id
		JOIN venues v ON c.venue_id = v.id
		WHERE v.owner_profile_id = $1 AND b.status IN ('CONFIRMED', 'PAID')
	`
	err = r.db.QueryRow(ctx, queryRevenue, ownerProfileID).Scan(&totalRevenue)
	if err != nil {
		return 0, 0, 0, err
	}

	return totalVenues, activeBookings, totalRevenue, nil
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
