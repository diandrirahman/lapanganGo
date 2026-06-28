ALTER TABLE bookings 
ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- Backfill existing pending bookings
UPDATE bookings 
SET expires_at = created_at + interval '30 minutes' 
WHERE status = 'PENDING_PAYMENT' AND expires_at IS NULL;
