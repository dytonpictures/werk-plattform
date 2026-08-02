\set ON_ERROR_STOP on

SET password_encryption = 'scram-sha-256';

SELECT 'CREATE ROLE werk_owner'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_owner')
\gexec

SELECT 'CREATE ROLE werk_migrator'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_migrator')
\gexec

SELECT 'CREATE ROLE werk_work_runtime'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_work_runtime')
\gexec

SELECT 'CREATE ROLE werk_identity_runtime'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_identity_runtime')
\gexec

SELECT 'CREATE ROLE werk_admin_runtime'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_admin_runtime')
\gexec

SELECT 'CREATE ROLE werk_service_runtime'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_service_runtime')
\gexec

SELECT 'CREATE ROLE werk_worker_runtime'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_worker_runtime')
\gexec

SELECT 'CREATE ROLE werk_backup_reader'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_backup_reader')
\gexec

SELECT 'CREATE ROLE werk_backup'
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'werk_backup')
\gexec

ALTER ROLE werk_owner
    NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;

ALTER ROLE werk_migrator
    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE werk_work_runtime
    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE werk_identity_runtime
    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE werk_admin_runtime
    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE werk_service_runtime
    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE werk_worker_runtime
    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
ALTER ROLE werk_backup_reader
    NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION BYPASSRLS;
ALTER ROLE werk_backup
    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS
    CONNECTION LIMIT 1;

SELECT pg_catalog.format('ALTER ROLE werk_migrator PASSWORD %L', :'migrator_password')
\gexec
SELECT pg_catalog.format('ALTER ROLE werk_work_runtime PASSWORD %L', :'work_password')
\gexec
SELECT pg_catalog.format('ALTER ROLE werk_identity_runtime PASSWORD %L', :'identity_password')
\gexec
SELECT pg_catalog.format('ALTER ROLE werk_admin_runtime PASSWORD %L', :'admin_password')
\gexec
SELECT pg_catalog.format('ALTER ROLE werk_service_runtime PASSWORD %L', :'service_password')
\gexec
SELECT pg_catalog.format('ALTER ROLE werk_worker_runtime PASSWORD %L', :'worker_password')
\gexec
SELECT pg_catalog.format('ALTER ROLE werk_backup PASSWORD %L', :'backup_password')
\gexec

GRANT werk_owner TO werk_migrator WITH INHERIT FALSE, SET TRUE;
GRANT werk_backup_reader TO werk_backup WITH INHERIT FALSE, SET TRUE;
SELECT pg_catalog.format('REVOKE werk_owner FROM %I', member_role.rolname)
FROM pg_catalog.pg_auth_members AS membership
JOIN pg_catalog.pg_roles AS granted_role ON granted_role.oid = membership.roleid
JOIN pg_catalog.pg_roles AS member_role ON member_role.oid = membership.member
WHERE granted_role.rolname = 'werk_owner'
  AND member_role.rolname IN (
      'werk_work_runtime', 'werk_identity_runtime', 'werk_admin_runtime', 'werk_service_runtime',
      'werk_worker_runtime', 'werk_backup', 'werk_backup_reader'
  )
\gexec

SELECT pg_catalog.format('REVOKE werk_backup_reader FROM %I', member_role.rolname)
FROM pg_catalog.pg_auth_members AS membership
JOIN pg_catalog.pg_roles AS granted_role ON granted_role.oid = membership.roleid
JOIN pg_catalog.pg_roles AS member_role ON member_role.oid = membership.member
WHERE granted_role.rolname = 'werk_backup_reader'
  AND member_role.rolname <> 'werk_backup'
\gexec

ALTER DATABASE :"database_name" OWNER TO werk_owner;
REVOKE ALL ON DATABASE :"database_name" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"database_name"
    TO werk_owner, werk_migrator, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
       werk_service_runtime, werk_worker_runtime, werk_backup;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA IF NOT EXISTS werk_core AUTHORIZATION werk_owner;
ALTER SCHEMA werk_core OWNER TO werk_owner;

SELECT pg_catalog.format(
    CASE relation.relkind
        WHEN 'S' THEN 'ALTER SEQUENCE %I.%I OWNER TO werk_owner'
        WHEN 'v' THEN 'ALTER VIEW %I.%I OWNER TO werk_owner'
        WHEN 'm' THEN 'ALTER MATERIALIZED VIEW %I.%I OWNER TO werk_owner'
        WHEN 'f' THEN 'ALTER FOREIGN TABLE %I.%I OWNER TO werk_owner'
        ELSE 'ALTER TABLE %I.%I OWNER TO werk_owner'
    END,
    namespace.nspname,
    relation.relname
)
FROM pg_catalog.pg_class AS relation
JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
WHERE namespace.nspname IN ('werk_core', 'werk_security')
  AND relation.relkind IN ('r', 'p', 'S', 'v', 'm', 'f')
  AND relation.relowner <> (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = 'werk_owner')
