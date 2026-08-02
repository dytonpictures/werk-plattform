-- Kafka readiness is a bounded, expiring worker observation. It carries no
-- broker, topic, ACL, credential, error or process coordinates and does not
-- replace PostgreSQL queue state as the durable delivery truth.

ALTER TABLE werk_core.platform_process_heartbeats
    ADD COLUMN kafka_state text NOT NULL DEFAULT 'unknown'
        CHECK (kafka_state IN ('unknown', 'disabled', 'ready', 'degraded')),
    ADD COLUMN kafka_observed_at timestamptz;

ALTER TABLE werk_core.platform_process_heartbeats
    ADD CONSTRAINT platform_process_heartbeats_kafka_observation_check
    CHECK (
        (kafka_state = 'unknown' AND kafka_observed_at IS NULL)
        OR
        (kafka_state IN ('disabled', 'ready', 'degraded')
         AND kafka_observed_at IS NOT NULL
         AND kafka_observed_at >= started_at
         AND kafka_observed_at <= last_seen_at)
    );

CREATE FUNCTION werk_security.beat_worker_heartbeat(
    candidate_run_id uuid,
    candidate_build_version text,
    candidate_ttl_seconds integer,
    candidate_retention_seconds integer,
    candidate_kafka_state text
)
RETURNS SETOF integer
LANGUAGE plpgsql
VOLATILE
STRICT
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    observed_at timestamptz := pg_catalog.clock_timestamp();
    affected_rows integer;
BEGIN
    IF candidate_ttl_seconds < 5 OR candidate_ttl_seconds > 600 THEN
        RAISE EXCEPTION 'worker heartbeat TTL is outside the allowed range'
            USING ERRCODE = '22023';
    END IF;
    IF candidate_retention_seconds < candidate_ttl_seconds
       OR candidate_retention_seconds > 2592000 THEN
        RAISE EXCEPTION 'worker heartbeat retention is outside the allowed range'
            USING ERRCODE = '22023';
    END IF;
    IF candidate_kafka_state NOT IN ('disabled', 'ready', 'degraded') THEN
        RAISE EXCEPTION 'worker Kafka observation is invalid'
            USING ERRCODE = '22023';
    END IF;

    DELETE FROM werk_core.platform_process_heartbeats
    WHERE component = 'worker'
      AND expires_at < observed_at
          - pg_catalog.make_interval(secs => candidate_retention_seconds);

    INSERT INTO werk_core.platform_process_heartbeats (
        run_id, component, build_version, started_at, last_seen_at, expires_at,
        kafka_state, kafka_observed_at
    ) VALUES (
        candidate_run_id,
        'worker',
        candidate_build_version,
        observed_at,
        observed_at,
        observed_at + pg_catalog.make_interval(secs => candidate_ttl_seconds),
        candidate_kafka_state,
        observed_at
    )
    ON CONFLICT (run_id) DO UPDATE
    SET last_seen_at = observed_at,
        expires_at = observed_at
            + pg_catalog.make_interval(secs => candidate_ttl_seconds),
        kafka_state = candidate_kafka_state,
        kafka_observed_at = observed_at
    WHERE platform_process_heartbeats.component = 'worker'
      AND platform_process_heartbeats.build_version = EXCLUDED.build_version;

    GET DIAGNOSTICS affected_rows = ROW_COUNT;
    IF affected_rows <> 1 THEN
        RAISE EXCEPTION 'worker heartbeat identity changed'
            USING ERRCODE = '40001';
    END IF;

    RETURN NEXT 1;
END
$function$;

REVOKE ALL ON FUNCTION
    werk_security.beat_worker_heartbeat(uuid, text, integer, integer, text)
FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    werk_security.beat_worker_heartbeat(uuid, text, integer, integer, text)
TO werk_worker_runtime;

COMMENT ON FUNCTION
    werk_security.beat_worker_heartbeat(uuid, text, integer, integer, text)
IS 'Records worker liveness and one minimized Kafka state using only PostgreSQL time; no external coordinates or errors are persisted.';

CREATE OR REPLACE VIEW werk_core.platform_operations_summary
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
    ) AS latest_migration_applied_at,
    CASE
        WHEN EXISTS (
            SELECT 1 FROM werk_core.platform_process_heartbeats AS heartbeat
            WHERE heartbeat.component = 'worker'
              AND heartbeat.expires_at > statement_timestamp()
              AND heartbeat.kafka_state = 'ready'
        ) THEN 'ready'
        WHEN EXISTS (
            SELECT 1 FROM werk_core.platform_process_heartbeats AS heartbeat
            WHERE heartbeat.component = 'worker'
              AND heartbeat.expires_at > statement_timestamp()
              AND heartbeat.kafka_state = 'degraded'
        ) THEN 'degraded'
        WHEN EXISTS (
            SELECT 1 FROM werk_core.platform_process_heartbeats AS heartbeat
            WHERE heartbeat.component = 'worker'
              AND heartbeat.expires_at > statement_timestamp()
              AND heartbeat.kafka_state = 'disabled'
        ) THEN 'disabled'
        ELSE 'unknown'
    END AS kafka_state,
    (
        SELECT max(heartbeat.kafka_observed_at)
        FROM werk_core.platform_process_heartbeats AS heartbeat
        WHERE heartbeat.component = 'worker'
          AND heartbeat.expires_at > statement_timestamp()
    ) AS kafka_observed_at;

COMMENT ON VIEW werk_core.platform_operations_summary IS
    'Sanitized installation-wide operations projection including an expiring minimized Kafka observation; no external coordinates or errors are exposed.';
