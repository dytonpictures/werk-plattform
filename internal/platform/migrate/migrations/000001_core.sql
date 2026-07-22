CREATE TABLE werk_core.tenants (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'suspended', 'archived')),
    default_locale text NOT NULL DEFAULT 'de-DE',
    default_timezone text NOT NULL DEFAULT 'Europe/Berlin',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE werk_core.organizational_units (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES werk_core.tenants(id),
    parent_id uuid NULL REFERENCES werk_core.organizational_units(id),
    unit_type text NOT NULL,
    name text NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id)
);

CREATE INDEX organizational_units_tenant_idx ON werk_core.organizational_units (tenant_id);

CREATE TABLE werk_core.admin_subjects (
    id uuid PRIMARY KEY,
    display_name text NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
