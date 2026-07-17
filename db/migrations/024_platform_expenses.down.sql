BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM platform_expenses)
       OR EXISTS (SELECT 1 FROM platform_expense_idempotency) THEN
        RAISE EXCEPTION
            'cannot remove platform expense migration after an expense fact exists';
    END IF;
END;
$$;

DROP TRIGGER prevent_platform_expense_idempotency_mutation ON platform_expense_idempotency;
DROP TRIGGER platform_expense_write_guard ON platform_expenses;

DROP TABLE platform_expense_idempotency;
DROP TABLE platform_expenses;

DROP FUNCTION prevent_platform_expense_idempotency_mutation();
DROP FUNCTION validate_platform_expense_write();
DROP FUNCTION platform_expense_occurred_at_is_allowed(TIMESTAMPTZ, TIMESTAMPTZ);

COMMIT;
