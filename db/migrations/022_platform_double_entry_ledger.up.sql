BEGIN;

-- Phase 3A owns a fixed, migration-seeded chart of accounts. Runtime code is
-- not allowed to create, alter, or delete catalog rows.
CREATE TABLE platform_accounts (
    code VARCHAR(80) PRIMARY KEY,
    account_type VARCHAR(30) NOT NULL CHECK (
        account_type IN ('ASSET', 'LIABILITY', 'REVENUE', 'CONTRA_REVENUE', 'EXPENSE')
    ),
    normal_side VARCHAR(6) NOT NULL CHECK (normal_side IN ('DEBIT', 'CREDIT')),
    owner_dimension VARCHAR(10) NOT NULL CHECK (owner_dimension IN ('REQUIRED', 'FORBIDDEN')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),

    CONSTRAINT chk_platform_account_type_side CHECK (
        (account_type IN ('ASSET', 'CONTRA_REVENUE', 'EXPENSE') AND normal_side = 'DEBIT')
        OR
        (account_type IN ('LIABILITY', 'REVENUE') AND normal_side = 'CREDIT')
    ),
    CONSTRAINT chk_platform_account_owner_dimension CHECK (
        (code IN ('OWNER_PAYABLE', 'OWNER_RECEIVABLE') AND owner_dimension = 'REQUIRED')
        OR
        (code NOT IN ('OWNER_PAYABLE', 'OWNER_RECEIVABLE') AND owner_dimension = 'FORBIDDEN')
    )
);

INSERT INTO platform_accounts (code, account_type, normal_side, owner_dimension)
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
    ('OPEX_OTHER', 'EXPENSE', 'DEBIT', 'FORBIDDEN');

-- The initial metadata contract is deliberately small. Domain-specific
-- extensions belong to later tasks and must update both the Go validator and
-- this database boundary together.
CREATE FUNCTION validate_platform_journal_metadata(p_metadata JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
DECLARE
    item RECORD;
    scalar_value TEXT;
BEGIN
    IF jsonb_typeof(p_metadata) <> 'object' THEN
        RETURN FALSE;
    END IF;

    FOR item IN SELECT key, value FROM jsonb_each(p_metadata) LOOP
        IF item.key NOT IN ('source_type', 'source_reference', 'reason_code', 'calculation_version') THEN
            RETURN FALSE;
        END IF;

        IF jsonb_typeof(item.value) <> 'string' THEN
            RETURN FALSE;
        END IF;

        scalar_value := item.value #>> '{}';
        IF scalar_value IS NULL
           OR BTRIM(scalar_value) <> scalar_value
           OR scalar_value = ''
           OR octet_length(scalar_value) > 191
           OR scalar_value ~* '(secret|token|password|authorization|credential|payload|pii|bearer)' THEN
            RETURN FALSE;
        END IF;
    END LOOP;

    RETURN TRUE;
END;
$$;

CREATE TABLE platform_journals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_key VARCHAR(191) NOT NULL UNIQUE,
    event_type VARCHAR(80) NOT NULL,
    payload_hash VARCHAR(64) NOT NULL,
    payload_hash_version VARCHAR(30) NOT NULL DEFAULT 'JOURNAL_PAYLOAD_V1',
    booking_id UUID NULL REFERENCES bookings(id) ON DELETE RESTRICT,
    owner_profile_id UUID NULL REFERENCES owner_profiles(id) ON DELETE RESTRICT,
    venue_id UUID NULL REFERENCES venues(id) ON DELETE RESTRICT,
    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    effective_at TIMESTAMPTZ NOT NULL,
    posted_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    reverses_journal_id UUID NULL UNIQUE REFERENCES platform_journals(id) ON DELETE RESTRICT,
    reversal_reason TEXT NULL,
    created_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    description TEXT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),

    -- Internal transaction identity seals the entry set without permitting a
    -- later append to an already committed journal.
    created_txid BIGINT NOT NULL DEFAULT txid_current(),

    CONSTRAINT chk_platform_journal_event_key CHECK (
        octet_length(event_key) BETWEEN 1 AND 191
        AND event_key ~ '^[a-z0-9][a-z0-9_-]*\.[a-z0-9][a-z0-9_-]*:[a-z0-9][a-z0-9._-]*(:[a-z0-9][a-z0-9._-]*)?$'
    ),
    CONSTRAINT chk_platform_journal_event_type CHECK (
        event_type ~ '^[A-Z][A-Z0-9_]{0,79}$'
    ),
    CONSTRAINT chk_platform_journal_payload_hash CHECK (
        payload_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_platform_journal_payload_hash_version CHECK (
        payload_hash_version = 'JOURNAL_PAYLOAD_V1'
    ),
    CONSTRAINT chk_platform_journal_currency CHECK (currency = 'IDR'),
    CONSTRAINT chk_platform_journal_effective_time CHECK (effective_at <= posted_at),
    CONSTRAINT chk_platform_journal_reversal_pair CHECK (
        (reverses_journal_id IS NULL AND reversal_reason IS NULL)
        OR
        (
            reverses_journal_id IS NOT NULL
            AND reversal_reason IS NOT NULL
            AND BTRIM(reversal_reason) = reversal_reason
            AND BTRIM(reversal_reason) <> ''
            AND octet_length(reversal_reason) <= 500
        )
    ),
    CONSTRAINT chk_platform_journal_metadata CHECK (
        validate_platform_journal_metadata(metadata)
    ),
    CONSTRAINT chk_platform_journal_reversal_event_key CHECK (
        (reverses_journal_id IS NULL AND event_key NOT LIKE 'journal.reversed:%')
        OR
        (reverses_journal_id IS NOT NULL AND event_key = 'journal.reversed:' || reverses_journal_id::TEXT)
    )
);

