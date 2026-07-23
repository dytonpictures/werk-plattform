CREATE TABLE werk_core.identity_account_classes (
    account_class text PRIMARY KEY CHECK (account_class ~ '^[a-z][a-z0-9-]*$'),
    interactive boolean NOT NULL,
    tenant_required boolean NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired'))
);

INSERT INTO werk_core.identity_account_classes (
    account_class, interactive, tenant_required
) VALUES
    ('work', true, true),
    ('admin', true, false),
    ('service', false, false),
    ('agent', false, true);

CREATE TABLE werk_core.identity_audiences (
    audience text PRIMARY KEY CHECK (audience ~ '^[a-z][a-z0-9-]*$'),
    access_plane text NOT NULL CHECK (access_plane IN ('work', 'admin', 'service')),
    authentication_kind text NOT NULL CHECK (authentication_kind IN ('interactive', 'workload')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired'))
);

INSERT INTO werk_core.identity_audiences (
    audience, access_plane, authentication_kind
) VALUES
    ('work', 'work', 'interactive'),
    ('admin', 'admin', 'interactive'),
    ('service', 'service', 'workload');

CREATE TABLE werk_core.identity_account_class_audiences (
    account_class text NOT NULL REFERENCES werk_core.identity_account_classes(account_class),
    audience text NOT NULL REFERENCES werk_core.identity_audiences(audience),
    PRIMARY KEY (account_class, audience)
);

INSERT INTO werk_core.identity_account_class_audiences (account_class, audience)
VALUES
    ('work', 'work'),
    ('admin', 'admin'),
    ('service', 'service'),
    ('agent', 'service');

ALTER TABLE werk_core.accounts
    DROP CONSTRAINT accounts_account_class_check,
    ADD CONSTRAINT accounts_account_class_fk
        FOREIGN KEY (account_class)
        REFERENCES werk_core.identity_account_classes(account_class);

CREATE TABLE werk_core.identity_agents (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id),
    agent_key text NOT NULL CHECK (agent_key ~ '^[a-z][a-z0-9.-]*$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, agent_key)
);

ALTER TABLE werk_core.accounts
    ADD COLUMN agent_subject_id uuid NULL,
    ADD CONSTRAINT accounts_agent_subject_fk
        FOREIGN KEY (tenant_id, agent_subject_id)
        REFERENCES werk_core.identity_agents(tenant_id, id),
    DROP CONSTRAINT accounts_check,
    ADD CONSTRAINT accounts_subject_shape_check CHECK (
        (account_class = 'work' AND tenant_id IS NOT NULL AND person_party_id IS NOT NULL
            AND admin_subject_id IS NULL AND agent_subject_id IS NULL)
        OR (account_class = 'admin' AND tenant_id IS NULL AND person_party_id IS NULL
            AND admin_subject_id IS NOT NULL AND agent_subject_id IS NULL)
        OR (account_class = 'service' AND person_party_id IS NULL
            AND admin_subject_id IS NULL AND agent_subject_id IS NULL)
        OR (account_class = 'agent' AND tenant_id IS NOT NULL AND person_party_id IS NULL
            AND admin_subject_id IS NULL AND agent_subject_id IS NOT NULL)
    );

DO $function$
BEGIN
    IF EXISTS (
        SELECT lower(btrim(login_name))
        FROM werk_core.accounts
        GROUP BY lower(btrim(login_name))
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'login names collide after canonicalization';
    END IF;
END
$function$;

UPDATE werk_core.accounts
SET login_name = lower(btrim(login_name));

ALTER TABLE werk_core.accounts
    ADD CONSTRAINT accounts_login_name_canonical_check
        CHECK (login_name = lower(btrim(login_name)) AND length(login_name) BETWEEN 1 AND 320);

CREATE TABLE werk_core.identity_providers (
    provider_key text PRIMARY KEY CHECK (provider_key ~ '^[a-z][a-z0-9.-]*$'),
    provider_kind text NOT NULL CHECK (provider_kind IN ('local', 'oidc', 'saml', 'ldap')),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 160),
    issuer text NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO werk_core.identity_providers (
    provider_key, provider_kind, display_name
) VALUES ('local', 'local', 'Local identity');

CREATE TABLE werk_core.account_identity_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    provider_key text NOT NULL REFERENCES werk_core.identity_providers(provider_key),
    provider_subject text NOT NULL CHECK (length(provider_subject) BETWEEN 1 AND 512),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_authenticated_at timestamptz NULL,
    revoked_at timestamptz NULL,
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL)),
    UNIQUE (provider_key, provider_subject),
    UNIQUE (account_id, provider_key, provider_subject)
);

INSERT INTO werk_core.account_identity_bindings (
    account_id, provider_key, provider_subject
)
SELECT id, 'local', id::text
FROM werk_core.accounts;

