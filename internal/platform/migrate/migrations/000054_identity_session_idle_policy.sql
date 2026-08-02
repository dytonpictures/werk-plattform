-- PostgreSQL remains the authoritative source for idle-session validity. The
-- application persists activity at a bounded cadence; caches may only reduce
-- redundant attempts and never extend this timestamp.

ALTER TABLE werk_core.sessions
    ADD COLUMN last_seen_at timestamptz;

UPDATE werk_core.sessions
SET last_seen_at = created_at
WHERE last_seen_at IS NULL;

ALTER TABLE werk_core.sessions
    ALTER COLUMN last_seen_at SET DEFAULT now(),
    ALTER COLUMN last_seen_at SET NOT NULL,
    ADD CONSTRAINT sessions_last_seen_after_creation
        CHECK (last_seen_at >= created_at);

CREATE INDEX sessions_active_account_activity_idx
    ON werk_core.sessions (account_id, last_seen_at DESC)
    WHERE revoked_at IS NULL;
