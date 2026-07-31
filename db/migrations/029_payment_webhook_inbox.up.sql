BEGIN;

CREATE FUNCTION payment_webhook_redacted_payload_is_valid(
    normalized_event_type TEXT,
    normalized_verification_state TEXT,
    payload JSONB
)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    amount_text TEXT;
    currency_text TEXT;
    state_text TEXT;
    reason_text TEXT;
BEGIN
    IF payload IS NULL
       OR jsonb_typeof(payload) <> 'object'
       OR octet_length(payload::text) > 2048
       OR normalized_event_type NOT IN (
           'payment_session.completed', 'payment_session.expired',
           'payment.capture', 'refund.succeeded', 'refund.failed'
       )
       OR normalized_verification_state NOT IN ('DIAGNOSTIC', 'VERIFIED', 'QUARANTINED')
       OR EXISTS (
           SELECT 1
           FROM jsonb_each(payload) AS entry(key_name, value)
           WHERE key_name NOT IN (
               'state', 'amount_rupiah', 'currency', 'payment_id',
               'payment_request_id', 'reason_code', 'source_reference'
           )
              OR jsonb_typeof(value) NOT IN ('string', 'number')
       ) THEN
        RETURN FALSE;
    END IF;

    IF payload ? 'state' THEN
        state_text := payload->>'state';
        IF state_text NOT IN ('PENDING', 'CAPTURED', 'FAILED', 'EXPIRED', 'CANCELLED', 'SUCCEEDED') THEN
            RETURN FALSE;
        END IF;
    ELSIF normalized_verification_state <> 'QUARANTINED' THEN
        RETURN FALSE;
    END IF;

    IF payload ? 'amount_rupiah' THEN
        amount_text := payload->>'amount_rupiah';
        IF jsonb_typeof(payload->'amount_rupiah') <> 'number'
           OR amount_text !~ '^[1-9][0-9]{0,18}$'
           OR amount_text::numeric > 9223372036854775807 THEN
            RETURN FALSE;
        END IF;
    ELSIF normalized_verification_state <> 'QUARANTINED' THEN
        RETURN FALSE;
    END IF;

    IF payload ? 'currency' THEN
        currency_text := payload->>'currency';
        IF jsonb_typeof(payload->'currency') <> 'string'
           OR currency_text !~ '^[A-Z]{3}$' THEN
            RETURN FALSE;
        END IF;
    ELSIF normalized_verification_state <> 'QUARANTINED' THEN
        RETURN FALSE;
    END IF;

    IF payload ? 'payment_id'
       AND (
           jsonb_typeof(payload->'payment_id') <> 'string'
           OR octet_length(payload->>'payment_id') NOT BETWEEN 1 AND 191
           OR payload->>'payment_id' !~ '^[A-Za-z0-9][A-Za-z0-9._:|/-]{0,190}$'
           OR lower(payload->>'payment_id') ~ '(token|secret|authorization|api[_-]?key|password|credential)'
       ) THEN
        RETURN FALSE;
    END IF;

    IF payload ? 'payment_request_id'
       AND (
           jsonb_typeof(payload->'payment_request_id') <> 'string'
           OR octet_length(payload->>'payment_request_id') NOT BETWEEN 1 AND 191
           OR payload->>'payment_request_id' !~ '^[A-Za-z0-9][A-Za-z0-9._:|/-]{0,190}$'
           OR lower(payload->>'payment_request_id') ~ '(token|secret|authorization|api[_-]?key|password|credential)'
       ) THEN
        RETURN FALSE;
    END IF;

    IF payload ? 'source_reference'
       AND (
           payload->>'source_reference' !~ '^sha256:[0-9a-f]{64}$'
       ) THEN
        RETURN FALSE;
    END IF;

    IF payload ? 'reason_code' THEN
        reason_text := payload->>'reason_code';
        IF reason_text NOT IN (
            'RETRYABLE_TIMEOUT', 'RETRYABLE_PROVIDER', 'RATE_LIMITED',
            'AUTHENTICATION_FAILED', 'INVALID_REQUEST', 'IDEMPOTENCY_CONFLICT',
            'REFERENCE_MISMATCH', 'AMOUNT_MISMATCH', 'CURRENCY_MISMATCH',
            'TERMINAL_PROVIDER', 'MALFORMED_RESPONSE', 'FUTURE_CREATED_SEMANTIC'
        ) THEN
            RETURN FALSE;
        END IF;
    END IF;

    IF reason_text = 'CURRENCY_MISMATCH' THEN
        RETURN normalized_verification_state = 'QUARANTINED'
           AND currency_text IS NOT NULL
           AND currency_text <> 'IDR'
           AND payload ? 'state'
           AND payload ? 'amount_rupiah';
    END IF;

    RETURN currency_text IS NULL OR currency_text = 'IDR';