ALTER TABLE werk_core.account_credentials
    ADD COLUMN id uuid NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN provider_key text NOT NULL DEFAULT 'local'
        REFERENCES werk_core.identity_providers(provider_key),
    ADD COLUMN display_name text NULL,
    ADD COLUMN public_id_hash bytea NULL,
    ADD COLUMN status text NOT NULL DEFAULT 'active',
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN expires_at timestamptz NULL,
    ADD COLUMN revoked_at timestamptz NULL,
    ADD COLUMN last_used_at timestamptz NULL,
    ADD COLUMN use_limit bigint NULL,
    ADD COLUMN use_count bigint NOT NULL DEFAULT 0,
    DROP CONSTRAINT account_credentials_pkey,
    DROP CONSTRAINT account_credentials_credential_kind_check,
    ADD CONSTRAINT account_credentials_pkey PRIMARY KEY (id),
    ADD CONSTRAINT account_credentials_kind_check
        CHECK (credential_kind IN ('password', 'service-token', 'api-key')),
    ADD CONSTRAINT account_credentials_status_check
        CHECK (status IN ('active', 'revoked')),
    ADD CONSTRAINT account_credentials_display_name_check
        CHECK (display_name IS NULL OR length(btrim(display_name)) BETWEEN 1 AND 120),
    ADD CONSTRAINT account_credentials_lifecycle_check
        CHECK ((status = 'active' AND revoked_at IS NULL)
            OR (status = 'revoked' AND revoked_at IS NOT NULL)),
    ADD CONSTRAINT account_credentials_expiry_check
        CHECK (expires_at IS NULL OR expires_at > created_at),
    ADD CONSTRAINT account_credentials_usage_check
        CHECK (use_count >= 0 AND (use_limit IS NULL OR use_limit > 0)
            AND (use_limit IS NULL OR use_count <= use_limit)),
    ADD CONSTRAINT account_credentials_api_key_shape_check
        CHECK (credential_kind <> 'api-key' OR public_id_hash IS NOT NULL);

CREATE UNIQUE INDEX account_credentials_active_password_idx
    ON werk_core.account_credentials (account_id)
    WHERE credential_kind = 'password' AND status = 'active';

CREATE UNIQUE INDEX account_credentials_public_id_idx
    ON werk_core.account_credentials (public_id_hash)
    WHERE public_id_hash IS NOT NULL;

CREATE INDEX account_credentials_account_status_idx
    ON werk_core.account_credentials (account_id, status, credential_kind);

ALTER TABLE werk_core.sessions
    DROP CONSTRAINT sessions_audience_check,
    ADD COLUMN authentication_kind text NOT NULL DEFAULT 'interactive'
        CHECK (authentication_kind IN ('interactive', 'workload'));

UPDATE werk_core.sessions AS session
SET audience = account.account_class,
    authentication_kind = CASE
        WHEN account.account_class IN ('service', 'agent') THEN 'workload'
        ELSE 'interactive'
    END
FROM werk_core.accounts AS account
WHERE account.id = session.account_id;

ALTER TABLE werk_core.sessions
    ADD CONSTRAINT sessions_audience_fk
        FOREIGN KEY (audience) REFERENCES werk_core.identity_audiences(audience);

CREATE FUNCTION werk_security.validate_identity_session_boundary()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE
    account_class_value text;
    account_tenant_value uuid;
    expected_authentication_kind text;
BEGIN
    SELECT account_class, tenant_id
    INTO account_class_value, account_tenant_value
    FROM werk_core.accounts
    WHERE id = NEW.account_id;

    SELECT audience.authentication_kind
    INTO expected_authentication_kind
    FROM werk_core.identity_account_class_audiences AS allowed
    JOIN werk_core.identity_audiences AS audience
      ON audience.audience = allowed.audience AND audience.status = 'active'
    WHERE allowed.account_class = account_class_value
      AND allowed.audience = NEW.audience;

    IF account_class_value IS NULL OR expected_authentication_kind IS NULL
       OR NEW.authentication_kind <> expected_authentication_kind
       OR account_tenant_value IS DISTINCT FROM NEW.tenant_id THEN
        RAISE EXCEPTION 'session identity boundary mismatch'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE TRIGGER sessions_validate_identity_boundary
BEFORE INSERT OR UPDATE OF account_id, audience, tenant_id, authentication_kind
ON werk_core.sessions
FOR EACH ROW EXECUTE FUNCTION werk_security.validate_identity_session_boundary();

