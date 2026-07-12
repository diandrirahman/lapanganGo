BEGIN;

-- Create extension for exclusion constraints (if not exists)
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- 1. Platform Audit Logs
CREATE TABLE IF NOT EXISTS platform_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_role VARCHAR(50) NOT NULL,
    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(100) NOT NULL,
    entity_id UUID,
    owner_profile_id UUID REFERENCES owner_profiles(id) ON DELETE SET NULL,
    venue_id UUID REFERENCES venues(id) ON DELETE SET NULL,
    correlation_id VARCHAR(255),
    metadata JSONB NOT NULL DEFAULT '{}',
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_actor ON platform_audit_logs(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_action ON platform_audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_entity ON platform_audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_owner ON platform_audit_logs(owner_profile_id);
CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_created_at ON platform_audit_logs(created_at);

-- 2. Platform Commercial Terms
CREATE TABLE IF NOT EXISTS platform_commercial_terms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_profile_id UUID REFERENCES owner_profiles(id) ON DELETE RESTRICT,
    scope_key VARCHAR(50) GENERATED ALWAYS AS (COALESCE(owner_profile_id::text, 'GLOBAL')) STORED,
    label VARCHAR(120) NOT NULL,
    phase VARCHAR(50) NOT NULL CHECK (phase IN ('TRIAL', 'INTRODUCTORY', 'STANDARD', 'CUSTOM')),
    finance_mode VARCHAR(50) NOT NULL CHECK (finance_mode IN ('SIMULATION', 'LIVE')),
    collection_method VARCHAR(50) NOT NULL CHECK (collection_method IN ('NONE', 'DEDUCT_FROM_PAYOUT')),
    commission_bps INTEGER NOT NULL CHECK (commission_bps >= 0 AND commission_bps <= 3000),
    valid_from TIMESTAMPTZ NOT NULL,
    valid_until TIMESTAMPTZ,
    supersedes_id UUID REFERENCES platform_commercial_terms(id) ON DELETE RESTRICT,
    created_by_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_pct_valid_until_after_from CHECK (valid_until IS NULL OR valid_until > valid_from)
);

-- Index for querying active terms
CREATE INDEX IF NOT EXISTS idx_pct_owner_valid_time ON platform_commercial_terms(owner_profile_id, valid_from, valid_until);

-- Exclusion constraint to prevent overlapping terms for the same owner (or global if owner_profile_id is NULL).
ALTER TABLE platform_commercial_terms
ADD CONSTRAINT excl_pct_no_overlap EXCLUDE USING gist (
    scope_key WITH =,
    tstzrange(valid_from, COALESCE(valid_until, 'infinity'), '[)') WITH &&
);

-- Partial unique open-ended as an additional protection
CREATE UNIQUE INDEX IF NOT EXISTS idx_pct_single_open_ended ON platform_commercial_terms(scope_key) WHERE valid_until IS NULL;

-- 3. Seed the global default term
INSERT INTO platform_commercial_terms (
    id,
    owner_profile_id,
    label,
    phase,
    finance_mode,
    collection_method,
    commission_bps,
    valid_from,
    created_at
) VALUES (
    gen_random_uuid(),
    NULL,
    'Global Default Term',
    'STANDARD',
    'SIMULATION',
    'NONE',
    700,
    now(),
    now()
);

COMMIT;
