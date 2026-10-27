-- Creates the role the API and worker connect as. It is not a superuser, does not own any
-- table and cannot bypass row-level security. Migrations run as cashflow_owner instead.
--
-- Dev and test only: the password below is a placeholder. In staging and production create
-- the role by hand with a real secret and run the same GRANT and ALTER DEFAULT PRIVILEGES
-- statements.
CREATE ROLE cashflow_app LOGIN PASSWORD 'cashflow_app_dev'
    NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;

GRANT CONNECT ON DATABASE cashflow TO cashflow_app;
GRANT USAGE ON SCHEMA public TO cashflow_app;

-- Tables and sequences that cashflow_owner creates later (every migration) become usable by
-- the app role automatically. A migration that must be append-only revokes UPDATE and DELETE
-- on its own table.
ALTER DEFAULT PRIVILEGES FOR ROLE cashflow_owner IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO cashflow_app;
ALTER DEFAULT PRIVILEGES FOR ROLE cashflow_owner IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO cashflow_app;
