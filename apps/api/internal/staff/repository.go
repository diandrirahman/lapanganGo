package staff

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

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type CreateStaffParams struct {
	OwnerProfileID  string
	Name            string
	Email           string
	Phone           *string
	PasswordHash    string
	Role            string
	Permissions     []string
	VenueIDs        []string
	CreatedByUserID string
	InviteTokenHash string
	InviteExpiresAt time.Time
}

func (r *Repository) CreateStaff(ctx context.Context, params CreateStaffParams) (StaffResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return StaffResponse{}, err
	}
	defer tx.Rollback(ctx)

	params.VenueIDs = normalizeVenueIDs(params.VenueIDs)
	if err := validateVenueOwnership(ctx, tx, params.OwnerProfileID, params.VenueIDs); err != nil {
		return StaffResponse{}, err
	}

	// 1. Create User
	var userID string
	err = tx.QueryRow(
		ctx,
		`INSERT INTO users (name, email, phone, password_hash, role, status)
		 VALUES ($1, $2, $3, $4, 'STAFF', 'ACTIVE')
		 RETURNING id::text`,
		params.Name,
		params.Email,
		params.Phone,
		params.PasswordHash,
	).Scan(&userID)
	if err != nil {
		return StaffResponse{}, err
	}

	// 2. Create Staff Membership
	var staffID string
	var createdAt, updatedAt time.Time
	var invitedAt time.Time
	err = tx.QueryRow(
		ctx,
		`INSERT INTO owner_staff_members (owner_profile_id, user_id, role, permissions, status, created_by_user_id, invitation_status, invited_at)
		 VALUES ($1, $2, $3, $4, 'ACTIVE', $5, 'INVITED', now())
		 RETURNING id::text, created_at, updated_at, invited_at`,
		params.OwnerProfileID,
		userID,
		params.Role,
		params.Permissions,
		params.CreatedByUserID,
	).Scan(&staffID, &createdAt, &updatedAt, &invitedAt)
	if err != nil {
		return StaffResponse{}, err
	}

	// 3. Create Venue Access
	for _, venueID := range params.VenueIDs {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO owner_staff_venue_access (staff_member_id, venue_id) VALUES ($1, $2)`,
			staffID,
			venueID,
		)
		if err != nil {
			return StaffResponse{}, err
		}
	}

	if params.InviteTokenHash != "" {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO owner_staff_invites (staff_member_id, owner_profile_id, staff_user_id, token_hash, purpose, expires_at, created_by_user_id)
			 VALUES ($1, $2, $3, $4, 'SET_PASSWORD', $5, $6)`,
			staffID,
			params.OwnerProfileID,
			userID,
			params.InviteTokenHash,
			params.InviteExpiresAt,
			params.CreatedByUserID,
		)
		if err != nil {
			return StaffResponse{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return StaffResponse{}, err
	}

	if params.Permissions == nil {
		params.Permissions = []string{}
	}
	if params.VenueIDs == nil {
		params.VenueIDs = []string{}
	}

	return StaffResponse{
		ID:               staffID,
		OwnerProfileID:   params.OwnerProfileID,
		UserID:           userID,
		Name:             params.Name,
		Email:            params.Email,
		Phone:            params.Phone,
		Role:             params.Role,
		Permissions:      params.Permissions,
		Status:           "ACTIVE",
		InvitationStatus: "INVITED",
		VenueIDs:         params.VenueIDs,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		InvitedAt:        &invitedAt,
	}, nil
}

