-- The column grant in migration 49 excludes all authenticator material. RLS
-- additionally limits the support read model to work accounts in the explicit
-- tenant context established by AdminDB.WithinTenantRead.
CREATE POLICY identity_mfa_factors_admin_work_metadata_read
    ON werk_core.identity_mfa_factors
    FOR SELECT TO werk_admin_runtime
    USING (EXISTS (
        SELECT 1
        FROM werk_core.accounts AS account
        WHERE account.id = identity_mfa_factors.account_id
          AND account.account_class = 'work'
          AND account.tenant_id = werk_security.current_tenant_id()
    ));

