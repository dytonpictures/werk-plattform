-- Complete the WebAuthn runtime contract without changing the previously
-- applied MFA foundation. Credential and ceremony payloads are encrypted by
-- the identity adapter; only the identifiers required for lookup remain in
-- clear text.

ALTER TABLE werk_core.identity_mfa_factors
    ADD COLUMN credential_id bytea NULL,
    ADD COLUMN credential_reference text NULL,
    ADD COLUMN relying_party_id text NULL;

ALTER TABLE werk_core.identity_mfa_factors
    ADD CONSTRAINT identity_mfa_webauthn_runtime_shape CHECK (
        (factor_kind = 'webauthn'
            AND credential_id IS NOT NULL
            AND octet_length(credential_id) BETWEEN 1 AND 1023
            AND credential_reference IS NOT NULL
            AND relying_party_id IS NOT NULL
            AND length(btrim(relying_party_id)) BETWEEN 1 AND 253)
        OR
        (factor_kind = 'totp'
            AND credential_id IS NULL
            AND credential_reference IS NULL
            AND relying_party_id IS NULL)
    ) NOT VALID;

ALTER TABLE werk_core.identity_mfa_factors
    VALIDATE CONSTRAINT identity_mfa_webauthn_runtime_shape;

ALTER TABLE werk_core.identity_mfa_challenges
    ADD COLUMN ceremony_token_hash bytea NULL,
    ADD COLUMN ceremony_reference text NULL,
    ADD COLUMN relying_party_id text NULL;

ALTER TABLE werk_core.identity_mfa_challenges
    ADD CONSTRAINT identity_mfa_webauthn_ceremony_shape CHECK (
        (ceremony_token_hash IS NULL AND ceremony_reference IS NULL AND relying_party_id IS NULL)
        OR
        (ceremony_token_hash IS NOT NULL
            AND octet_length(ceremony_token_hash) = 32
            AND ceremony_reference IS NOT NULL
            AND relying_party_id IS NOT NULL
            AND length(btrim(relying_party_id)) BETWEEN 1 AND 253)
    );

CREATE UNIQUE INDEX identity_mfa_ceremony_token_idx
    ON werk_core.identity_mfa_challenges (ceremony_token_hash)
    WHERE ceremony_token_hash IS NOT NULL;

-- TOTP remains a single fallback factor. WebAuthn intentionally supports
-- multiple devices/passkeys per account.
DROP INDEX IF EXISTS werk_core.identity_mfa_one_active_kind_idx;
CREATE UNIQUE INDEX identity_mfa_one_active_totp_idx
    ON werk_core.identity_mfa_factors (account_id)
    WHERE status = 'active' AND factor_kind = 'totp';
