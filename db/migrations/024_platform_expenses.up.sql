BEGIN;

CREATE TABLE platform_expenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category VARCHAR(40) NOT NULL,
    vendor VARCHAR(160) NULL,
    amount_rupiah BIGINT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    occurred_at TIMESTAMPTZ NOT NULL,
    payment_account VARCHAR(40) NOT NULL,
    external_reference VARCHAR(191) NULL,
    description TEXT NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'DRAFT',
    posted_journal_id UUID NULL REFERENCES platform_journals(id) ON DELETE RESTRICT,
    void_journal_id UUID NULL REFERENCES platform_journals(id) ON DELETE RESTRICT,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    approved_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    posted_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    voided_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    cancelled_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    cancel_reason TEXT NULL,
    void_reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    approved_at TIMESTAMPTZ NULL,
    posted_at TIMESTAMPTZ NULL,
    voided_at TIMESTAMPTZ NULL,
    cancelled_at TIMESTAMPTZ NULL,

    CONSTRAINT uq_platform_expenses_posted_journal UNIQUE (posted_journal_id),
    CONSTRAINT uq_platform_expenses_void_journal UNIQUE (void_journal_id),
    CONSTRAINT chk_platform_expense_distinct_journals CHECK (
        posted_journal_id IS NULL
        OR void_journal_id IS NULL
        OR posted_journal_id <> void_journal_id
    ),

    CONSTRAINT chk_platform_expense_category CHECK (
        category IN (
            'INFRASTRUCTURE',
            'MARKETING',
            'CUSTOMER_SUPPORT',
            'SALARY_CONTRACTOR',
            'LEGAL_COMPLIANCE',
            'PAYMENT_OPERATIONS',
            'OFFICE_ADMIN',
            'OTHER'
        )
    ),
    CONSTRAINT chk_platform_expense_amount CHECK (
        amount_rupiah BETWEEN 1 AND 1000000000
    ),
    CONSTRAINT chk_platform_expense_currency CHECK (currency = 'IDR'),
    CONSTRAINT chk_platform_expense_payment_account CHECK (
        payment_account IN ('FUNDING_CLEARING', 'ACCOUNTS_PAYABLE')
    ),
    CONSTRAINT chk_platform_expense_status CHECK (
        status IN ('DRAFT', 'APPROVED', 'POSTED', 'VOID', 'CANCELLED')
    ),
    CONSTRAINT chk_platform_expense_vendor CHECK (
        vendor IS NULL
        OR (
            vendor = BTRIM(vendor)
            AND vendor <> ''
            AND octet_length(vendor) <= 160
        )
    ),
    CONSTRAINT chk_platform_expense_external_reference CHECK (
        external_reference IS NULL
        OR (
            external_reference = BTRIM(external_reference)
            AND external_reference <> ''
            AND octet_length(external_reference) <= 191
        )
    ),
    CONSTRAINT chk_platform_expense_reference_vendor_dependency CHECK (
        external_reference IS NULL OR vendor IS NOT NULL
    ),
    CONSTRAINT chk_platform_expense_description CHECK (
        description = BTRIM(description)
        AND description <> ''
        AND octet_length(description) <= 500
    ),
    CONSTRAINT chk_platform_expense_cancel_reason CHECK (
        cancel_reason IS NULL
        OR (
            cancel_reason = BTRIM(cancel_reason)
            AND cancel_reason <> ''
            AND octet_length(cancel_reason) <= 500
        )
    ),
    CONSTRAINT chk_platform_expense_void_reason CHECK (
        void_reason IS NULL
        OR (
            void_reason = BTRIM(void_reason)
            AND void_reason <> ''
            AND octet_length(void_reason) <= 500
        )
    ),
    CONSTRAINT chk_platform_expense_timestamp_order CHECK (
        (approved_at IS NULL OR approved_at >= created_at)
        AND (posted_at IS NULL OR (approved_at IS NOT NULL AND posted_at >= approved_at))
        AND (voided_at IS NULL OR (posted_at IS NOT NULL AND voided_at >= posted_at))
        AND (cancelled_at IS NULL OR cancelled_at >= created_at)
    ),
    CONSTRAINT chk_platform_expense_state_shape CHECK (
        (
            status = 'DRAFT'
            AND approved_at IS NULL
            AND posted_at IS NULL
            AND voided_at IS NULL
            AND cancelled_at IS NULL
            AND posted_journal_id IS NULL
            AND void_journal_id IS NULL
            AND approved_by_user_id IS NULL
            AND posted_by_user_id IS NULL
            AND voided_by_user_id IS NULL
            AND cancelled_by_user_id IS NULL
            AND cancel_reason IS NULL
            AND void_reason IS NULL
        )
        OR (
            status = 'APPROVED'
            AND approved_at IS NOT NULL
            AND posted_at IS NULL
            AND voided_at IS NULL
            AND cancelled_at IS NULL
            AND posted_journal_id IS NULL
            AND void_journal_id IS NULL
            AND posted_by_user_id IS NULL
            AND voided_by_user_id IS NULL
            AND cancelled_by_user_id IS NULL
            AND cancel_reason IS NULL
            AND void_reason IS NULL
        )
        OR (
            status = 'POSTED'
            AND approved_at IS NOT NULL
            AND posted_at IS NOT NULL
            AND posted_journal_id IS NOT NULL
            AND voided_at IS NULL
            AND cancelled_at IS NULL
            AND void_journal_id IS NULL
            AND voided_by_user_id IS NULL
            AND cancelled_by_user_id IS NULL
            AND cancel_reason IS NULL
            AND void_reason IS NULL
        )
        OR (
            status = 'VOID'
            AND approved_at IS NOT NULL
            AND posted_at IS NOT NULL
            AND posted_journal_id IS NOT NULL
            AND voided_at IS NOT NULL
            AND void_journal_id IS NOT NULL
            AND void_reason IS NOT NULL
            AND cancelled_at IS NULL
            AND cancelled_by_user_id IS NULL
            AND cancel_reason IS NULL
        )
        OR (
            status = 'CANCELLED'
            AND cancelled_at IS NOT NULL
            AND cancel_reason IS NOT NULL
            AND approved_at IS NULL
            AND posted_at IS NULL
            AND voided_at IS NULL
            AND posted_journal_id IS NULL
            AND void_journal_id IS NULL
            AND approved_by_user_id IS NULL
            AND posted_by_user_id IS NULL
            AND voided_by_user_id IS NULL
            AND void_reason IS NULL
        )
    )
);

