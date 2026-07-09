-- 1. Add STAFF to user_role enum. We cannot safely remove this in down migration.
-- Note: PostgreSQL doesn't allow using the newly added enum value in the same transaction for default values or constraints if it's in a transaction block. 
-- Since we are just inserting it and our tables use it, it might be fine, but just a heads up.
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'STAFF';

-- 2. Create staff status enum
CREATE TYPE owner_staff_status AS ENUM (
    'ACTIVE',
    'INACTIVE'
);

-- 3. Create staff role enum (presets/labels)
CREATE TYPE owner_staff_role AS ENUM (
    'MANAGER',
    'CASHIER',
    'OPERATIONS'
);

-- 4. Create permission enum
CREATE TYPE owner_staff_permission AS ENUM (
    'DASHBOARD_VIEW',
    'ANALYTICS_READ',
    'VENUES_READ',
    'VENUES_WRITE',
    'COURTS_READ',
    'COURTS_WRITE',
    'SCHEDULE_READ',
    'SCHEDULE_WRITE',
    'BLOCKED_SLOTS_READ',
    'BLOCKED_SLOTS_WRITE',
    'BOOKINGS_READ',
    'BOOKINGS_WRITE',
    'OFFLINE_BOOKINGS_CREATE',
    'PAYMENT_VERIFY',
    'REFUNDS_READ',
    'REFUNDS_WRITE',
    'FINANCE_READ',
    'FINANCE_WRITE',
    'PROMOS_READ',
    'PROMOS_WRITE'
);

-- 5. Create staff members table
CREATE TABLE owner_staff_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_profile_id UUID NOT NULL REFERENCES owner_profiles(id) ON DELETE CASCADE,
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    role owner_staff_role NOT NULL,
    permissions owner_staff_permission[] NOT NULL DEFAULT '{}',
    status owner_staff_status NOT NULL DEFAULT 'ACTIVE',
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 6. Create venue access table
CREATE TABLE owner_staff_venue_access (
    staff_member_id UUID NOT NULL REFERENCES owner_staff_members(id) ON DELETE CASCADE,
    venue_id UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (staff_member_id, venue_id)
);

-- 7. Add indexes
CREATE INDEX idx_owner_staff_members_owner_profile_id ON owner_staff_members(owner_profile_id);
CREATE INDEX idx_owner_staff_members_user_id ON owner_staff_members(user_id);
CREATE INDEX idx_owner_staff_members_status ON owner_staff_members(status);
CREATE INDEX idx_owner_staff_venue_access_venue_id ON owner_staff_venue_access(venue_id);
