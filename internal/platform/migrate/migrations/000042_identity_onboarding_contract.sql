-- Initial work-account activation is a narrow one-time credential ceremony.
-- Raw invitation tokens and delivery addresses never enter PostgreSQL.

CREATE TABLE werk_core.identity_account_invitations (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    account_id uuid NOT NULL,
    purpose text NOT NULL DEFAULT 'initial-activation'
        CHECK (purpose = 'initial-activation'),
    token_hash bytea NOT NULL UNIQUE
        CHECK (octet_length(token_hash) = 32),
    created_by_account_id uuid NOT NULL REFERENCES werk_core.accounts(id),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    revoked_at timestamptz NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, account_id)
        REFERENCES werk_core.accounts(tenant_id, id) ON DELETE CASCADE,
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '7 days'),
    CHECK (consumed_at IS NULL OR consumed_at >= created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at),
    CHECK (NOT (consumed_at IS NOT NULL AND revoked_at IS NOT NULL))
);

CREATE UNIQUE INDEX identity_account_invitations_one_open_per_account_idx
    ON werk_core.identity_account_invitations (account_id, purpose)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE INDEX identity_account_invitations_expiry_idx
    ON werk_core.identity_account_invitations (expires_at)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;

COMMENT ON TABLE werk_core.identity_account_invitations IS
    'One-time initial work-account activations. Raw tokens, links and delivery addresses are never persisted.';

CREATE FUNCTION werk_security.validate_initial_account_invitation()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE
    target_class text;
    target_tenant uuid;
    target_status text;
    target_must_change boolean;
    credential_count bigint;
    creator_class text;
    creator_status text;
BEGIN
    SELECT account_class, tenant_id, status, must_change_password
    INTO target_class, target_tenant, target_status, target_must_change
    FROM werk_core.accounts
    WHERE id = NEW.account_id;

    SELECT count(*)
    INTO credential_count
    FROM werk_core.account_credentials
    WHERE account_id = NEW.account_id;

    SELECT account_class, status
    INTO creator_class, creator_status
    FROM werk_core.accounts
    WHERE id = NEW.created_by_account_id;

    IF target_class IS DISTINCT FROM 'work'
       OR target_tenant IS DISTINCT FROM NEW.tenant_id
       OR target_status IS DISTINCT FROM 'disabled'
       OR target_must_change
       OR credential_count <> 0
       OR creator_class IS DISTINCT FROM 'admin'
       OR creator_status IS DISTINCT FROM 'active' THEN
        RAISE EXCEPTION 'invalid initial work account invitation boundary'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.validate_initial_account_invitation() FROM PUBLIC;

CREATE TRIGGER identity_account_invitations_validate
BEFORE INSERT OR UPDATE OF tenant_id, account_id, purpose, created_by_account_id
ON werk_core.identity_account_invitations
FOR EACH ROW EXECUTE FUNCTION werk_security.validate_initial_account_invitation();

REVOKE ALL ON werk_core.identity_account_invitations
FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime,
     werk_service_runtime, werk_worker_runtime;

GRANT INSERT (id, tenant_id, account_id, purpose, token_hash,
              created_by_account_id, expires_at)
ON werk_core.identity_account_invitations TO werk_admin_runtime;
GRANT SELECT (id, tenant_id, account_id, purpose, expires_at,
              consumed_at, revoked_at)
ON werk_core.identity_account_invitations TO werk_admin_runtime;
GRANT SELECT ON werk_core.identity_account_invitations TO werk_backup_reader;

ALTER TABLE werk_core.identity_account_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.identity_account_invitations FORCE ROW LEVEL SECURITY;

CREATE POLICY identity_account_invitations_admin_tenant
    ON werk_core.identity_account_invitations
    TO werk_admin_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());

CREATE POLICY identity_account_invitations_backup_read
    ON werk_core.identity_account_invitations
    FOR SELECT TO werk_backup_reader
    USING (true);

