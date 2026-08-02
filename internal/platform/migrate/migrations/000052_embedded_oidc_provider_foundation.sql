-- OIDC Provider state is authoritative in PostgreSQL. Private signing keys and
-- pairwise-subject salts remain provider-owned secure material referenced only
-- by opaque handle and version.
CREATE TABLE werk_core.identity_oidc_issuers (
    issuer_key text PRIMARY KEY CHECK (issuer_key ~ '^[a-z][a-z0-9.-]{0,119}$'),
    issuer_uri text NOT NULL UNIQUE CHECK (issuer_uri ~ '^https://[^?#]+$'),
    status text NOT NULL DEFAULT 'disabled' CHECK (status IN ('disabled', 'active', 'retired')),
    pairwise_subject_material_handle text NOT NULL CHECK (length(btrim(pairwise_subject_material_handle)) BETWEEN 1 AND 512),
    pairwise_subject_material_version bigint NOT NULL CHECK (pairwise_subject_material_version > 0),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE werk_core.identity_oidc_signing_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer_key text NOT NULL REFERENCES werk_core.identity_oidc_issuers(issuer_key) ON DELETE RESTRICT,
    key_id text NOT NULL CHECK (length(btrim(key_id)) BETWEEN 8 AND 160),
    algorithm text NOT NULL CHECK (algorithm IN ('RS256', 'ES256', 'EdDSA')),
    material_handle text NOT NULL CHECK (length(btrim(material_handle)) BETWEEN 1 AND 512),
    material_version bigint NOT NULL CHECK (material_version > 0),
    public_jwk jsonb NOT NULL CHECK (jsonb_typeof(public_jwk) = 'object' AND octet_length(public_jwk::text) <= 16384),
    status text NOT NULL DEFAULT 'staged' CHECK (status IN ('staged', 'active', 'retiring', 'retired')),
    not_before timestamptz NOT NULL,
    not_after timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer_key, key_id),
    CHECK (not_after > not_before)
);

CREATE TABLE werk_core.identity_oidc_clients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer_key text NOT NULL REFERENCES werk_core.identity_oidc_issuers(issuer_key) ON DELETE RESTRICT,
    client_id text NOT NULL CHECK (length(btrim(client_id)) BETWEEN 8 AND 255),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    client_type text NOT NULL CHECK (client_type IN ('public', 'confidential')),
    allowed_account_class text NOT NULL CHECK (allowed_account_class IN ('work', 'admin')),
    token_endpoint_auth_method text NOT NULL CHECK (
        (client_type = 'public' AND token_endpoint_auth_method = 'none') OR
        (client_type = 'confidential' AND token_endpoint_auth_method IN ('client_secret_basic', 'private_key_jwt'))
    ),
    secret_material_handle text NULL,
    secret_material_version bigint NULL,
    require_pkce boolean NOT NULL DEFAULT true CHECK (require_pkce),
    consent_mode text NOT NULL DEFAULT 'explicit' CHECK (consent_mode IN ('explicit', 'trusted-first-party')),
    status text NOT NULL DEFAULT 'disabled' CHECK (status IN ('disabled', 'active', 'retired')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer_key, client_id),
    CHECK ((client_type = 'confidential') = (secret_material_handle IS NOT NULL AND secret_material_version IS NOT NULL)),
    CHECK (secret_material_handle IS NULL OR length(btrim(secret_material_handle)) BETWEEN 1 AND 512),
    CHECK (secret_material_version IS NULL OR secret_material_version > 0)
);

CREATE TABLE werk_core.identity_oidc_client_redirect_uris (
    client_id uuid NOT NULL REFERENCES werk_core.identity_oidc_clients(id) ON DELETE CASCADE,
    redirect_uri text NOT NULL CHECK (length(redirect_uri) BETWEEN 8 AND 2048 AND redirect_uri = btrim(redirect_uri)),
    PRIMARY KEY (client_id, redirect_uri)
);

CREATE TABLE werk_core.identity_oidc_scopes (
    scope_key text PRIMARY KEY CHECK (scope_key ~ '^[a-z][a-z0-9._-]{0,79}$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 1000),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired'))
);

INSERT INTO werk_core.identity_oidc_scopes (scope_key, display_name, description) VALUES
    ('openid', 'OpenID', 'OIDC subject identifier and ID token.'),
    ('profile', 'Profile', 'Display name and profile claims approved by the account.'),
    ('email', 'Email', 'Canonical login email claim when available and approved.');

CREATE TABLE werk_core.identity_oidc_client_scopes (
    client_id uuid NOT NULL REFERENCES werk_core.identity_oidc_clients(id) ON DELETE CASCADE,
    scope_key text NOT NULL REFERENCES werk_core.identity_oidc_scopes(scope_key) ON DELETE RESTRICT,
    default_scope boolean NOT NULL DEFAULT false,
    PRIMARY KEY (client_id, scope_key)
);

