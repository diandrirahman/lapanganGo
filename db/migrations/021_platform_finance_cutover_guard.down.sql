BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM platform_finance_cutovers
        WHERE id = 1
    ) THEN
        RAISE EXCEPTION
            'cannot remove finance cutover guard after cutover activation';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;
DROP FUNCTION IF EXISTS enforce_booking_snapshot_after_cutover();

COMMIT;
