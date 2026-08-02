-- Administrative Core observation is deliberately split from host control.
-- This migration exposes bounded, non-secret read models and a worker
-- heartbeat, but grants no shell, container, service-manager or update access.

INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES
    (
        '0196f000-0000-7000-8000-000000000502',
        'core.identity.work-account.update',
        'Arbeitskontostatus ändern',
        'core.identity',
        'admin',
        'critical'
    ),
    (
        '0196f000-0000-7000-8000-000000001001',
        'core.platform.operations.read',
        'Plattformbetrieb anzeigen',
        'core.platform',
        'admin',
        'high'
    ),
    (
        '0196f000-0000-7000-8000-000000001002',
        'core.identity.provider.read',
        'Anmeldeprovider anzeigen',
        'core.identity',
        'admin',
        'high'
    );

INSERT INTO werk_core.role_permissions (role_id, permission_id)
VALUES
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000000502'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000001001'),
    ('0196f000-0000-7000-8000-000000000111', '0196f000-0000-7000-8000-000000001002');

INSERT INTO werk_core.permission_resource_types (permission_id, resource_kind)
VALUES
    ('0196f000-0000-7000-8000-000000000502', 'core.identity.work-account'),
    ('0196f000-0000-7000-8000-000000001001', 'core.platform.installation'),
    ('0196f000-0000-7000-8000-000000001002', 'core.platform.installation');

INSERT INTO werk_core.permission_processing_policies (
    permission_id, resource_kind, processing_required,
    activity_key, purpose_key, legal_basis_ref
) VALUES
    (
        '0196f000-0000-7000-8000-000000000502',
        'core.identity.work-account',
        true,
        'core.identity.work-account-administration',
        'core.identity.access-management',
        'operator.processing-register.identity-access'
    ),
    (
        '0196f000-0000-7000-8000-000000001001',
        'core.platform.installation',
        false, NULL, NULL, NULL
    ),
    (
        '0196f000-0000-7000-8000-000000001002',
        'core.platform.installation',
        false, NULL, NULL, NULL
    );

-- Admins may change only the lifecycle columns of tenant-bound work accounts.
-- RLS supplies the explicit tenant boundary and this trigger supplies the
-- allowed state machine plus optimistic-concurrency/session invalidation.
GRANT UPDATE (status, updated_at, version, session_generation)
    ON werk_core.accounts TO werk_admin_runtime;

CREATE FUNCTION werk_security.protect_admin_work_account_status_update()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF current_user <> 'werk_admin_runtime' THEN
        RETURN NEW;
    END IF;

    IF OLD.account_class <> 'work'
       OR OLD.status NOT IN ('active', 'disabled')
       OR NEW.status NOT IN ('active', 'disabled')
       OR NEW.status = OLD.status THEN
        RAISE EXCEPTION 'admin work account status transition is not allowed'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.version <> OLD.version + 1
       OR NEW.session_generation <> OLD.session_generation + 1
       OR NEW.updated_at <= OLD.updated_at THEN
        RAISE EXCEPTION 'admin work account status update contract violated'
            USING ERRCODE = '40001';
    END IF;

    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.protect_admin_work_account_status_update()
    FROM PUBLIC;

CREATE TRIGGER accounts_protect_admin_status_update
BEFORE UPDATE OF status, updated_at, version, session_generation
ON werk_core.accounts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_admin_work_account_status_update();

-- The worker reports only a logical component, build version and bounded
-- timestamps. Hostnames, process IDs, addresses and credentials are excluded.
CREATE TABLE werk_core.platform_process_heartbeats (
    run_id uuid PRIMARY KEY,
    component text NOT NULL CHECK (component = 'worker'),
    build_version text NOT NULL CHECK (
        build_version = btrim(build_version)
        AND char_length(build_version) BETWEEN 1 AND 120
    ),
    started_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    CHECK (last_seen_at >= started_at),
    CHECK (expires_at > last_seen_at)
);

CREATE INDEX platform_process_heartbeats_component_seen_idx
    ON werk_core.platform_process_heartbeats (component, last_seen_at DESC);
CREATE INDEX platform_process_heartbeats_expiry_idx
    ON werk_core.platform_process_heartbeats (expires_at);

REVOKE ALL ON werk_core.platform_process_heartbeats
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON werk_core.platform_process_heartbeats
    TO werk_worker_runtime;
GRANT SELECT ON werk_core.platform_process_heartbeats TO werk_backup_reader;

ALTER TABLE werk_core.platform_process_heartbeats ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.platform_process_heartbeats FORCE ROW LEVEL SECURITY;

CREATE POLICY platform_process_heartbeats_worker_all
    ON werk_core.platform_process_heartbeats
    TO werk_worker_runtime
    USING (component = 'worker')
    WITH CHECK (component = 'worker');
