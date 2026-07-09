DROP TABLE IF EXISTS owner_staff_invites;

ALTER TABLE owner_staff_members 
  DROP COLUMN IF EXISTS invitation_status,
  DROP COLUMN IF EXISTS invited_at,
  DROP COLUMN IF EXISTS activated_at;

DROP TYPE IF EXISTS owner_staff_invite_purpose;
DROP TYPE IF EXISTS owner_staff_invitation_status;
