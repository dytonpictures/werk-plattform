CREATE TABLE werk_core.parties (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id),
    party_type text NOT NULL CHECK (party_type IN ('person', 'organization')),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, id, party_type)
);

CREATE INDEX parties_tenant_idx ON werk_core.parties (tenant_id);

CREATE TABLE werk_core.persons (
    party_id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    party_type text GENERATED ALWAYS AS ('person'::text) STORED,
    given_name text NOT NULL CHECK (char_length(given_name) BETWEEN 1 AND 120),
    family_name text NOT NULL CHECK (char_length(family_name) BETWEEN 1 AND 120),
    FOREIGN KEY (tenant_id, party_id, party_type)
        REFERENCES werk_core.parties (tenant_id, id, party_type)
        ON DELETE CASCADE
);

CREATE INDEX persons_tenant_idx ON werk_core.persons (tenant_id);

CREATE TABLE werk_core.organizations (
    party_id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    party_type text GENERATED ALWAYS AS ('organization'::text) STORED,
    legal_name text NOT NULL CHECK (char_length(legal_name) BETWEEN 1 AND 200),
    FOREIGN KEY (tenant_id, party_id, party_type)
        REFERENCES werk_core.parties (tenant_id, id, party_type)
        ON DELETE CASCADE
);

CREATE INDEX organizations_tenant_idx ON werk_core.organizations (tenant_id);

CREATE TABLE werk_core.memberships (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    party_id uuid NOT NULL,
    organizational_unit_id uuid NOT NULL,
    membership_type text NOT NULL CHECK (membership_type ~ '^[a-z][a-z0-9._-]*$' AND char_length(membership_type) <= 64),
    valid_from timestamptz NOT NULL,
    valid_until timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CHECK (valid_until IS NULL OR valid_until >= valid_from),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, party_id)
        REFERENCES werk_core.parties (tenant_id, id)
        ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, organizational_unit_id)
        REFERENCES werk_core.organizational_units (tenant_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX memberships_tenant_idx ON werk_core.memberships (tenant_id);
CREATE INDEX memberships_party_idx ON werk_core.memberships (tenant_id, party_id);
CREATE INDEX memberships_unit_idx ON werk_core.memberships (tenant_id, organizational_unit_id);

REVOKE ALL ON werk_core.parties, werk_core.persons, werk_core.organizations, werk_core.memberships
    FROM PUBLIC, werk_work_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;
GRANT SELECT ON werk_core.parties, werk_core.persons, werk_core.organizations, werk_core.memberships
    TO werk_work_runtime, werk_worker_runtime;

ALTER TABLE werk_core.parties ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.parties FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.persons ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.persons FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.organizations FORCE ROW LEVEL SECURITY;
ALTER TABLE werk_core.memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.memberships FORCE ROW LEVEL SECURITY;

CREATE POLICY parties_tenant_gate
    ON werk_core.parties
    AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY parties_work_read
    ON werk_core.parties FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY parties_worker_read
    ON werk_core.parties FOR SELECT TO werk_worker_runtime USING (true);
CREATE POLICY parties_owner_all
    ON werk_core.parties TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY persons_tenant_gate
    ON werk_core.persons
    AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY persons_work_read
    ON werk_core.persons FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY persons_worker_read
    ON werk_core.persons FOR SELECT TO werk_worker_runtime USING (true);
CREATE POLICY persons_owner_all
    ON werk_core.persons TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY organizations_tenant_gate
    ON werk_core.organizations
    AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY organizations_work_read
    ON werk_core.organizations FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY organizations_worker_read
    ON werk_core.organizations FOR SELECT TO werk_worker_runtime USING (true);
CREATE POLICY organizations_owner_all
    ON werk_core.organizations TO werk_owner USING (true) WITH CHECK (true);

CREATE POLICY memberships_tenant_gate
    ON werk_core.memberships
    AS RESTRICTIVE
    TO werk_work_runtime, werk_service_runtime, werk_worker_runtime
    USING (tenant_id = werk_security.current_tenant_id())
    WITH CHECK (tenant_id = werk_security.current_tenant_id());
CREATE POLICY memberships_work_read
    ON werk_core.memberships FOR SELECT TO werk_work_runtime USING (true);
CREATE POLICY memberships_worker_read
    ON werk_core.memberships FOR SELECT TO werk_worker_runtime USING (true);
CREATE POLICY memberships_owner_all
    ON werk_core.memberships TO werk_owner USING (true) WITH CHECK (true);
