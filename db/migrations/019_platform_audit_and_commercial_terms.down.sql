BEGIN;

DROP TABLE IF EXISTS platform_commercial_terms;
DROP TABLE IF EXISTS platform_audit_logs;

-- NOTE: We do not DROP EXTENSION btree_gist here because other tables/migrations 
-- might use it, and down migration shouldn't break shared extensions.

COMMIT;
