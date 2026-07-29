-- Local Docker bootstrap only. Schema migrations run as lapangango_user;
-- the API connects as this non-owner, non-superuser runtime role.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lapangango_app') THEN
        CREATE ROLE lapangango_app
            LOGIN
            PASSWORD 'lapangango_app_password'
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            NOINHERIT
            NOREPLICATION
            NOBYPASSRLS;
    END IF;
END;
$$;

ALTER ROLE lapangango_app
    WITH LOGIN
    PASSWORD 'lapangango_app_password'
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOINHERIT
    NOREPLICATION
    NOBYPASSRLS;

REVOKE SET, ALTER SYSTEM
    ON PARAMETER session_replication_role
    FROM lapangango_app;
ALTER ROLE lapangango_app RESET session_replication_role;
ALTER ROLE lapangango_app IN DATABASE lapangango_db
    RESET session_replication_role;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE lapangango_db TO lapangango_app;
GRANT USAGE ON SCHEMA public TO lapangango_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO lapangango_app;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO lapangango_app;

ALTER DEFAULT PRIVILEGES FOR ROLE lapangango_user IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO lapangango_app;
ALTER DEFAULT PRIVILEGES FOR ROLE lapangango_user IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO lapangango_app;