CREATE TABLE platform_ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_id UUID NOT NULL REFERENCES platform_journals(id) ON DELETE RESTRICT,
    account_code VARCHAR(80) NOT NULL REFERENCES platform_accounts(code) ON DELETE RESTRICT,
    owner_profile_id UUID NULL REFERENCES owner_profiles(id) ON DELETE RESTRICT,
    side VARCHAR(6) NOT NULL CHECK (side IN ('DEBIT', 'CREDIT')),
    amount_rupiah BIGINT NOT NULL CHECK (amount_rupiah > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    created_txid BIGINT NOT NULL DEFAULT txid_current()
);

CREATE INDEX idx_platform_journals_effective_at_id
    ON platform_journals (effective_at DESC, id DESC);
CREATE INDEX idx_platform_journals_event_type_effective_at
    ON platform_journals (event_type, effective_at DESC, id DESC);
CREATE INDEX idx_platform_journals_owner_effective_at
    ON platform_journals (owner_profile_id, effective_at DESC, id DESC);
CREATE INDEX idx_platform_journals_booking
    ON platform_journals (booking_id)
    WHERE booking_id IS NOT NULL;
CREATE INDEX idx_platform_ledger_entries_journal
    ON platform_ledger_entries (journal_id);
CREATE INDEX idx_platform_ledger_entries_account
    ON platform_ledger_entries (account_code, created_at DESC);
CREATE INDEX idx_platform_ledger_entries_owner
    ON platform_ledger_entries (owner_profile_id, created_at DESC)
    WHERE owner_profile_id IS NOT NULL;

CREATE FUNCTION prevent_platform_ledger_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'updates and deletes are forbidden for immutable platform ledger facts';
END;
$$;

CREATE FUNCTION prevent_platform_account_catalog_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'platform account catalog is migration-owned and immutable';
END;
$$;

CREATE TRIGGER prevent_platform_account_catalog_mutation
BEFORE INSERT OR UPDATE OR DELETE ON platform_accounts
FOR EACH ROW
EXECUTE FUNCTION prevent_platform_account_catalog_mutation();

