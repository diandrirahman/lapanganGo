BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payment_attempts)
       OR EXISTS (SELECT 1 FROM payment_capture_facts) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'cannot remove payment attempts migration after a payment attempt or capture fact exists';
    END IF;
END;
$$;

DROP TRIGGER prevent_payment_capture_fact_mutation ON payment_capture_facts;
DROP TRIGGER validate_payment_capture_fact ON payment_capture_facts;
DROP FUNCTION prevent_payment_capture_fact_mutation();
DROP FUNCTION validate_payment_capture_fact();
DROP TABLE payment_capture_facts;

DROP TRIGGER guard_payment_attempt_update ON payment_attempts;
DROP FUNCTION guard_payment_attempt_update();
DROP TABLE payment_attempts;

COMMIT;
