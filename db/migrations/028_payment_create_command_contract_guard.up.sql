BEGIN;

ALTER TABLE payment_provider_commands
    DROP CONSTRAINT chk_payment_provider_command_type;
ALTER TABLE payment_provider_commands
    ADD CONSTRAINT chk_payment_provider_command_type CHECK (
        command_type IN ('PAYMENT_CREATE', 'PAYMENT_INQUIRY')
    );

CREATE OR REPLACE FUNCTION guard_payment_provider_command_identity()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    attempt RECORD;
    expected_key TEXT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND (
           NEW.id IS DISTINCT FROM OLD.id
           OR NEW.command_type IS DISTINCT FROM OLD.command_type
           OR NEW.aggregate_type IS DISTINCT FROM OLD.aggregate_type
           OR NEW.aggregate_id IS DISTINCT FROM OLD.aggregate_id
           OR NEW.payment_attempt_id IS DISTINCT FROM OLD.payment_attempt_id
           OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
           OR NEW.request_hash IS DISTINCT FROM OLD.request_hash
           OR NEW.redacted_payload IS DISTINCT FROM OLD.redacted_payload
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment provider command identity and payload are immutable';
    END IF;

    IF NOT payment_provider_command_payload_is_safe(NEW.redacted_payload) THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment provider command payload is not canonical';
    END IF;

    SELECT booking_id, attempt_no, state, requested_method, currency::text,
           amount_rupiah, request_hash
    INTO attempt
    FROM payment_attempts
    WHERE id = NEW.payment_attempt_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23503',
            MESSAGE = 'payment provider command requires a referenced payment attempt';
    END IF;

    expected_key := CASE NEW.command_type
        WHEN 'PAYMENT_CREATE' THEN
            'payment:create:' || attempt.booking_id::text || ':' || attempt.attempt_no::text
        WHEN 'PAYMENT_INQUIRY' THEN
            'payment:inquiry:' || NEW.payment_attempt_id::text
        ELSE NULL
    END;

    IF expected_key IS NULL
       OR (
           TG_OP = 'INSERT'
           AND NEW.command_type = 'PAYMENT_INQUIRY'
           AND attempt.state <> 'PENDING'
       )
       OR NEW.aggregate_type <> 'PAYMENT_ATTEMPT'
       OR NEW.aggregate_id <> NEW.payment_attempt_id
       OR NEW.idempotency_key <> expected_key
       OR NEW.request_hash <> attempt.request_hash
       OR NEW.redacted_payload->>'attempt_id' <> NEW.payment_attempt_id::text
       OR (NEW.redacted_payload->>'amount_rupiah')::bigint <> attempt.amount_rupiah
       OR NEW.redacted_payload->>'currency' <> attempt.currency
       OR NEW.redacted_payload->>'requested_method' <> attempt.requested_method THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment provider command does not match immutable payment attempt facts';
    END IF;

    RETURN NEW;
END;
$$;

CREATE FUNCTION validate_payment_create_command_contract()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    contract_hash VARCHAR(64);
BEGIN
    IF NEW.command_type <> 'PAYMENT_CREATE' THEN
        RETURN NEW;
    END IF;

    SELECT request_hash INTO contract_hash
    FROM payment_create_contracts
    WHERE payment_attempt_id = NEW.payment_attempt_id
    FOR KEY SHARE;

    IF NOT FOUND
       OR contract_hash <> NEW.request_hash THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment create command requires the matching immutable create contract';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER validate_payment_create_command_contract
BEFORE INSERT OR UPDATE OF
    command_type,
    payment_attempt_id,
    request_hash
ON payment_provider_commands
FOR EACH ROW
EXECUTE FUNCTION validate_payment_create_command_contract();

ALTER TABLE payment_provider_commands
    ENABLE ALWAYS TRIGGER validate_payment_create_command_contract;

ALTER TABLE payment_attempts
    ENABLE ALWAYS TRIGGER guard_payment_attempt_update;

COMMENT ON FUNCTION validate_payment_create_command_contract() IS
    'Prevents a PAYMENT_CREATE outbox command without the exact immutable create contract.';

CREATE TABLE payment_create_cancellations (
    payment_attempt_id UUID PRIMARY KEY
        REFERENCES payment_attempts(id) ON DELETE RESTRICT,
    command_id UUID NOT NULL UNIQUE
        REFERENCES payment_provider_commands(id) ON DELETE RESTRICT,
    actor_user_id UUID
        REFERENCES users(id) ON DELETE RESTRICT,
    reason VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),

    CONSTRAINT chk_payment_create_cancellation_reason CHECK (
        reason = 'BOOKING_CANCELLED'
    )
);

