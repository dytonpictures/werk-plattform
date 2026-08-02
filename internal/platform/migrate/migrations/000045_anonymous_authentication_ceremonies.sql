-- Anonymous discoverable-passkey ceremonies deliberately remain separate
-- from account-bound MFA challenges. Before assertion verification there is no
-- trusted principal or authorization context.
CREATE TABLE werk_core.identity_authentication_ceremonies (
    id uuid PRIMARY KEY,
    ceremony_kind text NOT NULL CHECK (ceremony_kind IN ('passkey-login')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    challenge_hash bytea NOT NULL UNIQUE CHECK (octet_length(challenge_hash) = 32),
    session_reference text NOT NULL CHECK (length(session_reference) BETWEEN 32 AND 65536),
    relying_party_id text NOT NULL CHECK (length(btrim(relying_party_id)) BETWEEN 1 AND 253),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz NULL,
    CHECK (expires_at > created_at),
    CHECK (expires_at <= created_at + interval '10 minutes'),
    CHECK (used_at IS NULL OR used_at >= created_at)
);

CREATE INDEX identity_authentication_ceremony_expiry_idx
    ON werk_core.identity_authentication_ceremonies (expires_at)
    WHERE used_at IS NULL;

REVOKE ALL ON werk_core.identity_authentication_ceremonies
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON werk_core.identity_authentication_ceremonies
    TO werk_identity_runtime;
GRANT SELECT ON werk_core.identity_authentication_ceremonies TO werk_backup_reader;

ALTER TABLE werk_core.identity_authentication_ceremonies ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_authentication_ceremonies FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_authentication_ceremonies_identity_all
    ON werk_core.identity_authentication_ceremonies
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY identity_authentication_ceremonies_owner_all
    ON werk_core.identity_authentication_ceremonies
    TO werk_owner USING (true) WITH CHECK (true);
