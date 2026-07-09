CREATE TYPE owner_staff_invitation_status AS ENUM ('INVITED', 'ACTIVE', 'EXPIRED');
CREATE TYPE owner_staff_invite_purpose AS ENUM ('SET_PASSWORD', 'RESET_PASSWORD');

ALTER TABLE owner_staff_members 
  ADD COLUMN invitation_status owner_staff_invitation_status NOT NULL DEFAULT 'ACTIVE',
  ADD COLUMN invited_at TIMESTAMPTZ,
  ADD COLUMN activated_at TIMESTAMPTZ;

-- Backfill activated_at for existing ACTIVE staff
UPDATE owner_staff_members SET activated_at = created_at WHERE invitation_status = 'ACTIVE';

CREATE TABLE owner_staff_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    staff_member_id UUID NOT NULL REFERENCES owner_staff_members(id) ON DELETE CASCADE,
    owner_profile_id UUID NOT NULL REFERENCES owner_profiles(id) ON DELETE CASCADE,
    staff_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    purpose owner_staff_invite_purpose NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_owner_staff_invites_staff_member_created ON owner_staff_invites(staff_member_id, created_at DESC);
CREATE INDEX idx_owner_staff_invites_owner_created ON owner_staff_invites(owner_profile_id, created_at DESC);
CREATE INDEX idx_owner_staff_invites_expires ON owner_staff_invites(expires_at);
