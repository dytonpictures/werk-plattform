-- Tenant administration remains installation-level, while every tenant-bound
-- mutation requires an explicit transaction tenant context.
REVOKE DELETE ON werk_core.tenants, werk_core.organizational_units FROM werk_admin_runtime;

DROP POLICY tenants_admin_all ON werk_core.tenants;
CREATE POLICY tenants_admin_read ON werk_core.tenants
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY tenants_admin_insert ON werk_core.tenants
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (id = werk_security.current_tenant_id());
CREATE POLICY tenants_admin_update ON werk_core.tenants
    FOR UPDATE TO werk_admin_runtime
    USING (id = werk_security.current_tenant_id())
    WITH CHECK (id = werk_security.current_tenant_id());

DROP POLICY organizational_units_admin_all ON werk_core.organizational_units;
CREATE POLICY organizational_units_admin_tenant ON werk_core.organizational_units
    TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());

-- Security audit also records protected Core administration actions. Its event
-- key follows the same versioned contract syntax as the transactional outbox.
ALTER TABLE werk_core.security_audit_events
    DROP CONSTRAINT security_audit_events_event_type_check;
ALTER TABLE werk_core.security_audit_events
    ADD CONSTRAINT security_audit_events_event_type_check
    CHECK (event_type ~ '^[a-z][a-z0-9-]*([.][a-z][a-z0-9-]*)+[.]v[1-9][0-9]*$');

INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES
 ('0196f000-0000-7000-8000-000000000301', 'core.tenancy.tenant.read', 'Mandanten anzeigen', 'core.tenancy', 'admin', 'medium'),
 ('0196f000-0000-7000-8000-000000000302', 'core.tenancy.tenant.create', 'Mandanten anlegen', 'core.tenancy', 'admin', 'critical'),
 ('0196f000-0000-7000-8000-000000000303', 'core.tenancy.organizational-unit.read', 'Organisationseinheiten anzeigen', 'core.tenancy', 'admin', 'medium'),
 ('0196f000-0000-7000-8000-000000000304', 'core.tenancy.organizational-unit.create', 'Organisationseinheiten anlegen', 'core.tenancy', 'admin', 'high');

INSERT INTO werk_core.role_permissions (role_id, permission_id)
SELECT '0196f000-0000-7000-8000-000000000111'::uuid, permission.id
FROM werk_core.permissions AS permission
WHERE permission.permission_key IN (
    'core.tenancy.tenant.read',
    'core.tenancy.tenant.create',
    'core.tenancy.organizational-unit.read',
    'core.tenancy.organizational-unit.create'
);
