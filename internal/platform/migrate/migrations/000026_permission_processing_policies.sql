CREATE TABLE werk_core.permission_processing_policies (
    permission_id uuid NOT NULL,
    resource_kind text NOT NULL,
    processing_required boolean NOT NULL,
    activity_key text,
    purpose_key text,
    legal_basis_ref text,
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (permission_id, resource_kind),
    FOREIGN KEY (permission_id, resource_kind)
        REFERENCES werk_core.permission_resource_types(permission_id, resource_kind)
        ON UPDATE RESTRICT ON DELETE CASCADE,
    CHECK (
        (
            processing_required
            AND activity_key IS NOT NULL
            AND purpose_key IS NOT NULL
            AND legal_basis_ref IS NOT NULL
            AND activity_key ~ '^[a-z][a-z0-9.-]+$'
            AND purpose_key ~ '^[a-z][a-z0-9.-]+$'
            AND legal_basis_ref ~ '^[a-z][a-z0-9.-]+$'
        )
        OR
        (
            NOT processing_required
            AND activity_key IS NULL
            AND purpose_key IS NULL
            AND legal_basis_ref IS NULL
        )
    )
);

CREATE INDEX permission_processing_policies_resource_idx
    ON werk_core.permission_processing_policies (resource_kind, status, permission_id);

INSERT INTO werk_core.permission_processing_policies (
    permission_id, resource_kind, processing_required,
    activity_key, purpose_key, legal_basis_ref
)
SELECT permission.id, policy.resource_kind, true,
       policy.activity_key, policy.purpose_key, policy.legal_basis_ref
FROM (VALUES
    (
        'core.identity.work-account.create', 'core.platform.installation',
        'core.identity.work-account-administration', 'core.identity.access-provisioning',
        'operator.processing-register.identity-access'
    ),
    (
        'core.workspace.access', 'core.workspace.workspace',
        'core.workspace.context-access', 'core.workspace.work-delivery',
        'operator.processing-register.workspace'
    ),
    (
        'core.tenancy.tenant.read', 'core.platform.installation',
        'core.tenancy.tenant-administration', 'core.tenancy.platform-operation',
        'operator.processing-register.tenancy'
    ),
    (
        'core.tenancy.tenant.create', 'core.platform.installation',
        'core.tenancy.tenant-administration', 'core.tenancy.platform-operation',
        'operator.processing-register.tenancy'
    ),
    (
        'core.tenancy.tenant.update', 'core.tenancy.tenant',
        'core.tenancy.tenant-administration', 'core.tenancy.platform-operation',
        'operator.processing-register.tenancy'
    ),
    (
        'core.tenancy.organizational-unit.read', 'core.tenancy.tenant',
        'core.tenancy.organization-administration', 'core.tenancy.organization-management',
        'operator.processing-register.tenancy'
    ),
    (
        'core.tenancy.organizational-unit.create', 'core.tenancy.tenant',
        'core.tenancy.organization-administration', 'core.tenancy.organization-management',
        'operator.processing-register.tenancy'
    ),
    (
        'core.tenancy.organizational-unit.update', 'core.tenancy.organizational-unit',
        'core.tenancy.organization-administration', 'core.tenancy.organization-management',
        'operator.processing-register.tenancy'
    ),
    (
        'core.identity.work-account.read', 'core.tenancy.tenant',
        'core.identity.work-account-administration', 'core.identity.access-management',
        'operator.processing-register.identity-access'
    ),
    (
        'core.authorization.work-role.read', 'core.tenancy.tenant',
        'core.authorization.role-administration', 'core.authorization.access-control-management',
        'operator.processing-register.identity-access'
    ),
    (
        'core.authorization.work-role.create', 'core.tenancy.tenant',
        'core.authorization.role-administration', 'core.authorization.access-control-management',
        'operator.processing-register.identity-access'
    ),
    (
        'core.authorization.work-role.update', 'core.authorization.work-role',
        'core.authorization.role-administration', 'core.authorization.access-control-management',
        'operator.processing-register.identity-access'
    ),
    (
        'core.authorization.work-role.assign', 'core.identity.work-account',
        'core.authorization.role-administration', 'core.authorization.access-control-management',
        'operator.processing-register.identity-access'
    ),
    (
        'core.audit.security-event.read', 'core.audit.security-log',
        'core.audit.security-review', 'core.audit.security-and-accountability',
        'operator.processing-register.security-audit'
    )
) AS policy(permission_key, resource_kind, activity_key, purpose_key, legal_basis_ref)
JOIN werk_core.permissions AS permission
  ON permission.permission_key = policy.permission_key
JOIN werk_core.permission_resource_types AS target
  ON target.permission_id = permission.id
 AND target.resource_kind = policy.resource_kind;

REVOKE ALL ON werk_core.permission_processing_policies
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON werk_core.permission_processing_policies
TO werk_identity_runtime, werk_admin_runtime, werk_backup_reader;

ALTER TABLE werk_core.permission_processing_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.permission_processing_policies FORCE ROW LEVEL SECURITY;

CREATE POLICY permission_processing_policies_identity_read ON werk_core.permission_processing_policies
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY permission_processing_policies_admin_read ON werk_core.permission_processing_policies
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY permission_processing_policies_owner_all ON werk_core.permission_processing_policies
    TO werk_owner USING (true) WITH CHECK (true);

COMMENT ON TABLE werk_core.permission_processing_policies IS
    'Mandatory processing declaration for each permission/resource binding. References are structural and do not establish legal validity.';
COMMENT ON COLUMN werk_core.permission_processing_policies.legal_basis_ref IS
    'Server-controlled reference to a future operator-approved processing record; never supplied by a client request.';
