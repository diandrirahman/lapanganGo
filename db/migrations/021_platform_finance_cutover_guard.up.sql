BEGIN;

CREATE OR REPLACE FUNCTION enforce_booking_snapshot_after_cutover()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public, pg_temp
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM booking_fee_snapshots
        WHERE booking_id = NEW.id
    ) AND (
        current_setting('transaction_isolation') <> 'read committed'
        OR EXISTS (
            SELECT 1
            FROM platform_finance_cutovers
            WHERE id = 1
        )
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'booking snapshot is required after platform finance cutover',
            CONSTRAINT = 'booking_snapshot_required_after_cutover';
    END IF;

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS booking_snapshot_required_after_cutover ON bookings;

CREATE CONSTRAINT TRIGGER booking_snapshot_required_after_cutover
AFTER INSERT ON bookings
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_booking_snapshot_after_cutover();

COMMIT;
