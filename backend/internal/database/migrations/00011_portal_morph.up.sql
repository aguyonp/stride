-- 4.2.0 (§15.1, §15.5): physio/coach portal and Smart Workout Morphing.
-- Additive only: share link permissions (off for existing links), third-party notes, the share link
-- that prescribed a planned session or calendar event (with a label snapshot that survives the link)
-- and the planned trace kept when a session is morphed.

ALTER TABLE share_links ADD COLUMN allow_comments INTEGER NOT NULL DEFAULT 0;
ALTER TABLE share_links ADD COLUMN allow_planning INTEGER NOT NULL DEFAULT 0;
ALTER TABLE share_links ADD COLUMN allow_activity_details INTEGER NOT NULL DEFAULT 0;

-- One directive per link and one remark per (link, activity): upserts, enforced by the partial indexes.
CREATE TABLE share_notes (
    id                INTEGER PRIMARY KEY,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    share_link_id     INTEGER REFERENCES share_links(id) ON DELETE SET NULL,
    link_label        TEXT    NOT NULL DEFAULT '',
    kind              TEXT    NOT NULL CHECK (kind IN ('directive', 'remark')),
    activity_id       INTEGER REFERENCES activities(id) ON DELETE CASCADE,
    author_name       TEXT,
    author_role       TEXT    NOT NULL DEFAULT 'other' CHECK (author_role IN ('physio', 'coach', 'other')),
    body              TEXT    NOT NULL,
    restrictions_json TEXT,
    valid_until       TEXT,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    CHECK ((kind = 'remark') = (activity_id IS NOT NULL))
);
CREATE INDEX share_notes_user ON share_notes (user_id, kind, updated_at DESC);
CREATE INDEX share_notes_activity ON share_notes (activity_id) WHERE activity_id IS NOT NULL;
CREATE UNIQUE INDEX share_notes_directive ON share_notes (share_link_id) WHERE kind = 'directive' AND share_link_id IS NOT NULL;
CREATE UNIQUE INDEX share_notes_remark ON share_notes (share_link_id, activity_id) WHERE kind = 'remark' AND share_link_id IS NOT NULL;

ALTER TABLE planned_sessions ADD COLUMN share_link_id INTEGER REFERENCES share_links(id) ON DELETE SET NULL;
ALTER TABLE planned_sessions ADD COLUMN prescribed_label TEXT;
ALTER TABLE planned_sessions ADD COLUMN prescribed_author TEXT;
ALTER TABLE planned_sessions ADD COLUMN morph_json TEXT;
CREATE INDEX planned_sessions_share ON planned_sessions (share_link_id) WHERE share_link_id IS NOT NULL;

ALTER TABLE calendar_events ADD COLUMN share_link_id INTEGER REFERENCES share_links(id) ON DELETE SET NULL;
ALTER TABLE calendar_events ADD COLUMN prescribed_label TEXT;
ALTER TABLE calendar_events ADD COLUMN prescribed_author TEXT;
CREATE INDEX calendar_events_share ON calendar_events (share_link_id) WHERE share_link_id IS NOT NULL;
