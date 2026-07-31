BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payment_attempts)
       OR EXISTS (
           SELECT 1
           FROM payment_provider_commands
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'cannot remove the payment command contract guard while payment attempts or provider commands exist';
    END IF;
END;
$$;

DROP TRIGGER guard_sandbox_payment_attempt_isolation ON payment_attempts;
DROP FUNCTION guard_sandbox_payment_attempt_isolation();

DROP TRIGGER guard_sandbox_owner_cash_isolation ON owner_finance_transactions;
DROP FUNCTION guard_sandbox_owner_cash_isolation();

DROP TRIGGER guard_sandbox_booking_payment_isolation ON bookings;
DROP FUNCTION guard_sandbox_booking_payment_isolation();

DROP TRIGGER IF EXISTS validate_atomic_local_payment_cancellation ON payment_attempts;
DROP FUNCTION IF EXISTS validate_atomic_local_payment_cancellation();

DROP TRIGGER IF EXISTS guard_cancelled_payment_create_command_lifecycle ON payment_provider_commands;
DROP FUNCTION IF EXISTS guard_cancelled_payment_create_command_lifecycle();

DROP TRIGGER guard_payment_create_cancellation_truncate ON payment_create_cancellations;
DROP TRIGGER guard_payment_create_cancellation ON payment_create_cancellations;
DROP TABLE payment_create_cancellations;
DROP FUNCTION guard_payment_create_cancellation();

DROP TRIGGER validate_payment_create_command_contract ON payment_provider_commands;
DROP FUNCTION validate_payment_create_command_contract();

ALTER TABLE payment_attempts
    ENABLE TRIGGER guard_payment_attempt_update;

ALTER TABLE payment_provider_commands
    DROP CONSTRAINT chk_payment_provider_command_type;
ALTER TABLE payment_provider_commands
    ADD CONSTRAINT chk_payment_provider_command_type CHECK (
        command_type = 'PAYMENT_CREATE'
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

    SELECT booking_id, attempt_no, requested_method, currency::text,
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
        ELSE NULL
    END;

    IF expected_key IS NULL
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

COMMENT ON COLUMN payment_provider_commands.payment_attempt_id IS
    'RESTRICT reference for PAYMENT_CREATE. Inquiry is deferred to Task 5B-07 and refunds to their aggregate migration.';

COMMIT;