END;
$$;

CREATE TABLE payment_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(20) NOT NULL,
    provider_environment VARCHAR(10) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    provider_event_key VARCHAR(191) NOT NULL,
    provider_event_id VARCHAR(191),
    primary_object_id VARCHAR(191) NOT NULL,
    raw_body_hash VARCHAR(64) NOT NULL,
    auth_contract_version VARCHAR(64) NOT NULL,
    verification_state VARCHAR(16) NOT NULL,
    processing_state VARCHAR(16) NOT NULL,
    redacted_payload JSONB NOT NULL,
    payment_attempt_id UUID REFERENCES payment_attempts(id) ON DELETE RESTRICT,
    correlation_id VARCHAR(191) NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),

    CONSTRAINT uq_payment_webhook_event_identity UNIQUE (
        provider, provider_environment, provider_event_key
    ),
    CONSTRAINT chk_payment_webhook_event_provider CHECK (provider = 'XENDIT'),
    CONSTRAINT chk_payment_webhook_event_environment CHECK (provider_environment = 'TEST'),
    CONSTRAINT chk_payment_webhook_event_type CHECK (
        event_type IN (
            'payment_session.completed', 'payment_session.expired',
            'payment.capture', 'refund.succeeded', 'refund.failed'
        )
    ),
    CONSTRAINT chk_payment_webhook_event_key CHECK (
        provider_event_key = BTRIM(provider_event_key)
        AND octet_length(provider_event_key) BETWEEN 1 AND 191
        AND provider_event_key ~ '^[A-Za-z0-9][A-Za-z0-9._:|/-]{0,190}$'
        AND provider_event_key = provider || '|' || event_type || '|' || primary_object_id
    ),
    CONSTRAINT chk_payment_webhook_event_provider_id CHECK (
        provider_event_id IS NULL
        OR (
            provider_event_id = BTRIM(provider_event_id)
            AND octet_length(provider_event_id) BETWEEN 1 AND 191
        )
    ),
    CONSTRAINT chk_payment_webhook_event_primary_object CHECK (
        primary_object_id = BTRIM(primary_object_id)
        AND octet_length(primary_object_id) BETWEEN 1 AND 191
        AND primary_object_id ~ '^[A-Za-z0-9][A-Za-z0-9._:|/-]{0,190}$'
    ),
    CONSTRAINT chk_payment_webhook_event_raw_hash CHECK (
        raw_body_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_payment_webhook_event_auth_contract CHECK (
        auth_contract_version IN (
            'XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL',
            'XENDIT_CALLBACK_TOKEN_V1_VERIFIED'
        )
    ),
    CONSTRAINT chk_payment_webhook_event_verification CHECK (
        verification_state IN ('DIAGNOSTIC', 'VERIFIED', 'QUARANTINED')
    ),
    CONSTRAINT chk_payment_webhook_event_processing CHECK (
        processing_state IN ('RECEIVED', 'PROCESSING', 'PROCESSED', 'RETRYABLE', 'TERMINAL', 'DUPLICATE')
    ),
    CONSTRAINT chk_payment_webhook_event_payload CHECK (
        payment_webhook_redacted_payload_is_valid(
            event_type,
            verification_state,
            redacted_payload
        )
    ),
    CONSTRAINT chk_payment_webhook_event_correlation CHECK (
        correlation_id = BTRIM(correlation_id)
        AND octet_length(correlation_id) BETWEEN 1 AND 191
        AND correlation_id ~ '^[A-Za-z0-9][A-Za-z0-9._:|/-]{0,190}$'
    ),
    CONSTRAINT chk_payment_webhook_event_processed CHECK (
        (processing_state IN ('PROCESSED', 'TERMINAL', 'DUPLICATE') AND processed_at IS NOT NULL)
        OR (processing_state NOT IN ('PROCESSED', 'TERMINAL', 'DUPLICATE') AND processed_at IS NULL)
    ),
    CONSTRAINT chk_payment_webhook_event_quarantine CHECK (
        verification_state <> 'QUARANTINED'
        OR processing_state = 'TERMINAL'
    ),
    CONSTRAINT chk_payment_webhook_event_timestamps CHECK (
        received_at <= created_at
        AND updated_at >= created_at
        AND (processed_at IS NULL OR (processed_at >= received_at AND processed_at >= created_at))
    )
);

