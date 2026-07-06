CREATE TABLE owner_promos (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  venue_id UUID REFERENCES venues(id) ON DELETE CASCADE,
  code VARCHAR(50) NOT NULL,
  name VARCHAR(120) NOT NULL,
  description TEXT,
  discount_type VARCHAR(20) NOT NULL CHECK (discount_type IN ('PERCENTAGE', 'FIXED_AMOUNT')),
  discount_value NUMERIC(12,2) NOT NULL CHECK (discount_value > 0),
  starts_at TIMESTAMPTZ NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT owner_promos_valid_period CHECK (ends_at > starts_at)
);

CREATE UNIQUE INDEX idx_owner_promos_owner_code
  ON owner_promos(owner_id, UPPER(code));

CREATE INDEX idx_owner_promos_lookup
  ON owner_promos(owner_id, venue_id, status, starts_at, ends_at);

ALTER TABLE bookings
ADD COLUMN original_price NUMERIC(12,2),
ADD COLUMN discount_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
ADD COLUMN final_price NUMERIC(12,2),
ADD COLUMN promo_id UUID REFERENCES owner_promos(id),
ADD COLUMN promo_code VARCHAR(50);
