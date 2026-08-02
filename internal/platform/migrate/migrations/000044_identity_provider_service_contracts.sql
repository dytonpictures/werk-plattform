-- Phase A of ADR-033 registers the protocol-facing Identity contracts without
-- pretending that a configured or executable external provider already
-- exists. Service capabilities remain distinct from RBAC permissions and the
-- provider registry remains free of endpoints, credentials and health state.

INSERT INTO werk_core.platform_service_contracts (
    service_key, owner_module, contract_version, lifecycle
) VALUES
    ('core.identity.service.directory', 'core.identity', 1, 'active'),
    ('core.identity.service.login-federation', 'core.identity', 1, 'active');

INSERT INTO werk_core.platform_service_capability_contracts (
    service_key, service_contract_version, capability_key,
    capability_version, operation_boundary, lifecycle
) VALUES
    (
        'core.identity.service.directory', 1,
        'core.identity.service.directory.capability.user.read',
        1, 'tenant', 'active'
    ),
    (
        'core.identity.service.directory', 1,
        'core.identity.service.directory.capability.group.read',
        1, 'tenant', 'active'
    ),
    (
        'core.identity.service.directory', 1,
        'core.identity.service.directory.capability.group-membership.read',
        1, 'tenant', 'active'
    ),
    (
        'core.identity.service.directory', 1,
        'core.identity.service.directory.capability.change-set.read',
        1, 'tenant', 'active'
    ),
    (
        'core.identity.service.login-federation', 1,
        'core.identity.service.login-federation.capability.oidc.login',
        1, 'installation', 'active'
    ),
    (
        'core.identity.service.login-federation', 1,
        'core.identity.service.login-federation.capability.saml.login',
        1, 'installation', 'active'
    );

INSERT INTO werk_core.resource_type_registrations (
    resource_kind, owner_module, display_name, boundary
) VALUES
    (
        'core.identity.login-provider', 'core.identity',
        'Föderierter Anmeldeprovider', 'installation'
    ),
    (
        'core.identity.directory-provider', 'core.identity',
        'Externer Verzeichnisprovider', 'installation'
    ),
    (
        'core.identity.directory-import', 'core.identity',
        'Tenantgebundener Verzeichnisimport', 'tenant'
    );

INSERT INTO werk_core.resource_data_profiles (
    resource_kind, personal_data_category, confidentiality_level,
    processing_activity_required
) VALUES
    ('core.identity.login-provider', 'none', 'confidential', false),
    ('core.identity.directory-provider', 'none', 'confidential', false),
    ('core.identity.directory-import', 'personal', 'restricted', true);

INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES
    (
        '0196f000-0000-7000-8000-000000001003',
        'core.platform.provider-registry.read',
        'Service- und Provider-Registry anzeigen',
        'core.platform', 'admin', 'high'
    ),
    (
        '0196f000-0000-7000-8000-000000001004',
        'core.identity.login-provider.read',
        'Föderierte Anmeldeprovider anzeigen',
        'core.identity', 'admin', 'high'
    ),
    (
        '0196f000-0000-7000-8000-000000001005',
        'core.identity.login-provider.configure',
        'Föderierte Anmeldeprovider konfigurieren',
        'core.identity', 'admin', 'critical'
    ),
    (
        '0196f000-0000-7000-8000-000000001006',
        'core.identity.directory-provider.read',
        'Verzeichnisprovider anzeigen',
        'core.identity', 'admin', 'high'
    ),
    (
        '0196f000-0000-7000-8000-000000001007',
        'core.identity.directory-provider.configure',
        'Verzeichnisprovider konfigurieren',
        'core.identity', 'admin', 'critical'
    ),
    (
        '0196f000-0000-7000-8000-000000001008',
        'core.identity.directory-import.read',
        'Verzeichnisimporte anzeigen',
        'core.identity', 'work', 'high'
    ),
    (
        '0196f000-0000-7000-8000-000000001009',
        'core.identity.directory-import.execute',
        'Verzeichnisimporte ausführen',
        'core.identity', 'service', 'high'
    );

INSERT INTO werk_core.role_permissions (role_id, permission_id)
VALUES
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000001003'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000001004'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000001005'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000001006'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000001007');

INSERT INTO werk_core.permission_resource_types (permission_id, resource_kind)
VALUES
    ('0196f000-0000-7000-8000-000000001003', 'core.platform.installation'),
    ('0196f000-0000-7000-8000-000000001004', 'core.identity.login-provider'),
    ('0196f000-0000-7000-8000-000000001005', 'core.identity.login-provider'),
    ('0196f000-0000-7000-8000-000000001006', 'core.identity.directory-provider'),
    ('0196f000-0000-7000-8000-000000001007', 'core.identity.directory-provider'),
    ('0196f000-0000-7000-8000-000000001008', 'core.identity.directory-import'),
    ('0196f000-0000-7000-8000-000000001009', 'core.identity.directory-import');

INSERT INTO werk_core.permission_processing_policies (
    permission_id, resource_kind, processing_required,
    activity_key, purpose_key, legal_basis_ref
) VALUES
    (
        '0196f000-0000-7000-8000-000000001003',
        'core.platform.installation', false, NULL, NULL, NULL
    ),
    (
        '0196f000-0000-7000-8000-000000001004',
        'core.identity.login-provider', false, NULL, NULL, NULL
    ),
    (
        '0196f000-0000-7000-8000-000000001005',
        'core.identity.login-provider', false, NULL, NULL, NULL
    ),
    (
        '0196f000-0000-7000-8000-000000001006',
        'core.identity.directory-provider', false, NULL, NULL, NULL
    ),
    (
        '0196f000-0000-7000-8000-000000001007',
        'core.identity.directory-provider', false, NULL, NULL, NULL
    ),
    (
        '0196f000-0000-7000-8000-000000001008',
        'core.identity.directory-import', true,
        'core.identity.directory-import',
        'core.identity.access-provisioning',
        'operator.processing-register.identity-access'
    ),
    (
        '0196f000-0000-7000-8000-000000001009',
        'core.identity.directory-import', true,
        'core.identity.directory-import',
        'core.identity.access-provisioning',
        'operator.processing-register.identity-access'
    );

