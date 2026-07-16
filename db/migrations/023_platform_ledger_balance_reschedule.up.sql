BEGIN;

-- Migration 022 is already deployed in some environments. Re-arm the
-- balance and exact-reversal invariant whenever an entry is added so an
-- early evaluation of the journal header trigger cannot consume the guard.
CREATE FUNCTION validate_platform_journal_balance_for(p_journal_id UUID)
RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
    entry_count BIGINT;
    debit_total NUMERIC;
    credit_total NUMERIC;
    source_journal_id UUID;
BEGIN
    SELECT reverses_journal_id
    INTO source_journal_id
    FROM platform_journals
    WHERE id = p_journal_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23503',
            MESSAGE = 'journal balance validation target does not exist';
    END IF;

    SELECT
        COUNT(*),
        COALESCE(SUM(amount_rupiah) FILTER (WHERE side = 'DEBIT'), 0),
        COALESCE(SUM(amount_rupiah) FILTER (WHERE side = 'CREDIT'), 0)
    INTO entry_count, debit_total, credit_total
    FROM platform_ledger_entries
    WHERE journal_id = p_journal_id;

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

    IF source_journal_id IS NOT NULL AND EXISTS (
        SELECT 1
        FROM (
            (
                SELECT account_code, owner_profile_id, side, amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = p_journal_id
                EXCEPT ALL
                SELECT account_code, owner_profile_id,
                       CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END,
                       amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = source_journal_id
            )
            UNION ALL
            (
                SELECT account_code, owner_profile_id,
                       CASE side WHEN 'DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END,
                       amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = source_journal_id
                EXCEPT ALL
                SELECT account_code, owner_profile_id, side, amount_rupiah
                FROM platform_ledger_entries
                WHERE journal_id = p_journal_id
            )
        ) AS difference
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'platform_journal_reversal_guard',
            MESSAGE = 'reversal entries must exactly invert the source journal';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION validate_platform_journal_balance()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM validate_platform_journal_balance_for(NEW.id);
    RETURN NULL;
END;
$$;

CREATE FUNCTION validate_platform_ledger_entry_balance()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM validate_platform_journal_balance_for(NEW.journal_id);
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER platform_ledger_entry_balance_guard
AFTER INSERT ON platform_ledger_entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION validate_platform_ledger_entry_balance();

COMMIT;
