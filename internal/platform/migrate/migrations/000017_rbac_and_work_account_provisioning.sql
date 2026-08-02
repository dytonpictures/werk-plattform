CREATE TABLE werk_core.permissions (
    id uuid PRIMARY KEY,
    permission_key text NOT NULL UNIQUE CHECK (permission_key ~ '^[a-z][a-z0-9.-]+$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    owning_module text NOT NULL CHECK (owning_module ~ '^[a-z][a-z0-9.-]+$'),
    access_plane text NOT NULL CHECK (access_plane IN ('work', 'admin', 'service')),
    risk_level text NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE werk_core.roles (
    id uuid PRIMARY KEY,
    tenant_id uuid NULL REFERENCES werk_core.tenants(id),
    role_key text NOT NULL CHECK (role_key ~ '^[a-z][a-z0-9.-]+$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    access_plane text NOT NULL CHECK (access_plane IN ('work', 'admin', 'service')),
    system_role boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CHECK ((access_plane = 'admin' AND tenant_id IS NULL) OR
           (access_plane IN ('work', 'service') AND tenant_id IS NOT NULL)),
    UNIQUE NULLS NOT DISTINCT (tenant_id, access_plane, role_key)
);

CREATE TABLE werk_core.role_permissions (
    role_id uuid NOT NULL REFERENCES werk_core.roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES werk_core.permissions(id) ON DELETE RESTRICT,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE werk_core.role_assignments (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES werk_core.roles(id) ON DELETE CASCADE,
    access_plane text NOT NULL CHECK (access_plane IN ('work', 'admin', 'service')),
    scope_type text NOT NULL CHECK (scope_type IN ('installation', 'tenant', 'organizational-unit', 'resource')),
    scope_tenant_id uuid NULL REFERENCES werk_core.tenants(id),
    scope_id text NULL,
    granted_by_account_id uuid NULL REFERENCES werk_core.accounts(id),
    valid_from timestamptz NOT NULL DEFAULT now(),
    valid_until timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (valid_until IS NULL OR valid_until > valid_from),
    CHECK ((scope_type = 'installation' AND scope_tenant_id IS NULL AND scope_id IS NULL) OR
           (scope_type = 'tenant' AND scope_tenant_id IS NOT NULL AND scope_id IS NULL) OR
           (scope_type IN ('organizational-unit', 'resource') AND scope_tenant_id IS NOT NULL AND scope_id IS NOT NULL))
);

CREATE INDEX role_assignments_account_idx ON werk_core.role_assignments (account_id, valid_from, valid_until);
CREATE INDEX roles_tenant_idx ON werk_core.roles (tenant_id, access_plane);

CREATE FUNCTION werk_security.validate_role_binding()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE account_class_value text; account_tenant_value uuid; role_plane_value text; role_tenant_value uuid;
BEGIN
    SELECT account_class, tenant_id INTO account_class_value, account_tenant_value FROM werk_core.accounts WHERE id = NEW.account_id;
    SELECT access_plane, tenant_id INTO role_plane_value, role_tenant_value FROM werk_core.roles WHERE id = NEW.role_id;
    IF account_class_value IS NULL OR role_plane_value IS NULL OR account_class_value <> NEW.access_plane OR role_plane_value <> NEW.access_plane THEN
        RAISE EXCEPTION 'role assignment access plane mismatch';
    END IF;
    IF NEW.access_plane = 'admin' AND (account_tenant_value IS NOT NULL OR role_tenant_value IS NOT NULL OR NEW.scope_type <> 'installation') THEN
        RAISE EXCEPTION 'admin role assignment must be installation scoped';
    END IF;
    IF NEW.access_plane IN ('work', 'service') AND (account_tenant_value IS NULL OR role_tenant_value <> account_tenant_value OR NEW.scope_tenant_id <> account_tenant_value) THEN
        RAISE EXCEPTION 'tenant role assignment mismatch';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_role_permission()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE role_plane_value text; permission_plane_value text;
BEGIN
    SELECT access_plane INTO role_plane_value FROM werk_core.roles WHERE id = NEW.role_id;
    SELECT access_plane INTO permission_plane_value FROM werk_core.permissions WHERE id = NEW.permission_id;
    IF role_plane_value IS NULL OR permission_plane_value IS NULL OR role_plane_value <> permission_plane_value THEN
        RAISE EXCEPTION 'role permission access plane mismatch';
    END IF;
    RETURN NEW;
END
$function$;

CREATE TRIGGER role_assignments_validate BEFORE INSERT OR UPDATE ON werk_core.role_assignments
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_role_binding();
CREATE TRIGGER role_permissions_validate BEFORE INSERT OR UPDATE ON werk_core.role_permissions
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_role_permission();

REVOKE ALL ON werk_core.permissions, werk_core.roles, werk_core.role_permissions, werk_core.role_assignments
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.permissions, werk_core.roles, werk_core.role_permissions, werk_core.role_assignments TO werk_identity_runtime;
GRANT INSERT ON werk_core.role_assignments TO werk_identity_runtime;
GRANT SELECT ON werk_core.permissions, werk_core.roles, werk_core.role_permissions, werk_core.role_assignments TO werk_admin_runtime;
GRANT INSERT, UPDATE ON werk_core.roles, werk_core.role_permissions, werk_core.role_assignments TO werk_admin_runtime;
GRANT SELECT ON werk_core.permissions, werk_core.roles, werk_core.role_permissions, werk_core.role_assignments TO werk_backup_reader;

ALTER TABLE werk_core.permissions ENABLE ROW LEVEL SECURITY; ALTER TABLE werk_core.permissions FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.roles ENABLE ROW LEVEL SECURITY; ALTER TABLE werk_core.roles FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.role_permissions ENABLE ROW LEVEL SECURITY; ALTER TABLE werk_core.role_permissions FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.role_assignments ENABLE ROW LEVEL SECURITY; ALTER TABLE werk_core.role_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY permissions_identity_read ON werk_core.permissions FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY permissions_admin_read ON werk_core.permissions FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY permissions_owner_all ON werk_core.permissions TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY roles_identity_read ON werk_core.roles FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY roles_identity_insert ON werk_core.roles FOR INSERT TO werk_identity_runtime WITH CHECK (access_plane = 'admin' AND tenant_id IS NULL AND system_role);
CREATE POLICY roles_admin_manage ON werk_core.roles TO werk_admin_runtime
    USING (access_plane = 'admin' OR (access_plane = 'work' AND tenant_id = werk_security.current_tenant_id()))
    WITH CHECK (access_plane = 'work' AND tenant_id = werk_security.current_tenant_id());
CREATE POLICY roles_owner_all ON werk_core.roles TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY role_permissions_identity_read ON werk_core.role_permissions FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY role_permissions_admin_read ON werk_core.role_permissions FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY role_permissions_admin_insert ON werk_core.role_permissions FOR INSERT TO werk_admin_runtime
    WITH CHECK (
      EXISTS (SELECT 1 FROM werk_core.roles r WHERE r.id = role_id AND r.access_plane = 'work' AND r.tenant_id = werk_security.current_tenant_id())
      AND EXISTS (SELECT 1 FROM werk_core.permissions p WHERE p.id = permission_id AND p.access_plane = 'work')
    );
CREATE POLICY role_permissions_owner_all ON werk_core.role_permissions TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY role_assignments_identity_read ON werk_core.role_assignments FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY role_assignments_identity_insert ON werk_core.role_assignments FOR INSERT TO werk_identity_runtime WITH CHECK (access_plane = 'admin' AND scope_type = 'installation');
CREATE POLICY role_assignments_admin_manage ON werk_core.role_assignments TO werk_admin_runtime
    USING (access_plane = 'work' AND scope_tenant_id = werk_security.current_tenant_id())
    WITH CHECK (access_plane = 'work' AND scope_tenant_id = werk_security.current_tenant_id());
CREATE POLICY role_assignments_owner_all ON werk_core.role_assignments TO werk_owner USING (true) WITH CHECK (true);

-- Admin provisioning is tenant-explicit and limited to work identities.
GRANT SELECT ON werk_core.tenants, werk_core.organizational_units TO werk_admin_runtime;
GRANT SELECT, INSERT ON werk_core.parties, werk_core.persons, werk_core.memberships TO werk_admin_runtime;
GRANT SELECT, INSERT ON werk_core.accounts, werk_core.account_credentials TO werk_admin_runtime;
CREATE POLICY parties_admin_work_provision ON werk_core.parties TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id()) WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY persons_admin_work_provision ON werk_core.persons TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id()) WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY memberships_admin_work_provision ON werk_core.memberships TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id()) WITH CHECK (tenant_id = werk_security.current_tenant_id());
DROP POLICY accounts_admin_all ON werk_core.accounts;
CREATE POLICY accounts_admin_work_tenant ON werk_core.accounts TO werk_admin_runtime
    USING (account_class = 'work' AND tenant_id = werk_security.current_tenant_id())
    WITH CHECK (account_class = 'work' AND tenant_id = werk_security.current_tenant_id());
DROP POLICY credentials_admin_all ON werk_core.account_credentials;
CREATE POLICY credentials_admin_work_tenant ON werk_core.account_credentials TO werk_admin_runtime
    USING (EXISTS (SELECT 1 FROM werk_core.accounts a WHERE a.id = account_id AND a.account_class = 'work' AND a.tenant_id = werk_security.current_tenant_id()))
    WITH CHECK (EXISTS (SELECT 1 FROM werk_core.accounts a WHERE a.id = account_id AND a.account_class = 'work' AND a.tenant_id = werk_security.current_tenant_id()));

GRANT INSERT ON werk_core.security_audit_events, werk_core.outbox_events TO werk_admin_runtime;
CREATE POLICY security_audit_admin_insert ON werk_core.security_audit_events FOR INSERT TO werk_admin_runtime
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY outbox_events_admin_insert ON werk_core.outbox_events FOR INSERT TO werk_admin_runtime
    WITH CHECK (tenant_id = werk_security.current_tenant_id());

INSERT INTO werk_core.permissions (id, permission_key, display_name, owning_module, access_plane, risk_level)
VALUES
 ('0196f000-0000-7000-8000-000000000101', 'core.identity.work-account.create', 'Arbeitskonten anlegen', 'core.identity', 'admin', 'high'),
 ('0196f000-0000-7000-8000-000000000102', 'core.workspace.access', 'Workspace verwenden', 'core.workspace', 'work', 'low');
INSERT INTO werk_core.roles (id, role_key, display_name, access_plane, system_role)
VALUES ('0196f000-0000-7000-8000-000000000111', 'installation-administrator', 'Installationsadministration', 'admin', true);
INSERT INTO werk_core.role_permissions (role_id, permission_id)
VALUES ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000101');
INSERT INTO werk_core.role_assignments (id, account_id, role_id, access_plane, scope_type, valid_from)
SELECT gen_random_uuid(), id, '0196f000-0000-7000-8000-000000000111', 'admin', 'installation', now()
FROM werk_core.accounts WHERE account_class = 'admin' AND status = 'active'
ON CONFLICT DO NOTHING;
