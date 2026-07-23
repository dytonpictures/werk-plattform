CREATE TABLE werk_core.identity_mfa_factors (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    factor_kind text NOT NULL CHECK (factor_kind IN ('webauthn', 'totp')),
    status text NOT NULL CHECK (status IN ('pending', 'active', 'revoked')),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 120),
    credential_id_hash bytea NULL,
    public_key bytea NULL,
    secret_reference text NULL,
    sign_count bigint NOT NULL DEFAULT 0 CHECK (sign_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    activated_at timestamptz NULL,
    last_used_at timestamptz NULL,
    revoked_at timestamptz NULL,
    CHECK ((factor_kind = 'webauthn' AND credential_id_hash IS NOT NULL AND public_key IS NOT NULL AND secret_reference IS NULL)
        OR (factor_kind = 'totp' AND credential_id_hash IS NULL AND public_key IS NULL AND secret_reference IS NOT NULL)),
    CHECK ((status = 'pending' AND activated_at IS NULL AND revoked_at IS NULL)
        OR (status = 'active' AND activated_at IS NOT NULL AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL))
);

CREATE UNIQUE INDEX identity_mfa_webauthn_credential_idx
    ON werk_core.identity_mfa_factors (credential_id_hash)
    WHERE factor_kind = 'webauthn';
CREATE INDEX identity_mfa_account_idx
    ON werk_core.identity_mfa_factors (account_id, status);

CREATE TABLE werk_core.identity_mfa_challenges (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    factor_id uuid NULL REFERENCES werk_core.identity_mfa_factors(id),
    purpose text NOT NULL CHECK (purpose IN ('enrollment', 'authentication', 'reauthentication')),
    challenge_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at timestamptz NULL,
    CHECK (expires_at > created_at),
    CHECK (used_at IS NULL OR used_at >= created_at)
);

CREATE INDEX identity_mfa_challenge_expiry_idx
    ON werk_core.identity_mfa_challenges (expires_at)
    WHERE used_at IS NULL;

CREATE TABLE werk_core.security_audit_events (
    id uuid PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    event_type text NOT NULL CHECK (event_type ~ '^identity[.][a-z0-9-]+([.][a-z0-9-]+)+[.]v1$'),
    outcome text NOT NULL CHECK (outcome IN ('succeeded', 'denied', 'failed')),
    account_id uuid NULL REFERENCES werk_core.accounts(id),
    session_id uuid NULL REFERENCES werk_core.sessions(id),
    tenant_id uuid NULL,
    request_id uuid NOT NULL,
    correlation_id uuid NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    CHECK (jsonb_typeof(details) = 'object'),
    CHECK (octet_length(details::text) <= 8192)
);

CREATE INDEX security_audit_occurred_idx ON werk_core.security_audit_events (occurred_at DESC);
CREATE INDEX security_audit_account_idx ON werk_core.security_audit_events (account_id, occurred_at DESC)
    WHERE account_id IS NOT NULL;

REVOKE ALL ON werk_core.identity_mfa_factors, werk_core.identity_mfa_challenges, werk_core.security_audit_events
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

GRANT SELECT, INSERT, UPDATE ON werk_core.identity_mfa_factors, werk_core.identity_mfa_challenges
    TO werk_identity_runtime;
GRANT INSERT ON werk_core.security_audit_events TO werk_identity_runtime;
GRANT SELECT ON werk_core.security_audit_events TO werk_admin_runtime;
GRANT SELECT ON werk_core.identity_mfa_factors, werk_core.identity_mfa_challenges, werk_core.security_audit_events
    TO werk_backup_reader;

ALTER TABLE werk_core.identity_mfa_factors ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_mfa_factors FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_mfa_challenges ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_mfa_challenges FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.security_audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.security_audit_events FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_mfa_factors_identity_all ON werk_core.identity_mfa_factors
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY identity_mfa_factors_owner_all ON werk_core.identity_mfa_factors
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_mfa_challenges_identity_all ON werk_core.identity_mfa_challenges
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY identity_mfa_challenges_owner_all ON werk_core.identity_mfa_challenges
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY security_audit_identity_insert ON werk_core.security_audit_events
    FOR INSERT TO werk_identity_runtime WITH CHECK (true);
CREATE POLICY security_audit_admin_read ON werk_core.security_audit_events
    FOR SELECT TO werk_admin_runtime USING (true);
CREATE POLICY security_audit_owner_all ON werk_core.security_audit_events
    TO werk_owner USING (true) WITH CHECK (true);
