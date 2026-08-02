-- Bind sessions and MFA challenges to the account's current security epoch.
-- Password changes and assurance elevation advance the epoch atomically, so a
-- credential flow that started before the change cannot publish a new session
-- afterwards.

ALTER TABLE werk_core.accounts
    ADD COLUMN session_generation bigint NOT NULL DEFAULT 1
        CHECK (session_generation >= 1);

ALTER TABLE werk_core.sessions
    ADD COLUMN session_generation bigint NOT NULL DEFAULT 1
        CHECK (session_generation >= 1);

ALTER TABLE werk_core.identity_mfa_challenges
    ADD COLUMN session_generation bigint NOT NULL DEFAULT 1
        CHECK (session_generation >= 1);

-- Defaults are needed only to backfill existing rows. Every future
-- credential flow must state the account generation it actually observed.
ALTER TABLE werk_core.sessions
    ALTER COLUMN session_generation DROP DEFAULT;
ALTER TABLE werk_core.identity_mfa_challenges
    ALTER COLUMN session_generation DROP DEFAULT;

CREATE FUNCTION werk_security.validate_session_generation()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog, werk_core
AS $function$
DECLARE
    current_generation bigint;
BEGIN
    SELECT account.session_generation
    INTO current_generation
    FROM werk_core.accounts AS account
    WHERE account.id = NEW.account_id;

    IF current_generation IS NULL OR NEW.session_generation <> current_generation THEN
        RAISE EXCEPTION 'session generation mismatch'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.validate_session_generation() FROM PUBLIC;

CREATE TRIGGER sessions_validate_generation
BEFORE INSERT OR UPDATE OF account_id, session_generation
ON werk_core.sessions
FOR EACH ROW EXECUTE FUNCTION werk_security.validate_session_generation();

CREATE TRIGGER identity_mfa_challenges_validate_generation
BEFORE INSERT OR UPDATE OF account_id, session_generation
ON werk_core.identity_mfa_challenges
FOR EACH ROW EXECUTE FUNCTION werk_security.validate_session_generation();

CREATE FUNCTION werk_security.protect_account_session_generation()
RETURNS trigger
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = pg_catalog
AS $function$
BEGIN
    IF NEW.session_generation < OLD.session_generation THEN
        RAISE EXCEPTION 'account session generation cannot decrease'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$function$;

REVOKE ALL ON FUNCTION werk_security.protect_account_session_generation() FROM PUBLIC;

CREATE TRIGGER accounts_protect_session_generation
BEFORE UPDATE OF session_generation
ON werk_core.accounts
FOR EACH ROW EXECUTE FUNCTION werk_security.protect_account_session_generation();
