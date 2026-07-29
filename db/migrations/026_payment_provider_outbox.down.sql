BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payment_provider_commands) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'cannot remove payment provider outbox while command rows exist; verify the preserved table then force migration metadata back to version 26 if the runner marks version 25 dirty';
    END IF;
END;
$$;

DROP TABLE payment_provider_commands;
DROP FUNCTION IF EXISTS guard_payment_provider_command_lifecycle();
DROP FUNCTION IF EXISTS guard_payment_provider_command_identity();
DROP FUNCTION payment_provider_command_payload_is_safe(JSONB);

COMMIT;
