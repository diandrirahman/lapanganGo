BEGIN;

-- Do not restore the pre-gateway guard while a committed gateway-paid booking
-- exists. Those facts require the migration-030 transition contract.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM bookings b
        JOIN payment_attempts pa ON pa.booking_id = b.id
        JOIN payment_capture_facts cf ON cf.payment_attempt_id = pa.id
        WHERE b.status = 'PAID'
          AND pa.state = 'CAPTURED'
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'cannot remove gateway booking payment finalization while gateway-paid booking facts exist';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION guard_sandbox_booking_payment_isolation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(
        hashtextextended('payments:booking-flow:' || NEW.id::text, 0)
    );

    IF NEW.status IS DISTINCT FROM OLD.status
       AND NEW.status IN ('CONFIRMED', 'WAITING_VERIFICATION', 'PAID')
       AND EXISTS (
           SELECT 1
           FROM payment_attempts
           WHERE booking_id = NEW.id
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'legacy booking payment transition blocked by sandbox payment attempt';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status
       AND NEW.status = 'CANCELLED'
       AND EXISTS (
           SELECT 1
           FROM payment_attempts pa
           LEFT JOIN payment_provider_commands c
             ON c.payment_attempt_id = pa.id
            AND c.command_type = 'PAYMENT_CREATE'
           LEFT JOIN payment_create_cancellations pcc
             ON pcc.payment_attempt_id = pa.id
            AND pcc.command_id = c.id
           WHERE pa.booking_id = NEW.id
             AND (
                 pa.state = 'CREATED'
                 OR (
                     pa.state = 'CANCELLED'
                     AND c.state = 'PENDING'
                     AND c.attempt_count = 0
                     AND pcc.payment_attempt_id IS NULL
                 )
             )
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'sandbox booking cancellation requires durable payment cancellation facts';
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION guard_sandbox_booking_payment_isolation() IS
    'Prevents legacy booking payment transitions from mixing with sandbox payment attempts.';

COMMIT;
