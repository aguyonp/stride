-- 4.1.0 admin control center: replayable payloads of sync jobs (admin retry), a cross-user job index
-- and in-app user feedback. Only the sync kinds store their payload (dates, flags, Strava event ids):
-- never files, prompts or secrets.

ALTER TABLE async_jobs ADD COLUMN payload_json TEXT;
CREATE INDEX async_jobs_state ON async_jobs (state, updated_at DESC);

CREATE TABLE feedback (
    id            INTEGER PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category      TEXT    NOT NULL CHECK (category IN ('bug', 'idea', 'other')),
    message       TEXT    NOT NULL,
    contact_ok    INTEGER NOT NULL DEFAULT 0,
    contact_email TEXT,
    context_json  TEXT    NOT NULL DEFAULT '{}',
    status        TEXT    NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'read', 'done')),
    admin_note    TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    CHECK (contact_ok = 1 OR contact_email IS NULL)
);
CREATE INDEX feedback_status ON feedback (status, created_at DESC);
CREATE INDEX feedback_user ON feedback (user_id, created_at DESC);
