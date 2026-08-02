CREATE TABLE werk_core.identity_reauthentication_tickets (
    id uuid PRIMARY KEY,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id),
    session_id uuid NOT NULL REFERENCES werk_core.sessions(id),
    session_generation bigint NOT NULL CHECK (session_generation >= 1),
    permission_key text NOT NULL CHECK (length(btrim(permission_key)) BETWEEN 1 AND 160),
    resource_kind text NOT NULL CHECK (length(btrim(resource_kind)) BETWEEN 1 AND 160),
    resource_id text NOT NULL CHECK (length(btrim(resource_id)) BETWEEN 1 AND 255),
    authentication_method text NOT NULL CHECK (authentication_method = 'password'),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz NULL,
    CHECK (expires_at > created_at),
    CHECK (expires_at <= created_at + interval '5 minutes'),
    CHECK (used_at IS NULL OR used_at >= created_at)
);

CREATE INDEX identity_reauthentication_ticket_expiry_idx
    ON werk_core.identity_reauthentication_tickets (expires_at)
    WHERE used_at IS NULL;

REVOKE ALL ON werk_core.identity_reauthentication_tickets
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON werk_core.identity_reauthentication_tickets
    TO werk_identity_runtime;
GRANT SELECT ON werk_core.identity_reauthentication_tickets TO werk_backup_reader;

ALTER TABLE werk_core.identity_reauthentication_tickets ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_reauthentication_tickets FORCE ROW LEVEL SECURITY;
CREATE POLICY identity_reauthentication_tickets_identity_all
    ON werk_core.identity_reauthentication_tickets TO werk_identity_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY identity_reauthentication_tickets_owner_all
    ON werk_core.identity_reauthentication_tickets TO werk_owner
    USING (true) WITH CHECK (true);
