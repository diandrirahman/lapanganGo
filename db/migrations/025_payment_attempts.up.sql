BEGIN;

CREATE TABLE payment_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE RESTRICT,
    attempt_no SMALLINT NOT NULL,
    provider VARCHAR(20) NOT NULL,
    provider_environment VARCHAR(10) NOT NULL,
    requested_method VARCHAR(20) NOT NULL,
    integration_mode VARCHAR(20) NOT NULL,
    capture_method VARCHAR(20) NOT NULL,
    state VARCHAR(20) NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    amount_rupiah BIGINT NOT NULL,
    local_reference VARCHAR(64) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    provider_session_id VARCHAR(191),
    provider_payment_request_id VARCHAR(191),
    provider_payment_id VARCHAR(191),
    provider_status_code VARCHAR(80),
    checkout_url TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    captured_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),

    CONSTRAINT fk_payment_attempt_snapshot
        FOREIGN KEY (booking_id)
        REFERENCES booking_fee_snapshots(booking_id)
        ON DELETE RESTRICT,
    CONSTRAINT chk_payment_attempt_no_positive CHECK (attempt_no > 0),
    CONSTRAINT chk_payment_attempt_provider CHECK (provider = 'XENDIT'),
    CONSTRAINT chk_payment_attempt_provider_environment CHECK (provider_environment = 'TEST'),
    CONSTRAINT chk_payment_attempt_requested_method CHECK (
        requested_method IN ('BCA_VA', 'QRIS', 'CARD')
    ),
    CONSTRAINT chk_payment_attempt_integration_mode CHECK (integration_mode = 'PAYMENT_LINK'),
    CONSTRAINT chk_payment_attempt_capture_method CHECK (capture_method = 'AUTOMATIC'),
    CONSTRAINT chk_payment_attempt_state CHECK (
        state IN ('CREATED', 'PENDING', 'CAPTURED', 'FAILED', 'EXPIRED', 'CANCELLED')
    ),
    CONSTRAINT chk_payment_attempt_currency CHECK (currency = 'IDR'),
    CONSTRAINT chk_payment_attempt_amount_positive CHECK (amount_rupiah > 0),
    CONSTRAINT chk_payment_attempt_local_reference CHECK (
        local_reference = BTRIM(local_reference)
        AND octet_length(local_reference) BETWEEN 1 AND 64
        AND local_reference ~ '^[a-z0-9][a-z0-9._:-]*$'
    ),
    CONSTRAINT chk_payment_attempt_request_hash CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_payment_attempt_provider_session_id CHECK (
        provider_session_id IS NULL
        OR (
            provider_session_id = BTRIM(provider_session_id)
            AND octet_length(provider_session_id) BETWEEN 1 AND 191
        )
    ),
    CONSTRAINT chk_payment_attempt_provider_payment_request_id CHECK (
        provider_payment_request_id IS NULL
        OR (
            provider_payment_request_id = BTRIM(provider_payment_request_id)
            AND octet_length(provider_payment_request_id) BETWEEN 1 AND 191
        )
    ),
    CONSTRAINT chk_payment_attempt_provider_payment_id CHECK (
        provider_payment_id IS NULL
        OR (
            provider_payment_id = BTRIM(provider_payment_id)
            AND octet_length(provider_payment_id) BETWEEN 1 AND 191
        )
    ),
    CONSTRAINT chk_payment_attempt_provider_status_code CHECK (
        provider_status_code IS NULL
        OR provider_status_code IN (
            'PENDING', 'CAPTURED', 'FAILED', 'EXPIRED', 'CANCELLED',
            'REQUIRES_ACTION', 'ACCEPTING_PAYMENTS', 'RETRYABLE_TIMEOUT',
            'RETRYABLE_PROVIDER', 'RATE_LIMITED', 'AUTHENTICATION_FAILED',
            'INVALID_REQUEST', 'IDEMPOTENCY_CONFLICT', 'REFERENCE_MISMATCH',
            'AMOUNT_MISMATCH', 'CURRENCY_MISMATCH', 'TERMINAL_PROVIDER',
            'MALFORMED_RESPONSE'
        )
    ),
    CONSTRAINT chk_payment_attempt_checkout_url CHECK (
        checkout_url IS NULL
        OR (
            octet_length(checkout_url) BETWEEN 1 AND 2048
            AND checkout_url ~ '^https://[^[:space:]]+$'
        )
    ),
    CONSTRAINT chk_payment_attempt_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_payment_attempt_capture_time CHECK (
        (state = 'CAPTURED') = (captured_at IS NOT NULL)
    ),
    CONSTRAINT uq_payment_attempt_booking_attempt_no UNIQUE (booking_id, attempt_no),
    CONSTRAINT uq_payment_attempt_provider_reference UNIQUE (
        provider,
        provider_environment,
        local_reference
    )
);

CREATE UNIQUE INDEX uq_payment_attempt_booking_captured
    ON payment_attempts (booking_id)
    WHERE state = 'CAPTURED';

CREATE UNIQUE INDEX uq_payment_attempt_provider_session
    ON payment_attempts (provider, provider_environment, provider_session_id)
    WHERE provider_session_id IS NOT NULL;

CREATE UNIQUE INDEX uq_payment_attempt_provider_request
    ON payment_attempts (provider, provider_environment, provider_payment_request_id)
    WHERE provider_payment_request_id IS NOT NULL;

