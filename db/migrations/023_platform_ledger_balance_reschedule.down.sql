BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM platform_journals)
       OR EXISTS (SELECT 1 FROM platform_ledger_entries) THEN
        RAISE EXCEPTION
            'cannot remove platform ledger balance reschedule after a financial fact exists';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION validate_platform_journal_balance()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    entry_count BIGINT;
    debit_total NUMERIC;
    credit_total NUMERIC;
BEGIN
    SELECT
        COUNT(*),
        COALESCE(SUM(amount_rupiah) FILTER (WHERE side = 'DEBIT'), 0),
        COALESCE(SUM(amount_rupiah) FILTER (WHERE side = 'CREDIT'), 0)
    INTO entry_count, debit_total, credit_total
    FROM platform_ledger_entries
    WHERE journal_id = NEW.id;

    IF entry_count < 2 THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'platform_journal_balance_guard',
            MESSAGE = 'a platform journal requires at least two entries';
    END IF;

    IF debit_total <> credit_total THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'platform_journal_balance_guard',
            MESSAGE = 'platform journal debit and credit totals must balance exactly';
    END IF;

    IF NEW.reverses_journal_id IS NOT NULL AND EXISTS (
        SELECT 1
        FROM (
            (
                SELECT account_code, owner_profile_id, side, amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = NEW.id
                EXCEPT ALL
                SELECT account_code, owner_profile_id,
                       CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END,
                       amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = NEW.reverses_journal_id
            )
            UNION ALL
            (
                SELECT account_code, owner_profile_id,
                       CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END,
                       amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = NEW.reverses_journal_id
                EXCEPT ALL
                SELECT account_code, owner_profile_id, side, amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = NEW.id
            )
        ) AS difference
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'platform_journal_reversal_guard',
            MESSAGE = 'reversal entries must exactly invert the source journal';
    END IF;

    RETURN NULL;
END;
$$;

DROP TRIGGER platform_ledger_entry_balance_guard ON platform_ledger_entries;
DROP FUNCTION validate_platform_ledger_entry_balance();
DROP FUNCTION validate_platform_journal_balance_for(UUID);

COMMIT;
