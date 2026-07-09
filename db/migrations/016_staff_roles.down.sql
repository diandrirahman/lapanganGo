-- 1. Drop tables
DROP TABLE IF EXISTS owner_staff_venue_access;
DROP TABLE IF EXISTS owner_staff_members;

-- 2. Drop enums (only the ones we created in this migration)
DROP TYPE IF EXISTS owner_staff_permission;
DROP TYPE IF EXISTS owner_staff_role;
DROP TYPE IF EXISTS owner_staff_status;

-- Note: We DO NOT remove 'STAFF' from user_role enum because PostgreSQL 
-- enum value removal is unsafe in normal down migrations.
