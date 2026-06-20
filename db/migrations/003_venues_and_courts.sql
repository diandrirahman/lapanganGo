DO $$
BEGIN
  CREATE TYPE venue_status AS ENUM ('DRAFT', 'ACTIVE', 'INACTIVE', 'SUSPENDED');
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

DO $$
BEGIN
  CREATE TYPE court_status AS ENUM ('ACTIVE', 'INACTIVE', 'MAINTENANCE');
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

DO $$
BEGIN
  CREATE TYPE court_location_type AS ENUM ('INDOOR', 'OUTDOOR');
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS venues (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_profile_id UUID NOT NULL REFERENCES owner_profiles(id) ON DELETE CASCADE,
  name VARCHAR(150) NOT NULL,
  description TEXT,
  address TEXT NOT NULL,
  district VARCHAR(100),
  city VARCHAR(100) NOT NULL,
  province VARCHAR(100),
  postal_code VARCHAR(20),
  latitude NUMERIC(10, 7),
  longitude NUMERIC(10, 7),
  status venue_status NOT NULL DEFAULT 'DRAFT',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT venues_owner_name_unique UNIQUE (owner_profile_id, name),
  CONSTRAINT venues_latitude_range CHECK (latitude IS NULL OR (latitude >= -90 AND latitude <= 90)),
  CONSTRAINT venues_longitude_range CHECK (longitude IS NULL OR (longitude >= -180 AND longitude <= 180))
);

CREATE TABLE IF NOT EXISTS venue_facilities (
  venue_id UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (venue_id, facility_id)
);

CREATE TABLE IF NOT EXISTS courts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  venue_id UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  sport_id UUID NOT NULL REFERENCES sports(id) ON DELETE RESTRICT,
  name VARCHAR(120) NOT NULL,
  description TEXT,
  location_type court_location_type NOT NULL,
  surface_type VARCHAR(80),
  price_per_hour NUMERIC(12, 2) NOT NULL,
  status court_status NOT NULL DEFAULT 'ACTIVE',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT courts_venue_name_unique UNIQUE (venue_id, name),
  CONSTRAINT courts_price_non_negative CHECK (price_per_hour >= 0)
);

CREATE TABLE IF NOT EXISTS court_operating_hours (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  court_id UUID NOT NULL REFERENCES courts(id) ON DELETE CASCADE,
  day_of_week SMALLINT NOT NULL,
  open_time TIME,
  close_time TIME,
  is_closed BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT court_operating_hours_court_day_unique UNIQUE (court_id, day_of_week),
  CONSTRAINT court_operating_hours_day_range CHECK (day_of_week BETWEEN 0 AND 6),
  CONSTRAINT court_operating_hours_time_valid CHECK (
    (is_closed = true AND open_time IS NULL AND close_time IS NULL)
    OR
    (is_closed = false AND open_time IS NOT NULL AND close_time IS NOT NULL AND close_time > open_time)
  )
);

CREATE TABLE IF NOT EXISTS court_blocked_slots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  court_id UUID NOT NULL REFERENCES courts(id) ON DELETE CASCADE,
  start_at TIMESTAMPTZ NOT NULL,
  end_at TIMESTAMPTZ NOT NULL,
  reason VARCHAR(180),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT court_blocked_slots_time_valid CHECK (end_at > start_at)
);

CREATE INDEX IF NOT EXISTS idx_venues_owner_profile_id ON venues(owner_profile_id);
CREATE INDEX IF NOT EXISTS idx_venues_city_status ON venues(city, status);
CREATE INDEX IF NOT EXISTS idx_venue_facilities_facility_id ON venue_facilities(facility_id);
CREATE INDEX IF NOT EXISTS idx_courts_venue_id ON courts(venue_id);
CREATE INDEX IF NOT EXISTS idx_courts_sport_id ON courts(sport_id);
CREATE INDEX IF NOT EXISTS idx_courts_status ON courts(status);
CREATE INDEX IF NOT EXISTS idx_court_operating_hours_court_id ON court_operating_hours(court_id);
CREATE INDEX IF NOT EXISTS idx_court_blocked_slots_court_time ON court_blocked_slots(court_id, start_at, end_at);

COMMENT ON COLUMN court_operating_hours.day_of_week IS '0 = Sunday, 1 = Monday, ..., 6 = Saturday';
