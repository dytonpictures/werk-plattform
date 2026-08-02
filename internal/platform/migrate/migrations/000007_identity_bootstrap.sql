-- Singleton state for the first-admin bootstrap. The application must consume
-- this row and create the admin account in the same PostgreSQL transaction.
CREATE TABLE werk_core.identity_bootstrap (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    consumed_at timestamptz NULL,
    consumed_account_id uuid NULL REFERENCES werk_core.accounts(id),
    CHECK ((consumed_at IS NULL AND consumed_account_id IS NULL)
        OR (consumed_at IS NOT NULL AND consumed_account_id IS NOT NULL))
);

INSERT INTO werk_core.identity_bootstrap (id) VALUES (true);

REVOKE ALL ON werk_core.identity_bootstrap
    FROM PUBLIC, werk_work_runtime, werk_service_runtime, werk_worker_runtime;
GRANT SELECT, UPDATE ON werk_core.identity_bootstrap TO werk_admin_runtime;

ALTER TABLE werk_core.identity_bootstrap ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_bootstrap FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_bootstrap_admin_all
    ON werk_core.identity_bootstrap TO werk_admin_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY identity_bootstrap_owner_all
    ON werk_core.identity_bootstrap TO werk_owner
    USING (true) WITH CHECK (true);