CREATE POLICY identity_account_invitations_owner_all
    ON werk_core.identity_account_invitations
    TO werk_owner
    USING (true)
    WITH CHECK (true);

-- Trigger callers do not receive table access merely to enforce the pending
-- invitation invariant. This boolean gate exposes neither tenant, invitation
-- identity, digest nor expiry and is the only invitation read granted to the
-- admin and identity runtimes.
CREATE FUNCTION werk_security.has_open_initial_work_account_invitation(
    candidate_account_id uuid
)
RETURNS boolean
LANGUAGE sql
VOLATILE
STRICT
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
    SELECT EXISTS (
        SELECT 1
        FROM werk_core.identity_account_invitations AS invitation
        WHERE invitation.account_id = candidate_account_id
          AND invitation.purpose = 'initial-activation'
          AND invitation.consumed_at IS NULL
          AND invitation.revoked_at IS NULL
    )
$function$;

REVOKE ALL ON FUNCTION werk_security.has_open_initial_work_account_invitation(uuid)
FROM PUBLIC, werk_work_runtime, werk_service_runtime, werk_worker_runtime,
     werk_backup_reader;
GRANT EXECUTE ON FUNCTION werk_security.has_open_initial_work_account_invitation(uuid)
TO werk_admin_runtime, werk_identity_runtime;

-- A disabled account with an outstanding initial invitation is not an
-- ordinary administratively disabled account. It may become active only by
-- atomically consuming that invitation and creating its first credential.
-- The activation function consumes the invitation before changing credentials
-- or status; every caller therefore passes through the same invariant.
CREATE FUNCTION werk_security.protect_pending_invitation_account_status()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF werk_security.has_open_initial_work_account_invitation(OLD.id) THEN
        RAISE EXCEPTION 'pending invitation account status is activation-controlled'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.protect_pending_invitation_account_status()
FROM PUBLIC;

CREATE TRIGGER accounts_protect_pending_invitation_status
BEFORE UPDATE OF status
ON werk_core.accounts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_pending_invitation_account_status();

-- The admin runtime still creates password credentials for the explicit
-- initial-password path, and Core Identity owns established credential
-- lifecycles. Neither broad pre-existing privilege may inject, move, alter or
-- remove a credential while an initial invitation is open. The acceptance
-- function consumes the invitation before inserting the first credential, so
-- it remains the only transition through this guard.
CREATE FUNCTION werk_security.protect_pending_invitation_account_credential()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
DECLARE
    previous_account_id uuid;
    current_account_id uuid;
BEGIN
    IF TG_OP = 'INSERT' THEN
        current_account_id := NEW.account_id;
    ELSIF TG_OP = 'DELETE' THEN
        previous_account_id := OLD.account_id;
    ELSE
        previous_account_id := OLD.account_id;
        current_account_id := NEW.account_id;
    END IF;

    IF COALESCE(
           werk_security.has_open_initial_work_account_invitation(previous_account_id),
           false
       ) OR COALESCE(
           werk_security.has_open_initial_work_account_invitation(current_account_id),
           false
       ) THEN
        RAISE EXCEPTION 'pending invitation account credential is activation-controlled'
            USING ERRCODE = '23514';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.protect_pending_invitation_account_credential()
FROM PUBLIC;

CREATE TRIGGER account_credentials_protect_pending_invitation
BEFORE INSERT OR UPDATE OR DELETE
ON werk_core.account_credentials
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_pending_invitation_account_credential();

-- Re-issuing an initial invitation is a tenant-explicit administrative command.
-- This function owns only the state transition; its caller must write the
-- corresponding audit and outbox records in the same surrounding transaction.
CREATE FUNCTION werk_security.replace_initial_work_account_invitation(
    candidate_tenant_id uuid,
    candidate_account_id uuid,
    candidate_invitation_id uuid,
    candidate_token_hash bytea,
    candidate_creator_account_id uuid,
    candidate_expires_in_hours integer
)
RETURNS timestamptz
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE
    account_class_value text;
    account_tenant_id uuid;
    account_status_value text;
    account_must_change boolean;
    previous_invitation_id uuid;
    credential_count bigint;
    tenant_status_value text;
    creator_class_value text;
    creator_status_value text;
    observed_at timestamptz;
    replacement_expires_at timestamptz;
