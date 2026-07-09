package auth

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

type User struct {
	ID           string
	Name         string
	Email        string
	Phone        *string
	PasswordHash string
	Role         string
	Status       string
	CreatedAt    time.Time
}

type CreateUserParams struct {
	Name         string
	Email        string
	Phone        *string
	PasswordHash string
	Role         string
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (User, error) {
	query := `
		INSERT INTO users (name, email, phone, password_hash, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, name, email, phone, password_hash, role::text, status::text, created_at
	`

	var user User
	err := r.db.QueryRow(
		ctx,
		query,
		params.Name,
		params.Email,
		params.Phone,
		params.PasswordHash,
		params.Role,
	).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Phone,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.CreatedAt,
	)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (User, error) {
	query := `
		SELECT id::text, name, email, phone, password_hash, role::text, status::text, created_at
		FROM users
		WHERE email = $1
		LIMIT 1
	`

	var user User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Phone,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.CreatedAt,
	)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func UniqueViolationConstraint(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}

	return pgErr.ConstraintName
}

func (r *Repository) GetOwnerProfile(ctx context.Context, userID string) (*OwnerProfileResponse, error) {
	query := `SELECT id::text, business_name FROM owner_profiles WHERE user_id = $1 LIMIT 1`
	var profile OwnerProfileResponse
	err := r.db.QueryRow(ctx, query, userID).Scan(&profile.ID, &profile.Name)
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *Repository) GetStaffMemberships(ctx context.Context, userID string) ([]StaffMembershipResponse, error) {
	query := `
		SELECT
			m.id::text,
			p.id::text,
			p.business_name,
			m.role::text,
			m.invitation_status::text,
			m.permissions::text[]
		FROM owner_staff_members m
		JOIN owner_profiles p ON m.owner_profile_id = p.id
		WHERE m.user_id = $1 AND m.status = 'ACTIVE'
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []StaffMembershipResponse
	for rows.Next() {
		var m StaffMembershipResponse
		var perms []string
		if err := rows.Scan(&m.ID, &m.OwnerProfileID, &m.OwnerName, &m.Role, &m.InvitationStatus, &perms); err != nil {
			return nil, err
		}
		if perms == nil {
			perms = []string{}
		}
		m.Permissions = perms
		memberships = append(memberships, m)
	}
	if memberships == nil {
		memberships = []StaffMembershipResponse{}
	}
	return memberships, nil
}
