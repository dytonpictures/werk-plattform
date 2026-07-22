INSERT INTO werk_core.platform_modules (module_key, module_kind, display_name)
VALUES
    ('core.documents', 'core', 'Dokumente'),
    ('core.storage', 'core', 'Objektspeicher');

INSERT INTO werk_core.resource_type_registrations (
    resource_kind, owner_module, display_name, boundary
) VALUES
    ('core.documents.collection', 'core.documents', 'Dokumentsammlung', 'tenant'),
    ('core.documents.document', 'core.documents', 'Dokument', 'tenant'),
    ('core.documents.document-version', 'core.documents', 'Dokumentversion', 'tenant');

INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES
    ('0196f000-0000-7000-8000-000000000901', 'core.documents.document.create', 'Dokumente anlegen', 'core.documents', 'work', 'medium'),
    ('0196f000-0000-7000-8000-000000000902', 'core.documents.document.read', 'Dokumente lesen', 'core.documents', 'work', 'medium'),
    ('0196f000-0000-7000-8000-000000000903', 'core.documents.document.update', 'Dokumente bearbeiten', 'core.documents', 'work', 'medium'),
    ('0196f000-0000-7000-8000-000000000904', 'core.documents.version.create', 'Dokumentversionen anlegen', 'core.documents', 'work', 'medium'),
    ('0196f000-0000-7000-8000-000000000905', 'core.documents.content.download', 'Dokumentinhalt herunterladen', 'core.documents', 'work', 'high');

INSERT INTO werk_core.permission_resource_types (permission_id, resource_kind)
SELECT permission.id, target.resource_kind
FROM (VALUES
    ('core.documents.document.create', 'core.documents.collection'),
    ('core.documents.document.read', 'core.documents.document'),
    ('core.documents.document.update', 'core.documents.document'),
    ('core.documents.version.create', 'core.documents.document'),
    ('core.documents.content.download', 'core.documents.document-version')
) AS target(permission_key, resource_kind)
JOIN werk_core.permissions AS permission
  ON permission.permission_key = target.permission_key;

INSERT INTO werk_core.resource_data_profiles (
    resource_kind, personal_data_category, confidentiality_level,
    processing_activity_required
) VALUES
    ('core.documents.collection', 'personal', 'confidential', true),
    ('core.documents.document', 'personal', 'confidential', true),
    ('core.documents.document-version', 'personal', 'confidential', true);

INSERT INTO werk_core.permission_processing_policies (
    permission_id, resource_kind, processing_required,
    activity_key, purpose_key, legal_basis_ref
)
SELECT permission.id, policy.resource_kind, true,
       'core.documents.document-management', policy.purpose_key,
       'operator.processing-register.documents'
FROM (VALUES
    ('core.documents.document.create', 'core.documents.collection', 'core.documents.document-creation'),
    ('core.documents.document.read', 'core.documents.document', 'core.documents.document-use'),
    ('core.documents.document.update', 'core.documents.document', 'core.documents.document-maintenance'),
    ('core.documents.version.create', 'core.documents.document', 'core.documents.version-publication'),
    ('core.documents.content.download', 'core.documents.document-version', 'core.documents.content-delivery')
) AS policy(permission_key, resource_kind, purpose_key)
JOIN werk_core.permissions AS permission
  ON permission.permission_key = policy.permission_key
JOIN werk_core.permission_resource_types AS target
  ON target.permission_id = permission.id
 AND target.resource_kind = policy.resource_kind;

CREATE TABLE werk_core.storage_blobs (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    state text NOT NULL CHECK (state IN ('quarantined', 'available', 'rejected', 'missing', 'unknown')),
    size_bytes bigint NULL CHECK (size_bytes >= 0),
    sha256 bytea NULL CHECK (sha256 IS NULL OR octet_length(sha256) = 32),
    media_type text NULL CHECK (media_type IS NULL OR length(btrim(media_type)) BETWEEN 1 AND 255),
    created_by_account_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    verified_at timestamptz NULL,
    updated_at timestamptz NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (updated_at >= created_at),
    CHECK (
        (state = 'quarantined' AND size_bytes IS NULL AND sha256 IS NULL AND media_type IS NULL AND verified_at IS NULL AND version = 1)
        OR
        (state = 'available' AND size_bytes IS NOT NULL AND sha256 IS NOT NULL AND media_type IS NOT NULL AND verified_at IS NOT NULL AND verified_at >= created_at AND version >= 2)
        OR
        (state IN ('missing', 'unknown') AND size_bytes IS NOT NULL AND sha256 IS NOT NULL AND media_type IS NOT NULL AND verified_at IS NOT NULL AND verified_at >= created_at AND version >= 3)
        OR
        (state = 'rejected' AND size_bytes IS NULL AND sha256 IS NULL AND media_type IS NULL AND verified_at IS NULL AND version >= 2)
    )
);

