CREATE TABLE werk_core.business_object_view_contracts (
    resource_kind text NOT NULL,
    contract_version bigint NOT NULL CHECK (contract_version > 0),
    owner_module text NOT NULL,
    read_permission_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    retired_at timestamptz NULL,
    PRIMARY KEY (resource_kind, contract_version),
    UNIQUE (resource_kind, contract_version, owner_module),
    FOREIGN KEY (resource_kind)
        REFERENCES werk_core.resource_type_registrations(resource_kind)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (owner_module)
        REFERENCES werk_core.platform_modules(module_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (read_permission_id, resource_kind)
        REFERENCES werk_core.permission_resource_types(permission_id, resource_kind)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (read_permission_id, resource_kind)
        REFERENCES werk_core.permission_processing_policies(permission_id, resource_kind)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (
        (status = 'active' AND retired_at IS NULL)
        OR
        (status = 'retired' AND retired_at IS NOT NULL AND retired_at >= created_at)
    )
);

CREATE UNIQUE INDEX business_object_view_contracts_active_kind_idx
    ON werk_core.business_object_view_contracts (resource_kind)
    WHERE status = 'active';

CREATE FUNCTION werk_security.validate_business_object_view_contract()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'business object view contracts must be retired, not deleted';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF OLD.status <> 'active'
           OR NEW.status <> 'retired'
           OR NEW.resource_kind IS DISTINCT FROM OLD.resource_kind
           OR NEW.contract_version IS DISTINCT FROM OLD.contract_version
           OR NEW.owner_module IS DISTINCT FROM OLD.owner_module
           OR NEW.read_permission_id IS DISTINCT FROM OLD.read_permission_id
           OR NEW.created_at IS DISTINCT FROM OLD.created_at
           OR NEW.retired_at IS NULL THEN
            RAISE EXCEPTION 'business object view contract meaning is immutable';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.status <> 'active' OR NEW.retired_at IS NOT NULL OR NOT EXISTS (
        SELECT 1
        FROM werk_core.resource_type_registrations AS resource_type
        JOIN werk_core.platform_modules AS module
          ON module.module_key = resource_type.owner_module
        JOIN werk_core.resource_data_profiles AS profile
          ON profile.resource_kind = resource_type.resource_kind
        JOIN werk_core.permission_resource_types AS binding
          ON binding.resource_kind = resource_type.resource_kind
         AND binding.permission_id = NEW.read_permission_id
        JOIN werk_core.permissions AS permission
          ON permission.id = binding.permission_id
        JOIN werk_core.permission_processing_policies AS processing
          ON processing.permission_id = binding.permission_id
         AND processing.resource_kind = binding.resource_kind
        WHERE resource_type.resource_kind = NEW.resource_kind
          AND resource_type.owner_module = NEW.owner_module
          AND resource_type.boundary = 'tenant'
          AND resource_type.status = 'active'
          AND module.module_key = NEW.owner_module
          AND module.status = 'active'
          AND profile.status = 'active'
          AND permission.owning_module = NEW.owner_module
          AND permission.access_plane = 'work'
          AND permission.status = 'active'
          AND processing.status = 'active'
          AND (NOT profile.processing_activity_required OR processing.processing_required)
    ) THEN
        RAISE EXCEPTION 'business object view contract is not backed by an active tenant resource and work policy';
    END IF;

    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.validate_business_object_view_contract() FROM PUBLIC;

CREATE TRIGGER business_object_view_contracts_validate
    BEFORE INSERT OR UPDATE OR DELETE ON werk_core.business_object_view_contracts
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_business_object_view_contract();

CREATE FUNCTION werk_security.business_object_view_contract_active(
    requested_resource_kind text,
    requested_contract_version bigint,
    requested_owner_module text
)
RETURNS boolean
LANGUAGE sql
STABLE
PARALLEL SAFE
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
    SELECT EXISTS (
        SELECT 1
        FROM werk_core.business_object_view_contracts AS contract
        JOIN werk_core.resource_type_registrations AS resource_type
          ON resource_type.resource_kind = contract.resource_kind
         AND resource_type.owner_module = contract.owner_module
        JOIN werk_core.platform_modules AS module
          ON module.module_key = contract.owner_module
        JOIN werk_core.resource_data_profiles AS profile
          ON profile.resource_kind = contract.resource_kind
        JOIN werk_core.permission_resource_types AS binding
          ON binding.resource_kind = contract.resource_kind
         AND binding.permission_id = contract.read_permission_id
        JOIN werk_core.permissions AS permission
          ON permission.id = binding.permission_id
        JOIN werk_core.permission_processing_policies AS processing
          ON processing.permission_id = binding.permission_id
         AND processing.resource_kind = binding.resource_kind
        WHERE contract.resource_kind = requested_resource_kind
          AND contract.contract_version = requested_contract_version
          AND contract.owner_module = requested_owner_module
          AND contract.status = 'active'
          AND resource_type.boundary = 'tenant'
          AND resource_type.status = 'active'
          AND module.status = 'active'
          AND profile.status = 'active'
          AND permission.owning_module = contract.owner_module
          AND permission.access_plane = 'work'
          AND permission.status = 'active'
          AND processing.status = 'active'
          AND (NOT profile.processing_activity_required OR processing.processing_required)
    )
$function$;

CREATE FUNCTION werk_security.business_object_view_tenant_active(requested_tenant_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
PARALLEL SAFE
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
    SELECT EXISTS (
        SELECT 1
        FROM werk_core.tenants AS tenant
        WHERE tenant.id = requested_tenant_id
          AND tenant.status = 'active'
    )
$function$;

REVOKE ALL ON FUNCTION werk_security.business_object_view_contract_active(text, bigint, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.business_object_view_tenant_active(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION werk_security.business_object_view_contract_active(text, bigint, text)
    TO werk_work_runtime, werk_admin_runtime;
GRANT EXECUTE ON FUNCTION werk_security.business_object_view_tenant_active(uuid)
    TO werk_work_runtime;

CREATE VIEW werk_core.active_business_object_view_contracts
WITH (security_barrier = true)
AS
SELECT
    contract.resource_kind,
    contract.contract_version,
    contract.owner_module,
    permission.permission_key AS read_permission_key,
    module.module_kind,
    module.contract_version AS module_contract_version,
    resource_type.boundary AS resource_boundary,
    resource_type.contract_version AS resource_contract_version,
    profile.personal_data_category,
    profile.confidentiality_level,
    profile.processing_activity_required,
    profile.contract_version AS data_profile_contract_version
FROM werk_core.business_object_view_contracts AS contract
JOIN werk_core.resource_type_registrations AS resource_type
  ON resource_type.resource_kind = contract.resource_kind
 AND resource_type.owner_module = contract.owner_module
JOIN werk_core.platform_modules AS module
  ON module.module_key = contract.owner_module
JOIN werk_core.resource_data_profiles AS profile
  ON profile.resource_kind = contract.resource_kind
JOIN werk_core.permission_resource_types AS binding
  ON binding.resource_kind = contract.resource_kind
 AND binding.permission_id = contract.read_permission_id
JOIN werk_core.permissions AS permission
  ON permission.id = binding.permission_id
JOIN werk_core.permission_processing_policies AS processing
  ON processing.permission_id = binding.permission_id
 AND processing.resource_kind = binding.resource_kind
WHERE contract.status = 'active'
  AND resource_type.boundary = 'tenant'
  AND resource_type.status = 'active'
  AND module.status = 'active'
  AND profile.status = 'active'
  AND permission.owning_module = contract.owner_module
  AND permission.access_plane = 'work'
  AND permission.status = 'active'
  AND processing.status = 'active'
  AND (NOT profile.processing_activity_required OR processing.processing_required);

REVOKE ALL ON werk_core.active_business_object_view_contracts
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.active_business_object_view_contracts
    TO werk_work_runtime, werk_admin_runtime, werk_backup_reader;

CREATE TABLE werk_core.business_object_views (
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    resource_kind text NOT NULL,
    resource_id text NOT NULL CHECK (
        resource_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$'
    ),
    contract_version bigint NOT NULL CHECK (contract_version > 0),
    owner_module text NOT NULL,
    title text NULL CHECK (
        title IS NULL
        OR (title = btrim(title) AND char_length(title) BETWEEN 1 AND 240)
    ),
    classification text NULL CHECK (
        classification IN ('public', 'internal', 'confidential', 'restricted')
    ),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'withdrawn')),
    source_version bigint NOT NULL CHECK (source_version > 0),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at timestamptz NOT NULL,
    projected_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
    PRIMARY KEY (tenant_id, resource_kind, resource_id),
    FOREIGN KEY (resource_kind, contract_version, owner_module)
        REFERENCES werk_core.business_object_view_contracts(
            resource_kind, contract_version, owner_module
        )
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (
        (state = 'active' AND title IS NOT NULL)
        OR
        (state = 'withdrawn' AND title IS NULL AND classification IS NULL)
    )
);

CREATE INDEX business_object_views_active_updated_idx
    ON werk_core.business_object_views (
        tenant_id, resource_kind, updated_at DESC, resource_id DESC
    )
    WHERE state = 'active';

CREATE FUNCTION werk_security.validate_business_object_view()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    baseline_classification text;
    baseline_rank integer;
    object_rank integer;
BEGIN
    IF NOT werk_security.business_object_view_contract_active(
        NEW.resource_kind, NEW.contract_version, NEW.owner_module
    ) THEN
        RAISE EXCEPTION 'business object view contract is not active';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
           OR NEW.resource_kind IS DISTINCT FROM OLD.resource_kind
           OR NEW.resource_id IS DISTINCT FROM OLD.resource_id
           OR NEW.owner_module IS DISTINCT FROM OLD.owner_module THEN
            RAISE EXCEPTION 'business object view identity and owner are immutable';
        END IF;
        IF NEW.contract_version < OLD.contract_version
           OR NEW.source_version <= OLD.source_version
           OR NEW.version <> OLD.version + 1
           OR NEW.updated_at < OLD.updated_at THEN
            RAISE EXCEPTION 'business object view source revision is stale';
        END IF;
    ELSIF NEW.version <> 1 THEN
        RAISE EXCEPTION 'a new business object view must start at version one';
    END IF;

    IF NEW.state = 'active'
       AND NOT werk_security.business_object_view_tenant_active(NEW.tenant_id) THEN
        RAISE EXCEPTION 'an active business object view requires an active tenant';
    END IF;

    SELECT profile.confidentiality_level
      INTO baseline_classification
      FROM werk_core.resource_data_profiles AS profile
     WHERE profile.resource_kind = NEW.resource_kind
       AND profile.status = 'active';

    IF baseline_classification IS NULL THEN
        RAISE EXCEPTION 'business object view data profile is not active';
    END IF;

    IF NEW.classification IS NOT NULL THEN
        baseline_rank := CASE baseline_classification
            WHEN 'public' THEN 0
            WHEN 'internal' THEN 1
            WHEN 'confidential' THEN 2
            WHEN 'restricted' THEN 3
            ELSE NULL
        END;
        object_rank := CASE NEW.classification
            WHEN 'public' THEN 0
            WHEN 'internal' THEN 1
            WHEN 'confidential' THEN 2
            WHEN 'restricted' THEN 3
            ELSE NULL
        END;
        IF baseline_rank IS NULL OR object_rank IS NULL OR object_rank < baseline_rank THEN
            RAISE EXCEPTION 'business object view classification cannot weaken its resource profile';
        END IF;
    END IF;

    NEW.projected_at := transaction_timestamp();
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.validate_business_object_view() FROM PUBLIC;

CREATE TRIGGER business_object_views_validate
    BEFORE INSERT OR UPDATE ON werk_core.business_object_views
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_business_object_view();

REVOKE ALL ON
    werk_core.business_object_view_contracts,
    werk_core.business_object_views
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
     werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON werk_core.business_object_view_contracts
    TO werk_admin_runtime, werk_backup_reader;
GRANT SELECT ON werk_core.business_object_views
    TO werk_work_runtime, werk_admin_runtime, werk_backup_reader;
GRANT INSERT, UPDATE ON werk_core.business_object_views
    TO werk_admin_runtime;

ALTER TABLE werk_core.business_object_view_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.business_object_view_contracts FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.business_object_views ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.business_object_views FORCE ROW LEVEL SECURITY;

CREATE POLICY business_object_view_contracts_admin_read
    ON werk_core.business_object_view_contracts
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY business_object_view_contracts_owner_all
    ON werk_core.business_object_view_contracts
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY business_object_views_tenant_gate
    ON werk_core.business_object_views
    AS RESTRICTIVE TO werk_work_runtime, werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY business_object_views_work_read
    ON werk_core.business_object_views
    FOR SELECT TO werk_work_runtime
    USING (
        state = 'active'
        AND werk_security.business_object_view_tenant_active(tenant_id)
        AND werk_security.business_object_view_contract_active(
            resource_kind, contract_version, owner_module
        )
    );
CREATE POLICY business_object_views_admin_workspace_manage
    ON werk_core.business_object_views
    TO werk_admin_runtime
    USING (
        resource_kind = 'core.workspace.workspace'
        AND resource_id = tenant_id::text
        AND werk_security.business_object_view_contract_active(
            resource_kind, contract_version, owner_module
        )
    )
    WITH CHECK (
        resource_kind = 'core.workspace.workspace'
        AND resource_id = tenant_id::text
        AND werk_security.business_object_view_contract_active(
            resource_kind, contract_version, owner_module
        )
    );
CREATE POLICY business_object_views_owner_all
    ON werk_core.business_object_views
    TO werk_owner USING (true) WITH CHECK (true);

INSERT INTO werk_core.business_object_view_contracts (
    resource_kind, contract_version, owner_module, read_permission_id
)
SELECT
    'core.workspace.workspace', 1, 'core.workspace', permission.id
FROM werk_core.permissions AS permission
WHERE permission.permission_key = 'core.workspace.access';

DO $assert_workspace_business_object_contract$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM werk_core.business_object_view_contracts AS contract
        WHERE contract.resource_kind = 'core.workspace.workspace'
          AND contract.contract_version = 1
          AND contract.owner_module = 'core.workspace'
          AND contract.status = 'active'
    ) THEN
        RAISE EXCEPTION 'workspace business object view contract could not be registered';
    END IF;
END
$assert_workspace_business_object_contract$;

INSERT INTO werk_core.business_object_views (
    tenant_id, resource_kind, resource_id, contract_version, owner_module,
    title, classification, state, source_version, updated_at
)
SELECT
    tenant.id,
    'core.workspace.workspace',
    tenant.id::text,
    1,
    'core.workspace',
    CASE WHEN tenant.status = 'active' THEN btrim(tenant.name) ELSE NULL END,
    NULL,
    CASE WHEN tenant.status = 'active' THEN 'active' ELSE 'withdrawn' END,
    tenant.version,
    tenant.updated_at
FROM werk_core.tenants AS tenant;

COMMENT ON TABLE werk_core.business_object_view_contracts IS
    'Versioned, owner-bound allow-list for minimized BusinessObjectView projections. V1 registers only the tenant workspace and its server-side Work read permission.';
COMMENT ON VIEW werk_core.active_business_object_view_contracts IS
    'Read-only resolved BusinessObjectView contracts whose module, tenant resource, Work permission binding, data profile, and processing policy are all active.';
COMMENT ON TABLE werk_core.business_object_views IS
    'Minimized tenant projections for navigation and authorized context reads; owner modules retain all domain truth. Runtime deletion is intentionally not granted.';
COMMENT ON COLUMN werk_core.business_object_views.source_version IS
    'Monotonic version supplied by the owning resource. Higher versions may withdraw or reactivate a projection; identical retries are handled by the Go store.';
COMMENT ON COLUMN werk_core.business_object_views.version IS
    'Core-owned projection version exposed as the strong HTTP entity tag; it advances once for every effective projection change and does not reveal the owner source version.';
COMMENT ON COLUMN werk_core.business_object_views.classification IS
    'Optional stricter object classification. NULL inherits the mandatory resource data profile and never weakens it.';