func (r *Repository) ListStaffByOwner(ctx context.Context, ownerProfileID string) ([]StaffResponse, error) {
	query := `
		SELECT 
			m.id::text, 
			m.owner_profile_id::text, 
			m.user_id::text, 
			u.name, 
			u.email, 
			u.phone, 
			m.role::text, 
			m.permissions::text[], 
			m.status::text,
			m.invitation_status::text,
			m.created_at, 
			m.updated_at,
			m.invited_at,
			m.activated_at,
			COALESCE(array_agg(v.venue_id::text) FILTER (WHERE v.venue_id IS NOT NULL), '{}') as venue_ids
		FROM owner_staff_members m
		JOIN users u ON m.user_id = u.id
		LEFT JOIN owner_staff_venue_access v ON m.id = v.staff_member_id
		WHERE m.owner_profile_id = $1
		GROUP BY m.id, u.id
		ORDER BY m.created_at DESC
	`

	rows, err := r.db.Query(ctx, query, ownerProfileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var staffList []StaffResponse
	for rows.Next() {
		var s StaffResponse
		var permissions []string
		var venueIDs []string
		err := rows.Scan(
			&s.ID,
			&s.OwnerProfileID,
			&s.UserID,
			&s.Name,
			&s.Email,
			&s.Phone,
			&s.Role,
			&permissions,
			&s.Status,
			&s.InvitationStatus,
			&s.CreatedAt,
			&s.UpdatedAt,
			&s.InvitedAt,
			&s.ActivatedAt,
			&venueIDs,
		)
		if err != nil {
			return nil, err
		}
		s.Permissions = permissions
		s.VenueIDs = venueIDs
		if s.Permissions == nil {
			s.Permissions = []string{}
		}
		if s.VenueIDs == nil {
			s.VenueIDs = []string{}
		}
		staffList = append(staffList, s)
	}

	return staffList, nil
}

func (r *Repository) GetStaffByID(ctx context.Context, ownerProfileID, staffID string) (StaffResponse, error) {
	query := `
		SELECT 
			m.id::text, 
			m.owner_profile_id::text, 
			m.user_id::text, 
			u.name, 
			u.email, 
			u.phone, 
			m.role::text, 
			m.permissions::text[], 
			m.status::text,
			m.invitation_status::text,
			m.created_at, 
			m.updated_at,
			m.invited_at,
			m.activated_at,
			COALESCE(array_agg(v.venue_id::text) FILTER (WHERE v.venue_id IS NOT NULL), '{}') as venue_ids
		FROM owner_staff_members m
		JOIN users u ON m.user_id = u.id
		LEFT JOIN owner_staff_venue_access v ON m.id = v.staff_member_id
		WHERE m.owner_profile_id = $1 AND m.id = $2
		GROUP BY m.id, u.id
	`

	var s StaffResponse
	var permissions []string
	var venueIDs []string
	err := r.db.QueryRow(ctx, query, ownerProfileID, staffID).Scan(
		&s.ID,
		&s.OwnerProfileID,
		&s.UserID,
		&s.Name,
		&s.Email,
		&s.Phone,
		&s.Role,
		&permissions,
		&s.Status,
		&s.InvitationStatus,
		&s.CreatedAt,
		&s.UpdatedAt,
		&s.InvitedAt,
		&s.ActivatedAt,
		&venueIDs,
	)
	if err != nil {
		return StaffResponse{}, err
	}

	s.Permissions = permissions
	s.VenueIDs = venueIDs
	if s.Permissions == nil {
		s.Permissions = []string{}
	}
	if s.VenueIDs == nil {
		s.VenueIDs = []string{}
	}
	return s, nil
}

type UpdateStaffParams struct {
	ID             string
	OwnerProfileID string
	Name           string
	Phone          *string
	Role           string
	Permissions    []string
	VenueIDs       []string
}

func (r *Repository) UpdateStaff(ctx context.Context, params UpdateStaffParams) (StaffResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return StaffResponse{}, err
	}
	defer tx.Rollback(ctx)

	params.VenueIDs = normalizeVenueIDs(params.VenueIDs)

	// Update user info
	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM owner_staff_members WHERE id = $1 AND owner_profile_id = $2`, params.ID, params.OwnerProfileID).Scan(&userID)
	if err != nil {
		return StaffResponse{}, err
	}

	if err := validateVenueOwnership(ctx, tx, params.OwnerProfileID, params.VenueIDs); err != nil {
		return StaffResponse{}, err
	}

	_, err = tx.Exec(ctx, `UPDATE users SET name = $1, phone = $2 WHERE id = $3`, params.Name, params.Phone, userID)
	if err != nil {
		return StaffResponse{}, err
	}

	// Update membership info
	_, err = tx.Exec(
		ctx,
		`UPDATE owner_staff_members SET role = $1, permissions = $2, updated_at = now() WHERE id = $3`,
		params.Role,
		params.Permissions,
		params.ID,
	)
	if err != nil {
		return StaffResponse{}, err
	}

	// Update venue access
	_, err = tx.Exec(ctx, `DELETE FROM owner_staff_venue_access WHERE staff_member_id = $1`, params.ID)
	if err != nil {
		return StaffResponse{}, err
	}

	for _, venueID := range params.VenueIDs {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO owner_staff_venue_access (staff_member_id, venue_id) VALUES ($1, $2)`,
			params.ID,
			venueID,
		)
		if err != nil {
			return StaffResponse{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return StaffResponse{}, err
	}

	return r.GetStaffByID(ctx, params.OwnerProfileID, params.ID)
}

