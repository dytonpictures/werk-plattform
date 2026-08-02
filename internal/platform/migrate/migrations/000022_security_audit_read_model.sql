-- The audit timeline is an installation-level administration contract. Reads
-- are separately authorized and record their own access without acquiring a
-- tenant context. The policy intentionally permits no other global mutation.
INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES (
    '0196f000-0000-7000-8000-000000000801',
    'core.audit.security-event.read',
    'Sicherheitsereignisse anzeigen',
    'core.audit',
    'admin',
    'high'
);

INSERT INTO werk_core.role_permissions (role_id, permission_id)
VALUES (
    '0196f000-0000-7000-8000-000000000111',
    '0196f000-0000-7000-8000-000000000801'
);

ALTER TABLE werk_core.security_audit_events
    ADD CONSTRAINT security_audit_events_event_type_length_check
    CHECK (char_length(event_type) <= 256);

-- The timeline needs to distinguish work, admin and service actors without
-- granting installation-wide SELECT on the accounts table. This projection
-- exposes exactly one non-secret classification for an already known UUID.
CREATE FUNCTION werk_security.security_audit_account_class(candidate_account_id uuid)
RETURNS text
LANGUAGE sql
STABLE
STRICT
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
    SELECT account.account_class
    FROM werk_core.accounts AS account
    WHERE account.id = candidate_account_id
$function$;

REVOKE ALL ON FUNCTION werk_security.security_audit_account_class(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION werk_security.security_audit_account_class(uuid)
    TO werk_admin_runtime;

CREATE POLICY security_audit_admin_installation_read_audit
    ON werk_core.security_audit_events
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (
        werk_security.current_tenant_id() IS NULL
        AND tenant_id IS NULL
        AND event_type = 'core.audit.security-events-listed.v1'
        AND outcome = 'succeeded'
        AND account_id IS NOT NULL
    );

CREATE INDEX security_audit_timeline_idx
    ON werk_core.security_audit_events (occurred_at DESC, id DESC);
CREATE INDEX security_audit_tenant_timeline_idx
    ON werk_core.security_audit_events (tenant_id, occurred_at DESC, id DESC)
    WHERE tenant_id IS NOT NULL;