\gexec

SELECT pg_catalog.format(
    CASE procedure.prokind
        WHEN 'p' THEN 'ALTER PROCEDURE %I.%I(%s) OWNER TO werk_owner'
        ELSE 'ALTER FUNCTION %I.%I(%s) OWNER TO werk_owner'
    END,
    namespace.nspname,
    procedure.proname,
    pg_catalog.pg_get_function_identity_arguments(procedure.oid)
)
FROM pg_catalog.pg_proc AS procedure
JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
WHERE namespace.nspname IN ('werk_core', 'werk_security')
  AND procedure.prokind IN ('f', 'p')
  AND procedure.proowner <> (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = 'werk_owner')
\gexec

ALTER ROLE werk_migrator IN DATABASE :"database_name"
    SET search_path = pg_catalog, werk_core;
ALTER ROLE werk_work_runtime IN DATABASE :"database_name"
    SET search_path = pg_catalog, werk_core;
ALTER ROLE werk_identity_runtime IN DATABASE :"database_name"
    SET search_path = pg_catalog, werk_core;
ALTER ROLE werk_admin_runtime IN DATABASE :"database_name"
    SET search_path = pg_catalog, werk_core;
ALTER ROLE werk_service_runtime IN DATABASE :"database_name"
    SET search_path = pg_catalog, werk_core;
ALTER ROLE werk_worker_runtime IN DATABASE :"database_name"
    SET search_path = pg_catalog, werk_core;
ALTER ROLE werk_backup IN DATABASE :"database_name"
    SET search_path = pg_catalog;
ALTER ROLE werk_backup IN DATABASE :"database_name"
    SET default_transaction_read_only = on;

DO $assertions$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_roles
        WHERE rolname IN (
            'werk_owner', 'werk_migrator', 'werk_work_runtime', 'werk_identity_runtime',
            'werk_admin_runtime', 'werk_service_runtime', 'werk_worker_runtime',
            'werk_backup'
        )
          AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)
    ) THEN
        RAISE EXCEPTION 'a WERK application role has forbidden PostgreSQL attributes';
    END IF;

    IF (SELECT rolcanlogin FROM pg_catalog.pg_roles WHERE rolname = 'werk_owner') THEN
        RAISE EXCEPTION 'werk_owner must not be able to log in';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_roles
        WHERE rolname = 'werk_backup_reader'
          AND NOT rolcanlogin
          AND NOT rolsuper
          AND NOT rolcreatedb
          AND NOT rolcreaterole
          AND NOT rolinherit
          AND NOT rolreplication
          AND rolbypassrls
    ) THEN
        RAISE EXCEPTION 'werk_backup_reader must be a NOLOGIN, BYPASSRLS-only capability role';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_roles
        WHERE rolname = 'werk_backup'
          AND rolcanlogin
          AND NOT rolsuper
          AND NOT rolcreatedb
          AND NOT rolcreaterole
          AND NOT rolinherit
          AND NOT rolreplication
          AND NOT rolbypassrls
          AND rolconnlimit = 1
    ) THEN
        RAISE EXCEPTION 'werk_backup must be a constrained login role';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_auth_members AS membership
        JOIN pg_catalog.pg_roles AS granted_role ON granted_role.oid = membership.roleid
        JOIN pg_catalog.pg_roles AS member_role ON member_role.oid = membership.member
        WHERE granted_role.rolname = 'werk_owner'
          AND member_role.rolname <> 'werk_migrator'
    ) THEN
        RAISE EXCEPTION 'only werk_migrator may be a member of werk_owner';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_auth_members AS membership
        JOIN pg_catalog.pg_roles AS granted_role ON granted_role.oid = membership.roleid
        JOIN pg_catalog.pg_roles AS member_role ON member_role.oid = membership.member
        WHERE granted_role.rolname = 'werk_owner'
          AND member_role.rolname = 'werk_migrator'
          AND NOT membership.inherit_option
          AND membership.set_option
    ) THEN
        RAISE EXCEPTION 'werk_migrator must have SET-only membership in werk_owner';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_catalog.pg_auth_members AS membership
        JOIN pg_catalog.pg_roles AS granted_role ON granted_role.oid = membership.roleid
        JOIN pg_catalog.pg_roles AS member_role ON member_role.oid = membership.member
        WHERE granted_role.rolname = 'werk_backup_reader'
          AND member_role.rolname = 'werk_backup'
          AND NOT membership.inherit_option
          AND membership.set_option
    ) THEN
        RAISE EXCEPTION 'werk_backup must have SET-only membership in werk_backup_reader';
    END IF;
END
$assertions$;
