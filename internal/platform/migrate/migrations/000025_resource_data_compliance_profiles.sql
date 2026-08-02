CREATE TABLE werk_core.resource_data_profiles (
    resource_kind text PRIMARY KEY REFERENCES werk_core.resource_type_registrations(resource_kind)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    personal_data_category text NOT NULL CHECK (
        personal_data_category IN ('none', 'personal', 'special-category', 'criminal-offence')
    ),
    confidentiality_level text NOT NULL CHECK (
        confidentiality_level IN ('public', 'internal', 'confidential', 'restricted')
    ),
    processing_activity_required boolean NOT NULL,
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (personal_data_category = 'none' OR processing_activity_required)
);

INSERT INTO werk_core.resource_data_profiles (
    resource_kind, personal_data_category, confidentiality_level,
    processing_activity_required
) VALUES
    ('core.platform.installation', 'none', 'internal', false),
    ('core.tenancy.tenant', 'personal', 'confidential', true),
    ('core.tenancy.organizational-unit', 'personal', 'confidential', true),
    ('core.identity.work-account', 'personal', 'restricted', true),
    ('core.authorization.work-role', 'none', 'internal', false),
    ('core.audit.security-log', 'personal', 'restricted', true),
    ('core.workspace.workspace', 'personal', 'confidential', true);

REVOKE ALL ON werk_core.resource_data_profiles
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON werk_core.resource_data_profiles
TO werk_identity_runtime, werk_admin_runtime, werk_backup_reader;

ALTER TABLE werk_core.resource_data_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.resource_data_profiles FORCE ROW LEVEL SECURITY;

CREATE POLICY resource_data_profiles_identity_read ON werk_core.resource_data_profiles
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY resource_data_profiles_admin_read ON werk_core.resource_data_profiles
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY resource_data_profiles_owner_all ON werk_core.resource_data_profiles
    TO werk_owner USING (true) WITH CHECK (true);

COMMENT ON TABLE werk_core.resource_data_profiles IS
    'Mandatory data-risk classification for authorizable resource types. It does not establish a legal basis for processing.';
COMMENT ON COLUMN werk_core.resource_data_profiles.processing_activity_required IS
    'Marks resource types whose later processing commands must resolve an approved server-side processing activity.';
