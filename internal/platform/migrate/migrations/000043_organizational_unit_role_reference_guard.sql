-- Organizational-unit administration must not expose service-plane role
-- assignments to the admin runtime, but it still has to preserve those durable
-- references while archiving a unit. This narrow function returns only the
-- integrity decision and locks every matching row in deterministic order.

CREATE FUNCTION werk_security.lock_organizational_unit_role_assignment_references(
    candidate_tenant_id uuid,
    candidate_organizational_unit_id uuid,
    candidate_evaluated_at timestamptz
)
RETURNS boolean
LANGUAGE plpgsql
VOLATILE
STRICT
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    assignment_record record;
    has_reference boolean := false;
BEGIN
    IF werk_security.current_tenant_id() IS DISTINCT FROM candidate_tenant_id THEN
        RAISE EXCEPTION 'organizational-unit role reference check requires the matching tenant context'
            USING ERRCODE = '42501';
    END IF;

    FOR assignment_record IN
        SELECT assignment.valid_until
        FROM werk_core.role_assignments AS assignment
        WHERE assignment.scope_tenant_id = candidate_tenant_id
          AND assignment.scope_type = 'organizational-unit'
          AND assignment.scope_id = candidate_organizational_unit_id::text
        ORDER BY assignment.id
        FOR UPDATE OF assignment
    LOOP
        IF assignment_record.valid_until IS NULL
           OR assignment_record.valid_until > candidate_evaluated_at THEN
            has_reference := true;
        END IF;
    END LOOP;

    RETURN has_reference;
END
$function$;

REVOKE ALL ON FUNCTION
    werk_security.lock_organizational_unit_role_assignment_references(uuid, uuid, timestamptz)
FROM PUBLIC;

GRANT EXECUTE ON FUNCTION
    werk_security.lock_organizational_unit_role_assignment_references(uuid, uuid, timestamptz)
TO werk_admin_runtime;

COMMENT ON FUNCTION
    werk_security.lock_organizational_unit_role_assignment_references(uuid, uuid, timestamptz)
IS 'Locks all organizational-unit role assignments across work and service planes and returns whether an unexpired reference exists, without exposing assignment rows to the admin runtime.';
