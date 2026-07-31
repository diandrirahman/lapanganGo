BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payment_create_contracts) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'cannot remove payment create contracts while immutable rows exist';
    END IF;
END;
$$;

DROP TRIGGER prevent_payment_create_contract_truncate ON payment_create_contracts;
DROP TRIGGER prevent_payment_create_contract_mutation ON payment_create_contracts;
DROP TRIGGER validate_payment_create_contract ON payment_create_contracts;
DROP FUNCTION prevent_payment_create_contract_mutation();
DROP FUNCTION validate_payment_create_contract();
DROP TABLE payment_create_contracts;
DROP FUNCTION IF EXISTS payment_return_origin_is_safe(TEXT);

COMMIT;
