INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES (
    '0196f000-0000-7000-8000-000000000907',
    'core.documents.document.visibility-manage',
    'Direkte Dokumentsichtbarkeit verwalten',
    'core.documents',
    'work',
    'high'
);

INSERT INTO werk_core.permission_resource_types (permission_id, resource_kind)
SELECT permission.id, 'core.documents.document'
FROM werk_core.permissions AS permission
WHERE permission.permission_key = 'core.documents.document.visibility-manage';

INSERT INTO werk_core.permission_processing_policies (
    permission_id, resource_kind, processing_required,
    activity_key, purpose_key, legal_basis_ref
)
SELECT permission.id, 'core.documents.document', true,
       'core.documents.document-management',
       'core.documents.document-sharing',
       'operator.processing-register.documents'
FROM werk_core.permissions AS permission
WHERE permission.permission_key = 'core.documents.document.visibility-manage';

UPDATE werk_core.permissions
SET display_name = 'Sichtbare Dokumente auflisten'
WHERE permission_key = 'core.documents.document.list';

INSERT INTO werk_core.audit_action_contracts (
    event_type, action_key, permission_id, resource_kind
)
SELECT contract.event_type, contract.action_key, permission.id, 'core.documents.document'
FROM (VALUES
    ('core.documents.document-visibility-granted.v1', 'core.documents.document.visibility-grant'),
    ('core.documents.document-visibility-revoked.v1', 'core.documents.document.visibility-revoke')
) AS contract(event_type, action_key)
JOIN werk_core.permissions AS permission
  ON permission.permission_key = 'core.documents.document.visibility-manage';

CREATE TABLE werk_core.document_account_visibility_bindings (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id) ON DELETE RESTRICT,
    document_id uuid NOT NULL,
    grantee_account_id uuid NOT NULL,
    granted_by_account_id uuid NOT NULL,
    granted_at timestamptz NOT NULL,
    revoked_by_account_id uuid NULL,
    revoked_at timestamptz NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, document_id)
        REFERENCES werk_core.documents(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, grantee_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, granted_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, revoked_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK (grantee_account_id <> granted_by_account_id),
    CHECK (
        (revoked_at IS NULL AND revoked_by_account_id IS NULL AND version = 1)
        OR
        (revoked_at IS NOT NULL AND revoked_by_account_id IS NOT NULL
            AND revoked_at >= granted_at AND version >= 2)
    )
);

CREATE UNIQUE INDEX document_account_visibility_active_idx
    ON werk_core.document_account_visibility_bindings
       (tenant_id, document_id, grantee_account_id)
    WHERE revoked_at IS NULL;

CREATE INDEX document_account_visibility_grantee_idx
    ON werk_core.document_account_visibility_bindings
       (tenant_id, grantee_account_id, document_id)
    WHERE revoked_at IS NULL;