CREATE FUNCTION guard_payment_webhook_event_identity()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND (
           NEW.id IS DISTINCT FROM OLD.id
           OR NEW.provider IS DISTINCT FROM OLD.provider
           OR NEW.provider_environment IS DISTINCT FROM OLD.provider_environment
           OR NEW.event_type IS DISTINCT FROM OLD.event_type
           OR NEW.provider_event_key IS DISTINCT FROM OLD.provider_event_key
           OR NEW.provider_event_id IS DISTINCT FROM OLD.provider_event_id
           OR NEW.primary_object_id IS DISTINCT FROM OLD.primary_object_id
           OR NEW.raw_body_hash IS DISTINCT FROM OLD.raw_body_hash
           OR NEW.auth_contract_version IS DISTINCT FROM OLD.auth_contract_version
           OR NEW.redacted_payload IS DISTINCT FROM OLD.redacted_payload
           OR NEW.payment_attempt_id IS DISTINCT FROM OLD.payment_attempt_id
           OR NEW.correlation_id IS DISTINCT FROM OLD.correlation_id
           OR NEW.received_at IS DISTINCT FROM OLD.received_at
           OR NEW.created_at IS DISTINCT FROM OLD.created_at
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment webhook event identity and facts are immutable';
    END IF;

    RETURN NEW;
END;
$$;

CREATE FUNCTION guard_payment_webhook_event_lifecycle()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP IN ('DELETE', 'TRUNCATE') THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment webhook events are append-only and cannot be deleted or truncated';
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF (NEW.processing_state = 'RECEIVED' AND NEW.processed_at IS NULL)
           OR (NEW.processing_state = 'TERMINAL'
               AND NEW.verification_state = 'QUARANTINED'
               AND NEW.processed_at IS NOT NULL) THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment webhook events must start received or as a quarantined terminal fact';
    END IF;

    IF NEW.updated_at < OLD.updated_at
       OR (OLD.processed_at IS NOT NULL AND NEW.processed_at IS DISTINCT FROM OLD.processed_at)
       OR OLD.verification_state = 'VERIFIED'
          AND NEW.verification_state <> 'VERIFIED'
       OR OLD.verification_state = 'QUARANTINED'
          AND NEW.verification_state <> 'QUARANTINED' THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'payment webhook event lifecycle is monotonic';
    END IF;

    IF OLD.processing_state IN ('PROCESSED', 'TERMINAL', 'DUPLICATE') THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'terminal payment webhook events are immutable';
    END IF;

    IF OLD.processing_state = 'RECEIVED'
       AND NEW.processing_state IN ('PROCESSING', 'RETRYABLE', 'PROCESSED', 'TERMINAL', 'DUPLICATE') THEN
        RETURN NEW;
    END IF;

    IF OLD.processing_state = 'PROCESSING'
       AND NEW.processing_state IN ('RETRYABLE', 'PROCESSED', 'TERMINAL', 'DUPLICATE') THEN
        RETURN NEW;
    END IF;

    IF OLD.processing_state = 'RETRYABLE'
       AND NEW.processing_state IN ('PROCESSING', 'PROCESSED', 'TERMINAL', 'DUPLICATE') THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'illegal payment webhook event lifecycle transition';
END;
$$;

CREATE TRIGGER guard_payment_webhook_event_identity
BEFORE UPDATE ON payment_webhook_events
FOR EACH ROW
EXECUTE FUNCTION guard_payment_webhook_event_identity();

CREATE TRIGGER guard_payment_webhook_event_lifecycle
BEFORE INSERT OR UPDATE OR DELETE ON payment_webhook_events
FOR EACH ROW
EXECUTE FUNCTION guard_payment_webhook_event_lifecycle();

CREATE TRIGGER guard_payment_webhook_event_truncate
BEFORE TRUNCATE ON payment_webhook_events
FOR EACH STATEMENT
EXECUTE FUNCTION guard_payment_webhook_event_lifecycle();

ALTER TABLE payment_webhook_events
    ENABLE ALWAYS TRIGGER guard_payment_webhook_event_identity;
ALTER TABLE payment_webhook_events
    ENABLE ALWAYS TRIGGER guard_payment_webhook_event_lifecycle;
ALTER TABLE payment_webhook_events
    ENABLE ALWAYS TRIGGER guard_payment_webhook_event_truncate;

CREATE INDEX idx_payment_webhook_event_claim
    ON payment_webhook_events (processing_state, received_at, id)
    WHERE processing_state IN ('RECEIVED', 'RETRYABLE');

CREATE INDEX idx_payment_webhook_event_verification
    ON payment_webhook_events (verification_state, received_at, id)
    WHERE verification_state = 'QUARANTINED';

CREATE INDEX idx_payment_webhook_event_attempt
    ON payment_webhook_events (payment_attempt_id, received_at, id)
    WHERE payment_attempt_id IS NOT NULL;

COMMENT ON TABLE payment_webhook_events IS
    'Append-only, sanitized Xendit Test Mode webhook inbox. Raw bodies, headers, credentials, and customer data are forbidden.';
COMMENT ON COLUMN payment_webhook_events.redacted_payload IS
    'Allowlisted normalized facts only; never raw webhook input, credentials, payment tokens, PAN, CVV, or customer PII.';

COMMIT;
