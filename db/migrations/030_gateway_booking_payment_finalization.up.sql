BEGIN;

-- Task 5C-05A: preserve the legacy-isolation guard from migration 028 while
-- allowing the one gateway-owned booking transition. A booking with a sandbox
-- attempt can become PAID only after its linked attempt is CAPTURED and its
-- immutable capture fact and payment-transition audit already exist in the
-- same transaction. Direct legacy updates remain blocked.
CREATE OR REPLACE FUNCTION guard_sandbox_booking_payment_isolation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(
        hashtextextended('payments:booking-flow:' || NEW.id::text, 0)
    );

    IF NEW.status IS DISTINCT FROM OLD.status
       AND NEW.status IN ('CONFIRMED', 'WAITING_VERIFICATION')
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
       AND NEW.status = 'PAID'
       AND EXISTS (
           SELECT 1
           FROM payment_attempts
           WHERE booking_id = NEW.id
       )
       AND NOT (
           OLD.status = 'PENDING_PAYMENT'
           AND EXISTS (
               SELECT 1
               FROM payment_attempts pa
               JOIN payment_capture_facts cf
                 ON cf.payment_attempt_id = pa.id
               JOIN platform_audit_logs pal
                 ON pal.action = 'payment_state_transition'
                AND pal.entity_type = 'PAYMENT_ATTEMPT'
                AND pal.entity_id = pa.id
                AND pal.metadata->>'to_state' = 'CAPTURED'
               WHERE pa.booking_id = NEW.id
                 AND pa.state = 'CAPTURED'
                 AND cf.provider = pa.provider
                 AND cf.provider_environment = pa.provider_environment
                 AND cf.amount_rupiah = pa.amount_rupiah
                 AND cf.currency = pa.currency
           )
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'gateway booking paid transition requires captured payment facts';
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
    'Blocks legacy booking payment transitions while allowing only an audited captured gateway attempt to move PENDING_PAYMENT to PAID.';

COMMIT;
