BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payment_webhook_events) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'cannot remove payment webhook inbox while webhook event facts exist; verify the preserved table then force migration metadata back to version 29 if the runner marks version 28 dirty';
    END IF;
END;
$$;

DROP TABLE payment_webhook_events;
DROP FUNCTION guard_payment_webhook_event_lifecycle();
DROP FUNCTION guard_payment_webhook_event_identity();
DROP FUNCTION payment_webhook_redacted_payload_is_valid(TEXT, TEXT, JSONB);

COMMIT;
