-- Identity owns protocol configuration and its exact Registry coupling. The
-- Registry remains metadata-only and receives no back-reference or secret.
CREATE TABLE werk_core.identity_oidc_client_configurations (
    id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision >= 1),
    identity_provider_key text NOT NULL REFERENCES werk_core.identity_providers(provider_key),
    issuer_uri text NOT NULL CHECK (length(btrim(issuer_uri)) BETWEEN 1 AND 2048),
    client_id text NOT NULL CHECK (length(btrim(client_id)) BETWEEN 1 AND 2048),
    redirect_uri text NOT NULL CHECK (length(btrim(redirect_uri)) BETWEEN 1 AND 2048),
    audience text NOT NULL REFERENCES werk_core.identity_audiences(audience)
        CHECK (audience IN ('work', 'admin')),
    client_authentication text NOT NULL CHECK (client_authentication IN ('none', 'client-secret-basic')),
    secret_material_handle text NULL CHECK (secret_material_handle IS NULL OR length(secret_material_handle) BETWEEN 1 AND 1024),
    secret_material_version bigint NULL CHECK (secret_material_version IS NULL OR secret_material_version >= 1),
    created_at timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES werk_core.accounts(id),
    PRIMARY KEY (id, revision),
    CHECK (
        (client_authentication = 'none' AND secret_material_handle IS NULL AND secret_material_version IS NULL)
        OR
        (client_authentication = 'client-secret-basic' AND secret_material_handle IS NOT NULL AND secret_material_version IS NOT NULL)
    )
);

CREATE TABLE werk_core.identity_federated_login_provider_bindings (
    identity_provider_key text PRIMARY KEY REFERENCES werk_core.identity_providers(provider_key),
    provider_kind text NOT NULL CHECK (provider_kind = 'oidc'),
    registry_provider_id uuid NOT NULL,
    registry_contract_version bigint NOT NULL CHECK (registry_contract_version = 1),
    service_contract_version bigint NOT NULL CHECK (service_contract_version = 1),
    capability_key text NOT NULL,
    capability_version bigint NOT NULL CHECK (capability_version = 1),
    configuration_id uuid NOT NULL,
    configuration_revision bigint NOT NULL CHECK (configuration_revision >= 1),
    status text NOT NULL CHECK (status IN ('active', 'disabled', 'retired')),
    revision bigint NOT NULL CHECK (revision >= 1),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    FOREIGN KEY (configuration_id, configuration_revision)
        REFERENCES werk_core.identity_oidc_client_configurations(id, revision),
    CHECK (capability_key = 'core.identity.service.login-federation.capability.oidc.login')
);

CREATE UNIQUE INDEX identity_federated_login_configuration_binding_idx
    ON werk_core.identity_federated_login_provider_bindings
       (configuration_id, configuration_revision);

REVOKE ALL ON werk_core.identity_oidc_client_configurations,
              werk_core.identity_federated_login_provider_bindings
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.identity_oidc_client_configurations,
                werk_core.identity_federated_login_provider_bindings
    TO werk_identity_runtime;
GRANT SELECT ON werk_core.identity_oidc_client_configurations,
                werk_core.identity_federated_login_provider_bindings
    TO werk_backup_reader;

ALTER TABLE werk_core.identity_oidc_client_configurations ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_client_configurations FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_federated_login_provider_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_federated_login_provider_bindings FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_oidc_client_configurations_identity_read
    ON werk_core.identity_oidc_client_configurations FOR SELECT TO werk_identity_runtime
    USING (true);
CREATE POLICY identity_oidc_client_configurations_owner_all
    ON werk_core.identity_oidc_client_configurations TO werk_owner
    USING (true) WITH CHECK (true);
CREATE POLICY identity_federated_login_bindings_identity_read
    ON werk_core.identity_federated_login_provider_bindings FOR SELECT TO werk_identity_runtime
    USING (true);
CREATE POLICY identity_federated_login_bindings_owner_all
    ON werk_core.identity_federated_login_provider_bindings TO werk_owner
    USING (true) WITH CHECK (true);