-- This observation policy is intentionally exact. It does not grant a
-- general installation-wide audit writer to the admin runtime.
CREATE POLICY security_audit_admin_provider_registry_observation
    ON werk_core.security_audit_events
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (
        werk_security.current_tenant_id() IS NULL
        AND tenant_id IS NULL
        AND event_type = 'core.platform.provider-registry-listed.v1'
        AND outcome = 'succeeded'
        AND account_id IS NOT NULL
    );

COMMENT ON POLICY security_audit_admin_provider_registry_observation
    ON werk_core.security_audit_events
IS 'Allows only the self-audit record produced by the bounded Provider Registry administration inventory.';

-- Runtime consumers receive no table-wide registry grant. They can resolve
-- one fully specified tuple through this function and must validate the same
-- tuple again against the in-process providerregistry contract. Row locks keep
-- lifecycle changes ordered with the caller's transaction.
CREATE FUNCTION werk_security.resolve_platform_provider_registry(
    candidate_provider_id uuid,
    candidate_registry_contract_version bigint,
    candidate_service_key text,
    candidate_service_contract_version bigint,
    candidate_capability_key text,
    candidate_capability_version bigint,
    candidate_operation_boundary text,
    candidate_tenant_id uuid
)
RETURNS TABLE (
    service_owner_module text,
    service_key text,
    service_contract_version bigint,
    service_lifecycle text,
    capability_service_key text,
    capability_service_contract_version bigint,
    capability_key text,
    capability_version bigint,
    capability_operation_boundary text,
    capability_lifecycle text,
    provider_id uuid,
    provider_service_key text,
    provider_service_contract_version bigint,
    provider_key text,
    adapter_key text,
    provider_config_scope text,
    provider_tenant_id uuid,
    provider_lifecycle text,
    provider_revision bigint,
    provider_registry_contract_version bigint,
    binding_provider_id uuid,
    binding_service_key text,
    binding_service_contract_version bigint,
    binding_capability_key text,
    binding_capability_version bigint,
    binding_lifecycle text,
    binding_revision bigint
)
LANGUAGE sql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
    SELECT
        service.owner_module,
        service.service_key,
        service.contract_version,
        service.lifecycle,
        capability.service_key,
        capability.service_contract_version,
        capability.capability_key,
        capability.capability_version,
        capability.operation_boundary,
        capability.lifecycle,
        provider.id,
        provider.service_key,
        provider.service_contract_version,
        provider.provider_key,
        provider.adapter_key,
        provider.config_scope,
        provider.tenant_id,
        provider.lifecycle,
        provider.revision,
        provider.registry_contract_version,
        binding.provider_id,
        binding.service_key,
        binding.service_contract_version,
        binding.capability_key,
        binding.capability_version,
        binding.lifecycle,
        binding.revision
    FROM werk_core.platform_service_contracts AS service
    JOIN werk_core.platform_service_capability_contracts AS capability
      ON capability.service_key = service.service_key
     AND capability.service_contract_version = service.contract_version
    JOIN werk_core.platform_provider_registrations AS provider
      ON provider.service_key = service.service_key
     AND provider.service_contract_version = service.contract_version
    JOIN werk_core.platform_provider_capability_bindings AS binding
      ON binding.provider_id = provider.id
     AND binding.service_key = provider.service_key
     AND binding.service_contract_version = provider.service_contract_version
     AND binding.capability_key = capability.capability_key
     AND binding.capability_version = capability.capability_version
    WHERE provider.id = candidate_provider_id
      AND provider.registry_contract_version = candidate_registry_contract_version
      AND service.service_key = candidate_service_key
      AND service.contract_version = candidate_service_contract_version
      AND capability.capability_key = candidate_capability_key
      AND capability.capability_version = candidate_capability_version
      AND capability.operation_boundary = candidate_operation_boundary
      AND (
          (
              candidate_operation_boundary = 'installation'
              AND candidate_tenant_id IS NULL
              AND provider.config_scope = 'installation'
              AND provider.tenant_id IS NULL
          )
          OR
          (
              candidate_operation_boundary = 'tenant'
              AND candidate_tenant_id IS NOT NULL
              AND (
                  (provider.config_scope = 'installation' AND provider.tenant_id IS NULL)
                  OR
                  (
                      provider.config_scope = 'tenant'
                      AND provider.tenant_id = candidate_tenant_id
                  )
              )
          )
      )
      AND (
          session_user <> 'werk_service_runtime'
          OR (
              candidate_operation_boundary = 'tenant'
              AND candidate_tenant_id = werk_security.current_tenant_id()
          )
      )
    FOR SHARE OF service, capability, provider, binding
$function$;

REVOKE ALL ON FUNCTION werk_security.resolve_platform_provider_registry(
    uuid, bigint, text, bigint, text, bigint, text, uuid
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION werk_security.resolve_platform_provider_registry(
    uuid, bigint, text, bigint, text, bigint, text, uuid
) TO werk_identity_runtime, werk_service_runtime;

COMMENT ON FUNCTION werk_security.resolve_platform_provider_registry(
    uuid, bigint, text, bigint, text, bigint, text, uuid
) IS 'Fail-closed exact Provider Registry tuple resolution. It performs no provider selection and exposes no configuration, secret, endpoint or health state.';
