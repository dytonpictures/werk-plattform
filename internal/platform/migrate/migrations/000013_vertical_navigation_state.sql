UPDATE werk_core.account_ui_preferences
SET navigation_mode = 'bar', updated_at = now()
WHERE navigation_mode = 'grid';

ALTER TABLE werk_core.account_ui_preferences
    DROP CONSTRAINT account_ui_preferences_navigation_mode_check;
ALTER TABLE werk_core.account_ui_preferences
    ADD CONSTRAINT account_ui_preferences_navigation_mode_check
    CHECK (navigation_mode IN ('bar', 'collapsed'));

COMMENT ON COLUMN werk_core.account_ui_preferences.navigation_mode IS
    'bar is the expanded global left navigation; collapsed keeps its compact icon rail.';
