-- Bind every authorization artifact to one coherent issuer, client, account
-- class and security generation. These constraints are intentionally in the
-- database because protocol code must not be the only integrity boundary.

ALTER TABLE werk_core.accounts
    ADD CONSTRAINT accounts_id_account_class_unique UNIQUE (id, account_class);
ALTER TABLE werk_core.sessions
    ADD CONSTRAINT sessions_id_account_generation_unique
        UNIQUE (id, account_id, session_generation);
ALTER TABLE werk_core.identity_oidc_clients
    ADD CONSTRAINT identity_oidc_clients_id_issuer_unique UNIQUE (id, issuer_key),
    ADD CONSTRAINT identity_oidc_clients_id_account_class_unique UNIQUE (id, allowed_account_class);

ALTER TABLE werk_core.identity_oidc_consents ADD COLUMN account_class text;
UPDATE werk_core.identity_oidc_consents AS consent
SET account_class = account.account_class
FROM werk_core.accounts AS account
WHERE account.id = consent.account_id;
ALTER TABLE werk_core.identity_oidc_consents
    ALTER COLUMN account_class SET NOT NULL,
    ADD CONSTRAINT identity_oidc_consents_account_class_check CHECK (account_class IN ('work', 'admin')),
    ADD CONSTRAINT identity_oidc_consents_client_class_fk
        FOREIGN KEY (client_id, account_class)
        REFERENCES werk_core.identity_oidc_clients (id, allowed_account_class) ON DELETE CASCADE,
    ADD CONSTRAINT identity_oidc_consents_account_class_fk
        FOREIGN KEY (account_id, account_class)
        REFERENCES werk_core.accounts (id, account_class) ON DELETE CASCADE;

ALTER TABLE werk_core.identity_oidc_authorization_codes ADD COLUMN account_class text;
UPDATE werk_core.identity_oidc_authorization_codes AS code
SET account_class = account.account_class
FROM werk_core.accounts AS account
WHERE account.id = code.account_id;
ALTER TABLE werk_core.identity_oidc_authorization_codes
    ALTER COLUMN account_class SET NOT NULL,
    ADD CONSTRAINT identity_oidc_codes_account_class_check CHECK (account_class IN ('work', 'admin')),
    ADD CONSTRAINT identity_oidc_codes_client_issuer_fk
        FOREIGN KEY (client_id, issuer_key)
        REFERENCES werk_core.identity_oidc_clients (id, issuer_key) ON DELETE CASCADE,
    ADD CONSTRAINT identity_oidc_codes_client_class_fk
        FOREIGN KEY (client_id, account_class)
        REFERENCES werk_core.identity_oidc_clients (id, allowed_account_class) ON DELETE CASCADE,
    ADD CONSTRAINT identity_oidc_codes_account_class_fk
        FOREIGN KEY (account_id, account_class)
        REFERENCES werk_core.accounts (id, account_class) ON DELETE CASCADE,
    ADD CONSTRAINT identity_oidc_codes_session_generation_fk
        FOREIGN KEY (session_id, account_id, session_generation)
        REFERENCES werk_core.sessions (id, account_id, session_generation) ON DELETE CASCADE;

ALTER TABLE werk_core.identity_oidc_signing_keys
    ADD CONSTRAINT identity_oidc_signing_keys_public_only CHECK (
        NOT (public_jwk ?| ARRAY['d','p','q','dp','dq','qi','oth','k'])
        AND length(COALESCE(public_jwk->>'kty', '')) > 0
        AND public_jwk->>'kid' = key_id
        AND public_jwk->>'alg' = algorithm
        AND public_jwk->>'use' = 'sig'
    );

CREATE FUNCTION werk_security.validate_oidc_authorized_scopes()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE
    distinct_scope_count integer;
BEGIN
    SELECT count(DISTINCT requested.scope_key)
    INTO distinct_scope_count
    FROM unnest(NEW.scope_keys) AS requested(scope_key);

    IF distinct_scope_count <> cardinality(NEW.scope_keys)
       OR NOT ('openid' = ANY(NEW.scope_keys))
       OR EXISTS (
            SELECT 1
            FROM unnest(NEW.scope_keys) AS requested(scope_key)
            LEFT JOIN werk_core.identity_oidc_client_scopes AS allowed
              ON allowed.client_id = NEW.client_id
             AND allowed.scope_key = requested.scope_key
            LEFT JOIN werk_core.identity_oidc_scopes AS scope
              ON scope.scope_key = requested.scope_key
             AND scope.status = 'active'
            WHERE allowed.scope_key IS NULL OR scope.scope_key IS NULL
       ) THEN
        RAISE EXCEPTION 'OIDC scope set is not active and assigned to the client'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.validate_oidc_authorized_scopes() FROM PUBLIC;

CREATE TRIGGER identity_oidc_consents_validate_scopes
BEFORE INSERT OR UPDATE OF client_id, scope_keys
ON werk_core.identity_oidc_consents
FOR EACH ROW EXECUTE FUNCTION werk_security.validate_oidc_authorized_scopes();

CREATE TRIGGER identity_oidc_codes_validate_scopes
BEFORE INSERT OR UPDATE OF client_id, scope_keys
ON werk_core.identity_oidc_authorization_codes
FOR EACH ROW EXECUTE FUNCTION werk_security.validate_oidc_authorized_scopes();
