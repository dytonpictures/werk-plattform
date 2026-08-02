-- Durable events receive a bounded, queryable context contract. Existing and
-- direct SQL producers inherit conservative defaults; application producers
-- may only replace them with an equally explicit classification.
ALTER TABLE werk_core.outbox_events
    ADD COLUMN tags jsonb NOT NULL DEFAULT jsonb_build_object(
        'data.classification', 'restricted',
        'processing.purpose', 'platform-event-delivery',
        'retention.class', 'domain-event'
    );

ALTER TABLE werk_core.outbox_events
    ADD CONSTRAINT outbox_events_tags_object_check
        CHECK (jsonb_typeof(tags) = 'object'),
    ADD CONSTRAINT outbox_events_tags_size_check
        CHECK (octet_length(tags::text) <= 8192),
    ADD CONSTRAINT outbox_events_required_tags_check
        CHECK (
            tags ? 'data.classification'
            AND tags ? 'processing.purpose'
            AND tags ? 'retention.class'
            AND jsonb_typeof(tags -> 'data.classification') = 'string'
            AND jsonb_typeof(tags -> 'processing.purpose') = 'string'
            AND jsonb_typeof(tags -> 'retention.class') = 'string'
            AND tags ->> 'data.classification' IN ('public', 'internal', 'confidential', 'restricted')
        );

CREATE FUNCTION werk_security.valid_event_tags(candidate jsonb)
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
STRICT
SET search_path = pg_catalog
AS $function$
DECLARE
    entry record;
    text_value text;
    entry_count integer := 0;
BEGIN
    IF jsonb_typeof(candidate) <> 'object' THEN
        RETURN false;
    END IF;
    FOR entry IN SELECT key, value FROM jsonb_each(candidate)
    LOOP
        entry_count := entry_count + 1;
        IF entry_count > 32 OR entry.key !~ '^[a-z][a-z0-9._-]{0,63}$'
           OR jsonb_typeof(entry.value) <> 'string' THEN
            RETURN false;
        END IF;
        text_value := entry.value #>> '{}';
        IF text_value !~ '^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,127}$' THEN
            RETURN false;
        END IF;
    END LOOP;
    RETURN entry_count >= 3;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.valid_event_tags(jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION werk_security.valid_event_tags(jsonb)
    TO werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

ALTER TABLE werk_core.outbox_events
    ADD CONSTRAINT outbox_events_tag_contract_check
        CHECK (werk_security.valid_event_tags(tags));

-- Audit export is deliberately separate from the audit record. The audit row
-- remains authoritative and the queue only tracks delivery of its minimized
-- projection to an operator-controlled stream.
CREATE TABLE werk_core.security_audit_export_queue (
    audit_event_id uuid PRIMARY KEY
        REFERENCES werk_core.security_audit_events(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'retry', 'completed', 'dead')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 12 CHECK (max_attempts BETWEEN 1 AND 100),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text NULL,
    lease_expires_at timestamptz NULL,
    last_error text NULL CHECK (last_error IS NULL OR length(last_error) <= 2000),
    completed_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'processing' AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
        OR (status <> 'processing' AND lease_owner IS NULL AND lease_expires_at IS NULL)),
    CHECK ((status = 'completed' AND completed_at IS NOT NULL)
        OR (status <> 'completed' AND completed_at IS NULL))
);

CREATE INDEX security_audit_export_claim_idx
    ON werk_core.security_audit_export_queue (available_at, audit_event_id)
    WHERE status IN ('pending', 'retry', 'processing');
CREATE INDEX security_audit_export_dead_idx
    ON werk_core.security_audit_export_queue (created_at DESC)
    WHERE status = 'dead';

CREATE FUNCTION werk_security.enqueue_security_audit_export()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
BEGIN
    INSERT INTO werk_core.security_audit_export_queue (audit_event_id)
    VALUES (NEW.id);
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.enqueue_security_audit_export() FROM PUBLIC;

CREATE TRIGGER security_audit_enqueue_export
    AFTER INSERT ON werk_core.security_audit_events
    FOR EACH ROW EXECUTE FUNCTION werk_security.enqueue_security_audit_export();

-- An upgrade makes the existing audit history available to the same export
-- contract. Stable audit IDs let downstream consumers deduplicate a replay.
INSERT INTO werk_core.security_audit_export_queue (audit_event_id)
SELECT audit.id
FROM werk_core.security_audit_events AS audit
ON CONFLICT (audit_event_id) DO NOTHING;

REVOKE ALL ON werk_core.security_audit_export_queue
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT, UPDATE ON werk_core.security_audit_export_queue TO werk_worker_runtime;
GRANT SELECT ON werk_core.security_audit_events TO werk_worker_runtime;
GRANT SELECT ON werk_core.security_audit_export_queue TO werk_backup_reader;

ALTER TABLE werk_core.security_audit_export_queue ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.security_audit_export_queue FORCE ROW LEVEL SECURITY;

CREATE POLICY security_audit_export_worker_all ON werk_core.security_audit_export_queue
    TO werk_worker_runtime USING (true) WITH CHECK (true);
CREATE POLICY security_audit_export_owner_all ON werk_core.security_audit_export_queue
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY security_audit_worker_read ON werk_core.security_audit_events
    FOR SELECT TO werk_worker_runtime USING (true);
