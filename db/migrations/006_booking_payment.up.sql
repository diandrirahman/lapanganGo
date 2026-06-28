-- Migration: Add payment_reference to bookings

ALTER TABLE bookings 
ADD COLUMN IF NOT EXISTS payment_reference VARCHAR(255);

-- Update booking status check constraint to include WAITING_VERIFICATION and PAID
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_status_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_status_check 
CHECK (status IN ('PENDING_PAYMENT', 'WAITING_VERIFICATION', 'CONFIRMED', 'PAID', 'CANCELLED', 'COMPLETED'));
