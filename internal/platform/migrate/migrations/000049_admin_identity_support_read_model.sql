-- Administrators may inspect lifecycle metadata needed for identity support,
-- but never MFA secrets, credential identifiers, public keys, or challenges.
GRANT SELECT (
    id, account_id, factor_kind, status, display_name,
    created_at, activated_at, last_used_at, revoked_at
) ON werk_core.identity_mfa_factors TO werk_admin_runtime;