BEGIN
    IF candidate_tenant_id IS NULL
       OR candidate_account_id IS NULL
       OR candidate_invitation_id IS NULL
       OR candidate_token_hash IS NULL
       OR octet_length(candidate_token_hash) <> 32
       OR candidate_creator_account_id IS NULL
       OR candidate_expires_in_hours IS NULL
       OR candidate_expires_in_hours < 1
       OR candidate_expires_in_hours > 168
       OR werk_security.current_tenant_id() IS DISTINCT FROM candidate_tenant_id THEN
        RAISE EXCEPTION 'invalid initial work account invitation replacement boundary'
            USING ERRCODE = '23514';
    END IF;

    -- Every replacement and activation starts with the target account lock.
    SELECT account.account_class, account.tenant_id, account.status,
           account.must_change_password
    INTO account_class_value, account_tenant_id, account_status_value,
         account_must_change
    FROM werk_core.accounts AS account
    WHERE account.id = candidate_account_id
    FOR UPDATE;

    IF account_class_value IS DISTINCT FROM 'work'
       OR account_tenant_id IS DISTINCT FROM candidate_tenant_id
       OR account_status_value IS DISTINCT FROM 'disabled'
       OR account_must_change THEN
        RAISE EXCEPTION 'invalid initial work account invitation replacement boundary'
            USING ERRCODE = '23514';
    END IF;

    -- The existing open invitation is always the second row lock. Expired rows
    -- remain open until explicitly revoked and are therefore replaceable here.
    SELECT invitation.id
    INTO previous_invitation_id
    FROM werk_core.identity_account_invitations AS invitation
    WHERE invitation.tenant_id = candidate_tenant_id
      AND invitation.account_id = candidate_account_id
      AND invitation.purpose = 'initial-activation'
      AND invitation.consumed_at IS NULL
      AND invitation.revoked_at IS NULL
    ORDER BY invitation.id
    LIMIT 1
    FOR UPDATE;

    IF previous_invitation_id IS NULL
       OR previous_invitation_id = candidate_invitation_id THEN
        RAISE EXCEPTION 'invalid initial work account invitation replacement boundary'
            USING ERRCODE = '23514';
    END IF;

    SELECT count(*)
    INTO credential_count
    FROM werk_core.account_credentials AS credential
    WHERE credential.account_id = candidate_account_id;

    -- Keep the tenant active through the surrounding transaction. Returning a
    -- fresh link for a suspended or archived tenant would be misleading even
    -- though activation itself also fails closed for inactive tenants.
    SELECT tenant.status
    INTO tenant_status_value
    FROM werk_core.tenants AS tenant
    WHERE tenant.id = candidate_tenant_id
    FOR SHARE;

    SELECT creator.account_class, creator.status
    INTO creator_class_value, creator_status_value
    FROM werk_core.accounts AS creator
    WHERE creator.id = candidate_creator_account_id
    FOR SHARE;

    IF credential_count <> 0
       OR tenant_status_value IS DISTINCT FROM 'active'
       OR creator_class_value IS DISTINCT FROM 'admin'
       OR creator_status_value IS DISTINCT FROM 'active' THEN
        RAISE EXCEPTION 'invalid initial work account invitation replacement boundary'
            USING ERRCODE = '23514';
    END IF;

    observed_at := pg_catalog.clock_timestamp();
    replacement_expires_at := observed_at
        + (candidate_expires_in_hours * interval '1 hour');

    UPDATE werk_core.identity_account_invitations AS previous_invitation
    SET revoked_at = observed_at
    WHERE previous_invitation.id = previous_invitation_id
      AND previous_invitation.consumed_at IS NULL
      AND previous_invitation.revoked_at IS NULL;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'invalid initial work account invitation replacement boundary'
            USING ERRCODE = '40001';
    END IF;

    INSERT INTO werk_core.identity_account_invitations (
        id, tenant_id, account_id, purpose, token_hash,
        created_by_account_id, created_at, expires_at
    ) VALUES (
        candidate_invitation_id, candidate_tenant_id, candidate_account_id,
        'initial-activation', candidate_token_hash,
        candidate_creator_account_id, observed_at, replacement_expires_at
    );

    RETURN replacement_expires_at;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.replace_initial_work_account_invitation(
    uuid, uuid, uuid, bytea, uuid, integer
) FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_service_runtime,
       werk_worker_runtime, werk_backup_reader;