CREATE TABLE werk_core.storage_blob_locations (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    blob_id uuid NOT NULL,
    provider_key text NOT NULL CHECK (provider_key ~ '^[a-z][a-z0-9.-]+$'),
    opaque_key uuid NOT NULL,
    state text NOT NULL CHECK (state IN ('quarantined', 'available', 'missing')),
    provider_checksum text NULL CHECK (provider_checksum IS NULL OR length(provider_checksum) <= 256),
    created_at timestamptz NOT NULL,
    activated_at timestamptz NULL,
    updated_at timestamptz NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, provider_key, opaque_key),
    FOREIGN KEY (tenant_id, blob_id)
        REFERENCES werk_core.storage_blobs(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (updated_at >= created_at),
    CHECK (
        (state = 'quarantined' AND provider_checksum IS NULL AND activated_at IS NULL AND version = 1)
        OR
        (state = 'available' AND activated_at IS NOT NULL AND activated_at >= created_at AND version >= 2)
        OR
        (state = 'missing' AND activated_at IS NOT NULL AND activated_at >= created_at AND version >= 3)
    )
);

CREATE TABLE werk_core.documents (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 240),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    source_module text NOT NULL REFERENCES werk_core.platform_modules(module_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    created_by_account_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (updated_at >= created_at)
);

