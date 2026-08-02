ALTER TABLE werk_core.identity_mfa_challenges
    ADD COLUMN failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5);

ALTER TABLE werk_core.identity_mfa_factors
    ADD COLUMN failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5);

CREATE TABLE werk_core.identity_auth_throttles (
    subject_hash bytea PRIMARY KEY,
    failure_count integer NOT NULL CHECK (failure_count > 0),
    window_started_at timestamptz NOT NULL,
    locked_until timestamptz NULL,
    updated_at timestamptz NOT NULL,
    CHECK (locked_until IS NULL OR locked_until >= window_started_at)
);

CREATE INDEX identity_auth_throttles_expiry_idx
    ON werk_core.identity_auth_throttles (locked_until)
    WHERE locked_until IS NOT NULL;

REVOKE ALL ON werk_core.identity_auth_throttles
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON werk_core.identity_auth_throttles
    TO werk_identity_runtime;
GRANT SELECT ON werk_core.identity_auth_throttles TO werk_backup_reader;

ALTER TABLE werk_core.identity_auth_throttles ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_auth_throttles FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_auth_throttles_identity_all ON werk_core.identity_auth_throttles
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY identity_auth_throttles_owner_all ON werk_core.identity_auth_throttles
    TO werk_owner USING (true) WITH CHECK (true);
