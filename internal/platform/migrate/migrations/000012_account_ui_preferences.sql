CREATE TABLE werk_core.account_ui_preferences (
    account_id uuid PRIMARY KEY REFERENCES werk_core.accounts(id) ON DELETE CASCADE,
    navigation_mode text NOT NULL DEFAULT 'bar' CHECK (navigation_mode IN ('bar', 'grid')),
    updated_at timestamptz NOT NULL DEFAULT now()
);

REVOKE ALL ON werk_core.account_ui_preferences
    FROM PUBLIC, werk_work_runtime, werk_identity_runtime, werk_admin_runtime, werk_service_runtime, werk_worker_runtime;
GRANT SELECT, INSERT, UPDATE ON werk_core.account_ui_preferences TO werk_identity_runtime;
GRANT SELECT ON werk_core.account_ui_preferences TO werk_backup_reader;

ALTER TABLE werk_core.account_ui_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE werk_core.account_ui_preferences FORCE ROW LEVEL SECURITY;

CREATE POLICY account_ui_preferences_identity_all ON werk_core.account_ui_preferences
    TO werk_identity_runtime USING (true) WITH CHECK (true);
CREATE POLICY account_ui_preferences_owner_all ON werk_core.account_ui_preferences
    TO werk_owner USING (true) WITH CHECK (true);
