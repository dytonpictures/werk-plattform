CREATE TABLE werk_core.outbox_events (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id),
    event_type text NOT NULL CHECK (event_type ~ '^[a-z][a-z0-9-]*([.][a-z][a-z0-9-]*)+[.]v[1-9][0-9]*$'),
    producer text NOT NULL CHECK (producer ~ '^[a-z][a-z0-9._-]{0,127}$'),
    subject_kind text NOT NULL CHECK (subject_kind ~ '^[a-z][a-z0-9._-]{0,127}$'),
    subject_id uuid NOT NULL,
    partition_key text NOT NULL CHECK (length(btrim(partition_key)) BETWEEN 1 AND 200),
    occurred_at timestamptz NOT NULL,
    correlation_id uuid NOT NULL,
    causation_id uuid NULL,
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 1048576),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'retry', 'completed', 'dead')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 8 CHECK (max_attempts BETWEEN 1 AND 100),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text NULL,
    lease_expires_at timestamptz NULL,
    last_error text NULL CHECK (last_error IS NULL OR length(last_error) <= 2000),
    completed_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'processing' AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
        OR (status <> 'processing' AND lease_owner IS NULL AND lease_expires_at IS NULL)),
    CHECK ((status = 'completed' AND completed_at IS NOT NULL)
        OR (status <> 'completed' AND completed_at IS NULL)),
    UNIQUE (tenant_id, id)
);

CREATE INDEX outbox_events_claim_idx
    ON werk_core.outbox_events (available_at, occurred_at, id)
    WHERE status IN ('pending', 'retry', 'processing');
CREATE INDEX outbox_events_partition_idx
    ON werk_core.outbox_events (tenant_id, partition_key, occurred_at);
CREATE INDEX outbox_events_dead_idx
    ON werk_core.outbox_events (tenant_id, occurred_at DESC)
    WHERE status = 'dead';

CREATE TABLE werk_core.event_consumer_receipts (
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id),
    event_id uuid NOT NULL,
    consumer_key text NOT NULL CHECK (consumer_key ~ '^[a-z][a-z0-9._-]{0,127}$'),
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, consumer_key),
    FOREIGN KEY (tenant_id, event_id)
        REFERENCES werk_core.outbox_events (tenant_id, id) ON DELETE CASCADE
);

REVOKE ALL ON werk_core.outbox_events, werk_core.event_consumer_receipts
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;
GRANT INSERT ON werk_core.outbox_events TO werk_work_runtime, werk_service_runtime;
GRANT SELECT, UPDATE ON werk_core.outbox_events TO werk_worker_runtime;
GRANT SELECT, INSERT ON werk_core.event_consumer_receipts TO werk_worker_runtime;
GRANT SELECT ON werk_core.outbox_events, werk_core.event_consumer_receipts TO werk_backup_reader;

ALTER TABLE werk_core.outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.outbox_events FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.event_consumer_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.event_consumer_receipts FORCE ROW LEVEL SECURITY;

CREATE POLICY outbox_events_tenant_gate ON werk_core.outbox_events AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY outbox_events_work_insert ON werk_core.outbox_events
    FOR INSERT TO werk_work_runtime WITH CHECK (true);
CREATE POLICY outbox_events_service_insert ON werk_core.outbox_events
    FOR INSERT TO werk_service_runtime WITH CHECK (true);
CREATE POLICY outbox_events_worker_all ON werk_core.outbox_events
    TO werk_worker_runtime USING (true) WITH CHECK (true);
CREATE POLICY outbox_events_owner_all ON werk_core.outbox_events
    TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY event_consumer_receipts_worker_all ON werk_core.event_consumer_receipts
    TO werk_worker_runtime USING (true) WITH CHECK (true);
CREATE POLICY event_consumer_receipts_owner_all ON werk_core.event_consumer_receipts
    TO werk_owner USING (true) WITH CHECK (true);
