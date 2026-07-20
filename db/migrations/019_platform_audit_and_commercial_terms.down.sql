BEGIN;

DO $$
DECLARE
    term_count INTEGER;
    valid_seed_count INTEGER;
BEGIN
    IF EXISTS (SELECT 1 FROM platform_audit_logs LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot rollback: platform_audit_logs contains data.';
    END IF;

    SELECT COUNT(*) INTO term_count FROM platform_commercial_terms;
    
    IF term_count != 1 THEN
        RAISE EXCEPTION 'Cannot rollback: platform_commercial_terms must contain exactly one frozen seed term.';
    END IF;

    SELECT COUNT(*) INTO valid_seed_count FROM platform_commercial_terms
    WHERE owner_profile_id IS NULL
      AND scope_key = 'GLOBAL'
      AND label = 'Global Default Term'
      AND phase = 'STANDARD'
      AND finance_mode = 'SIMULATION'
      AND collection_method = 'NONE'
      AND commission_bps = 700
      AND valid_until IS NULL
      AND supersedes_id IS NULL
      AND created_by_user_id IS NULL;

    IF valid_seed_count != 1 THEN
        RAISE EXCEPTION 'Cannot rollback: frozen global default term seed is mutated.';
    END IF;
END $$;

DROP TABLE IF EXISTS platform_commercial_terms;
DROP TABLE IF EXISTS platform_audit_logs;

-- NOTE: We do not DROP EXTENSION btree_gist here because other tables/migrations 
-- might use it, and down migration shouldn't break shared extensions.

COMMIT;
