-- Core Identity may resolve a work account's display name, but Core Party
-- remains the owner. No write grant is intentionally given here.
GRANT SELECT ON werk_core.parties TO werk_identity_runtime;

CREATE POLICY parties_identity_read
    ON werk_core.parties FOR SELECT TO werk_identity_runtime
    USING (true);