CREATE FUNCTION guard_payment_create_cancellation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    booking_id UUID;
    command_matches BOOLEAN := FALSE;
BEGIN
    IF TG_OP IN ('DELETE', 'TRUNCATE') THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment create cancellation facts are immutable';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment create cancellation facts are immutable';
    END IF;

    SELECT pa.booking_id
    INTO booking_id
    FROM payment_provider_commands c
    JOIN payment_attempts pa ON pa.id = c.payment_attempt_id
    WHERE c.id = NEW.command_id
      AND c.payment_attempt_id = NEW.payment_attempt_id;

    IF booking_id IS NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment create cancellation requires a cancellable command and cancelled attempt';
    END IF;

    PERFORM pg_advisory_xact_lock(
        hashtextextended('payments:booking-flow:' || booking_id::text, 0)
    );

    SELECT TRUE
    INTO command_matches
    FROM payment_provider_commands c
    JOIN payment_attempts pa ON pa.id = c.payment_attempt_id
    WHERE c.id = NEW.command_id
      AND c.command_type = 'PAYMENT_CREATE'
      AND c.aggregate_type = 'PAYMENT_ATTEMPT'
      AND c.aggregate_id = NEW.payment_attempt_id
      AND c.payment_attempt_id = NEW.payment_attempt_id
      AND c.state = 'PENDING'
      AND c.attempt_count = 0
      AND pa.state = 'CANCELLED'
    FOR UPDATE OF c, pa;

    IF command_matches IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment create cancellation requires a cancellable command and cancelled attempt';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER guard_payment_create_cancellation
BEFORE INSERT OR UPDATE OR DELETE
ON payment_create_cancellations
FOR EACH ROW
EXECUTE FUNCTION guard_payment_create_cancellation();

CREATE TRIGGER guard_payment_create_cancellation_truncate
BEFORE TRUNCATE
ON payment_create_cancellations
FOR EACH STATEMENT
EXECUTE FUNCTION guard_payment_create_cancellation();

ALTER TABLE payment_create_cancellations
    ENABLE ALWAYS TRIGGER guard_payment_create_cancellation;
ALTER TABLE payment_create_cancellations
    ENABLE ALWAYS TRIGGER guard_payment_create_cancellation_truncate;

