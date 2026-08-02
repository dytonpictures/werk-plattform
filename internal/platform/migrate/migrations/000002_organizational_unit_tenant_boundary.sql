ALTER TABLE werk_core.organizational_units
    DROP CONSTRAINT organizational_units_parent_id_fkey;

ALTER TABLE werk_core.organizational_units
    ADD CONSTRAINT organizational_units_parent_same_tenant_fk
    FOREIGN KEY (tenant_id, parent_id)
    REFERENCES werk_core.organizational_units (tenant_id, id)
    ON DELETE RESTRICT;

CREATE INDEX organizational_units_parent_idx
    ON werk_core.organizational_units (tenant_id, parent_id)
    WHERE parent_id IS NOT NULL;
