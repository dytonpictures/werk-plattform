ALTER TABLE werk_core.persons
    ADD CONSTRAINT persons_tenant_party_unique UNIQUE (tenant_id, party_id);

CREATE TABLE werk_core.accounts (
    id uuid PRIMARY KEY,
    account_class text NOT NULL CHECK (account_class IN ('work', 'admin', 'service')),
    tenant_id uuid NULL,
    person_party_id uuid NULL,
    admin_subject_id uuid NULL REFERENCES werk_core.admin_subjects(id),
    login_name text NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'disabled', 'locked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (login_name),
    CHECK ((account_class = 'work' AND tenant_id IS NOT NULL AND person_party_id IS NOT NULL AND admin_subject_id IS NULL)
        OR (account_class = 'admin' AND tenant_id IS NULL AND person_party_id IS NULL AND admin_subject_id IS NOT NULL)
        OR (account_class = 'service' AND person_party_id IS NULL AND admin_subject_id IS NULL))
);

ALTER TABLE werk_core.accounts
    ADD CONSTRAINT accounts_work_person_fk
    FOREIGN KEY (tenant_id, person_party_id)
    REFERENCES werk_core.persons (tenant_id, party_id);

CREATE UNIQUE INDEX accounts_work_person_idx
    ON werk_core.accounts (tenant_id, person_party_id)
    WHERE account_class = 'work';

CREATE TABLE werk_core.account_credentials (
    account_id uuid PRIMARY KEY REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    credential_kind text NOT NULL CHECK (credential_kind IN ('password', 'service-token')),
    secret_hash bytea NOT NULL,
    assurance text NOT NULL CHECK (assurance IN ('unknown', 'single-factor', 'multi-factor')),
    changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE werk_core.sessions (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    audience text NOT NULL CHECK (audience IN ('werk-work', 'werk-admin', 'werk-service')),
    tenant_id uuid NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz NULL,
    CHECK (expires_at > created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX sessions_account_idx ON werk_core.sessions (account_id, expires_at);

REVOKE ALL ON werk_core.accounts, werk_core.account_credentials, werk_core.sessions
    FROM PUBLIC, werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.accounts, werk_core.sessions
    TO werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;

ALTER TABLE werk_core.accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.account_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.account_credentials FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.sessions FORCE ROW LEVEL SECURITY;

CREATE POLICY accounts_tenant_gate ON werk_core.accounts AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY accounts_work_read ON werk_core.accounts FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY accounts_admin_all ON werk_core.accounts TO werk_admin_runtime USING (true) WITH CHECK (true);
CREATE POLICY accounts_owner_all ON werk_core.accounts TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY credentials_admin_all ON werk_core.account_credentials TO werk_admin_runtime USING (true) WITH CHECK (true);
CREATE POLICY credentials_owner_all ON werk_core.account_credentials TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY sessions_tenant_gate ON werk_core.sessions AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY sessions_admin_all ON werk_core.sessions TO werk_admin_runtime USING (true) WITH CHECK (true);
CREATE POLICY sessions_owner_all ON werk_core.sessions TO werk_owner USING (true) WITH CHECK (true);
