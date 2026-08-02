ALTER TABLE werk_core.tenants
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

ALTER TABLE werk_core.organizational_units
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD CONSTRAINT organizational_units_not_own_parent
        CHECK (parent_id IS NULL OR parent_id <> id);

CREATE SCHEMA werk_security AUTHORIZATION werk_owner;
REVOKE ALL ON SCHEMA werk_security FROM PUBLIC;

CREATE FUNCTION werk_security.current_tenant_id()
RETURNS uuid
LANGUAGE sql
STABLE
PARALLEL SAFE
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
    SELECT NULLIF(pg_catalog.current_setting('werk.tenant_id', true), '')::uuid
$function$;

REVOKE ALL ON FUNCTION werk_security.current_tenant_id() FROM PUBLIC;
GRANT USAGE ON SCHEMA werk_security
    TO werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;
GRANT EXECUTE ON FUNCTION werk_security.current_tenant_id()
    TO werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

REVOKE ALL ON SCHEMA werk_core FROM PUBLIC;
GRANT USAGE ON SCHEMA werk_core
    TO werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

REVOKE ALL ON ALL TABLES IN SCHEMA werk_core
    FROM PUBLIC, werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON werk_core.tenants, werk_core.organizational_units
    TO werk_work_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE
    ON werk_core.tenants, werk_core.organizational_units
    TO werk_admin_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON werk_core.admin_subjects
    TO werk_admin_runtime;

ALTER TABLE werk_core.tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.tenants FORCE ROW LEVEL SECURITY;

CREATE POLICY tenants_tenant_gate
    ON werk_core.tenants
    AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (id = werk_security.current_tenant_id())
    WITH CHECK (id = werk_security.current_tenant_id());

CREATE POLICY tenants_work_read
    ON werk_core.tenants
    FOR SELECT
    TO werk_work_runtime
    USING (true);

CREATE POLICY tenants_worker_read
    ON werk_core.tenants
    FOR SELECT
    TO werk_worker_runtime
    USING (true);

CREATE POLICY tenants_admin_all
    ON werk_core.tenants
    TO werk_admin_runtime
    USING (true)
    WITH CHECK (true);

CREATE POLICY tenants_owner_all
    ON werk_core.tenants
    TO werk_owner
    USING (true)
    WITH CHECK (true);

ALTER TABLE werk_core.organizational_units ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.organizational_units FORCE ROW LEVEL SECURITY;

CREATE POLICY organizational_units_tenant_gate
    ON werk_core.organizational_units
    AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());

CREATE POLICY organizational_units_work_read
    ON werk_core.organizational_units
    FOR SELECT
    TO werk_work_runtime
    USING (true);

CREATE POLICY organizational_units_worker_read
    ON werk_core.organizational_units
    FOR SELECT
    TO werk_worker_runtime
    USING (true);

CREATE POLICY organizational_units_admin_all
    ON werk_core.organizational_units
    TO werk_admin_runtime
    USING (true)
    WITH CHECK (true);

CREATE POLICY organizational_units_owner_all
    ON werk_core.organizational_units
    TO werk_owner
    USING (true)
    WITH CHECK (true);
