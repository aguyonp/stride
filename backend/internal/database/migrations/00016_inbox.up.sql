-- 4.7.0: per-user notification centre (issue #142). The toast stays the immediate signal;
-- this table is its memory. Two sources for now: the biomechanical / load alerts already
-- published over SSE, and the writes of the physio/coach portal.
--
-- code anchors the client-side i18n of the entries the server cannot know the language of
-- (a portal write is stored once, for a user whose interface may change): share writes store the
-- share kind, update entries store the version, alerts store their analysis code.
--
-- dedup_key is stable per (kind, target) — an alert keeps one entry per day, activity and code,
-- a portal write one entry per target — so a refresh moves the entry back to the top and marks
-- it unread instead of stacking duplicates. Empty keys are never deduplicated.
-- Retention (10 days / 50 entries by default) is applied by the domain on write and when the
-- admin settings change.
CREATE TABLE inbox_entries (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL,
    level       TEXT    NOT NULL,
    title       TEXT    NOT NULL,
    detail      TEXT    NOT NULL DEFAULT '',
    url         TEXT    NOT NULL DEFAULT '',
    code        TEXT    NOT NULL DEFAULT '',
    dedup_key   TEXT    NOT NULL DEFAULT '',
    activity_id INTEGER,
    date        TEXT,
    created_at  INTEGER NOT NULL,
    read_at     INTEGER
);
CREATE UNIQUE INDEX inbox_entries_dedup ON inbox_entries(user_id, dedup_key) WHERE dedup_key <> '';
CREATE INDEX inbox_entries_created ON inbox_entries(user_id, created_at DESC, id DESC);
CREATE INDEX inbox_entries_unread ON inbox_entries(user_id, read_at) WHERE read_at IS NULL;
