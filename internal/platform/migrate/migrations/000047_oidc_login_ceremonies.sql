-- OIDC state exists before WERK has authenticated a principal. It therefore
-- remains account- and tenant-neutral and is accessible only to Identity.
CREATE TABLE werk_core.identity_oidc_login_ceremonies (
    state_digest bytea PRIMARY KEY CHECK (octet_length(state_digest) = 32),
    registry_provider_id uuid NOT NULL,
    redirect_uri text NOT NULL CHECK (length(btrim(redirect_uri)) BETWEEN 1 AND 2048),
    encrypted_payload text NOT NULL CHECK (length(encrypted_payload) BETWEEN 32 AND 65536),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > created_at),
    CHECK (expires_at <= created_at + interval '5 minutes')
);

CREATE INDEX identity_oidc_login_ceremony_expiry_idx
    ON werk_core.identity_oidc_login_ceremonies (expires_at);

REVOKE ALL ON werk_core.identity_oidc_login_ceremonies
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, DELETE ON werk_core.identity_oidc_login_ceremonies
    TO werk_identity_runtime;
GRANT SELECT ON werk_core.identity_oidc_login_ceremonies TO werk_backup_reader;

ALTER TABLE werk_core.identity_oidc_login_ceremonies ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_login_ceremonies FORCE ROW LEVEL SECURITY;
CREATE POLICY identity_oidc_login_ceremonies_identity_all
    ON werk_core.identity_oidc_login_ceremonies TO werk_identity_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_login_ceremonies_owner_all
    ON werk_core.identity_oidc_login_ceremonies TO werk_owner
    USING (true) WITH CHECK (true);
