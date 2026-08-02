CREATE TABLE werk_core.platform_modules (
    module_key text PRIMARY KEY CHECK (module_key ~ '^[a-z][a-z0-9.-]+$'),
    module_kind text NOT NULL CHECK (module_kind IN ('core', 'app')),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((module_kind = 'core' AND module_key LIKE 'core.%') OR
           (module_kind = 'app' AND module_key LIKE 'app.%'))
);

INSERT INTO werk_core.platform_modules (module_key, module_kind, display_name)
VALUES
    ('core.platform', 'core', 'Plattform'),
    ('core.identity', 'core', 'Identität'),
    ('core.tenancy', 'core', 'Mandanten und Organisation'),
    ('core.authorization', 'core', 'Autorisierung'),
    ('core.audit', 'core', 'Audit'),
    ('core.workspace', 'core', 'Workspace');

ALTER TABLE werk_core.permissions
    ADD COLUMN contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0);

ALTER TABLE werk_core.permissions
    ADD CONSTRAINT permissions_owning_module_fkey
    FOREIGN KEY (owning_module) REFERENCES werk_core.platform_modules(module_key)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

CREATE TABLE werk_core.resource_type_registrations (
    resource_kind text PRIMARY KEY CHECK (resource_kind ~ '^[a-z][a-z0-9.-]+$'),
    owner_module text NOT NULL REFERENCES werk_core.platform_modules(module_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    boundary text NOT NULL CHECK (boundary IN ('installation', 'tenant')),
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (resource_kind LIKE owner_module || '.%')
);

CREATE INDEX resource_type_registrations_owner_idx
    ON werk_core.resource_type_registrations (owner_module, status);

INSERT INTO werk_core.resource_type_registrations (
    resource_kind, owner_module, display_name, boundary
) VALUES
    ('core.platform.installation', 'core.platform', 'Installation', 'installation'),
    ('core.tenancy.tenant', 'core.tenancy', 'Mandant (Verwaltungsressource)', 'installation'),
    ('core.tenancy.organizational-unit', 'core.tenancy', 'Organisationseinheit (Verwaltungsressource)', 'installation'),
    ('core.identity.work-account', 'core.identity', 'Arbeitskonto (Verwaltungsressource)', 'installation'),
    ('core.authorization.work-role', 'core.authorization', 'Arbeitsrolle (Verwaltungsressource)', 'installation'),
    ('core.audit.security-log', 'core.audit', 'Sicherheitsprotokoll', 'installation'),
    ('core.workspace.workspace', 'core.workspace', 'Tenant-Workspace', 'tenant');

CREATE TABLE werk_core.permission_resource_types (
    permission_id uuid NOT NULL REFERENCES werk_core.permissions(id)
        ON UPDATE RESTRICT ON DELETE CASCADE,
    resource_kind text NOT NULL REFERENCES werk_core.resource_type_registrations(resource_kind)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (permission_id, resource_kind)
);

CREATE INDEX permission_resource_types_kind_idx
    ON werk_core.permission_resource_types (resource_kind, permission_id);

INSERT INTO werk_core.permission_resource_types (permission_id, resource_kind)
SELECT permission.id, target.resource_kind
FROM (VALUES
    ('core.identity.work-account.create', 'core.platform.installation'),
    ('core.workspace.access', 'core.workspace.workspace'),
    ('core.tenancy.tenant.read', 'core.platform.installation'),
    ('core.tenancy.tenant.create', 'core.platform.installation'),
    ('core.tenancy.tenant.update', 'core.tenancy.tenant'),
    ('core.tenancy.organizational-unit.read', 'core.tenancy.tenant'),
    ('core.tenancy.organizational-unit.create', 'core.tenancy.tenant'),
    ('core.tenancy.organizational-unit.update', 'core.tenancy.organizational-unit'),
    ('core.identity.work-account.read', 'core.tenancy.tenant'),
    ('core.authorization.work-role.read', 'core.tenancy.tenant'),
    ('core.authorization.work-role.create', 'core.tenancy.tenant'),
    ('core.authorization.work-role.update', 'core.authorization.work-role'),
    ('core.authorization.work-role.assign', 'core.identity.work-account'),
    ('core.audit.security-event.read', 'core.audit.security-log')
) AS target(permission_key, resource_kind)
JOIN werk_core.permissions AS permission
  ON permission.permission_key = target.permission_key;

REVOKE ALL ON
    werk_core.platform_modules,
    werk_core.resource_type_registrations,
    werk_core.permission_resource_types
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON
    werk_core.platform_modules,
    werk_core.resource_type_registrations,
    werk_core.permission_resource_types
TO werk_identity_runtime, werk_admin_runtime, werk_backup_reader;

ALTER TABLE werk_core.platform_modules ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_modules FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.resource_type_registrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.resource_type_registrations FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.permission_resource_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.permission_resource_types FORCE ROW LEVEL SECURITY;

CREATE POLICY platform_modules_identity_read ON werk_core.platform_modules
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY platform_modules_admin_read ON werk_core.platform_modules
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY platform_modules_owner_all ON werk_core.platform_modules
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY resource_type_registrations_identity_read ON werk_core.resource_type_registrations
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY resource_type_registrations_admin_read ON werk_core.resource_type_registrations
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY resource_type_registrations_owner_all ON werk_core.resource_type_registrations
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY permission_resource_types_identity_read ON werk_core.permission_resource_types
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY permission_resource_types_admin_read ON werk_core.permission_resource_types
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY permission_resource_types_owner_all ON werk_core.permission_resource_types
    TO werk_owner USING (true) WITH CHECK (true);

COMMENT ON TABLE werk_core.platform_modules IS
    'Platform-global registry of core and app namespaces; owner-only writes in the initial contract.';
COMMENT ON TABLE werk_core.resource_type_registrations IS
    'Registered authorization target kinds and their explicit installation or tenant boundary.';
COMMENT ON TABLE werk_core.permission_resource_types IS
    'Allow-list of resource kinds each permission may address; absence fails authorization closed.';
