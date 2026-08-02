ALTER TABLE werk_core.sessions
    ADD COLUMN authentication_assurance text NOT NULL DEFAULT 'single-factor'
        CHECK (authentication_assurance IN ('single-factor', 'multi-factor'));

CREATE TABLE werk_core.identity_mfa_recovery_codes (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    factor_id uuid NOT NULL REFERENCES werk_core.identity_mfa_factors(id) ON DELETE CASCADE,
    code_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at timestamptz NULL,
    CHECK (used_at IS NULL OR used_at >= created_at)
);

CREATE INDEX identity_mfa_recovery_account_idx
    ON werk_core.identity_mfa_recovery_codes (account_id, factor_id)
    WHERE used_at IS NULL;

CREATE UNIQUE INDEX identity_mfa_one_active_kind_idx
    ON werk_core.identity_mfa_factors (account_id, factor_kind)
    WHERE status = 'active';

REVOKE ALL ON werk_core.identity_mfa_recovery_codes
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, UPDATE ON werk_core.identity_mfa_recovery_codes
    TO werk_identity_runtime;
GRANT SELECT ON werk_core.identity_mfa_recovery_codes TO werk_backup_reader;

ALTER TABLE werk_core.identity_mfa_recovery_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_mfa_recovery_codes FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_mfa_recovery_identity_all ON werk_core.identity_mfa_recovery_codes
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY identity_mfa_recovery_owner_all ON werk_core.identity_mfa_recovery_codes
    TO werk_owner USING (true) WITH CHECK (true);
