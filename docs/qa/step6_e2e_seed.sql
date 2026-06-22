-- Seed Data for E2E QA Testing (Idempotent)

-- 1. Create Users
INSERT INTO users (id, name, email, phone, password_hash, role, status, created_at, updated_at)
VALUES 
('55555555-5555-5555-5555-555555555555', 'QA Customer', 'qa.customer@lapanggo.test', '08120000001', '$2a$10$WryVY8GjgvTf3Hg9yGcqxeaaV4bq0b1h2YZQFSSrtC1CvSW.v7/Ni', 'CUSTOMER', 'ACTIVE', NOW(), NOW()),
('66666666-6666-6666-6666-666666666666', 'QA Owner', 'qa.owner@lapanggo.test', '08120000002', '$2a$10$WryVY8GjgvTf3Hg9yGcqxeaaV4bq0b1h2YZQFSSrtC1CvSW.v7/Ni', 'OWNER', 'ACTIVE', NOW(), NOW())
ON CONFLICT (email) DO UPDATE SET 
    name = EXCLUDED.name,
    phone = EXCLUDED.phone,
    password_hash = EXCLUDED.password_hash,
    role = EXCLUDED.role,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

-- 2. Create Owner Profile
INSERT INTO owner_profiles (id, user_id, bank_name, bank_account_name, bank_account_number, business_name, verification_status, created_at, updated_at)
VALUES 
('77777777-7777-7777-7777-777777777777', '66666666-6666-6666-6666-666666666666', 'BCA', 'QA Owner', '1234567890', 'QA Business', 'APPROVED', NOW(), NOW())
ON CONFLICT (user_id) DO UPDATE SET 
    business_name = EXCLUDED.business_name,
    bank_name = EXCLUDED.bank_name,
    bank_account_name = EXCLUDED.bank_account_name,
    bank_account_number = EXCLUDED.bank_account_number,
    verification_status = EXCLUDED.verification_status,
    updated_at = EXCLUDED.updated_at;

-- 3. Create Venue
INSERT INTO venues (id, owner_profile_id, name, description, address, city, latitude, longitude, status, created_at, updated_at)
VALUES 
('88888888-8888-8888-8888-888888888888', '77777777-7777-7777-7777-777777777777', 'QA Venue', 'Venue for E2E QA', 'Jalan QA No. 1', 'Jakarta', -6.200000, 106.816666, 'ACTIVE', NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET 
    owner_profile_id = EXCLUDED.owner_profile_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    address = EXCLUDED.address,
    city = EXCLUDED.city,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

-- 4. Create Court (Using subquery to fetch sport_id for Futsal)
INSERT INTO courts (id, venue_id, sport_id, name, description, location_type, price_per_hour, status, created_at, updated_at)
VALUES 
('99999999-9999-9999-9999-999999999999', '88888888-8888-8888-8888-888888888888', (SELECT id FROM sports WHERE name = 'Futsal' LIMIT 1), 'QA Court 1', 'Court for E2E QA', 'INDOOR', 100000.00, 'ACTIVE', NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET 
    venue_id = EXCLUDED.venue_id,
    sport_id = EXCLUDED.sport_id,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    location_type = EXCLUDED.location_type,
    price_per_hour = EXCLUDED.price_per_hour,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

-- 5. Create Court Operating Hours
INSERT INTO court_operating_hours (id, court_id, day_of_week, open_time, close_time, is_closed, created_at, updated_at)
VALUES
('00000000-0000-0000-0000-000000000000', '99999999-9999-9999-9999-999999999999', 0, '08:00', '22:00', false, NOW(), NOW()),
('11111111-1111-1111-1111-111111111111', '99999999-9999-9999-9999-999999999999', 1, '08:00', '22:00', false, NOW(), NOW()),
('22222222-2222-2222-2222-222222222222', '99999999-9999-9999-9999-999999999999', 2, '08:00', '22:00', false, NOW(), NOW()),
('33333333-3333-3333-3333-333333333333', '99999999-9999-9999-9999-999999999999', 3, '08:00', '22:00', false, NOW(), NOW()),
('44444444-4444-4444-4444-444444444444', '99999999-9999-9999-9999-999999999999', 4, '08:00', '22:00', false, NOW(), NOW()),
('55555555-0000-0000-0000-000000000000', '99999999-9999-9999-9999-999999999999', 5, '08:00', '22:00', false, NOW(), NOW()),
('66666666-0000-0000-0000-000000000000', '99999999-9999-9999-9999-999999999999', 6, '08:00', '22:00', false, NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET 
    open_time = EXCLUDED.open_time,
    close_time = EXCLUDED.close_time,
    is_closed = EXCLUDED.is_closed,
    updated_at = EXCLUDED.updated_at;
