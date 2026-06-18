package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
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
