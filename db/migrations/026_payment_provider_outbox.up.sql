BEGIN;

CREATE FUNCTION payment_provider_command_payload_is_safe(payload JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
    IF payload IS NULL
       OR jsonb_typeof(payload) <> 'object'
       OR octet_length(payload::text) > 16384 THEN
        RETURN FALSE;
    END IF;

    IF (SELECT count(*) FROM jsonb_object_keys(payload)) <> 4
       OR NOT (payload ?& ARRAY['attempt_id', 'amount_rupiah', 'currency', 'requested_method']) THEN
        RETURN FALSE;
    END IF;

    IF jsonb_typeof(payload->'attempt_id') <> 'string'
       OR (payload->>'attempt_id') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
       OR jsonb_typeof(payload->'amount_rupiah') <> 'number'
       OR (payload->>'amount_rupiah') !~ '^[1-9][0-9]{0,18}$'
       OR (payload->>'amount_rupiah')::numeric > 9223372036854775807
       OR jsonb_typeof(payload->'currency') <> 'string'
       OR payload->>'currency' <> 'IDR'
       OR jsonb_typeof(payload->'requested_method') <> 'string'
       OR payload->>'requested_method' NOT IN ('BCA_VA', 'QRIS', 'CARD') THEN
        RETURN FALSE;
    END IF;

    RETURN TRUE;
END;
$$;

CREATE FUNCTION guard_payment_provider_command_identity()
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

CREATE FUNCTION guard_payment_provider_command_lifecycle()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP IN ('DELETE', 'TRUNCATE') THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment provider commands are durable and cannot be deleted or truncated';
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.state <> 'PENDING'
           OR NEW.attempt_count <> 0
           OR NEW.malformed_response_count <> 0
           OR NEW.lease_owner IS NOT NULL
           OR NEW.lease_token IS NOT NULL
           OR NEW.lease_expires_at IS NOT NULL
           OR NEW.last_error_code IS NOT NULL
           OR NEW.provider_reference IS NOT NULL
           OR NEW.completed_at IS NOT NULL THEN
            RAISE EXCEPTION USING
                ERRCODE = '23514',
                MESSAGE = 'payment provider commands must start in the pending state';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.attempt_count < OLD.attempt_count
       OR NEW.malformed_response_count < OLD.malformed_response_count
       OR NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment provider command lifecycle facts are monotonic';
    END IF;

    IF OLD.state IN ('SUCCEEDED', 'TERMINAL') THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'terminal payment provider commands are immutable';
    END IF;

    IF OLD.state = 'PENDING'
       AND NEW.state = 'LEASED'
       AND NEW.attempt_count = OLD.attempt_count + 1
       AND NEW.malformed_response_count = OLD.malformed_response_count
       AND NEW.lease_expires_at > transaction_timestamp()
       AND NEW.last_error_code IS NULL
       AND NEW.provider_reference IS NULL THEN
        RETURN NEW;
    END IF;

    IF OLD.state = 'RETRYABLE'
       AND NEW.state = 'LEASED'
       AND NEW.attempt_count = OLD.attempt_count + 1
       AND NEW.malformed_response_count = OLD.malformed_response_count
       AND NEW.lease_expires_at > transaction_timestamp()
       AND NEW.last_error_code IS NOT DISTINCT FROM OLD.last_error_code
       AND NEW.provider_reference IS NULL THEN
        RETURN NEW;
    END IF;

    IF OLD.state = 'LEASED'
       AND NEW.state = 'LEASED'
       AND OLD.lease_expires_at <= transaction_timestamp()
       AND NEW.attempt_count = OLD.attempt_count + 1
       AND NEW.malformed_response_count = OLD.malformed_response_count
       AND NEW.lease_token IS DISTINCT FROM OLD.lease_token
       AND NEW.last_error_code = 'LEASE_EXPIRED'
       AND NEW.lease_expires_at > transaction_timestamp()
       AND NEW.provider_reference IS NULL THEN
        RETURN NEW;
    END IF;

    IF OLD.state = 'LEASED'
       AND NEW.attempt_count = OLD.attempt_count
       AND OLD.lease_expires_at > transaction_timestamp() THEN
        IF NEW.state = 'RETRYABLE'
           AND (
               (
                   NEW.last_error_code = 'MALFORMED_RESPONSE'
                   AND OLD.malformed_response_count = 0
                   AND NEW.malformed_response_count = 1
               )
               OR (
                   NEW.last_error_code <> 'MALFORMED_RESPONSE'
                   AND NEW.malformed_response_count = OLD.malformed_response_count
               )
           ) THEN
            RETURN NEW;
        END IF;

        IF NEW.state = 'SUCCEEDED'
           AND NEW.malformed_response_count = OLD.malformed_response_count THEN
            RETURN NEW;
        END IF;

        IF NEW.state = 'TERMINAL'
           AND (
               (
                   NEW.last_error_code = 'MALFORMED_RESPONSE'
                   AND OLD.malformed_response_count = 1
                   AND NEW.malformed_response_count = 2
               )
               OR (
                   NEW.last_error_code <> 'MALFORMED_RESPONSE'
                   AND NEW.malformed_response_count = OLD.malformed_response_count
               )
           ) THEN
            RETURN NEW;
        END IF;
    END IF;

    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'illegal payment provider command lifecycle transition';
END;
$$;

CREATE TABLE payment_provider_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    command_type VARCHAR(32) NOT NULL,
    aggregate_type VARCHAR(32) NOT NULL,
    aggregate_id UUID NOT NULL,
    payment_attempt_id UUID REFERENCES payment_attempts(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(191) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    redacted_payload JSONB NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    malformed_response_count SMALLINT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    lease_owner VARCHAR(191),
    lease_token UUID,
    lease_expires_at TIMESTAMPTZ,
    last_error_code VARCHAR(64),
    provider_reference VARCHAR(191),
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    completed_at TIMESTAMPTZ,

    CONSTRAINT chk_payment_provider_command_type CHECK (
        command_type = 'PAYMENT_CREATE'
    ),
    CONSTRAINT chk_payment_provider_command_aggregate_type CHECK (
        aggregate_type = 'PAYMENT_ATTEMPT'
    ),
    CONSTRAINT chk_payment_provider_command_aggregate_match CHECK (
        payment_attempt_id IS NOT NULL
        AND payment_attempt_id = aggregate_id
    ),
    CONSTRAINT chk_payment_provider_command_idempotency_key CHECK (
        idempotency_key = BTRIM(idempotency_key)
        AND idempotency_key ~ '^[a-z0-9][a-z0-9:._-]{0,190}$'
    ),
    CONSTRAINT chk_payment_provider_command_request_hash CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_payment_provider_command_payload CHECK (
        payment_provider_command_payload_is_safe(redacted_payload)
        AND redacted_payload->>'attempt_id' = aggregate_id::text
    ),
    CONSTRAINT chk_payment_provider_command_state CHECK (
        state IN ('PENDING', 'LEASED', 'RETRYABLE', 'SUCCEEDED', 'TERMINAL')
    ),
    CONSTRAINT chk_payment_provider_command_attempt_count CHECK (attempt_count >= 0),
    CONSTRAINT chk_payment_provider_command_malformed_response_count CHECK (
        malformed_response_count BETWEEN 0 AND 2
    ),
    CONSTRAINT chk_payment_provider_command_lease CHECK (
        (
            state = 'LEASED'
            AND lease_owner IS NOT NULL
            AND lease_token IS NOT NULL
            AND lease_expires_at IS NOT NULL
        )
        OR (
            state <> 'LEASED'
            AND lease_owner IS NULL
            AND lease_token IS NULL
            AND lease_expires_at IS NULL
        )
    ),
    CONSTRAINT chk_payment_provider_command_completed CHECK (
        (state IN ('SUCCEEDED', 'TERMINAL') AND completed_at IS NOT NULL)
        OR (state NOT IN ('SUCCEEDED', 'TERMINAL') AND completed_at IS NULL)
    ),
    CONSTRAINT chk_payment_provider_command_lease_owner CHECK (
        lease_owner IS NULL
        OR (
            lease_owner = BTRIM(lease_owner)
            AND lease_owner ~ '^worker:[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
        )
    ),
    CONSTRAINT chk_payment_provider_command_error_code CHECK (
        last_error_code IS NULL
        OR last_error_code IN (
            'RETRYABLE_TIMEOUT', 'RETRYABLE_PROVIDER', 'RATE_LIMITED',
            'AUTHENTICATION_FAILED', 'INVALID_REQUEST', 'IDEMPOTENCY_CONFLICT',
            'REFERENCE_MISMATCH', 'AMOUNT_MISMATCH', 'CURRENCY_MISMATCH',
            'TERMINAL_PROVIDER', 'MALFORMED_RESPONSE', 'LEASE_EXPIRED'
        )
    ),
    CONSTRAINT chk_payment_provider_command_error_state CHECK (
        (
            state = 'RETRYABLE'
            AND last_error_code IS NOT NULL
            AND last_error_code IN (
                'RETRYABLE_TIMEOUT', 'RETRYABLE_PROVIDER', 'RATE_LIMITED', 'MALFORMED_RESPONSE'
            )
            AND (
                (last_error_code = 'MALFORMED_RESPONSE' AND malformed_response_count = 1)
                OR last_error_code <> 'MALFORMED_RESPONSE'
            )
        )
        OR (
            state = 'TERMINAL'
            AND last_error_code IS NOT NULL
            AND last_error_code IN (
                'AUTHENTICATION_FAILED', 'INVALID_REQUEST', 'IDEMPOTENCY_CONFLICT',
                'REFERENCE_MISMATCH', 'AMOUNT_MISMATCH', 'CURRENCY_MISMATCH',
                'TERMINAL_PROVIDER', 'MALFORMED_RESPONSE'
            )
            AND (
                (last_error_code = 'MALFORMED_RESPONSE' AND malformed_response_count = 2)
                OR last_error_code <> 'MALFORMED_RESPONSE'
            )
        )
        OR (state = 'SUCCEEDED' AND last_error_code IS NULL)
        OR (state = 'PENDING' AND last_error_code IS NULL)
        OR state = 'LEASED'
    ),
    CONSTRAINT chk_payment_provider_command_provider_reference CHECK (
        (
            state = 'SUCCEEDED'
            AND provider_reference IS NOT NULL
            AND provider_reference ~ '^sha256:[0-9a-f]{64}$'
        )
        OR (state <> 'SUCCEEDED' AND provider_reference IS NULL)
    ),
    CONSTRAINT chk_payment_provider_command_timestamps CHECK (
        updated_at >= created_at
        AND (completed_at IS NULL OR completed_at >= created_at)
    ),
    CONSTRAINT uq_payment_provider_command_idempotency UNIQUE (command_type, idempotency_key),
    CONSTRAINT uq_payment_provider_command_aggregate UNIQUE (
        command_type,
        aggregate_type,
        aggregate_id
    )
);

CREATE TRIGGER guard_payment_provider_command_identity
BEFORE INSERT OR UPDATE OF
    id,
    command_type,
    aggregate_type,
    aggregate_id,
    payment_attempt_id,
    idempotency_key,
    request_hash,
    redacted_payload
ON payment_provider_commands
FOR EACH ROW
EXECUTE FUNCTION guard_payment_provider_command_identity();

CREATE TRIGGER guard_payment_provider_command_lifecycle
BEFORE INSERT OR UPDATE OR DELETE
ON payment_provider_commands
FOR EACH ROW
EXECUTE FUNCTION guard_payment_provider_command_lifecycle();

CREATE TRIGGER guard_payment_provider_command_truncate
BEFORE TRUNCATE
ON payment_provider_commands
FOR EACH STATEMENT
EXECUTE FUNCTION guard_payment_provider_command_lifecycle();

ALTER TABLE payment_provider_commands
    ENABLE ALWAYS TRIGGER guard_payment_provider_command_identity;
ALTER TABLE payment_provider_commands
    ENABLE ALWAYS TRIGGER guard_payment_provider_command_lifecycle;
ALTER TABLE payment_provider_commands
    ENABLE ALWAYS TRIGGER guard_payment_provider_command_truncate;

CREATE INDEX idx_payment_provider_command_claim
    ON payment_provider_commands (state, available_at, created_at)
    WHERE state IN ('PENDING', 'RETRYABLE');

CREATE INDEX idx_payment_provider_command_lease
    ON payment_provider_commands (state, lease_expires_at)
    WHERE state = 'LEASED';

CREATE INDEX idx_payment_provider_command_aggregate
    ON payment_provider_commands (aggregate_type, aggregate_id, created_at DESC);

COMMENT ON TABLE payment_provider_commands IS
    'Durable provider command outbox. Provider calls occur only after the owning transaction commits.';
COMMENT ON COLUMN payment_provider_commands.payment_attempt_id IS
    'RESTRICT reference for PAYMENT_CREATE. Inquiry is deferred to Task 5B-07 and refunds to their aggregate migration.';
COMMENT ON COLUMN payment_provider_commands.lease_token IS
    'Opaque per-claim generation token required to reject stale worker completion after lease recovery.';
COMMENT ON COLUMN payment_provider_commands.malformed_response_count IS
    'Error-specific retry counter. Claim/restart attempts do not consume the one malformed-response retry.';
COMMENT ON COLUMN payment_provider_commands.provider_reference IS
    'SHA-256 digest of the opaque provider result reference. Raw provider IDs, credentials, account/card data, URLs, and response fragments are forbidden.';

COMMIT;
