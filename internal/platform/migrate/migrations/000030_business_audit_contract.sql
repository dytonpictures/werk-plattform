-- Extend the existing authoritative audit log instead of creating a second
-- Documents-specific audit world. The table name remains for compatibility;
-- the structured columns are the Core business-audit contract.
ALTER TABLE werk_core.security_audit_events
    ADD COLUMN initiated_by_account_id uuid NULL,
    ADD COLUMN executed_by_account_id uuid NULL,
    ADD COLUMN action_key text NULL,
    ADD COLUMN subject_boundary text NULL,
    ADD COLUMN subject_tenant_id uuid NULL,
    ADD COLUMN subject_kind text NULL,
    ADD COLUMN subject_id text NULL,
    ADD COLUMN permission_key text NULL,
    ADD COLUMN policy_contract_version bigint NULL,
    ADD COLUMN processing_required boolean NULL,
    ADD COLUMN processing_activity_key text NULL,
    ADD COLUMN processing_purpose_key text NULL,
    ADD COLUMN legal_basis_ref text NULL;

ALTER TABLE werk_core.security_audit_events
    ADD CONSTRAINT security_audit_tenant_fk
        FOREIGN KEY (tenant_id)
        REFERENCES werk_core.tenants(id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT security_audit_initiator_tenant_fk
        FOREIGN KEY (tenant_id, initiated_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT security_audit_executor_tenant_fk
        FOREIGN KEY (tenant_id, executed_by_account_id)
        REFERENCES werk_core.accounts(tenant_id, id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT security_audit_subject_tenant_fk
        FOREIGN KEY (subject_tenant_id)
        REFERENCES werk_core.tenants(id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT security_audit_subject_kind_fk
        FOREIGN KEY (subject_kind)
        REFERENCES werk_core.resource_type_registrations(resource_kind)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD CONSTRAINT security_audit_permission_key_fk
        FOREIGN KEY (permission_key)
        REFERENCES werk_core.permissions(permission_key)
        ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE werk_core.security_audit_events
    ADD CONSTRAINT security_audit_business_shape_check CHECK (
        (
            initiated_by_account_id IS NULL
            AND executed_by_account_id IS NULL
            AND action_key IS NULL
            AND subject_boundary IS NULL
            AND subject_tenant_id IS NULL
            AND subject_kind IS NULL
            AND subject_id IS NULL
            AND permission_key IS NULL
            AND policy_contract_version IS NULL
            AND processing_required IS NULL
            AND processing_activity_key IS NULL
            AND processing_purpose_key IS NULL
            AND legal_basis_ref IS NULL
        )
        OR
        (
            tenant_id IS NOT NULL
            AND account_id = initiated_by_account_id
            AND initiated_by_account_id IS NOT NULL
            AND executed_by_account_id IS NOT NULL
            AND action_key ~ '^[a-z][a-z0-9.-]+$'
            AND char_length(action_key) <= 160
            AND subject_boundary = 'tenant'
            AND subject_tenant_id = tenant_id
            AND subject_kind ~ '^[a-z][a-z0-9.-]+$'
            AND subject_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$'
            AND permission_key IS NOT NULL
            AND policy_contract_version > 0
            AND processing_required IS NOT NULL
            AND (
                (
                    processing_required
                    AND processing_activity_key ~ '^[a-z][a-z0-9.-]+$'
                    AND processing_purpose_key ~ '^[a-z][a-z0-9.-]+$'
                    AND legal_basis_ref ~ '^[a-z][a-z0-9.-]+$'
                )
                OR
                (
                    NOT processing_required
                    AND processing_activity_key IS NULL
                    AND processing_purpose_key IS NULL
                    AND legal_basis_ref IS NULL
                )
            )
        )
    );

-- Each business-audit fact has one versioned action contract. Changing the
-- permission or resource meaning requires a new event/action version instead
-- of reinterpreting already written audit history.
CREATE TABLE werk_core.audit_action_contracts (
    event_type text NOT NULL CHECK (
        event_type ~ '^[a-z][a-z0-9-]*([.][a-z][a-z0-9-]*)+[.]v[1-9][0-9]*$'
        AND char_length(event_type) <= 256
    ),
    action_key text NOT NULL CHECK (
        action_key ~ '^[a-z][a-z0-9.-]+$'
        AND char_length(action_key) <= 160
    ),
    permission_id uuid NOT NULL,
    resource_kind text NOT NULL,
    contract_version bigint NOT NULL DEFAULT 1 CHECK (contract_version > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_type, action_key),
    FOREIGN KEY (permission_id, resource_kind)
        REFERENCES werk_core.permission_resource_types(permission_id, resource_kind)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

INSERT INTO werk_core.audit_action_contracts (
    event_type, action_key, permission_id, resource_kind
)
SELECT
    'core.documents.document-published.v1',
    'core.documents.document.publish',
    permission.id,
    'core.documents.collection'
FROM werk_core.permissions AS permission
WHERE permission.permission_key = 'core.documents.document.create';

REVOKE ALL ON werk_core.audit_action_contracts
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
     werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.audit_action_contracts TO werk_backup_reader;

ALTER TABLE werk_core.audit_action_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.audit_action_contracts FORCE ROW LEVEL SECURITY;

CREATE POLICY audit_action_contracts_owner_all ON werk_core.audit_action_contracts
    TO werk_owner USING (true) WITH CHECK (true);

CREATE FUNCTION werk_security.protect_audit_action_contract()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF TG_OP = 'UPDATE'
       AND OLD.status = 'active'
       AND NEW.status = 'retired'
       AND NEW.event_type = OLD.event_type
       AND NEW.action_key = OLD.action_key
       AND NEW.permission_id = OLD.permission_id
       AND NEW.resource_kind = OLD.resource_kind
       AND NEW.contract_version = OLD.contract_version
       AND NEW.created_at = OLD.created_at THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'audit action contract meaning is immutable';
END
$function$;

CREATE TRIGGER audit_action_contracts_protect_meaning
    BEFORE UPDATE OR DELETE ON werk_core.audit_action_contracts
    FOR EACH ROW EXECUTE FUNCTION werk_security.protect_audit_action_contract();

REVOKE ALL ON FUNCTION werk_security.protect_audit_action_contract() FROM PUBLIC;

CREATE FUNCTION werk_security.validate_business_audit_entry()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    initiator_class text;
    initiator_status text;
    executor_class text;
    executor_status text;
    expected_processing_required boolean;
    profile_processing_required boolean;
    expected_activity_key text;
    expected_purpose_key text;
    expected_legal_basis_ref text;
    expected_contract_version bigint;
BEGIN
    IF NEW.action_key IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT account.account_class, account.status
      INTO initiator_class, initiator_status
      FROM werk_core.accounts AS account
     WHERE account.tenant_id = NEW.tenant_id
       AND account.id = NEW.initiated_by_account_id;
    SELECT account.account_class, account.status
      INTO executor_class, executor_status
      FROM werk_core.accounts AS account
     WHERE account.tenant_id = NEW.tenant_id
       AND account.id = NEW.executed_by_account_id;

    IF initiator_class IS NULL OR initiator_status <> 'active'
       OR initiator_class NOT IN ('work', 'service', 'agent') THEN
        RAISE EXCEPTION 'business audit initiator is not an active tenant actor';
    END IF;
    IF executor_class IS NULL OR executor_status <> 'active'
       OR executor_class NOT IN ('service', 'agent') THEN
        RAISE EXCEPTION 'business audit executor is not an active service actor';
    END IF;

    SELECT policy.processing_required, profile.processing_activity_required,
           policy.activity_key, policy.purpose_key,
           policy.legal_basis_ref, policy.contract_version
      INTO expected_processing_required, profile_processing_required,
           expected_activity_key,
           expected_purpose_key, expected_legal_basis_ref,
           expected_contract_version
      FROM werk_core.audit_action_contracts AS action_contract
      JOIN werk_core.permissions AS permission
        ON permission.id = action_contract.permission_id
      JOIN werk_core.permission_processing_policies AS policy
        ON policy.permission_id = permission.id
       AND policy.resource_kind = action_contract.resource_kind
      JOIN werk_core.resource_data_profiles AS profile
        ON profile.resource_kind = policy.resource_kind
      JOIN werk_core.resource_type_registrations AS registration
        ON registration.resource_kind = policy.resource_kind
     WHERE action_contract.event_type = NEW.event_type
       AND action_contract.action_key = NEW.action_key
       AND action_contract.resource_kind = NEW.subject_kind
       AND action_contract.status = 'active'
       AND permission.permission_key = NEW.permission_key
       AND permission.status = 'active'
       AND policy.status = 'active'
       AND profile.status = 'active'
       AND registration.status = 'active'
       AND registration.boundary = NEW.subject_boundary;

    IF expected_processing_required IS NULL
       OR (profile_processing_required AND NOT expected_processing_required)
       OR NEW.processing_required IS DISTINCT FROM expected_processing_required
       OR NEW.processing_activity_key IS DISTINCT FROM expected_activity_key
       OR NEW.processing_purpose_key IS DISTINCT FROM expected_purpose_key
       OR NEW.legal_basis_ref IS DISTINCT FROM expected_legal_basis_ref
       OR NEW.policy_contract_version IS DISTINCT FROM expected_contract_version THEN
        RAISE EXCEPTION 'business audit policy snapshot does not match active server policy';
    END IF;

    RETURN NEW;
END
$function$;

CREATE FUNCTION werk_security.protect_audit_entry()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    RAISE EXCEPTION 'audit entries are append-only';
END
$function$;

CREATE TRIGGER security_audit_validate_business_entry
    BEFORE INSERT ON werk_core.security_audit_events
    FOR EACH ROW EXECUTE FUNCTION werk_security.validate_business_audit_entry();
CREATE TRIGGER security_audit_protect_immutable
    BEFORE UPDATE OR DELETE ON werk_core.security_audit_events
    FOR EACH ROW EXECUTE FUNCTION werk_security.protect_audit_entry();

REVOKE ALL ON FUNCTION werk_security.validate_business_audit_entry() FROM PUBLIC;
REVOKE ALL ON FUNCTION werk_security.protect_audit_entry() FROM PUBLIC;

REVOKE UPDATE, DELETE ON werk_core.security_audit_events
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
     werk_service_runtime, werk_worker_runtime;
GRANT INSERT ON werk_core.security_audit_events TO werk_service_runtime;

-- The historical identity policy was intentionally broad while the table only
-- accepted identity-shaped columns. Keep that producer on its legacy contract
-- now that structured business columns exist.
DROP POLICY security_audit_identity_insert ON werk_core.security_audit_events;
CREATE POLICY security_audit_identity_insert
    ON werk_core.security_audit_events
    FOR INSERT TO werk_identity_runtime
    WITH CHECK (
        event_type LIKE 'identity.%'
        AND action_key IS NULL
    );

DROP POLICY security_audit_admin_insert ON werk_core.security_audit_events;
CREATE POLICY security_audit_admin_insert
    ON werk_core.security_audit_events
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (
        tenant_id = werk_security.current_tenant_id()
        AND action_key IS NULL
    );

CREATE POLICY security_audit_service_business_insert
    ON werk_core.security_audit_events
    FOR INSERT TO werk_service_runtime
    WITH CHECK (
        tenant_id = werk_security.current_tenant_id()
        AND subject_tenant_id = werk_security.current_tenant_id()
        AND initiated_by_account_id IS NOT NULL
        AND executed_by_account_id IS NOT NULL
        AND action_key IS NOT NULL
        AND (
            (event_type LIKE 'core.documents.%' AND action_key LIKE 'core.documents.%')
            OR (event_type LIKE 'core.storage.%' AND action_key LIKE 'core.storage.%')
        )
    );

CREATE INDEX security_audit_business_subject_idx
    ON werk_core.security_audit_events (
        tenant_id, subject_kind, subject_id, occurred_at DESC, id DESC
    )
    WHERE action_key IS NOT NULL;
CREATE INDEX security_audit_business_action_idx
    ON werk_core.security_audit_events (
        tenant_id, action_key, outcome, occurred_at DESC, id DESC
    )
    WHERE action_key IS NOT NULL;

COMMENT ON TABLE werk_core.security_audit_events IS
    'Authoritative append-only Core audit log. Legacy identity rows use account_id; business rows additionally preserve both actors, subject, action, and the server policy snapshot.';
COMMENT ON COLUMN werk_core.security_audit_events.initiated_by_account_id IS
    'Authenticated tenant actor that caused the protected operation; never inferred from request body data.';
COMMENT ON COLUMN werk_core.security_audit_events.executed_by_account_id IS
    'Authenticated service or agent principal that executed the operation; no implicit user impersonation.';
COMMENT ON COLUMN werk_core.security_audit_events.subject_id IS
    'Canonical resource identifier only. Titles, object paths, transfer tickets, hashes, and credentials are forbidden.';
COMMENT ON COLUMN werk_core.security_audit_events.legal_basis_ref IS
    'Server-resolved reference copied from the active processing policy; not a legal conclusion and never client supplied.';
COMMENT ON TABLE werk_core.audit_action_contracts IS
    'Versioned server-side mapping from an audit event/action to exactly one permission and resource kind. Meaning is immutable; only active to retired is allowed.';
