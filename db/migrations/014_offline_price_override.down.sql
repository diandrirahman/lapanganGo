ALTER TABLE offline_booking_customers
DROP COLUMN IF EXISTS system_price,
DROP COLUMN IF EXISTS final_price,
DROP COLUMN IF EXISTS price_override_reason;