CREATE TABLE werk_core.document_versions (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    document_id uuid NOT NULL,
    version_number bigint NOT NULL CHECK (version_number > 0),
    blob_id uuid NOT NULL,
    source text NOT NULL CHECK (source IN ('upload', 'import', 'collaboration', 'signature')),
    created_by_account_id uuid NOT NULL,
    published_at timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, document_id, version_number),
    FOREIGN KEY (tenant_id, document_id)
        REFERENCES werk_core.documents(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, blob_id)
        REFERENCES werk_core.storage_blobs(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, created_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE werk_core.document_classification_revisions (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    document_id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    classification text NOT NULL CHECK (classification IN ('internal', 'confidential', 'restricted')),
    retention_class text NOT NULL CHECK (retention_class ~ '^[a-z][a-z0-9.-]+$'),
    retention_until timestamptz NULL,
    legal_hold boolean NOT NULL DEFAULT false,
    recorded_by_account_id uuid NOT NULL,
    recorded_at timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, document_id, revision),
    FOREIGN KEY (tenant_id, document_id)
        REFERENCES werk_core.documents(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, recorded_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE INDEX storage_blobs_tenant_state_idx
    ON werk_core.storage_blobs (tenant_id, state, created_at);
CREATE INDEX storage_blob_locations_blob_idx
    ON werk_core.storage_blob_locations (tenant_id, blob_id, state);
CREATE INDEX documents_tenant_status_idx
    ON werk_core.documents (tenant_id, status, updated_at DESC);
CREATE INDEX document_versions_document_idx
    ON werk_core.document_versions (tenant_id, document_id, version_number DESC);
CREATE INDEX document_classification_revisions_document_idx
    ON werk_core.document_classification_revisions (tenant_id, document_id, revision DESC);

CREATE FUNCTION werk_security.protect_storage_blob_transition()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
BEGIN
    IF NOT (
        (OLD.state = 'quarantined' AND NEW.state IN ('available', 'rejected'))
        OR (OLD.state = 'available' AND NEW.state IN ('unknown', 'missing'))
        OR (OLD.state = 'unknown' AND NEW.state IN ('available', 'missing'))
        OR (OLD.state = 'missing' AND NEW.state = 'available')
    ) THEN
        RAISE EXCEPTION 'storage blob transition is not allowed';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id
       OR NEW.created_by_account_id <> OLD.created_by_account_id
       OR NEW.created_at <> OLD.created_at
       OR NEW.version <> OLD.version + 1
       OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'storage blob identity or version is immutable';
    END IF;
    IF OLD.state <> 'quarantined' AND (
        NEW.size_bytes IS DISTINCT FROM OLD.size_bytes
        OR NEW.sha256 IS DISTINCT FROM OLD.sha256
        OR NEW.media_type IS DISTINCT FROM OLD.media_type
        OR NEW.verified_at IS DISTINCT FROM OLD.verified_at
    ) THEN
        RAISE EXCEPTION 'sealed storage blob content metadata is immutable';
    END IF;
    IF NEW.state = 'available' AND NOT EXISTS (
        SELECT 1
        FROM werk_core.storage_blob_locations AS location
        WHERE location.tenant_id = NEW.tenant_id
          AND location.blob_id = NEW.id
          AND location.state = 'available'
    ) THEN
        RAISE EXCEPTION 'available storage blob requires an available location';
    END IF;
    IF NEW.state = 'missing' AND (
        EXISTS (
            SELECT 1
            FROM werk_core.storage_blob_locations AS location
            WHERE location.tenant_id = NEW.tenant_id
              AND location.blob_id = NEW.id
              AND location.state = 'available'
        )
        OR NOT EXISTS (
            SELECT 1
            FROM werk_core.storage_blob_locations AS location
            WHERE location.tenant_id = NEW.tenant_id
              AND location.blob_id = NEW.id
              AND location.state = 'missing'
        )
    ) THEN
        RAISE EXCEPTION 'missing storage blob requires confirmed missing locations';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_storage_blob_insert()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF NEW.state <> 'quarantined' OR NEW.version <> 1 THEN
        RAISE EXCEPTION 'storage blobs must begin in quarantine';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.protect_storage_location_transition()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
BEGIN
    PERFORM 1
    FROM werk_core.storage_blobs AS blob
    WHERE blob.tenant_id = OLD.tenant_id
      AND blob.id = OLD.blob_id
    FOR UPDATE;

    IF NOT (
        (OLD.state = 'quarantined' AND NEW.state = 'available')
        OR (OLD.state = 'available' AND NEW.state = 'missing')
    ) THEN
        RAISE EXCEPTION 'storage location transition is not allowed';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.blob_id <> OLD.blob_id
       OR NEW.provider_key <> OLD.provider_key OR NEW.opaque_key <> OLD.opaque_key
       OR NEW.created_at <> OLD.created_at OR NEW.version <> OLD.version + 1
       OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'storage location identity or version is immutable';
    END IF;
    IF OLD.state <> 'quarantined' AND (
        NEW.provider_checksum IS DISTINCT FROM OLD.provider_checksum
        OR NEW.activated_at IS DISTINCT FROM OLD.activated_at
    ) THEN
        RAISE EXCEPTION 'activated storage location verification is immutable';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_storage_location_insert()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF NEW.state <> 'quarantined' OR NEW.version <> 1 THEN
        RAISE EXCEPTION 'storage locations must begin in quarantine';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_blob_location_consistency()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE
    affected_tenant_id uuid;
    affected_blob_id uuid;
    blob_state text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_tenant_id := OLD.tenant_id;
        affected_blob_id := OLD.blob_id;
    ELSE
        affected_tenant_id := NEW.tenant_id;
        affected_blob_id := NEW.blob_id;
    END IF;

    SELECT blob.state INTO blob_state
    FROM werk_core.storage_blobs AS blob
    WHERE blob.tenant_id = affected_tenant_id
      AND blob.id = affected_blob_id
    FOR UPDATE;

    IF blob_state = 'available' AND NOT EXISTS (
        SELECT 1
        FROM werk_core.storage_blob_locations AS location
        WHERE location.tenant_id = affected_tenant_id
          AND location.blob_id = affected_blob_id
          AND location.state = 'available'
    ) THEN
        RAISE EXCEPTION 'available storage blob requires an available location';
    END IF;
    IF blob_state = 'missing' AND EXISTS (
        SELECT 1
        FROM werk_core.storage_blob_locations AS location
        WHERE location.tenant_id = affected_tenant_id
          AND location.blob_id = affected_blob_id
          AND location.state = 'available'
    ) THEN
        RAISE EXCEPTION 'missing storage blob cannot retain an available location';
    END IF;
    RETURN NULL;
END
$function$;

CREATE FUNCTION werk_security.protect_document_update()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
BEGIN
    IF OLD.status = 'archived' OR (OLD.status = 'active' AND NEW.status NOT IN ('active', 'archived')) THEN
        RAISE EXCEPTION 'document transition is not allowed';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.source_module <> OLD.source_module
       OR NEW.created_by_account_id <> OLD.created_by_account_id OR NEW.created_at <> OLD.created_at
       OR NEW.version <> OLD.version + 1 OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'document identity or version is immutable';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_document_insert()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF NEW.status <> 'active' OR NEW.version <> 1 OR NEW.updated_at <> NEW.created_at THEN
        RAISE EXCEPTION 'documents must begin active at version one';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_document_version_blob()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM werk_core.documents AS document
        WHERE document.tenant_id = NEW.tenant_id
          AND document.id = NEW.document_id
          AND document.status = 'active'
    ) THEN
        RAISE EXCEPTION 'document versions require an active tenant document';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM werk_core.storage_blobs AS blob
        WHERE blob.tenant_id = NEW.tenant_id
          AND blob.id = NEW.blob_id
          AND blob.state = 'available'
    ) THEN
        RAISE EXCEPTION 'document version requires an available tenant blob';
    END IF;
    IF NEW.version_number <> (
        SELECT COALESCE(max(version.version_number), 0) + 1
        FROM werk_core.document_versions AS version
        WHERE version.tenant_id = NEW.tenant_id
          AND version.document_id = NEW.document_id
    ) THEN
        RAISE EXCEPTION 'document version number must advance monotonically';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_document_classification_revision()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