func (r *Repository) UpdateStatus(ctx context.Context, ownerProfileID, staffID, status string) (StaffResponse, error) {
	_, err := r.db.Exec(
		ctx,
		`UPDATE owner_staff_members SET status = $1, updated_at = now() WHERE id = $2 AND owner_profile_id = $3`,
		status,
		staffID,
		ownerProfileID,
	)
	if err != nil {
		return StaffResponse{}, err
	}
	return r.GetStaffByID(ctx, ownerProfileID, staffID)
}

func (r *Repository) UpdateVenues(ctx context.Context, ownerProfileID, staffID string, venueIDs []string) (StaffResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return StaffResponse{}, err
	}
	defer tx.Rollback(ctx)

	venueIDs = normalizeVenueIDs(venueIDs)

	// Ensure staff belongs to this owner
	var count int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM owner_staff_members WHERE id = $1 AND owner_profile_id = $2`, staffID, ownerProfileID).Scan(&count)
	if err != nil {
		return StaffResponse{}, err
	}
	if count == 0 {
		return StaffResponse{}, pgx.ErrNoRows
	}

	if err := validateVenueOwnership(ctx, tx, ownerProfileID, venueIDs); err != nil {
		return StaffResponse{}, err
	}

	_, err = tx.Exec(ctx, `DELETE FROM owner_staff_venue_access WHERE staff_member_id = $1`, staffID)
	if err != nil {
		return StaffResponse{}, err
	}

	for _, venueID := range venueIDs {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO owner_staff_venue_access (staff_member_id, venue_id) VALUES ($1, $2)`,
			staffID,
			venueID,
		)
		if err != nil {
			return StaffResponse{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return StaffResponse{}, err
	}

	return r.GetStaffByID(ctx, ownerProfileID, staffID)
}

func normalizeVenueIDs(venueIDs []string) []string {
	if len(venueIDs) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(venueIDs))
	normalized := make([]string, 0, len(venueIDs))
	for _, venueID := range venueIDs {
		if venueID == "" {
			continue
		}
		if _, ok := seen[venueID]; ok {
			continue
		}
		seen[venueID] = struct{}{}
		normalized = append(normalized, venueID)
	}
	return normalized
}

func validateVenueOwnership(ctx context.Context, tx pgx.Tx, ownerProfileID string, venueIDs []string) error {
	if len(venueIDs) == 0 {
		return nil
	}

	var count int
	err := tx.QueryRow(
		ctx,
		`SELECT count(*) FROM venues WHERE owner_profile_id = $1 AND id = ANY($2::uuid[])`,
		ownerProfileID,
		venueIDs,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count != len(venueIDs) {
		return ErrInvalidVenueAccess
	}
	return nil
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

func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
