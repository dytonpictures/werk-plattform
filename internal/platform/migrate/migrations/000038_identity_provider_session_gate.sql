-- Session issuance and assurance elevation must be linearized with provider
-- and account-binding lifecycle changes. Identity runtime deliberately has no
-- UPDATE privilege on the provider registry, so this narrowly scoped function
-- locks only the exact active rows needed by an already authenticated flow.

ALTER TABLE werk_core.identity_providers
    ADD CONSTRAINT identity_providers_key_length_check
        CHECK (char_length(provider_key) <= 120),
    ADD CONSTRAINT identity_providers_issuer_length_check
        CHECK (
            issuer IS NULL
            OR (issuer = btrim(issuer) AND char_length(issuer) BETWEEN 1 AND 2048)
        );

CREATE UNIQUE INDEX account_identity_bindings_one_active_local_idx
    ON werk_core.account_identity_bindings (account_id, provider_key)
    WHERE status = 'active' AND provider_key = 'local';

-- Password-derived MFA ceremonies remember the exact credential that passed
-- the first factor. A credential replacement therefore invalidates the
-- pending ceremony instead of silently changing its provider identity.
ALTER TABLE werk_core.identity_mfa_challenges
    ADD COLUMN credential_id uuid NULL
        REFERENCES werk_core.account_credentials(id) ON DELETE CASCADE;

CREATE INDEX identity_mfa_challenges_credential_idx
    ON werk_core.identity_mfa_challenges (credential_id)
    WHERE credential_id IS NOT NULL AND used_at IS NULL;

DROP POLICY account_identity_bindings_admin_work_insert
    ON werk_core.account_identity_bindings;
CREATE POLICY account_identity_bindings_admin_work_insert
    ON werk_core.account_identity_bindings
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (
        provider_key = 'local'
        AND provider_subject = account_id::text
        AND EXISTS (
            SELECT 1
            FROM werk_core.accounts AS account
            WHERE account.id = account_id
              AND account.account_class = 'work'
              AND account.tenant_id = werk_security.current_tenant_id()
        )
    );

CREATE FUNCTION werk_security.lock_active_identity_provider_binding(
    candidate_account_id uuid,
    candidate_provider_key text
)
RETURNS TABLE (provider_kind text)
LANGUAGE sql
VOLATILE
STRICT
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
    SELECT provider.provider_kind
    FROM werk_core.identity_providers AS provider
    JOIN werk_core.account_identity_bindings AS binding
      ON binding.provider_key = provider.provider_key
    WHERE provider.provider_key = candidate_provider_key
      AND provider.status = 'active'
      AND binding.account_id = candidate_account_id
      AND binding.status = 'active'
      AND (
          provider.provider_key <> 'local'
          OR binding.provider_subject = candidate_account_id::text
      )
    ORDER BY binding.id
    LIMIT 1
    FOR SHARE OF provider, binding
$function$;

REVOKE ALL ON FUNCTION
    werk_security.lock_active_identity_provider_binding(uuid, text)
FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    werk_security.lock_active_identity_provider_binding(uuid, text)
TO werk_identity_runtime;

COMMENT ON FUNCTION
    werk_security.lock_active_identity_provider_binding(uuid, text)
IS 'Fail-closed identity gate that holds the selected provider and account-binding lifecycle rows until the caller transaction ends.';

-- External adapters resolve the exact subject supplied by their verified
-- proof. This separate contract never substitutes another active subject of
-- the same provider/account pair.
CREATE FUNCTION werk_security.lock_verified_identity_binding(
    candidate_provider_key text,
    candidate_provider_subject text
)
RETURNS TABLE (
    binding_id uuid,
    account_id uuid,
    provider_kind text
)
LANGUAGE sql
VOLATILE
STRICT
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
    SELECT binding.id, binding.account_id, provider.provider_kind
    FROM werk_core.identity_providers AS provider
    JOIN werk_core.account_identity_bindings AS binding
      ON binding.provider_key = provider.provider_key
    WHERE provider.provider_key = candidate_provider_key
      AND provider.status = 'active'
      AND binding.provider_subject = candidate_provider_subject
      AND binding.status = 'active'
    FOR SHARE OF provider, binding
$function$;

REVOKE ALL ON FUNCTION
    werk_security.lock_verified_identity_binding(text, text)
FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    werk_security.lock_verified_identity_binding(text, text)
TO werk_identity_runtime;

COMMENT ON FUNCTION
    werk_security.lock_verified_identity_binding(text, text)
IS 'Locks one exact active external identity subject and its provider for a verified-proof resolution transaction.';