BEGIN
    IF NEW.revision <> (
        SELECT COALESCE(max(revision.revision), 0) + 1
        FROM werk_core.document_classification_revisions AS revision
        WHERE revision.tenant_id = NEW.tenant_id
          AND revision.document_id = NEW.document_id
    ) THEN
        RAISE EXCEPTION 'document classification revision must advance monotonically';
    END IF;
    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.validate_document_initial_records()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM werk_core.document_versions AS version
        WHERE version.tenant_id = NEW.tenant_id
          AND version.document_id = NEW.id
          AND version.version_number = 1
    ) OR NOT EXISTS (
        SELECT 1
        FROM werk_core.document_classification_revisions AS revision
        WHERE revision.tenant_id = NEW.tenant_id
          AND revision.document_id = NEW.id
          AND revision.revision = 1
    ) THEN
        RAISE EXCEPTION 'document requires initial version and classification in the same transaction';
    END IF;
    RETURN NULL;
END
$function$;

CREATE FUNCTION werk_security.protect_immutable_document_record()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    RAISE EXCEPTION 'published document records are immutable';
END
$function$;

CREATE TRIGGER storage_blobs_protect_transition
    BEFORE UPDATE ON werk_core.storage_blobs
    FOR EACH ROW EXECUTE FUNCTION werk_security.protect_storage_blob_transition();
CREATE TRIGGER storage_blobs_validate_insert
    BEFORE INSERT ON werk_core.storage_blobs
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_storage_blob_insert();
CREATE TRIGGER storage_blob_locations_protect_transition
    BEFORE UPDATE ON werk_core.storage_blob_locations
    FOR EACH ROW EXECUTE FUNCTION werk_security.protect_storage_location_transition();
CREATE TRIGGER storage_blob_locations_validate_insert
    BEFORE INSERT ON werk_core.storage_blob_locations
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_storage_location_insert();
CREATE CONSTRAINT TRIGGER storage_blob_locations_validate_blob_consistency
    AFTER INSERT OR UPDATE OR DELETE ON werk_core.storage_blob_locations
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_blob_location_consistency();
CREATE TRIGGER documents_protect_update
    BEFORE UPDATE ON werk_core.documents
    FOR EACH ROW EXECUTE FUNCTION werk_security.protect_document_update();
CREATE TRIGGER documents_validate_insert
    BEFORE INSERT ON werk_core.documents
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_document_insert();
CREATE CONSTRAINT TRIGGER documents_validate_initial_records
    AFTER INSERT ON werk_core.documents
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_document_initial_records();
CREATE TRIGGER document_versions_validate_blob
    BEFORE INSERT ON werk_core.document_versions
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_document_version_blob();
CREATE TRIGGER document_versions_protect_immutable
    BEFORE UPDATE OR DELETE ON werk_core.document_versions
    FOR EACH ROW EXECUTE FUNCTION werk_security.protect_immutable_document_record();
CREATE TRIGGER document_classification_revisions_protect_immutable
    BEFORE UPDATE OR DELETE ON werk_core.document_classification_revisions
    FOR EACH ROW EXECUTE FUNCTION werk_security.protect_immutable_document_record();
CREATE TRIGGER document_classification_revisions_validate_insert
    BEFORE INSERT ON werk_core.document_classification_revisions
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_document_classification_revision();

REVOKE ALL ON FUNCTION werk_security.protect_storage_blob_transition() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_storage_blob_insert() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_storage_location_transition() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_storage_location_insert() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_blob_location_consistency() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_document_update() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_document_insert() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_document_version_blob() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_document_classification_revision() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.validate_document_initial_records() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_immutable_document_record() FROM PUBLIC;

