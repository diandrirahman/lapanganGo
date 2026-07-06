ALTER TABLE offline_booking_customers
ADD COLUMN system_price NUMERIC(12,2),
ADD COLUMN final_price NUMERIC(12,2),
ADD COLUMN price_override_reason TEXT;
