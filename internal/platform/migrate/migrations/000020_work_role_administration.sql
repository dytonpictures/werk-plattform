CREATE UNIQUE INDEX role_assignments_current_unique
    ON werk_core.role_assignments (
        account_id, role_id, scope_type, scope_tenant_id, scope_id
    ) NULLS NOT DISTINCT
    WHERE valid_until IS NULL;

INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES
    (
        '0196f000-0000-7000-8000-000000000601',
        'core.authorization.work-role.read',
        'Arbeitsrollen anzeigen',
        'core.authorization',
        'admin',
        'medium'
    ),
    (
        '0196f000-0000-7000-8000-000000000602',
        'core.authorization.work-role.create',
        'Arbeitsrollen anlegen',
        'core.authorization',
        'admin',
        'high'
    ),
    (
        '0196f000-0000-7000-8000-000000000603',
        'core.authorization.work-role.assign',
        'Arbeitsrollen zuweisen',
        'core.authorization',
        'admin',
        'high'
    );

INSERT INTO werk_core.role_permissions (role_id, permission_id)
VALUES
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000601'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000602'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000603');