REVOKE ALL ON
    werk_core.storage_blobs,
    werk_core.storage_blob_locations,
    werk_core.documents,
    werk_core.document_versions,
    werk_core.document_classification_revisions
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON
    werk_core.documents,
    werk_core.document_versions,
    werk_core.document_classification_revisions
TO werk_work_runtime;

GRANT SELECT, INSERT, UPDATE ON werk_core.documents TO werk_service_runtime;
GRANT SELECT, INSERT ON
    werk_core.document_versions,
    werk_core.document_classification_revisions
TO werk_service_runtime;
GRANT SELECT, INSERT, UPDATE ON
    werk_core.storage_blobs,
    werk_core.storage_blob_locations
TO werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON
    werk_core.storage_blobs,
    werk_core.storage_blob_locations,
    werk_core.documents,
    werk_core.document_versions,
    werk_core.document_classification_revisions
TO werk_backup_reader;

ALTER TABLE werk_core.storage_blobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.storage_blobs FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.storage_blob_locations ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.storage_blob_locations FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.documents FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.document_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.document_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.document_classification_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.document_classification_revisions FORCE ROW LEVEL SECURITY;

CREATE POLICY storage_blobs_tenant_gate ON werk_core.storage_blobs
    AS RESTRICTIVE TO werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY storage_blobs_runtime_manage ON werk_core.storage_blobs
    TO werk_service_runtime, werk_worker_runtime USING (true) WITH CHECK (true);
CREATE POLICY storage_blobs_owner_all ON werk_core.storage_blobs
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY storage_blob_locations_tenant_gate ON werk_core.storage_blob_locations
    AS RESTRICTIVE TO werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY storage_blob_locations_runtime_manage ON werk_core.storage_blob_locations
    TO werk_service_runtime, werk_worker_runtime USING (true) WITH CHECK (true);
CREATE POLICY storage_blob_locations_owner_all ON werk_core.storage_blob_locations
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY documents_tenant_gate ON werk_core.documents
    AS RESTRICTIVE TO werk_work_runtime, werk_service_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY documents_work_read ON werk_core.documents
    FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY documents_service_manage ON werk_core.documents
    TO werk_service_runtime USING (true) WITH CHECK (true);
CREATE POLICY documents_owner_all ON werk_core.documents
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY document_versions_tenant_gate ON werk_core.document_versions
    AS RESTRICTIVE TO werk_work_runtime, werk_service_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY document_versions_work_read ON werk_core.document_versions
    FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY document_versions_service_manage ON werk_core.document_versions
    TO werk_service_runtime USING (true) WITH CHECK (true);
CREATE POLICY document_versions_owner_all ON werk_core.document_versions
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY document_classification_revisions_tenant_gate ON werk_core.document_classification_revisions
    AS RESTRICTIVE TO werk_work_runtime, werk_service_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY document_classification_revisions_work_read ON werk_core.document_classification_revisions
    FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY document_classification_revisions_service_manage ON werk_core.document_classification_revisions
    TO werk_service_runtime USING (true) WITH CHECK (true);
CREATE POLICY document_classification_revisions_owner_all ON werk_core.document_classification_revisions
    TO werk_owner USING (true) WITH CHECK (true);

COMMENT ON TABLE werk_core.storage_blobs IS
    'Tenant-bound immutable content metadata. No provider path, transfer token, or client hash is authoritative here.';
COMMENT ON TABLE werk_core.storage_blob_locations IS
    'Opaque physical locations of tenant blobs. provider_key selects an internal adapter; opaque_key contains no business name.';
COMMENT ON TABLE werk_core.documents IS
    'Mutable document metadata owned by Core Documents; source_module records provenance, not data ownership.';
COMMENT ON TABLE werk_core.document_versions IS
    'Published append-only document versions bound to already available tenant blobs.';
COMMENT ON TABLE werk_core.document_classification_revisions IS
    'Append-only classification and retention decisions. Physical deletion is intentionally not part of this foundation.';
COMMENT ON COLUMN werk_core.storage_blobs.sha256 IS
    'Server-verified SHA-256 digest; tenant-local equality never authorizes cross-tenant reuse.';
COMMENT ON COLUMN werk_core.storage_blob_locations.opaque_key IS
    'Random internal object locator. Clients must never select or interpret this value.';