CREATE TABLE werk_core.identity_oidc_consents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES werk_core.identity_oidc_clients(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    scope_keys text[] NOT NULL CHECK (cardinality(scope_keys) > 0 AND cardinality(scope_keys) <= 32),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    granted_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz NULL,
    UNIQUE (client_id, account_id),
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

CREATE TABLE werk_core.identity_oidc_authorization_codes (
    code_hash bytea PRIMARY KEY CHECK (octet_length(code_hash) = 32),
    issuer_key text NOT NULL REFERENCES werk_core.identity_oidc_issuers(issuer_key) ON DELETE RESTRICT,
    client_id uuid NOT NULL REFERENCES werk_core.identity_oidc_clients(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES werk_core.sessions(id) ON DELETE CASCADE,
    session_generation bigint NOT NULL CHECK (session_generation > 0),
    redirect_uri text NOT NULL,
    scope_keys text[] NOT NULL CHECK (cardinality(scope_keys) > 0 AND cardinality(scope_keys) <= 32),
    code_challenge text NOT NULL CHECK (length(code_challenge) BETWEEN 43 AND 128),
    code_challenge_method text NOT NULL CHECK (code_challenge_method = 'S256'),
    nonce text NULL CHECK (nonce IS NULL OR length(nonce) BETWEEN 1 AND 512),
    authenticated_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '10 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at >= created_at),
    FOREIGN KEY (client_id, redirect_uri)
        REFERENCES werk_core.identity_oidc_client_redirect_uris(client_id, redirect_uri) ON DELETE RESTRICT
);

CREATE INDEX identity_oidc_authorization_codes_expiry_idx
    ON werk_core.identity_oidc_authorization_codes (expires_at) WHERE consumed_at IS NULL;
CREATE INDEX identity_oidc_consents_account_idx
    ON werk_core.identity_oidc_consents (account_id, status);

REVOKE ALL ON werk_core.identity_oidc_issuers, werk_core.identity_oidc_signing_keys,
    werk_core.identity_oidc_clients, werk_core.identity_oidc_client_redirect_uris,
    werk_core.identity_oidc_scopes, werk_core.identity_oidc_client_scopes,
    werk_core.identity_oidc_consents, werk_core.identity_oidc_authorization_codes
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
         werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON werk_core.identity_oidc_issuers, werk_core.identity_oidc_signing_keys,
    werk_core.identity_oidc_clients, werk_core.identity_oidc_client_redirect_uris,
    werk_core.identity_oidc_scopes, werk_core.identity_oidc_client_scopes
    TO werk_identity_runtime;
GRANT SELECT, INSERT, UPDATE ON werk_core.identity_oidc_consents,
    werk_core.identity_oidc_authorization_codes TO werk_identity_runtime;
GRANT SELECT ON werk_core.identity_oidc_issuers, werk_core.identity_oidc_signing_keys,
    werk_core.identity_oidc_clients, werk_core.identity_oidc_client_redirect_uris,
    werk_core.identity_oidc_scopes, werk_core.identity_oidc_client_scopes,
    werk_core.identity_oidc_consents, werk_core.identity_oidc_authorization_codes
    TO werk_backup_reader;

ALTER TABLE werk_core.identity_oidc_issuers ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_issuers FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_signing_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_signing_keys FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_clients ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_clients FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_client_redirect_uris ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_client_redirect_uris FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_scopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_scopes FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_client_scopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_client_scopes FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_consents ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_consents FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_authorization_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_oidc_authorization_codes FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_oidc_issuers_identity_read ON werk_core.identity_oidc_issuers FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_oidc_signing_keys_identity_read ON werk_core.identity_oidc_signing_keys FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_oidc_clients_identity_read ON werk_core.identity_oidc_clients FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_oidc_redirects_identity_read ON werk_core.identity_oidc_client_redirect_uris FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_oidc_scopes_identity_read ON werk_core.identity_oidc_scopes FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_oidc_client_scopes_identity_read ON werk_core.identity_oidc_client_scopes FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_oidc_consents_identity_all ON werk_core.identity_oidc_consents TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_codes_identity_all ON werk_core.identity_oidc_authorization_codes TO werk_identity_runtime USING (true) WITH CHECK (true);

CREATE POLICY identity_oidc_issuers_owner_all ON werk_core.identity_oidc_issuers TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_signing_keys_owner_all ON werk_core.identity_oidc_signing_keys TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_clients_owner_all ON werk_core.identity_oidc_clients TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_redirects_owner_all ON werk_core.identity_oidc_client_redirect_uris TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_scopes_owner_all ON werk_core.identity_oidc_scopes TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_client_scopes_owner_all ON werk_core.identity_oidc_client_scopes TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_consents_owner_all ON werk_core.identity_oidc_consents TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_oidc_codes_owner_all ON werk_core.identity_oidc_authorization_codes TO werk_owner USING (true) WITH CHECK (true);

