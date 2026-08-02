-- Declarative, platform-global metadata for logical services and their
-- explicitly selected providers. This registry contains no provider
-- configuration, endpoint, credential, key, certificate, token, or health.

CREATE TABLE werk_core.platform_service_contracts (
    service_key text NOT NULL CHECK (
        length(service_key) BETWEEN 2 AND 160
        AND service_key ~ '^[a-z][a-z0-9.-]+$'
    ),
    owner_module text NOT NULL REFERENCES werk_core.platform_modules(module_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    contract_version bigint NOT NULL CHECK (contract_version > 0),
    lifecycle text NOT NULL DEFAULT 'active'
        CHECK (lifecycle IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_key, contract_version),
    CHECK (service_key LIKE owner_module || '.service.%')
);

CREATE TABLE werk_core.platform_service_capability_contracts (
    service_key text NOT NULL,
    service_contract_version bigint NOT NULL,
    capability_key text NOT NULL CHECK (
        length(capability_key) BETWEEN 2 AND 160
        AND capability_key ~ '^[a-z][a-z0-9.-]+$'
    ),
    capability_version bigint NOT NULL CHECK (capability_version > 0),
    operation_boundary text NOT NULL
        CHECK (operation_boundary IN ('installation', 'tenant')),
    lifecycle text NOT NULL DEFAULT 'active'
        CHECK (lifecycle IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (
        service_key, service_contract_version,
        capability_key, capability_version
    ),
    FOREIGN KEY (service_key, service_contract_version)
        REFERENCES werk_core.platform_service_contracts(service_key, contract_version)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (capability_key LIKE service_key || '.capability.%')
);

CREATE TABLE werk_core.platform_provider_registrations (
    id uuid PRIMARY KEY,
    service_key text NOT NULL,
    service_contract_version bigint NOT NULL,
    provider_key text NOT NULL CHECK (
        length(provider_key) BETWEEN 2 AND 160
        AND provider_key ~ '^[a-z][a-z0-9.-]+$'
    ),
    adapter_key text NOT NULL CHECK (
        length(adapter_key) BETWEEN 2 AND 160
        AND adapter_key ~ '^[a-z][a-z0-9.-]+$'
    ),
    config_scope text NOT NULL CHECK (config_scope IN ('installation', 'tenant')),
    tenant_id uuid NULL REFERENCES werk_core.tenants(id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    registry_contract_version bigint NOT NULL CHECK (registry_contract_version > 0),
    lifecycle text NOT NULL DEFAULT 'disabled'
        CHECK (lifecycle IN ('active', 'disabled', 'retired')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, service_key, service_contract_version),
    UNIQUE NULLS NOT DISTINCT (
        service_key, service_contract_version, tenant_id, provider_key,
        registry_contract_version
    ),
    FOREIGN KEY (service_key, service_contract_version)
        REFERENCES werk_core.platform_service_contracts(service_key, contract_version)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (provider_key LIKE service_key || '.provider.%'),
    CHECK (
        (config_scope = 'installation' AND tenant_id IS NULL)
        OR (config_scope = 'tenant' AND tenant_id IS NOT NULL)
    )
);

CREATE TABLE werk_core.platform_provider_capability_bindings (
    provider_id uuid NOT NULL,
    service_key text NOT NULL,
    service_contract_version bigint NOT NULL,
    capability_key text NOT NULL,
    capability_version bigint NOT NULL,
    lifecycle text NOT NULL DEFAULT 'disabled'
        CHECK (lifecycle IN ('active', 'disabled', 'retired')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_id, capability_key, capability_version),
    FOREIGN KEY (provider_id, service_key, service_contract_version)
        REFERENCES werk_core.platform_provider_registrations(
            id, service_key, service_contract_version
        ) ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (
        service_key, service_contract_version,
        capability_key, capability_version
    ) REFERENCES werk_core.platform_service_capability_contracts(
        service_key, service_contract_version,
        capability_key, capability_version
    ) ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE INDEX platform_provider_registrations_service_idx
    ON werk_core.platform_provider_registrations (
        service_key, service_contract_version, lifecycle
    );
CREATE INDEX platform_provider_registrations_tenant_idx
    ON werk_core.platform_provider_registrations (tenant_id, lifecycle)
    WHERE tenant_id IS NOT NULL;
CREATE INDEX platform_provider_capability_lookup_idx
    ON werk_core.platform_provider_capability_bindings (
        service_key, service_contract_version,
        capability_key, capability_version, lifecycle
    );

CREATE FUNCTION werk_security.protect_platform_provider_registry_lifecycle()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF OLD.lifecycle = 'retired' AND NEW.lifecycle <> 'retired' THEN
        RAISE EXCEPTION 'retired provider registry entries cannot be reactivated'
            USING ERRCODE = '23514';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.protect_platform_provider_registration()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF OLD.id <> NEW.id
       OR OLD.service_key <> NEW.service_key
       OR OLD.service_contract_version <> NEW.service_contract_version
       OR OLD.provider_key <> NEW.provider_key
       OR OLD.adapter_key <> NEW.adapter_key
       OR OLD.config_scope <> NEW.config_scope
       OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
       OR OLD.registry_contract_version <> NEW.registry_contract_version
       OR OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'provider registry identity is immutable'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.revision <> OLD.revision + 1 THEN
        RAISE EXCEPTION 'provider registry revision must increase by one'
            USING ERRCODE = '40001';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.protect_platform_service_contract()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF OLD.service_key <> NEW.service_key
       OR OLD.owner_module <> NEW.owner_module
       OR OLD.contract_version <> NEW.contract_version
       OR OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'service contract identity is immutable'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.protect_platform_service_capability_contract()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF OLD.service_key <> NEW.service_key
       OR OLD.service_contract_version <> NEW.service_contract_version
       OR OLD.capability_key <> NEW.capability_key
       OR OLD.capability_version <> NEW.capability_version
       OR OLD.operation_boundary <> NEW.operation_boundary
       OR OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'service capability contract meaning is immutable'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.protect_platform_provider_binding()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF OLD.provider_id <> NEW.provider_id
       OR OLD.service_key <> NEW.service_key
       OR OLD.service_contract_version <> NEW.service_contract_version
       OR OLD.capability_key <> NEW.capability_key
       OR OLD.capability_version <> NEW.capability_version
       OR OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'provider capability binding identity is immutable'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.revision <> OLD.revision + 1 THEN
        RAISE EXCEPTION 'provider capability binding revision must increase by one'
            USING ERRCODE = '40001';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_platform_provider_binding_scope()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
DECLARE
    provider_scope text;
    capability_boundary text;
BEGIN
    SELECT registration.config_scope
    INTO provider_scope
    FROM werk_core.platform_provider_registrations AS registration
    WHERE registration.id = NEW.provider_id
      AND registration.service_key = NEW.service_key
      AND registration.service_contract_version = NEW.service_contract_version;

    SELECT capability.operation_boundary
    INTO capability_boundary
    FROM werk_core.platform_service_capability_contracts AS capability
    WHERE capability.service_key = NEW.service_key
      AND capability.service_contract_version = NEW.service_contract_version
      AND capability.capability_key = NEW.capability_key
      AND capability.capability_version = NEW.capability_version;

    IF capability_boundary = 'installation' AND provider_scope <> 'installation' THEN
        RAISE EXCEPTION 'tenant-scoped provider cannot implement an installation-bound capability'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.reject_platform_provider_registry_delete()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    RAISE EXCEPTION 'provider registry entries must be retired, not deleted'
        USING ERRCODE = '23514';
    RETURN OLD;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.protect_platform_provider_registry_lifecycle() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_platform_service_contract() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_platform_service_capability_contract() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_platform_provider_registration() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_platform_provider_binding() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_platform_provider_binding_scope() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.reject_platform_provider_registry_delete() FROM PUBLIC;

CREATE TRIGGER platform_service_contracts_protect_lifecycle
BEFORE UPDATE ON werk_core.platform_service_contracts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_provider_registry_lifecycle();
CREATE TRIGGER platform_service_capability_contracts_protect_lifecycle
BEFORE UPDATE ON werk_core.platform_service_capability_contracts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_provider_registry_lifecycle();
CREATE TRIGGER platform_provider_registrations_protect_lifecycle
BEFORE UPDATE ON werk_core.platform_provider_registrations
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_provider_registry_lifecycle();
CREATE TRIGGER platform_provider_capability_bindings_protect_lifecycle
BEFORE UPDATE ON werk_core.platform_provider_capability_bindings
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_provider_registry_lifecycle();

CREATE TRIGGER platform_provider_registrations_protect_identity
BEFORE UPDATE ON werk_core.platform_provider_registrations
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_provider_registration();
CREATE TRIGGER platform_provider_capability_bindings_protect_identity
BEFORE UPDATE ON werk_core.platform_provider_capability_bindings
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_provider_binding();
CREATE TRIGGER platform_provider_capability_bindings_validate_scope
BEFORE INSERT OR UPDATE ON werk_core.platform_provider_capability_bindings
FOR EACH ROW EXECUTE FUNCTION werk_security.validate_platform_provider_binding_scope();
CREATE TRIGGER platform_service_contracts_protect_identity
BEFORE UPDATE ON werk_core.platform_service_contracts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_service_contract();
CREATE TRIGGER platform_service_capability_contracts_protect_identity
BEFORE UPDATE ON werk_core.platform_service_capability_contracts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_platform_service_capability_contract();

CREATE TRIGGER platform_service_contracts_reject_delete
BEFORE DELETE ON werk_core.platform_service_contracts
FOR EACH ROW EXECUTE FUNCTION werk_security.reject_platform_provider_registry_delete();
CREATE TRIGGER platform_service_capability_contracts_reject_delete
BEFORE DELETE ON werk_core.platform_service_capability_contracts
FOR EACH ROW EXECUTE FUNCTION werk_security.reject_platform_provider_registry_delete();
CREATE TRIGGER platform_provider_registrations_reject_delete
BEFORE DELETE ON werk_core.platform_provider_registrations
FOR EACH ROW EXECUTE FUNCTION werk_security.reject_platform_provider_registry_delete();
CREATE TRIGGER platform_provider_capability_bindings_reject_delete
BEFORE DELETE ON werk_core.platform_provider_capability_bindings
FOR EACH ROW EXECUTE FUNCTION werk_security.reject_platform_provider_registry_delete();

REVOKE ALL ON
    werk_core.platform_service_contracts,
    werk_core.platform_service_capability_contracts,
    werk_core.platform_provider_registrations,
    werk_core.platform_provider_capability_bindings
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
     werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON
    werk_core.platform_service_contracts,
    werk_core.platform_service_capability_contracts,
    werk_core.platform_provider_registrations,
    werk_core.platform_provider_capability_bindings
TO werk_admin_runtime, werk_backup_reader;

ALTER TABLE werk_core.platform_service_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_service_contracts FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_service_capability_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_service_capability_contracts FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_provider_registrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_provider_registrations FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_provider_capability_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_provider_capability_bindings FORCE ROW LEVEL SECURITY;

CREATE POLICY platform_service_contracts_admin_read
    ON werk_core.platform_service_contracts
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY platform_service_contracts_backup_read
    ON werk_core.platform_service_contracts
    FOR SELECT TO werk_backup_reader USING (true);
CREATE POLICY platform_service_contracts_owner_all
    ON werk_core.platform_service_contracts
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY platform_service_capability_contracts_admin_read
    ON werk_core.platform_service_capability_contracts
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY platform_service_capability_contracts_backup_read
    ON werk_core.platform_service_capability_contracts
    FOR SELECT TO werk_backup_reader USING (true);
CREATE POLICY platform_service_capability_contracts_owner_all
    ON werk_core.platform_service_capability_contracts
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY platform_provider_registrations_admin_read
    ON werk_core.platform_provider_registrations
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY platform_provider_registrations_backup_read
    ON werk_core.platform_provider_registrations
    FOR SELECT TO werk_backup_reader USING (true);
CREATE POLICY platform_provider_registrations_owner_all
    ON werk_core.platform_provider_registrations
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY platform_provider_capability_bindings_admin_read
    ON werk_core.platform_provider_capability_bindings
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY platform_provider_capability_bindings_backup_read
    ON werk_core.platform_provider_capability_bindings
    FOR SELECT TO werk_backup_reader USING (true);
CREATE POLICY platform_provider_capability_bindings_owner_all
    ON werk_core.platform_provider_capability_bindings
    TO werk_owner USING (true) WITH CHECK (true);

COMMENT ON TABLE werk_core.platform_service_contracts IS
    'Versioned logical backend services. A row is metadata, not a process, endpoint, permission, or health signal.';
COMMENT ON TABLE werk_core.platform_service_capability_contracts IS
    'Allow-listed, versioned technical service capabilities with an explicit operation boundary.';
COMMENT ON TABLE werk_core.platform_provider_registrations IS
    'Service-specific provider identities and adapter keys. Secrets, endpoints, paths, and health are forbidden.';
COMMENT ON TABLE werk_core.platform_provider_capability_bindings IS
    'Explicit provider-to-capability allow-list. It grants no RBAC permission and performs no automatic routing.';
