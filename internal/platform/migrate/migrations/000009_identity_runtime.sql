GRANT USAGE ON SCHEMA werk_core TO werk_identity_runtime;

GRANT SELECT, INSERT, UPDATE ON werk_core.admin_subjects TO werk_identity_runtime;
GRANT SELECT, INSERT, UPDATE ON werk_core.accounts TO werk_identity_runtime;
GRANT SELECT, INSERT, UPDATE ON werk_core.account_credentials TO werk_identity_runtime;
GRANT SELECT, INSERT, UPDATE ON werk_core.sessions TO werk_identity_runtime;
GRANT SELECT, UPDATE ON werk_core.identity_bootstrap TO werk_identity_runtime;

ALTER TABLE werk_core.admin_subjects ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.admin_subjects FORCE ROW LEVEL SECURITY;

CREATE POLICY admin_subjects_admin_all
    ON werk_core.admin_subjects TO werk_admin_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY admin_subjects_identity_all
    ON werk_core.admin_subjects TO werk_identity_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY admin_subjects_owner_all
    ON werk_core.admin_subjects TO werk_owner
    USING (true) WITH CHECK (true);
CREATE POLICY accounts_identity_all
    ON werk_core.accounts TO werk_identity_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY credentials_identity_all
    ON werk_core.account_credentials TO werk_identity_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY sessions_identity_all
    ON werk_core.sessions TO werk_identity_runtime
    USING (true) WITH CHECK (true);
CREATE POLICY identity_bootstrap_identity_all
    ON werk_core.identity_bootstrap TO werk_identity_runtime
    USING (true) WITH CHECK (true);
