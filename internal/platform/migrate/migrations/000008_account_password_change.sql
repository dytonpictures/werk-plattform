ALTER TABLE werk_core.accounts
    ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN werk_core.accounts.must_change_password IS
    'Temporary bootstrap state; interactive work is blocked until the credential is changed.';
