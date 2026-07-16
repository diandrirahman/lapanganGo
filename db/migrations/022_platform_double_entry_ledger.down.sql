BEGIN;

DO $$
DECLARE
    catalog_drift BOOLEAN;
BEGIN
    IF EXISTS (SELECT 1 FROM platform_journals)
       OR EXISTS (SELECT 1 FROM platform_ledger_entries) THEN
        RAISE EXCEPTION
            'cannot remove platform ledger migration after a financial fact exists';
    END IF;

    WITH expected(code, account_type, normal_side, owner_dimension) AS (
        VALUES
            ('BANK_CASH', 'ASSET', 'DEBIT', 'FORBIDDEN'),
            ('PSP_CLEARING', 'ASSET', 'DEBIT', 'FORBIDDEN'),
            ('FUNDING_CLEARING', 'LIABILITY', 'CREDIT', 'FORBIDDEN'),
            ('ACCOUNTS_PAYABLE', 'LIABILITY', 'CREDIT', 'FORBIDDEN'),
            ('OWNER_RECEIVABLE', 'ASSET', 'DEBIT', 'REQUIRED'),
            ('OWNER_PAYABLE', 'LIABILITY', 'CREDIT', 'REQUIRED'),
            ('CUSTOMER_REFUND_PAYABLE', 'LIABILITY', 'CREDIT', 'FORBIDDEN'),
            ('REFUND_CLEARING', 'ASSET', 'DEBIT', 'FORBIDDEN'),
            ('PAYOUT_CLEARING', 'ASSET', 'DEBIT', 'FORBIDDEN'),
            ('UNEARNED_COMMISSION', 'LIABILITY', 'CREDIT', 'FORBIDDEN'),
            ('UNEARNED_SERVICE_FEE', 'LIABILITY', 'CREDIT', 'FORBIDDEN'),
            ('COMMISSION_REVENUE', 'REVENUE', 'CREDIT', 'FORBIDDEN'),
            ('SERVICE_FEE_REVENUE', 'REVENUE', 'CREDIT', 'FORBIDDEN'),
            ('COMMISSION_REFUND', 'CONTRA_REVENUE', 'DEBIT', 'FORBIDDEN'),
            ('PAYMENT_PROCESSING_EXPENSE', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('REFUND_FEE_EXPENSE', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('PAYOUT_FEE_EXPENSE', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('CHARGEBACK_LOSS', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_INFRASTRUCTURE', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_MARKETING', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_CUSTOMER_SUPPORT', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_SALARY_CONTRACTOR', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_LEGAL_COMPLIANCE', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_PAYMENT_OPERATIONS', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_OFFICE_ADMIN', 'EXPENSE', 'DEBIT', 'FORBIDDEN'),
            ('OPEX_OTHER', 'EXPENSE', 'DEBIT', 'FORBIDDEN')
    ),
    mismatch AS (
        SELECT
            actual.code AS actual_code,
            expected.code AS expected_code,
            actual.account_type AS actual_type,
            expected.account_type AS expected_type,
            actual.normal_side AS actual_side,
            expected.normal_side AS expected_side,
            actual.owner_dimension AS actual_owner_dimension,
            expected.owner_dimension AS expected_owner_dimension
        FROM platform_accounts AS actual
        FULL OUTER JOIN expected ON expected.code = actual.code
    )
    SELECT
        (SELECT COUNT(*) FROM platform_accounts) <> 26
        OR EXISTS (
            SELECT 1
            FROM mismatch
            WHERE actual_code IS NULL
               OR expected_code IS NULL
               OR actual_type IS DISTINCT FROM expected_type
               OR actual_side IS DISTINCT FROM expected_side
               OR actual_owner_dimension IS DISTINCT FROM expected_owner_dimension
        )
    INTO catalog_drift;

    IF catalog_drift THEN
        RAISE EXCEPTION
            'cannot remove platform ledger migration after account catalog drift';
    END IF;
END;
$$;

DROP TRIGGER platform_journal_balance_guard ON platform_journals;
DROP TRIGGER validate_platform_ledger_entry_insert ON platform_ledger_entries;
DROP TRIGGER validate_platform_journal_reversal_source ON platform_journals;
DROP TRIGGER stamp_platform_journal_creation ON platform_journals;
DROP TRIGGER prevent_platform_ledger_entry_mutation ON platform_ledger_entries;
DROP TRIGGER prevent_platform_journal_mutation ON platform_journals;
DROP TRIGGER prevent_platform_account_catalog_mutation ON platform_accounts;

DROP TABLE platform_ledger_entries;
DROP TABLE platform_journals;
DROP TABLE platform_accounts;

DROP FUNCTION validate_platform_journal_balance();
DROP FUNCTION validate_platform_ledger_entry_insert();
DROP FUNCTION validate_platform_journal_reversal_source();
DROP FUNCTION stamp_platform_journal_creation();
DROP FUNCTION prevent_platform_ledger_mutation();
DROP FUNCTION prevent_platform_account_catalog_mutation();
DROP FUNCTION validate_platform_journal_metadata(JSONB);

COMMIT;
