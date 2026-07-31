BEGIN;

CREATE FUNCTION payment_return_origin_is_safe(origin TEXT)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    authority TEXT;
    hostname TEXT;
    port_text TEXT;
    labels TEXT[];
    label TEXT;
BEGIN
    IF origin IS NULL
       OR octet_length(origin) NOT BETWEEN 9 AND 267
       OR origin !~ '^https://[a-z0-9.-]+(:[0-9]{1,5})?$' THEN
        RETURN FALSE;
    END IF;

    authority := substring(origin FROM 9);
    IF length(authority) - length(replace(authority, ':', '')) > 1 THEN
        RETURN FALSE;
    END IF;

    IF position(':' IN authority) > 0 THEN
        hostname := split_part(authority, ':', 1);
        port_text := split_part(authority, ':', 2);
        IF port_text !~ '^[1-9][0-9]{0,4}$'
           OR port_text::integer < 1
           OR port_text::integer > 65535 THEN
            RETURN FALSE;
        END IF;
    ELSE
        hostname := authority;
    END IF;

    IF hostname = ''
       OR octet_length(hostname) > 253 THEN
        RETURN FALSE;
    END IF;

    labels := string_to_array(hostname, '.');
    IF hostname ~ '^[0-9.]+$' AND position('.' IN hostname) > 0 THEN
        IF cardinality(labels) <> 4 THEN
            RETURN FALSE;
        END IF;
        FOREACH label IN ARRAY labels LOOP
            IF label !~ '^(0|[1-9][0-9]{0,2})$'
               OR label::integer > 255 THEN
                RETURN FALSE;
            END IF;
        END LOOP;
        RETURN TRUE;
    END IF;

    FOREACH label IN ARRAY labels LOOP
        IF octet_length(label) NOT BETWEEN 1 AND 63
           OR label !~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$' THEN
            RETURN FALSE;
        END IF;
    END LOOP;
    RETURN TRUE;
END;
$$;

CREATE TABLE payment_create_contracts (
    payment_attempt_id UUID PRIMARY KEY
        REFERENCES payment_attempts(id) ON DELETE RESTRICT,
    request_hash VARCHAR(64) NOT NULL,
    requested_expires_at TIMESTAMPTZ NOT NULL,
    success_return_url TEXT NOT NULL,
    cancel_return_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),

    CONSTRAINT chk_payment_create_contract_hash CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_payment_create_contract_expiry CHECK (
        requested_expires_at > created_at
    ),
    CONSTRAINT chk_payment_create_contract_success_url CHECK (
        octet_length(success_return_url) BETWEEN 1 AND 2048
        AND success_return_url ~ '^https://[a-z0-9.-]+(:[0-9]{1,5})?/payments/return/[a-z0-9][a-z0-9._:-]*/success$'
    ),
    CONSTRAINT chk_payment_create_contract_cancel_url CHECK (
        octet_length(cancel_return_url) BETWEEN 1 AND 2048
        AND cancel_return_url ~ '^https://[a-z0-9.-]+(:[0-9]{1,5})?/payments/return/[a-z0-9][a-z0-9._:-]*/cancel$'
    ),
    CONSTRAINT chk_payment_create_contract_distinct_urls CHECK (
        success_return_url <> cancel_return_url
    )
);

CREATE FUNCTION validate_payment_create_contract()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    attempt payment_attempts%ROWTYPE;
    success_suffix TEXT;
    cancel_suffix TEXT;
    success_origin TEXT;
    cancel_origin TEXT;
BEGIN
    SELECT * INTO attempt
    FROM payment_attempts
    WHERE id = NEW.payment_attempt_id
    FOR KEY SHARE;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23503',
            MESSAGE = 'payment attempt does not exist';
    END IF;

    success_suffix := '/payments/return/' || attempt.local_reference || '/success';
    cancel_suffix := '/payments/return/' || attempt.local_reference || '/cancel';

    IF attempt.provider <> 'XENDIT'
       OR attempt.provider_environment <> 'TEST'
       OR attempt.request_hash <> NEW.request_hash
       OR attempt.expires_at IS DISTINCT FROM NEW.requested_expires_at
       OR RIGHT(NEW.success_return_url, length(success_suffix)) <> success_suffix
       OR RIGHT(NEW.cancel_return_url, length(cancel_suffix)) <> cancel_suffix THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment create contract must exactly match its payment attempt';
    END IF;

    success_origin := LEFT(
        NEW.success_return_url,
        length(NEW.success_return_url) - length(success_suffix)
    );
    cancel_origin := LEFT(
        NEW.cancel_return_url,
        length(NEW.cancel_return_url) - length(cancel_suffix)
    );
    IF success_origin <> cancel_origin THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment create return URLs must use the same origin';
    END IF;
    IF NOT payment_return_origin_is_safe(success_origin) THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'payment create return origin is invalid';
    END IF;

    RETURN NEW;
END;
$$;

CREATE FUNCTION prevent_payment_create_contract_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'payment create contracts are immutable';
END;
$$;

CREATE TRIGGER validate_payment_create_contract
BEFORE INSERT ON payment_create_contracts
FOR EACH ROW
EXECUTE FUNCTION validate_payment_create_contract();

CREATE TRIGGER prevent_payment_create_contract_mutation
BEFORE UPDATE OR DELETE ON payment_create_contracts
FOR EACH ROW
EXECUTE FUNCTION prevent_payment_create_contract_mutation();

CREATE TRIGGER prevent_payment_create_contract_truncate
BEFORE TRUNCATE ON payment_create_contracts
FOR EACH STATEMENT
EXECUTE FUNCTION prevent_payment_create_contract_mutation();

ALTER TABLE payment_create_contracts
    ENABLE ALWAYS TRIGGER validate_payment_create_contract;
ALTER TABLE payment_create_contracts
    ENABLE ALWAYS TRIGGER prevent_payment_create_contract_mutation;
ALTER TABLE payment_create_contracts
    ENABLE ALWAYS TRIGGER prevent_payment_create_contract_truncate;
COMMENT ON TABLE payment_create_contracts IS
    'Immutable provider-create request facts. Provider result expiry is stored separately on payment_attempts.';

COMMIT;
