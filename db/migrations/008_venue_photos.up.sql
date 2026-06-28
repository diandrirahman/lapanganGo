CREATE TABLE IF NOT EXISTS venue_photos (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  venue_id UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
  image_url TEXT NOT NULL,
  alt_text VARCHAR(255),
  sort_order INT NOT NULL DEFAULT 0,
  is_primary BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ensure only one primary photo per venue
CREATE UNIQUE INDEX IF NOT EXISTS idx_venue_photos_primary ON venue_photos (venue_id) WHERE is_primary = true;

-- Index for querying photos by venue and order
CREATE INDEX IF NOT EXISTS idx_venue_photos_venue_id_sort ON venue_photos (venue_id, sort_order);