CREATE UNIQUE INDEX uq_payment_attempt_provider_payment
    ON payment_attempts (provider, provider_environment, provider_payment_id)
    WHERE provider_payment_id IS NOT NULL;

CREATE INDEX idx_payment_attempt_booking_created
    ON payment_attempts (booking_id, created_at DESC);

CREATE INDEX idx_payment_attempt_state_updated
    ON payment_attempts (state, updated_at);

CREATE FUNCTION guard_payment_attempt_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.booking_id IS DISTINCT FROM OLD.booking_id
       OR NEW.attempt_no IS DISTINCT FROM OLD.attempt_no
       OR NEW.provider IS DISTINCT FROM OLD.provider
       OR NEW.provider_environment IS DISTINCT FROM OLD.provider_environment
       OR NEW.requested_method IS DISTINCT FROM OLD.requested_method
       OR NEW.integration_mode IS DISTINCT FROM OLD.integration_mode
       OR NEW.capture_method IS DISTINCT FROM OLD.capture_method
       OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.amount_rupiah IS DISTINCT FROM OLD.amount_rupiah
       OR NEW.local_reference IS DISTINCT FROM OLD.local_reference
       OR NEW.request_hash IS DISTINCT FROM OLD.request_hash THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment attempt identity and requested money fields are immutable';
    END IF;

    IF OLD.captured_at IS NOT NULL
       AND NEW.captured_at IS DISTINCT FROM OLD.captured_at THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment attempt captured_at is immutable once set';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER guard_payment_attempt_update
BEFORE UPDATE ON payment_attempts
FOR EACH ROW
EXECUTE FUNCTION guard_payment_attempt_update();

CREATE TABLE payment_capture_facts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_attempt_id UUID NOT NULL REFERENCES payment_attempts(id) ON DELETE RESTRICT,
    provider VARCHAR(20) NOT NULL,
    provider_environment VARCHAR(10) NOT NULL,
    provider_payment_id VARCHAR(191) NOT NULL,
    provider_payment_request_id VARCHAR(191),
    amount_rupiah BIGINT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    captured_at TIMESTAMPTZ NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    authority VARCHAR(32) NOT NULL,
    source_reference VARCHAR(191) NOT NULL,
    payload_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),

    CONSTRAINT uq_payment_capture_fact_attempt UNIQUE (payment_attempt_id),
    CONSTRAINT uq_payment_capture_fact_provider_payment UNIQUE (
        provider,
        provider_environment,
        provider_payment_id
    ),
    CONSTRAINT uq_payment_capture_fact_provider_request UNIQUE (
        provider,
        provider_environment,
        provider_payment_request_id
    ),
    CONSTRAINT chk_payment_capture_fact_provider CHECK (provider = 'XENDIT'),
    CONSTRAINT chk_payment_capture_fact_provider_environment CHECK (provider_environment = 'TEST'),
    CONSTRAINT chk_payment_capture_fact_currency CHECK (currency = 'IDR'),
    CONSTRAINT chk_payment_capture_fact_amount_positive CHECK (amount_rupiah > 0),
    CONSTRAINT chk_payment_capture_fact_provider_payment_id CHECK (
        provider_payment_id = BTRIM(provider_payment_id)
        AND octet_length(provider_payment_id) BETWEEN 1 AND 191
    ),
    CONSTRAINT chk_payment_capture_fact_provider_payment_request_id CHECK (
        provider_payment_request_id IS NULL
        OR (
            provider_payment_request_id = BTRIM(provider_payment_request_id)
            AND octet_length(provider_payment_request_id) BETWEEN 1 AND 191
        )
    ),
    CONSTRAINT chk_payment_capture_fact_observed_at CHECK (observed_at >= captured_at),
    CONSTRAINT chk_payment_capture_fact_authority CHECK (
        authority IN ('VERIFIED_WEBHOOK', 'AUTHENTICATED_INQUIRY')
    ),
    CONSTRAINT chk_payment_capture_fact_source_reference CHECK (
        source_reference = BTRIM(source_reference)
        AND octet_length(source_reference) BETWEEN 1 AND 191
    ),
    CONSTRAINT chk_payment_capture_fact_payload_hash CHECK (
        payload_hash ~ '^[0-9a-f]{64}$'
    )
);

CREATE FUNCTION validate_payment_capture_fact()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    attempt payment_attempts%ROWTYPE;
BEGIN
    SELECT * INTO attempt
    FROM payment_attempts
    WHERE id = NEW.payment_attempt_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23503',
            MESSAGE = 'payment attempt does not exist';
    END IF;

    IF attempt.state <> 'CAPTURED'
       OR attempt.captured_at IS DISTINCT FROM NEW.captured_at THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment capture fact requires the matching captured payment attempt';
    END IF;

    IF attempt.provider <> NEW.provider
       OR attempt.provider_environment <> NEW.provider_environment
       OR attempt.currency <> NEW.currency
       OR attempt.amount_rupiah <> NEW.amount_rupiah THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment capture fact must exactly match its payment attempt';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER validate_payment_capture_fact
BEFORE INSERT ON payment_capture_facts
FOR EACH ROW
EXECUTE FUNCTION validate_payment_capture_fact();

CREATE FUNCTION prevent_payment_capture_fact_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'payment capture facts are immutable';
END;
$$;

CREATE TRIGGER prevent_payment_capture_fact_mutation
BEFORE UPDATE OR DELETE ON payment_capture_facts
FOR EACH ROW
EXECUTE FUNCTION prevent_payment_capture_fact_mutation();

COMMIT;
