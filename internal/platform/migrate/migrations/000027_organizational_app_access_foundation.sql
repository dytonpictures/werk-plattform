ALTER TABLE werk_core.accounts
    ADD CONSTRAINT accounts_tenant_id_id_unique UNIQUE (tenant_id, id);

CREATE TABLE werk_core.tenant_app_installations (
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    app_module text NOT NULL REFERENCES werk_core.platform_modules(module_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'removed')),
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, app_module),
    CHECK (app_module LIKE 'app.%')
);

CREATE TABLE werk_core.access_groups (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    group_key text NOT NULL CHECK (group_key ~ '^[a-z][a-z0-9.-]+$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    governing_unit_id uuid NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, group_key),
    FOREIGN KEY (tenant_id, governing_unit_id)
        REFERENCES werk_core.organizational_units(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE werk_core.access_group_memberships (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    access_group_id uuid NOT NULL,
    account_id uuid NULL,
    organizational_unit_id uuid NULL,
    include_descendants boolean NOT NULL DEFAULT false,
    valid_from timestamptz NOT NULL DEFAULT now(),
    valid_until timestamptz NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(account_id, organizational_unit_id) = 1),
    CHECK (organizational_unit_id IS NOT NULL OR NOT include_descendants),
    CHECK (valid_until IS NULL OR valid_until > valid_from),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, access_group_id)
        REFERENCES werk_core.access_groups(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, organizational_unit_id)
        REFERENCES werk_core.organizational_units(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE UNIQUE INDEX access_group_memberships_active_subject_idx
    ON werk_core.access_group_memberships (
        tenant_id,
        access_group_id,
        COALESCE(account_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(organizational_unit_id, '00000000-0000-0000-0000-000000000000'::uuid),
        include_descendants
    )
    WHERE status = 'active';

CREATE TABLE werk_core.app_entitlements (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    app_module text NOT NULL,
    account_id uuid NULL,
    organizational_unit_id uuid NULL,
    access_group_id uuid NULL,
    include_descendants boolean NOT NULL DEFAULT false,
    valid_from timestamptz NOT NULL DEFAULT now(),
    valid_until timestamptz NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(account_id, organizational_unit_id, access_group_id) = 1),
    CHECK (organizational_unit_id IS NOT NULL OR NOT include_descendants),
    CHECK (valid_until IS NULL OR valid_until > valid_from),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, app_module)
        REFERENCES werk_core.tenant_app_installations(tenant_id, app_module)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, organizational_unit_id)
        REFERENCES werk_core.organizational_units(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, access_group_id)
        REFERENCES werk_core.access_groups(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE UNIQUE INDEX app_entitlements_active_subject_idx
    ON werk_core.app_entitlements (
        tenant_id,
        app_module,
        COALESCE(account_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(organizational_unit_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(access_group_id, '00000000-0000-0000-0000-000000000000'::uuid),
        include_descendants
    )
    WHERE status = 'active';

CREATE INDEX access_groups_governing_unit_idx
    ON werk_core.access_groups (tenant_id, governing_unit_id, status)
    WHERE governing_unit_id IS NOT NULL;
CREATE INDEX access_group_memberships_account_idx
    ON werk_core.access_group_memberships (tenant_id, account_id, status, valid_from, valid_until)
    WHERE account_id IS NOT NULL;
CREATE INDEX access_group_memberships_unit_idx
    ON werk_core.access_group_memberships (tenant_id, organizational_unit_id, status, valid_from, valid_until)
    WHERE organizational_unit_id IS NOT NULL;
CREATE INDEX app_entitlements_account_idx
    ON werk_core.app_entitlements (tenant_id, account_id, status, valid_from, valid_until)
    WHERE account_id IS NOT NULL;
CREATE INDEX app_entitlements_unit_idx
    ON werk_core.app_entitlements (tenant_id, organizational_unit_id, status, valid_from, valid_until)
    WHERE organizational_unit_id IS NOT NULL;
CREATE INDEX app_entitlements_group_idx
    ON werk_core.app_entitlements (tenant_id, access_group_id, status, valid_from, valid_until)
    WHERE access_group_id IS NOT NULL;

CREATE FUNCTION werk_security.validate_tenant_app_module()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE module_kind_value text; module_status_value text;
BEGIN
    SELECT module_kind, status INTO module_kind_value, module_status_value
    FROM werk_core.platform_modules
    WHERE module_key = NEW.app_module;
    IF module_kind_value IS DISTINCT FROM 'app' OR module_status_value IS DISTINCT FROM 'active' THEN
        RAISE EXCEPTION 'tenant app installation requires an active app module';
    END IF;
    RETURN NEW;
END
$function$;

CREATE TRIGGER tenant_app_installations_validate_module
    BEFORE INSERT OR UPDATE OF app_module ON werk_core.tenant_app_installations
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_tenant_app_module();

CREATE FUNCTION werk_security.validate_human_app_access_subject()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE account_class_value text;
BEGIN
    IF NEW.account_id IS NOT NULL THEN
        SELECT account_class INTO account_class_value
        FROM werk_core.accounts
        WHERE id = NEW.account_id AND tenant_id = NEW.tenant_id;
        IF account_class_value IS DISTINCT FROM 'work' THEN
            RAISE EXCEPTION 'direct app access subjects must be tenant-bound work accounts';
        END IF;
    END IF;
    RETURN NEW;
END
$function$;

CREATE TRIGGER access_group_memberships_validate_human_subject
    BEFORE INSERT OR UPDATE OF tenant_id, account_id ON werk_core.access_group_memberships
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_human_app_access_subject();
CREATE TRIGGER app_entitlements_validate_human_subject
    BEFORE INSERT OR UPDATE OF tenant_id, account_id ON werk_core.app_entitlements
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_human_app_access_subject();

REVOKE ALL ON FUNCTION werk_security.validate_tenant_app_module() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_human_app_access_subject() FROM PUBLIC;

REVOKE ALL ON
    werk_core.tenant_app_installations,
    werk_core.access_groups,
    werk_core.access_group_memberships,
    werk_core.app_entitlements
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON
    werk_core.tenant_app_installations,
    werk_core.access_groups,
    werk_core.access_group_memberships,
    werk_core.app_entitlements
TO werk_identity_runtime, werk_admin_runtime, werk_backup_reader;

GRANT INSERT, UPDATE ON
    werk_core.tenant_app_installations,
    werk_core.access_groups,
    werk_core.access_group_memberships,
    werk_core.app_entitlements
TO werk_admin_runtime;

ALTER TABLE werk_core.tenant_app_installations ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.tenant_app_installations FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.access_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.access_groups FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.access_group_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.access_group_memberships FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.app_entitlements ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.app_entitlements FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_app_installations_identity_read ON werk_core.tenant_app_installations
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY tenant_app_installations_admin_manage ON werk_core.tenant_app_installations
    TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY tenant_app_installations_owner_all ON werk_core.tenant_app_installations
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY access_groups_identity_read ON werk_core.access_groups
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY access_groups_admin_manage ON werk_core.access_groups
    TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY access_groups_owner_all ON werk_core.access_groups
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY access_group_memberships_identity_read ON werk_core.access_group_memberships
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY access_group_memberships_admin_manage ON werk_core.access_group_memberships
    TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY access_group_memberships_owner_all ON werk_core.access_group_memberships
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY app_entitlements_identity_read ON werk_core.app_entitlements
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY app_entitlements_admin_manage ON werk_core.app_entitlements
    TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY app_entitlements_owner_all ON werk_core.app_entitlements
    TO werk_owner USING (true) WITH CHECK (true);

COMMENT ON TABLE werk_core.tenant_app_installations IS
    'Tenant-local activation state for a platform-registered app module; no entitlement or role is implied.';
COMMENT ON TABLE werk_core.access_groups IS
    'Tenant-bound cross-cutting subject groups; governing_unit_id defines delegated administration scope, not a new tenant.';
COMMENT ON TABLE werk_core.access_group_memberships IS
    'Direct work-account or organizational-unit edges into an access group; nested access groups are intentionally unsupported.';
COMMENT ON TABLE werk_core.app_entitlements IS
    'Explicit app availability gate for one work account, organizational unit, or access group; permissions remain separate.';
