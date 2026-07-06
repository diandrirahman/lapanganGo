ALTER TABLE bookings
DROP COLUMN IF EXISTS original_price,
DROP COLUMN IF EXISTS discount_amount,
DROP COLUMN IF EXISTS final_price,
DROP COLUMN IF EXISTS promo_id,
DROP COLUMN IF EXISTS promo_code;

DROP INDEX IF EXISTS idx_owner_promos_lookup;
DROP INDEX IF EXISTS idx_owner_promos_owner_code;

DROP TABLE IF EXISTS owner_promos;
