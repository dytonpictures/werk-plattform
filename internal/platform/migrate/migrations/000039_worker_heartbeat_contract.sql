-- Worker liveness remains an ephemeral observation, but the runtime must not
-- receive direct access to its backing table. PostgreSQL owns all timestamps
-- and validates the bounded expiry/pruning contract at the privilege boundary.

ALTER TABLE werk_core.platform_process_heartbeats
    ADD CONSTRAINT platform_process_heartbeats_maximum_ttl_check
        CHECK (expires_at <= last_seen_at + interval '10 minutes');

REVOKE ALL ON werk_core.platform_process_heartbeats
    FROM werk_worker_runtime;

DROP POLICY platform_process_heartbeats_worker_all
    ON werk_core.platform_process_heartbeats;

CREATE FUNCTION werk_security.beat_worker_heartbeat(
    candidate_run_id uuid,
    candidate_build_version text,
    candidate_ttl_seconds integer,
    candidate_retention_seconds integer
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

    DELETE FROM werk_core.platform_process_heartbeats
    WHERE component = 'worker'
      AND expires_at < observed_at
          - pg_catalog.make_interval(secs => candidate_retention_seconds);

    INSERT INTO werk_core.platform_process_heartbeats (
        run_id, component, build_version, started_at, last_seen_at, expires_at
    ) VALUES (
        candidate_run_id,
        'worker',
        candidate_build_version,
        observed_at,
        observed_at,
        observed_at + pg_catalog.make_interval(secs => candidate_ttl_seconds)
    )
    ON CONFLICT (run_id) DO UPDATE
    SET last_seen_at = observed_at,
        expires_at = observed_at
            + pg_catalog.make_interval(secs => candidate_ttl_seconds)
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

CREATE FUNCTION werk_security.remove_worker_heartbeat(candidate_run_id uuid)
RETURNS SETOF integer
LANGUAGE plpgsql
VOLATILE
STRICT
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    affected_rows integer;
BEGIN
    DELETE FROM werk_core.platform_process_heartbeats
    WHERE run_id = candidate_run_id
      AND component = 'worker';

    GET DIAGNOSTICS affected_rows = ROW_COUNT;
    RETURN NEXT affected_rows;
END
$function$;

REVOKE ALL ON FUNCTION
    werk_security.beat_worker_heartbeat(uuid, text, integer, integer)
FROM PUBLIC;
REVOKE ALL ON FUNCTION
    werk_security.remove_worker_heartbeat(uuid)
FROM PUBLIC;

GRANT EXECUTE ON FUNCTION
    werk_security.beat_worker_heartbeat(uuid, text, integer, integer)
TO werk_worker_runtime;
GRANT EXECUTE ON FUNCTION
    werk_security.remove_worker_heartbeat(uuid)
TO werk_worker_runtime;

COMMENT ON FUNCTION
    werk_security.beat_worker_heartbeat(uuid, text, integer, integer)
IS 'Prunes stale worker observations and records one run using only PostgreSQL time; TTL and retention are bounded server-side.';
COMMENT ON FUNCTION werk_security.remove_worker_heartbeat(uuid)
IS 'Withdraws one random worker run observation without exposing the heartbeat table to the worker runtime.';
