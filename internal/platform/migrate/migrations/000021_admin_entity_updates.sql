-- Existing organization and authorization records are changed only through
-- version-checked commands. The runtime may replace permission bindings for
-- tenant-owned custom roles, while system roles remain database-protected.
GRANT DELETE ON werk_core.role_permissions TO werk_admin_runtime;

-- Core Identity may only observe the tenant key and lifecycle status required
-- to reject sessions for suspended or archived tenant contexts.
GRANT SELECT (id, status) ON werk_core.tenants TO werk_identity_runtime;
CREATE POLICY tenants_identity_status_read ON werk_core.tenants
    FOR SELECT TO werk_identity_runtime
    USING (true);

DROP POLICY role_permissions_admin_insert ON werk_core.role_permissions;
CREATE POLICY role_permissions_admin_insert ON werk_core.role_permissions
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (
      EXISTS (
        SELECT 1
        FROM werk_core.roles AS role
        JOIN werk_core.permissions AS permission ON permission.id = permission_id
        WHERE role.id = role_id
          AND role.access_plane = 'work'
          AND role.tenant_id = werk_security.current_tenant_id()
          AND permission.access_plane = 'work'
          AND (
            NOT role.system_role
            OR (role.role_key = 'workspace-member' AND permission.permission_key = 'core.workspace.access')
          )
      )
    );

CREATE POLICY role_permissions_admin_delete ON werk_core.role_permissions
    FOR DELETE TO werk_admin_runtime
    USING (
      EXISTS (
        SELECT 1
        FROM werk_core.roles AS role
        WHERE role.id = role_id
          AND role.access_plane = 'work'
          AND role.tenant_id = werk_security.current_tenant_id()
          AND NOT role.system_role
      )
    );

CREATE FUNCTION werk_security.protect_system_role_update()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $$
BEGIN
    IF OLD.system_role AND current_user = 'werk_admin_runtime' THEN
        RAISE EXCEPTION 'system roles are immutable for the admin runtime'
            USING ERRCODE = '42501';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER roles_protect_system_update
BEFORE UPDATE ON werk_core.roles
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_system_role_update();

INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES
    (
        '0196f000-0000-7000-8000-000000000701',
        'core.tenancy.tenant.update',
        'Mandanten ändern',
        'core.tenancy',
        'admin',
        'critical'
    ),
    (
        '0196f000-0000-7000-8000-000000000702',
        'core.tenancy.organizational-unit.update',
        'Organisationseinheiten ändern',
        'core.tenancy',
        'admin',
        'high'
    ),
    (
        '0196f000-0000-7000-8000-000000000703',
        'core.authorization.work-role.update',
        'Arbeitsrollen ändern',
        'core.authorization',
        'admin',
        'high'
    );

INSERT INTO werk_core.role_permissions (role_id, permission_id)
VALUES
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000701'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000702'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000703');
