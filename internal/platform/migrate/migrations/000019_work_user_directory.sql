INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES (
    '0196f000-0000-7000-8000-000000000501',
    'core.identity.work-account.read',
    'Arbeitskonten anzeigen',
    'core.identity',
    'admin',
    'high'
);

INSERT INTO werk_core.role_permissions (role_id, permission_id)
VALUES (
    '0196f000-0000-7000-8000-000000000111',
    '0196f000-0000-7000-8000-000000000501'
);