CREATE POLICY platform_process_heartbeats_owner_all
    ON werk_core.platform_process_heartbeats
    TO werk_owner USING (true) WITH CHECK (true);

-- Installation-wide operations are exposed as one bounded aggregate row. The
-- admin runtime receives no direct access to queues, heartbeats or migrations.
CREATE VIEW werk_core.platform_operations_summary
WITH (security_barrier = true)
AS
SELECT
    statement_timestamp() AS observed_at,
    (
        SELECT heartbeat.build_version
        FROM werk_core.platform_process_heartbeats AS heartbeat
        WHERE heartbeat.component = 'worker'
        ORDER BY heartbeat.last_seen_at DESC, heartbeat.run_id DESC
        LIMIT 1
    ) AS worker_build_version,
    (
        SELECT heartbeat.started_at
        FROM werk_core.platform_process_heartbeats AS heartbeat
        WHERE heartbeat.component = 'worker'
        ORDER BY heartbeat.last_seen_at DESC, heartbeat.run_id DESC
        LIMIT 1
    ) AS worker_started_at,
    (
        SELECT max(heartbeat.last_seen_at)
        FROM werk_core.platform_process_heartbeats AS heartbeat
        WHERE heartbeat.component = 'worker'
    ) AS worker_last_seen_at,
    (
        SELECT heartbeat.expires_at
        FROM werk_core.platform_process_heartbeats AS heartbeat
        WHERE heartbeat.component = 'worker'
        ORDER BY heartbeat.last_seen_at DESC, heartbeat.run_id DESC
        LIMIT 1
    ) AS worker_expires_at,
    (SELECT count(*) FROM werk_core.outbox_events WHERE status = 'pending') AS outbox_pending,
    (SELECT count(*) FROM werk_core.outbox_events WHERE status = 'processing') AS outbox_processing,
    (SELECT count(*) FROM werk_core.outbox_events WHERE status = 'retry') AS outbox_retry,
    (SELECT count(*) FROM werk_core.outbox_events WHERE status = 'dead') AS outbox_dead,
    (
        SELECT min(event.available_at)
        FROM werk_core.outbox_events AS event
        WHERE event.status IN ('pending', 'processing', 'retry')
    ) AS outbox_oldest_outstanding_at,
    (SELECT count(*) FROM werk_core.security_audit_export_queue WHERE status = 'pending') AS audit_pending,
    (SELECT count(*) FROM werk_core.security_audit_export_queue WHERE status = 'processing') AS audit_processing,
    (SELECT count(*) FROM werk_core.security_audit_export_queue WHERE status = 'retry') AS audit_retry,
    (SELECT count(*) FROM werk_core.security_audit_export_queue WHERE status = 'dead') AS audit_dead,
    (
        SELECT min(export.created_at)
        FROM werk_core.security_audit_export_queue AS export
        WHERE export.status IN ('pending', 'processing', 'retry')
    ) AS audit_oldest_outstanding_at,
    (SELECT count(*) FROM werk_core.schema_migrations) AS migration_count,
    (
        SELECT migration.name
        FROM werk_core.schema_migrations AS migration
        ORDER BY migration.applied_at DESC, migration.name DESC
        LIMIT 1
    ) AS latest_migration_name,
    (
        SELECT migration.applied_at
        FROM werk_core.schema_migrations AS migration
        ORDER BY migration.applied_at DESC, migration.name DESC
        LIMIT 1
    ) AS latest_migration_applied_at;

REVOKE ALL ON werk_core.platform_operations_summary
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.platform_operations_summary TO werk_admin_runtime;
GRANT SELECT ON werk_core.platform_operations_summary TO werk_backup_reader;

-- Identity provider metadata is readable, never writable, from the admin
-- plane. Provider credentials and secure material are not stored here.
GRANT SELECT ON werk_core.identity_providers TO werk_admin_runtime;
CREATE POLICY identity_providers_admin_read ON werk_core.identity_providers
    FOR SELECT TO werk_admin_runtime USING (true);

-- Each installation-level observation records its own access. This policy is
-- intentionally exact and cannot be reused for unrelated global audit writes.
CREATE POLICY security_audit_admin_core_observation
    ON werk_core.security_audit_events
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (
        werk_security.current_tenant_id() IS NULL
        AND tenant_id IS NULL
        AND event_type IN (
            'core.platform.operations-summary-read.v1',
            'core.identity.providers-listed.v1'
        )
        AND outcome = 'succeeded'
        AND account_id IS NOT NULL
    );

COMMENT ON TABLE werk_core.platform_process_heartbeats IS
    'Ephemeral process liveness observation; PostgreSQL remains the source read by the admin operations summary.';
COMMENT ON VIEW werk_core.platform_operations_summary IS
    'Sanitized installation-wide operations projection. It deliberately exposes no execution capability or sensitive payload data.';
