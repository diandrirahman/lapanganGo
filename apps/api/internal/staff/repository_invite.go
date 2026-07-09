package staff

import (
	"context"
	"time"
)

type StaffInvite struct {
	ID              string
	StaffMemberID   string
	OwnerProfileID  string
	StaffUserID     string
	TokenHash       string
	Purpose         string
	ExpiresAt       time.Time
	UsedAt          *time.Time
	CreatedByUserID string
	CreatedAt       time.Time
}

func (r *Repository) CreateStaffInvite(ctx context.Context, invite StaffInvite) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO owner_staff_invites (staff_member_id, owner_profile_id, staff_user_id, token_hash, purpose, expires_at, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text
	`,
		invite.StaffMemberID,
		invite.OwnerProfileID,
		invite.StaffUserID,
		invite.TokenHash,
		invite.Purpose,
		invite.ExpiresAt,
		invite.CreatedByUserID,
	).Scan(&id)
	return id, err
}

func (r *Repository) FindStaffInviteByTokenHash(ctx context.Context, hash, purpose string) (StaffInvite, error) {
	var inv StaffInvite
	err := r.db.QueryRow(ctx, `
		SELECT id::text, staff_member_id::text, owner_profile_id::text, staff_user_id::text, token_hash, purpose::text, expires_at, used_at, created_by_user_id::text, created_at
		FROM owner_staff_invites
		WHERE token_hash = $1 AND purpose = $2
	`, hash, purpose).Scan(
		&inv.ID,
		&inv.StaffMemberID,
		&inv.OwnerProfileID,
		&inv.StaffUserID,
		&inv.TokenHash,
		&inv.Purpose,
		&inv.ExpiresAt,
		&inv.UsedAt,
		&inv.CreatedByUserID,
		&inv.CreatedAt,
	)
	return inv, err
}

func (r *Repository) MarkStaffInviteUsed(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE owner_staff_invites SET used_at = now() WHERE id = $1`, id)
	return err
}

func (r *Repository) InvalidateStaffInvites(ctx context.Context, staffMemberID, purpose string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE owner_staff_invites 
		SET expires_at = now() 
		WHERE staff_member_id = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
	`, staffMemberID, purpose)
	return err
}

func (r *Repository) UpdateStaffInvitationStatus(ctx context.Context, staffID, status string) error {
	var query string
	if status == "ACTIVE" {
		query = `UPDATE owner_staff_members SET invitation_status = $1, activated_at = COALESCE(activated_at, now()), updated_at = now() WHERE id = $2`
	} else {
		query = `UPDATE owner_staff_members SET invitation_status = $1, updated_at = now() WHERE id = $2`
	}
	_, err := r.db.Exec(ctx, query, status, staffID)
	return err
}

func (r *Repository) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, passwordHash, userID)
	return err
}

