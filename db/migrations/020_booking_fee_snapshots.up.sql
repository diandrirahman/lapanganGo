BEGIN;

-- 1. Create table platform_finance_cutovers
CREATE TABLE platform_finance_cutovers (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    snapshot_cutover_at TIMESTAMPTZ NOT NULL,
    calculation_version VARCHAR(30) NOT NULL,
    release_reference VARCHAR(255) NOT NULL,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (BTRIM(calculation_version) <> ''),
    CHECK (BTRIM(release_reference) <> '')
);

-- 2. Create table booking_fee_snapshots
CREATE TABLE booking_fee_snapshots (
    booking_id UUID PRIMARY KEY REFERENCES bookings(id) ON DELETE RESTRICT,
    owner_profile_id UUID NOT NULL REFERENCES owner_profiles(id) ON DELETE RESTRICT,
    venue_id UUID NOT NULL REFERENCES venues(id) ON DELETE RESTRICT,
    commercial_term_id UUID NULL REFERENCES platform_commercial_terms(id) ON DELETE RESTRICT,

    terms_source VARCHAR(30) NOT NULL,
    booking_channel VARCHAR(30) NOT NULL,
    finance_mode VARCHAR(20) NOT NULL,

    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    currency_exponent SMALLINT NOT NULL DEFAULT 0,

    original_price_rupiah BIGINT NOT NULL,
    owner_price_adjustment_rupiah BIGINT NOT NULL,
    price_adjustment_reason TEXT NULL,
    final_booking_price_rupiah BIGINT NOT NULL,

    customer_service_fee_rupiah BIGINT NOT NULL DEFAULT 0,
    customer_charge_amount_rupiah BIGINT NOT NULL,
    commission_basis_amount_rupiah BIGINT NOT NULL,
    commission_bps INTEGER NOT NULL,
    commission_amount_rupiah BIGINT NOT NULL,
    owner_net_amount_rupiah BIGINT NOT NULL,

    calculation_version VARCHAR(30) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Allowlist constraints
    CHECK (terms_source IN ('POLICY', 'LEGACY_NO_COMMISSION')),
    CHECK (booking_channel IN ('MARKETPLACE_ONLINE', 'OWNER_WALK_IN')),
    CHECK (finance_mode IN ('SIMULATION', 'LIVE')),
    CHECK (currency = 'IDR'),
    CHECK (currency_exponent = 0),
    CHECK (BTRIM(calculation_version) <> ''),

    -- Money Constraints
    CHECK (original_price_rupiah >= 0),
    CHECK (final_booking_price_rupiah >= 0),
    CHECK (customer_service_fee_rupiah = 0),
    CHECK (customer_charge_amount_rupiah >= 0),
    CHECK (commission_basis_amount_rupiah >= 0),
    CHECK (commission_bps BETWEEN 0 AND 3000),
    CHECK (commission_amount_rupiah >= 0),
    CHECK (owner_net_amount_rupiah >= 0),
    CHECK (commission_amount_rupiah <= commission_basis_amount_rupiah),

    -- Arithmetic Constraints
    CHECK (final_booking_price_rupiah::numeric = original_price_rupiah::numeric + owner_price_adjustment_rupiah::numeric),
    CHECK (customer_charge_amount_rupiah::numeric = final_booking_price_rupiah::numeric + customer_service_fee_rupiah::numeric),
    CHECK (commission_basis_amount_rupiah::numeric = final_booking_price_rupiah::numeric),
    CHECK (owner_net_amount_rupiah::numeric = commission_basis_amount_rupiah::numeric - commission_amount_rupiah::numeric),
    CHECK (commission_amount_rupiah::numeric = ROUND((commission_basis_amount_rupiah::numeric * commission_bps::numeric) / 10000::numeric, 0)),

    -- Adjustment Reason
    CHECK (
        owner_price_adjustment_rupiah = 0
        OR (
            price_adjustment_reason IS NOT NULL
            AND BTRIM(price_adjustment_reason) <> ''
        )
    ),

    -- Term-source Consistency
    CHECK (
        (
            terms_source = 'POLICY'
            AND commercial_term_id IS NOT NULL
        )
        OR
        (
            terms_source = 'LEGACY_NO_COMMISSION'
            AND commercial_term_id IS NULL
        )
    ),

    -- Logic Invariants
    CHECK (
        NOT (booking_channel = 'OWNER_WALK_IN' AND (commission_bps <> 0 OR commission_amount_rupiah <> 0))
    ),
    CHECK (
        NOT (terms_source = 'LEGACY_NO_COMMISSION' AND (commission_bps <> 0 OR commission_amount_rupiah <> 0))
    )
);

-- 3. Immutability Function and Triggers
CREATE FUNCTION prevent_platform_finance_immutable_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Updates and Deletes are strictly forbidden on immutable platform finance tables';
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_mutation_platform_finance_cutovers
BEFORE UPDATE OR DELETE ON platform_finance_cutovers
FOR EACH ROW
EXECUTE FUNCTION prevent_platform_finance_immutable_mutation();

CREATE TRIGGER prevent_mutation_booking_fee_snapshots
BEFORE UPDATE OR DELETE ON booking_fee_snapshots
FOR EACH ROW
EXECUTE FUNCTION prevent_platform_finance_immutable_mutation();

COMMIT;