CREATE FUNCTION guard_cancelled_payment_create_command_lifecycle()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.command_type = 'PAYMENT_CREATE'
       AND (
           NEW.state <> 'PENDING'
           OR NEW.attempt_count <> 0
       )
       AND EXISTS (
           SELECT 1
           FROM payment_create_cancellations
           WHERE command_id = NEW.id
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'cancelled payment create command cannot enter provider lifecycle';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER guard_cancelled_payment_create_command_lifecycle
BEFORE UPDATE OF state, attempt_count
ON payment_provider_commands
FOR EACH ROW
EXECUTE FUNCTION guard_cancelled_payment_create_command_lifecycle();

ALTER TABLE payment_provider_commands
    ENABLE ALWAYS TRIGGER guard_cancelled_payment_create_command_lifecycle;

CREATE FUNCTION validate_atomic_local_payment_cancellation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    current_attempt_state TEXT;
    booking_status TEXT;
    create_command_count INTEGER;
    valid_cancellation_count INTEGER;
    transition_audit_count INTEGER;
BEGIN
    SELECT pa.state, b.status
    INTO current_attempt_state, booking_status
    FROM payment_attempts pa
    JOIN bookings b ON b.id = pa.booking_id
    WHERE pa.id = NEW.id;

    IF current_attempt_state IS DISTINCT FROM 'CANCELLED' THEN
        RETURN NULL;
    END IF;

    SELECT count(*)
    INTO create_command_count
    FROM payment_provider_commands
    WHERE command_type = 'PAYMENT_CREATE'
      AND payment_attempt_id = NEW.id;

    -- Repository-only attempts created outside the 5B-06 orchestration do not
    -- have a provider command and are outside this cancellation contract.
    IF create_command_count = 0 THEN
        RETURN NULL;
    END IF;

    SELECT count(*)
    INTO valid_cancellation_count
    FROM payment_provider_commands c
    JOIN payment_create_cancellations pcc
      ON pcc.command_id = c.id
     AND pcc.payment_attempt_id = c.payment_attempt_id
    WHERE c.command_type = 'PAYMENT_CREATE'
      AND c.payment_attempt_id = NEW.id
      AND c.state = 'PENDING'
      AND c.attempt_count = 0;

    SELECT count(*)
    INTO transition_audit_count
    FROM platform_audit_logs
    WHERE entity_type = 'PAYMENT_ATTEMPT'
      AND entity_id = NEW.id
      AND action = 'payment_state_transition'
      AND metadata->>'from_state' = 'CREATED'
      AND metadata->>'to_state' = 'CANCELLED';

    IF create_command_count <> 1
       OR valid_cancellation_count <> 1
       OR booking_status IS DISTINCT FROM 'CANCELLED'
       OR transition_audit_count <> 1 THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'local payment create cancellation must commit booking, tombstone, and audit atomically';
    END IF;

    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER validate_atomic_local_payment_cancellation
AFTER UPDATE ON payment_attempts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
WHEN (OLD.state = 'CREATED' AND NEW.state = 'CANCELLED')
EXECUTE FUNCTION validate_atomic_local_payment_cancellation();

ALTER TABLE payment_attempts
    ENABLE ALWAYS TRIGGER validate_atomic_local_payment_cancellation;

CREATE FUNCTION guard_sandbox_booking_payment_isolation()
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

CREATE TRIGGER guard_sandbox_booking_payment_isolation
BEFORE UPDATE OF status
ON bookings
FOR EACH ROW
EXECUTE FUNCTION guard_sandbox_booking_payment_isolation();

ALTER TABLE bookings
    ENABLE ALWAYS TRIGGER guard_sandbox_booking_payment_isolation;

CREATE FUNCTION guard_sandbox_owner_cash_isolation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.booking_id IS NOT NULL THEN
        PERFORM pg_advisory_xact_lock(
            hashtextextended('payments:booking-flow:' || NEW.booking_id::text, 0)
        );
    END IF;

    IF NEW.booking_id IS NOT NULL
       AND NEW.source = 'BOOKING'
       AND NEW.type = 'INCOME'
       AND EXISTS (
           SELECT 1
           FROM payment_attempts
           WHERE booking_id = NEW.booking_id
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'legacy owner cash insertion blocked by sandbox payment attempt';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER guard_sandbox_owner_cash_isolation
BEFORE INSERT OR UPDATE OF booking_id, source, type
ON owner_finance_transactions
FOR EACH ROW
EXECUTE FUNCTION guard_sandbox_owner_cash_isolation();

ALTER TABLE owner_finance_transactions
    ENABLE ALWAYS TRIGGER guard_sandbox_owner_cash_isolation;

CREATE FUNCTION guard_sandbox_payment_attempt_isolation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    booking_status TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock(
        hashtextextended('payments:booking-flow:' || NEW.booking_id::text, 0)
    );

    SELECT status INTO booking_status
    FROM bookings
    WHERE id = NEW.booking_id;

    IF booking_status IS DISTINCT FROM 'PENDING_PAYMENT'
       OR EXISTS (
           SELECT 1
           FROM owner_finance_transactions
           WHERE booking_id = NEW.booking_id
             AND source = 'BOOKING'
             AND type = 'INCOME'
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'sandbox payment attempt blocked by legacy booking payment facts';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER guard_sandbox_payment_attempt_isolation
BEFORE INSERT
ON payment_attempts
FOR EACH ROW
EXECUTE FUNCTION guard_sandbox_payment_attempt_isolation();

ALTER TABLE payment_attempts
    ENABLE ALWAYS TRIGGER guard_sandbox_payment_attempt_isolation;

COMMENT ON TABLE payment_create_cancellations IS
    'Immutable tombstone that makes an undispatched PAYMENT_CREATE command permanently ineligible after local booking cancellation.';
COMMENT ON COLUMN payment_provider_commands.payment_attempt_id IS
    'RESTRICT reference for PAYMENT_CREATE and PAYMENT_INQUIRY. Inquiry persistence is reserved for Task 5B-07; no provider inquiry worker is enabled by this migration.';
COMMENT ON FUNCTION guard_sandbox_booking_payment_isolation() IS
    'Prevents legacy booking payment transitions from mixing with sandbox payment attempts.';
COMMENT ON FUNCTION guard_sandbox_owner_cash_isolation() IS
    'Prevents sandbox-backed bookings from entering the legacy owner cashbook.';
COMMENT ON FUNCTION guard_sandbox_payment_attempt_isolation() IS
    'Prevents sandbox payment creation after legacy booking payment or owner cash facts.';
COMMENT ON FUNCTION validate_atomic_local_payment_cancellation() IS
    'Defers validation until commit so a local pre-dispatch cancellation cannot persist without its cancelled booking, immutable tombstone, and transition audit.';

COMMIT;