CREATE UNIQUE INDEX uq_platform_expenses_vendor_external_reference
    ON platform_expenses (
        LOWER(BTRIM(vendor)),
        LOWER(BTRIM(external_reference))
    )
    WHERE external_reference IS NOT NULL;

CREATE INDEX idx_platform_expenses_status_occurred_at
    ON platform_expenses (status, occurred_at DESC, created_at DESC, id DESC);

CREATE INDEX idx_platform_expenses_category_occurred_at
    ON platform_expenses (category, occurred_at DESC, id DESC);

CREATE FUNCTION platform_expense_occurred_at_is_allowed(
    p_occurred_at TIMESTAMPTZ,
    p_reference_at TIMESTAMPTZ
)
RETURNS BOOLEAN
LANGUAGE SQL
IMMUTABLE
STRICT
AS $$
    SELECT p_occurred_at BETWEEN p_reference_at - INTERVAL '90 days' AND p_reference_at;
$$;

CREATE FUNCTION validate_platform_expense_write()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    mutation_time TIMESTAMPTZ;
BEGIN
    mutation_time := clock_timestamp();

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'platform expenses are append-only; delete is forbidden';
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'DRAFT' THEN
            RAISE EXCEPTION USING
                ERRCODE = '23514',
                CONSTRAINT = 'chk_platform_expense_initial_state',
                MESSAGE = 'new platform expenses must start as DRAFT';
        END IF;

        IF NOT platform_expense_occurred_at_is_allowed(NEW.occurred_at, mutation_time) THEN
            RAISE EXCEPTION USING
                ERRCODE = '23514',
                CONSTRAINT = 'chk_platform_expense_occurred_at_policy',
                MESSAGE = 'occurred_at must be within the 90-day backdate window and not future-dated';
        END IF;

        NEW.created_at := mutation_time;
        RETURN NEW;
    END IF;

    IF ROW(
        NEW.category,
        NEW.vendor,
        NEW.amount_rupiah,
        NEW.currency,
        NEW.occurred_at,
        NEW.payment_account,
        NEW.external_reference,
        NEW.description,
        NEW.created_by_user_id,
        NEW.created_at
    ) IS DISTINCT FROM ROW(
        OLD.category,
        OLD.vendor,
        OLD.amount_rupiah,
        OLD.currency,
        OLD.occurred_at,
        OLD.payment_account,
        OLD.external_reference,
        OLD.description,
        OLD.created_by_user_id,
        OLD.created_at
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '55000',
            MESSAGE = 'platform expense business fields are immutable';
    END IF;

    IF NEW.occurred_at > mutation_time THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_occurred_at_policy',
            MESSAGE = 'occurred_at cannot be future-dated';
    END IF;

    IF OLD.status <> NEW.status THEN
        IF NEW.approved_by_user_id IS DISTINCT FROM OLD.approved_by_user_id
           AND NOT (
               NEW.status = 'APPROVED'
               AND OLD.approved_by_user_id IS NULL
               AND NEW.approved_by_user_id IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'approved actor is immutable across transitions';
        END IF;
        IF NEW.posted_by_user_id IS DISTINCT FROM OLD.posted_by_user_id
           AND NOT (
               NEW.status = 'POSTED'
               AND OLD.posted_by_user_id IS NULL
               AND NEW.posted_by_user_id IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'posted actor is immutable across transitions';
        END IF;
        IF NEW.voided_by_user_id IS DISTINCT FROM OLD.voided_by_user_id
           AND NOT (
               NEW.status = 'VOID'
               AND OLD.voided_by_user_id IS NULL
               AND NEW.voided_by_user_id IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'voided actor is immutable across transitions';
        END IF;
        IF NEW.cancelled_by_user_id IS DISTINCT FROM OLD.cancelled_by_user_id
           AND NOT (
               NEW.status = 'CANCELLED'
               AND OLD.cancelled_by_user_id IS NULL
               AND NEW.cancelled_by_user_id IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'cancelled actor is immutable across transitions';
        END IF;

        IF NEW.approved_at IS DISTINCT FROM OLD.approved_at
           AND NOT (
               NEW.status = 'APPROVED'
               AND OLD.approved_at IS NULL
               AND NEW.approved_at IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'approved timestamp is immutable across transitions';
        END IF;
        IF NEW.posted_at IS DISTINCT FROM OLD.posted_at
           AND NOT (
               NEW.status = 'POSTED'
               AND OLD.posted_at IS NULL
               AND NEW.posted_at IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'posted timestamp is immutable across transitions';
        END IF;
        IF NEW.voided_at IS DISTINCT FROM OLD.voided_at
           AND NOT (
               NEW.status = 'VOID'
               AND OLD.voided_at IS NULL
               AND NEW.voided_at IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'voided timestamp is immutable across transitions';
        END IF;
        IF NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at
           AND NOT (
               NEW.status = 'CANCELLED'
               AND OLD.cancelled_at IS NULL
               AND NEW.cancelled_at IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'cancelled timestamp is immutable across transitions';
        END IF;

        IF NEW.posted_journal_id IS DISTINCT FROM OLD.posted_journal_id
           AND NOT (
               NEW.status = 'POSTED'
               AND OLD.posted_journal_id IS NULL
               AND NEW.posted_journal_id IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'posted journal is immutable across transitions';
        END IF;
        IF NEW.void_journal_id IS DISTINCT FROM OLD.void_journal_id
           AND NOT (
               NEW.status = 'VOID'
               AND OLD.void_journal_id IS NULL
               AND NEW.void_journal_id IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'void journal is immutable across transitions';
        END IF;
        IF NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason
           AND NOT (
               NEW.status = 'CANCELLED'
               AND OLD.cancel_reason IS NULL
               AND NEW.cancel_reason IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'cancel reason is immutable across transitions';
        END IF;
        IF NEW.void_reason IS DISTINCT FROM OLD.void_reason
           AND NOT (
               NEW.status = 'VOID'
               AND OLD.void_reason IS NULL
               AND NEW.void_reason IS NOT NULL
           ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'void reason is immutable across transitions';
        END IF;
    END IF;

    IF OLD.status = NEW.status THEN
        IF NEW.approved_by_user_id IS DISTINCT FROM OLD.approved_by_user_id
           AND NOT (OLD.approved_by_user_id IS NOT NULL AND NEW.approved_by_user_id IS NULL) THEN
            RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'approved actor is immutable';
        END IF;
        IF NEW.posted_by_user_id IS DISTINCT FROM OLD.posted_by_user_id
           AND NOT (OLD.posted_by_user_id IS NOT NULL AND NEW.posted_by_user_id IS NULL) THEN
            RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'posted actor is immutable';
        END IF;
        IF NEW.voided_by_user_id IS DISTINCT FROM OLD.voided_by_user_id
           AND NOT (OLD.voided_by_user_id IS NOT NULL AND NEW.voided_by_user_id IS NULL) THEN
            RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'voided actor is immutable';
        END IF;
        IF NEW.cancelled_by_user_id IS DISTINCT FROM OLD.cancelled_by_user_id
           AND NOT (OLD.cancelled_by_user_id IS NOT NULL AND NEW.cancelled_by_user_id IS NULL) THEN
            RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'cancelled actor is immutable';
        END IF;

        IF ROW(
            NEW.approved_at,
            NEW.posted_at,
            NEW.voided_at,
            NEW.cancelled_at,
            NEW.posted_journal_id,
            NEW.void_journal_id,
            NEW.cancel_reason,
            NEW.void_reason
        ) IS DISTINCT FROM ROW(
            OLD.approved_at,
            OLD.posted_at,
            OLD.voided_at,
            OLD.cancelled_at,
            OLD.posted_journal_id,
            OLD.void_journal_id,
            OLD.cancel_reason,
            OLD.void_reason
        ) THEN
            RAISE EXCEPTION USING
                ERRCODE = '55000',
                MESSAGE = 'platform expense state fields require a valid transition';
        END IF;

        RETURN NEW;
    END IF;

    IF NOT (
        (OLD.status = 'DRAFT' AND NEW.status IN ('APPROVED', 'CANCELLED'))
        OR (OLD.status = 'APPROVED' AND NEW.status = 'POSTED')
        OR (OLD.status = 'POSTED' AND NEW.status = 'VOID')
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_transition',
            MESSAGE = 'invalid platform expense state transition';
    END IF;

    IF NEW.status = 'APPROVED' AND NEW.approved_by_user_id IS NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_approved_actor',
            MESSAGE = 'approved transition requires an actor';
    END IF;
    IF NEW.status = 'POSTED' AND NEW.posted_by_user_id IS NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_posted_actor',
            MESSAGE = 'posted transition requires an actor';
    END IF;
    IF NEW.status = 'VOID' AND NEW.voided_by_user_id IS NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_voided_actor',
            MESSAGE = 'void transition requires an actor';
    END IF;
    IF NEW.status = 'CANCELLED' AND NEW.cancelled_by_user_id IS NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_cancelled_actor',
            MESSAGE = 'cancel transition requires an actor';
    END IF;

    IF NEW.approved_at IS NOT NULL AND NEW.approved_at > mutation_time THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_approved_at_policy',
            MESSAGE = 'approved_at cannot be future-dated';
    END IF;
    IF NEW.posted_at IS NOT NULL AND NEW.posted_at > mutation_time THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_posted_at_policy',
            MESSAGE = 'posted_at cannot be future-dated';
    END IF;
    IF NEW.voided_at IS NOT NULL AND NEW.voided_at > mutation_time THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_voided_at_policy',
            MESSAGE = 'voided_at cannot be future-dated';
    END IF;
    IF NEW.cancelled_at IS NOT NULL AND NEW.cancelled_at > mutation_time THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'chk_platform_expense_cancelled_at_policy',
            MESSAGE = 'cancelled_at cannot be future-dated';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER platform_expense_write_guard
BEFORE INSERT OR UPDATE OR DELETE ON platform_expenses
FOR EACH ROW
EXECUTE FUNCTION validate_platform_expense_write();

CREATE TABLE platform_expense_idempotency (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action VARCHAR(8) NOT NULL CHECK (action IN ('CREATE', 'CANCEL', 'APPROVE', 'POST', 'VOID')),
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    expense_id UUID NOT NULL REFERENCES platform_expenses(id) ON DELETE RESTRICT,
    response_status SMALLINT NOT NULL CHECK (response_status BETWEEN 200 AND 599),
    response_body JSONB NOT NULL CHECK (jsonb_typeof(response_body) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),

    CONSTRAINT chk_platform_expense_idempotency_key CHECK (
        idempotency_key = BTRIM(idempotency_key)
        AND octet_length(idempotency_key) BETWEEN 1 AND 255
    ),
    CONSTRAINT chk_platform_expense_idempotency_hash CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT uq_platform_expense_idempotency_scope UNIQUE (
        actor_user_id,
        action,
        idempotency_key
    )
);

CREATE INDEX idx_platform_expense_idempotency_expense
    ON platform_expense_idempotency (expense_id);

CREATE FUNCTION prevent_platform_expense_idempotency_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'platform expense idempotency records are immutable';
END;
$$;

CREATE TRIGGER prevent_platform_expense_idempotency_mutation
BEFORE UPDATE OR DELETE ON platform_expense_idempotency
FOR EACH ROW
EXECUTE FUNCTION prevent_platform_expense_idempotency_mutation();

COMMIT;