CREATE FUNCTION werk_security.protect_account_identity_boundary()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF OLD.account_class IS DISTINCT FROM NEW.account_class
       OR OLD.tenant_id IS DISTINCT FROM NEW.tenant_id
       OR OLD.person_party_id IS DISTINCT FROM NEW.person_party_id
       OR OLD.admin_subject_id IS DISTINCT FROM NEW.admin_subject_id
       OR OLD.agent_subject_id IS DISTINCT FROM NEW.agent_subject_id THEN
        RAISE EXCEPTION 'account identity boundary is immutable'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

CREATE TRIGGER accounts_protect_identity_boundary
BEFORE UPDATE ON werk_core.accounts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_account_identity_boundary();

CREATE OR REPLACE FUNCTION werk_security.validate_role_binding()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE account_class_value text; account_tenant_value uuid; role_plane_value text; role_tenant_value uuid;
BEGIN
    SELECT account_class, tenant_id INTO account_class_value, account_tenant_value FROM werk_core.accounts WHERE id = NEW.account_id;
    SELECT access_plane, tenant_id INTO role_plane_value, role_tenant_value FROM werk_core.roles WHERE id = NEW.role_id;
    IF account_class_value IS NULL OR role_plane_value IS NULL
       OR role_plane_value <> NEW.access_plane
       OR NOT (account_class_value = NEW.access_plane OR (account_class_value = 'agent' AND NEW.access_plane = 'service')) THEN
        RAISE EXCEPTION 'role assignment access plane mismatch';
    END IF;
    IF NEW.access_plane = 'admin' AND (account_tenant_value IS NOT NULL OR role_tenant_value IS NOT NULL OR NEW.scope_type <> 'installation') THEN
        RAISE EXCEPTION 'admin role assignment must be installation scoped';
    END IF;
    IF NEW.access_plane IN ('work', 'service') AND (account_tenant_value IS NULL OR role_tenant_value <> account_tenant_value OR NEW.scope_tenant_id <> account_tenant_value) THEN
        RAISE EXCEPTION 'tenant role assignment mismatch';
    END IF;
    RETURN NEW;
END
$function$;

CREATE INDEX identity_auth_throttles_updated_idx
    ON werk_core.identity_auth_throttles (updated_at);

REVOKE ALL ON
    werk_core.identity_account_classes,
    werk_core.identity_audiences,
    werk_core.identity_account_class_audiences,
    werk_core.identity_agents,
    werk_core.identity_providers,
    werk_core.account_identity_bindings
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
     werk_service_runtime, werk_worker_runtime;

GRANT SELECT ON
    werk_core.identity_account_classes,
    werk_core.identity_audiences,
    werk_core.identity_account_class_audiences,
    werk_core.identity_providers
TO werk_identity_runtime;

GRANT SELECT, INSERT, UPDATE ON
    werk_core.identity_agents,
    werk_core.account_identity_bindings
TO werk_identity_runtime;

GRANT INSERT ON werk_core.account_identity_bindings TO werk_admin_runtime;

GRANT SELECT ON
    werk_core.identity_account_classes,
    werk_core.identity_audiences,
    werk_core.identity_account_class_audiences,
    werk_core.identity_agents,
    werk_core.identity_providers,
    werk_core.account_identity_bindings
TO werk_backup_reader;

ALTER TABLE werk_core.identity_account_classes ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_account_classes FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_audiences ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_audiences FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_account_class_audiences ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_account_class_audiences FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_agents FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_providers FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.account_identity_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.account_identity_bindings FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_account_classes_identity_read ON werk_core.identity_account_classes
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_account_classes_owner_all ON werk_core.identity_account_classes
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_audiences_identity_read ON werk_core.identity_audiences
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_audiences_owner_all ON werk_core.identity_audiences
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_account_class_audiences_identity_read ON werk_core.identity_account_class_audiences
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_account_class_audiences_owner_all ON werk_core.identity_account_class_audiences
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_agents_identity_all ON werk_core.identity_agents
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY identity_agents_owner_all ON werk_core.identity_agents
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY identity_providers_identity_read ON werk_core.identity_providers
    FOR SELECT TO werk_identity_runtime USING (true);
CREATE POLICY identity_providers_owner_all ON werk_core.identity_providers
    TO werk_owner USING (true) WITH CHECK (true);
CREATE POLICY account_identity_bindings_identity_all ON werk_core.account_identity_bindings
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY account_identity_bindings_admin_work_insert ON werk_core.account_identity_bindings
    FOR INSERT TO werk_admin_runtime
    WITH CHECK (
        provider_key = 'local'
        AND EXISTS (
            SELECT 1
            FROM werk_core.accounts AS account
            WHERE account.id = account_id
              AND account.account_class = 'work'
              AND account.tenant_id = werk_security.current_tenant_id()
        )
    );
CREATE POLICY account_identity_bindings_owner_all ON werk_core.account_identity_bindings
    TO werk_owner USING (true) WITH CHECK (true);
