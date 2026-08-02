-- The public operations contract accepts build identifiers up to 128
-- characters. Align the heartbeat persistence boundary with that established
-- Core limit instead of failing only when the worker writes its first beat.
ALTER TABLE werk_core.platform_process_heartbeats
    DROP CONSTRAINT platform_process_heartbeats_build_version_check,
    ADD CONSTRAINT platform_process_heartbeats_build_version_check
        CHECK (
            build_version = btrim(build_version)
            AND char_length(build_version) BETWEEN 1 AND 128
        );