CREATE TRIGGER prevent_platform_journal_mutation
BEFORE UPDATE OR DELETE ON platform_journals
FOR EACH ROW
EXECUTE FUNCTION prevent_platform_ledger_mutation();

CREATE TRIGGER prevent_platform_ledger_entry_mutation
BEFORE UPDATE OR DELETE ON platform_ledger_entries
FOR EACH ROW
EXECUTE FUNCTION prevent_platform_ledger_mutation();

CREATE FUNCTION stamp_platform_journal_creation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    posting_time TIMESTAMPTZ;
BEGIN
    -- Use wall-clock time at the insert, not the transaction start. A
    -- transaction-aware caller may validate effective_at after BEGIN, and a
    -- valid current event must not look future-dated because the transaction
    -- remained open for a short time.
    posting_time := clock_timestamp();
    NEW.posted_at := posting_time;
    NEW.created_at := posting_time;
    NEW.created_txid := txid_current();
    RETURN NEW;
END;
$$;

CREATE TRIGGER stamp_platform_journal_creation
BEFORE INSERT ON platform_journals
FOR EACH ROW
EXECUTE FUNCTION stamp_platform_journal_creation();

CREATE FUNCTION validate_platform_journal_reversal_source()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    source_reversal UUID;
    source_txid BIGINT;
BEGIN
    IF NEW.reverses_journal_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT reverses_journal_id, created_txid
    INTO source_reversal, source_txid
    FROM platform_journals
    WHERE id = NEW.reverses_journal_id
    FOR KEY SHARE;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23503',
            MESSAGE = 'reversal source journal does not exist';
    END IF;

    IF source_reversal IS NOT NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'a reversal journal cannot itself be reversed';
    END IF;

    IF source_txid = txid_current() THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'a journal cannot be reversed in its creation transaction';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER validate_platform_journal_reversal_source
BEFORE INSERT ON platform_journals
FOR EACH ROW
EXECUTE FUNCTION validate_platform_journal_reversal_source();

CREATE FUNCTION validate_platform_ledger_entry_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    journal_txid BIGINT;
    journal_owner UUID;
    required_owner_dimension VARCHAR(10);
BEGIN
    SELECT created_txid, owner_profile_id
    INTO journal_txid, journal_owner
    FROM platform_journals
    WHERE id = NEW.journal_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23503',
            MESSAGE = 'entry journal does not exist';
    END IF;

    IF journal_txid <> txid_current() THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'entries cannot be appended after the journal transaction';
    END IF;

    SELECT owner_dimension
    INTO required_owner_dimension
    FROM platform_accounts
    WHERE code = NEW.account_code;

    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23503',
            MESSAGE = 'entry account does not exist';
    END IF;

    IF required_owner_dimension = 'REQUIRED' THEN
        IF NEW.owner_profile_id IS NULL OR journal_owner IS NULL OR NEW.owner_profile_id <> journal_owner THEN
            RAISE EXCEPTION USING
                ERRCODE = '23514',
                MESSAGE = 'owner dimension is required and must match the journal owner';
        END IF;
    ELSIF NEW.owner_profile_id IS NOT NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'owner dimension is forbidden for this account';
    END IF;

    NEW.created_at := transaction_timestamp();
    NEW.created_txid := txid_current();
    RETURN NEW;
END;
$$;

CREATE TRIGGER validate_platform_ledger_entry_insert
BEFORE INSERT ON platform_ledger_entries
FOR EACH ROW
EXECUTE FUNCTION validate_platform_ledger_entry_insert();

CREATE FUNCTION validate_platform_journal_balance()
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

CREATE CONSTRAINT TRIGGER platform_journal_balance_guard
AFTER INSERT ON platform_journals
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION validate_platform_journal_balance();

COMMIT;