GRANT EXECUTE ON FUNCTION werk_security.replace_initial_work_account_invitation(
    uuid, uuid, uuid, bytea, uuid, integer
) TO werk_admin_runtime;

CREATE FUNCTION werk_security.accept_initial_work_account_invitation(
    candidate_token_hash bytea,
    candidate_credential_id uuid,
    candidate_secret_hash bytea,
    candidate_audit_id uuid,
    candidate_event_id uuid,
    candidate_request_id uuid,
    candidate_correlation_id uuid
)
RETURNS TABLE (account_id uuid, tenant_id uuid)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE
    candidate_account_id uuid;
    invitation_id uuid;
    invitation_tenant_id uuid;
    invitation_expires_at timestamptz;
    account_class_value text;
    account_tenant_id uuid;
    account_status_value text;
    account_must_change boolean;
    provider_kind_value text;
    observed_at timestamptz;
BEGIN
    IF candidate_token_hash IS NULL
       OR octet_length(candidate_token_hash) <> 32
       OR candidate_credential_id IS NULL
       OR candidate_secret_hash IS NULL
       OR octet_length(candidate_secret_hash) < 32
       OR candidate_audit_id IS NULL
       OR candidate_event_id IS NULL
       OR candidate_request_id IS NULL
       OR candidate_correlation_id IS NULL THEN
        RETURN;
    END IF;

    -- The first lookup is deliberately non-locking. The final decision starts
    -- with the account lock and then re-reads the exact invitation FOR UPDATE,
    -- yielding one lock order for activation, replay and future revocation.
    SELECT invitation.account_id
    INTO candidate_account_id
    FROM werk_core.identity_account_invitations AS invitation
    WHERE invitation.token_hash = candidate_token_hash
      AND invitation.purpose = 'initial-activation';

    IF candidate_account_id IS NULL THEN
        RETURN;
    END IF;

    SELECT account.account_class, account.tenant_id, account.status,
           account.must_change_password
    INTO account_class_value, account_tenant_id, account_status_value,
         account_must_change
    FROM werk_core.accounts AS account
    WHERE account.id = candidate_account_id
    FOR UPDATE;

    IF account_class_value IS DISTINCT FROM 'work'
       OR account_tenant_id IS NULL
       OR account_status_value IS DISTINCT FROM 'disabled'
       OR account_must_change THEN
        RETURN;
    END IF;

    SELECT invitation.id, invitation.tenant_id, invitation.expires_at
    INTO invitation_id, invitation_tenant_id, invitation_expires_at
    FROM werk_core.identity_account_invitations AS invitation
    WHERE invitation.token_hash = candidate_token_hash
      AND invitation.account_id = candidate_account_id
      AND invitation.purpose = 'initial-activation'
      AND invitation.consumed_at IS NULL
      AND invitation.revoked_at IS NULL
    FOR UPDATE;

    IF invitation_id IS NULL
       OR invitation_tenant_id IS DISTINCT FROM account_tenant_id
       OR EXISTS (
           SELECT 1 FROM werk_core.account_credentials AS credential
           WHERE credential.account_id = candidate_account_id
       )
       OR NOT EXISTS (
           SELECT 1
           FROM werk_core.tenants AS tenant
           WHERE tenant.id = account_tenant_id AND tenant.status = 'active'
       )
       THEN
        RETURN;
    END IF;

    -- Hold the exact provider and binding rows until commit. A concurrent
    -- provider disablement therefore cannot race the first credential into an
    -- account whose local authentication path has just been switched off.
    SELECT locked.provider_kind
    INTO provider_kind_value
    FROM werk_security.lock_active_identity_provider_binding(
        candidate_account_id, 'local'
    ) AS locked;

    IF provider_kind_value IS DISTINCT FROM 'local' THEN
        RETURN;
    END IF;

    -- Expiry is decided only after every potentially blocking account,
    -- invitation, provider and binding lock has been acquired. A request that
    -- began before expiry but waited beyond it must fail closed.
    observed_at := pg_catalog.clock_timestamp();
    IF observed_at >= invitation_expires_at THEN
        RETURN;
    END IF;

    -- Consume first inside the same transaction. The credential guard therefore
    -- also remains fail-closed if the acceptance implementation is later moved
    -- away from the werk_owner definer; any following failure rolls consumption
    -- back together with the rest of the activation.
    UPDATE werk_core.identity_account_invitations AS accepted_invitation
    SET consumed_at = observed_at
    WHERE accepted_invitation.id = invitation_id
      AND accepted_invitation.consumed_at IS NULL
      AND accepted_invitation.revoked_at IS NULL;

    IF NOT FOUND THEN
        RETURN;
    END IF;

    INSERT INTO werk_core.account_credentials (
        id, account_id, provider_key, credential_kind, secret_hash,
        assurance, status, created_at, changed_at
    ) VALUES (
        candidate_credential_id, candidate_account_id, 'local', 'password',
        candidate_secret_hash, 'single-factor', 'active', observed_at, observed_at
    );

    UPDATE werk_core.identity_account_invitations AS other_invitation
    SET revoked_at = observed_at
    WHERE other_invitation.account_id = candidate_account_id
      AND other_invitation.purpose = 'initial-activation'
      AND other_invitation.id <> invitation_id
      AND other_invitation.consumed_at IS NULL
      AND other_invitation.revoked_at IS NULL;

    UPDATE werk_core.identity_mfa_challenges AS challenge
    SET used_at = observed_at
    WHERE challenge.account_id = candidate_account_id
      AND challenge.used_at IS NULL;

    UPDATE werk_core.sessions AS session
    SET revoked_at = observed_at
    WHERE session.account_id = candidate_account_id
      AND session.revoked_at IS NULL;

    UPDATE werk_core.accounts AS activated_account
    SET status = 'active', must_change_password = false,
        session_generation = session_generation + 1,
        version = version + 1, updated_at = observed_at
    WHERE activated_account.id = candidate_account_id;

    INSERT INTO werk_core.security_audit_events (
        id, occurred_at, event_type, outcome, account_id, tenant_id,
        request_id, correlation_id, details
    ) VALUES (
        candidate_audit_id, observed_at,
        'identity.work-account-invitation.accepted.v1', 'succeeded',
        candidate_account_id, account_tenant_id,
        candidate_request_id, candidate_correlation_id,
        jsonb_build_object('invitation_id', invitation_id::text)
    );

    INSERT INTO werk_core.outbox_events (
        id, tenant_id, event_type, producer, subject_kind, subject_id,
        partition_key, occurred_at, correlation_id, payload
    ) VALUES (
        candidate_event_id, account_tenant_id,
        'core.identity.work-account-invitation-accepted.v1',
        'core.identity', 'core.identity.work-account', candidate_account_id,
        candidate_account_id::text, observed_at, candidate_correlation_id,
        jsonb_build_object('account_id', candidate_account_id::text)
    );

    account_id := candidate_account_id;
    tenant_id := account_tenant_id;
    RETURN NEXT;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.accept_initial_work_account_invitation(
    bytea, uuid, bytea, uuid, uuid, uuid, uuid
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION werk_security.accept_initial_work_account_invitation(
    bytea, uuid, bytea, uuid, uuid, uuid, uuid
) TO werk_identity_runtime;