CREATE FUNCTION werk_security.validate_document_account_visibility_binding()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    document_creator uuid;
    document_status text;
    document_created_at timestamptz;
    grantee_class text;
    grantee_status text;
    actor_class text;
    actor_status text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'document visibility bindings cannot be deleted';
    END IF;

    SELECT document.created_by_account_id, document.status, document.created_at
      INTO document_creator, document_status, document_created_at
      FROM werk_core.documents AS document
     WHERE document.tenant_id = NEW.tenant_id
       AND document.id = NEW.document_id;

    IF document_creator IS NULL THEN
        RAISE EXCEPTION 'document visibility binding document is not available in tenant';
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.revoked_at IS NOT NULL OR NEW.revoked_by_account_id IS NOT NULL OR NEW.version <> 1 THEN
            RAISE EXCEPTION 'document visibility binding must start active';
        END IF;
        IF document_status <> 'active' OR NEW.granted_at < document_created_at THEN
            RAISE EXCEPTION 'document visibility binding requires an active published document';
        END IF;
        IF NEW.granted_by_account_id <> document_creator OR NEW.grantee_account_id = document_creator THEN
            RAISE EXCEPTION 'only the document creator may grant direct visibility to another account';
        END IF;

        SELECT account.account_class, account.status
          INTO grantee_class, grantee_status
          FROM werk_core.accounts AS account
         WHERE account.tenant_id = NEW.tenant_id
           AND account.id = NEW.grantee_account_id;
        SELECT account.account_class, account.status
          INTO actor_class, actor_status
          FROM werk_core.accounts AS account
         WHERE account.tenant_id = NEW.tenant_id
           AND account.id = NEW.granted_by_account_id;

        IF grantee_class IS DISTINCT FROM 'work' OR grantee_status IS DISTINCT FROM 'active'
           OR actor_class IS DISTINCT FROM 'work' OR actor_status IS DISTINCT FROM 'active' THEN
            RAISE EXCEPTION 'document visibility binding requires active work accounts';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
       OR NEW.document_id IS DISTINCT FROM OLD.document_id
       OR NEW.grantee_account_id IS DISTINCT FROM OLD.grantee_account_id
       OR NEW.granted_by_account_id IS DISTINCT FROM OLD.granted_by_account_id
       OR NEW.granted_at IS DISTINCT FROM OLD.granted_at THEN
        RAISE EXCEPTION 'document visibility binding identity is immutable';
    END IF;
    IF OLD.revoked_at IS NOT NULL OR NEW.revoked_at IS NULL
       OR NEW.revoked_by_account_id IS NULL OR NEW.version <> OLD.version + 1 THEN
        RAISE EXCEPTION 'document visibility binding may only be revoked once';
    END IF;
    IF NEW.revoked_by_account_id <> document_creator THEN
        RAISE EXCEPTION 'only the document creator may revoke direct visibility';
    END IF;

    SELECT account.account_class, account.status
      INTO actor_class, actor_status
      FROM werk_core.accounts AS account
     WHERE account.tenant_id = NEW.tenant_id
       AND account.id = NEW.revoked_by_account_id;
    IF actor_class IS DISTINCT FROM 'work' OR actor_status IS DISTINCT FROM 'active' THEN
        RAISE EXCEPTION 'document visibility revocation requires an active work account';
    END IF;
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.validate_document_account_visibility_binding() FROM PUBLIC;

CREATE TRIGGER document_account_visibility_bindings_validate
    BEFORE INSERT OR UPDATE OR DELETE ON werk_core.document_account_visibility_bindings
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_document_account_visibility_binding();

REVOKE ALL ON werk_core.document_account_visibility_bindings
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.document_account_visibility_bindings
    TO werk_work_runtime, werk_service_runtime, werk_backup_reader;
GRANT INSERT, UPDATE ON werk_core.document_account_visibility_bindings
    TO werk_service_runtime;

ALTER TABLE werk_core.document_account_visibility_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.document_account_visibility_bindings FORCE ROW LEVEL SECURITY;

CREATE POLICY document_account_visibility_tenant_gate
    ON werk_core.document_account_visibility_bindings
    AS RESTRICTIVE TO werk_work_runtime, werk_service_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY document_account_visibility_work_read
    ON werk_core.document_account_visibility_bindings
    FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY document_account_visibility_service_manage
    ON werk_core.document_account_visibility_bindings
    TO werk_service_runtime USING (true) WITH CHECK (true);
CREATE POLICY document_account_visibility_owner_all
    ON werk_core.document_account_visibility_bindings
    TO werk_owner USING (true) WITH CHECK (true);

COMMENT ON TABLE werk_core.document_account_visibility_bindings IS
    'Document-owned direct Work-account visibility. A binding grants no platform permission, storage access, update, version, or download capability.';

COMMENT ON COLUMN werk_core.documents.created_by_account_id IS
    'Document creator and V1 visibility owner. Work reads include this account plus active direct bindings owned by Core Documents.';
