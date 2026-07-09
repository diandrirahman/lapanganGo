package owneraccess

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type OwnerContextInfo struct {
	OwnerProfileID     string
	OwnerUserID        string
	VerificationStatus string
	OwnerUserStatus    string
}

type StaffContextInfo struct {
	StaffMemberID   string
	Role            string
	Permissions     []string
	VenueIDs        []string
	StaffStatus     string
	OwnerProfileID  string
	OwnerUserID     string
	OwnerStatus     string
	StaffUserStatus string
}

func (r *Repository) GetOwnerContextByUserID(ctx context.Context, userID string) (OwnerContextInfo, error) {
	query := `
		SELECT 
			p.id::text, 
			p.user_id::text, 
			p.verification_status::text,
			u.status::text
		FROM owner_profiles p
		JOIN users u ON p.user_id = u.id
		WHERE p.user_id = $1
	`
	var info OwnerContextInfo
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&info.OwnerProfileID,
		&info.OwnerUserID,
		&info.VerificationStatus,
		&info.OwnerUserStatus,
	)
	if err != nil {
		return OwnerContextInfo{}, err
	}
	return info, nil
}

func (r *Repository) GetStaffContextByUserID(ctx context.Context, userID string) (StaffContextInfo, error) {
	query := `
		SELECT 
			m.id::text, 
			m.role::text, 
			m.permissions::text[], 
			m.status::text,
			p.id::text,
			p.user_id::text,
			u.status::text,
			staff_user.status::text,
			COALESCE(array_agg(v.venue_id::text) FILTER (WHERE v.venue_id IS NOT NULL), '{}') as venue_ids
		FROM owner_staff_members m
		JOIN owner_profiles p ON m.owner_profile_id = p.id
		JOIN users u ON p.user_id = u.id
		JOIN users staff_user ON staff_user.id = m.user_id
		LEFT JOIN owner_staff_venue_access v ON m.id = v.staff_member_id
		WHERE m.user_id = $1
		GROUP BY m.id, p.id, u.id, staff_user.id
	`
	var info StaffContextInfo
	var permissions []string
	var venueIDs []string
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&info.StaffMemberID,
		&info.Role,
		&permissions,
		&info.StaffStatus,
		&info.OwnerProfileID,
		&info.OwnerUserID,
		&info.OwnerStatus,
		&info.StaffUserStatus,
		&venueIDs,
	)
	if err != nil {
		return StaffContextInfo{}, err
	}
	info.Permissions = permissions
	info.VenueIDs = venueIDs
	if info.Permissions == nil {
		info.Permissions = []string{}
	}
	if info.VenueIDs == nil {
		info.VenueIDs = []string{}
	}
	return info, nil
}
