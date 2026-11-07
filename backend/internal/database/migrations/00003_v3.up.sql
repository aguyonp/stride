-- V3: reconciliation markers, LLM connector, structured workouts, notifications, rescue imports.
-- Additive only; activity_blobs is rebuilt (data preserved) because its kind CHECK gains import kinds.

ALTER TABLE activities ADD COLUMN import_format TEXT;
ALTER TABLE activities ADD COLUMN notified_at INTEGER;

ALTER TABLE activity_notes ADD COLUMN shoe_auto INTEGER NOT NULL DEFAULT 0;

ALTER TABLE shoes ADD COLUMN strava_distance_m REAL;

ALTER TABLE planned_sessions ADD COLUMN steps TEXT NOT NULL DEFAULT '[]';
ALTER TABLE planned_sessions ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE planned_sessions ADD COLUMN garmin_status TEXT NOT NULL DEFAULT 'none';
ALTER TABLE planned_sessions ADD COLUMN garmin_workout_id INTEGER;
ALTER TABLE planned_sessions ADD COLUMN garmin_pushed_at INTEGER;
ALTER TABLE planned_sessions ADD COLUMN garmin_error TEXT;

CREATE TABLE activity_blobs_v3 (
    activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL CHECK (kind IN ('garmin_list', 'garmin_detail', 'garmin_details', 'fit', 'strava_detail', 'strava_streams', 'weather',
                                                 'import_fit', 'import_gpx', 'import_tcx')),
    data        BLOB    NOT NULL,
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (activity_id, kind)
);
INSERT INTO activity_blobs_v3 (activity_id, user_id, kind, data, fetched_at)
    SELECT activity_id, user_id, kind, data, fetched_at FROM activity_blobs;
DROP TABLE activity_blobs;
ALTER TABLE activity_blobs_v3 RENAME TO activity_blobs;
CREATE INDEX activity_blobs_user ON activity_blobs (user_id);

CREATE TABLE llm_configs (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    provider   TEXT    NOT NULL CHECK (provider IN ('gemini', 'anthropic', 'openai', 'openai_compat')),
    model      TEXT    NOT NULL,
    base_url   TEXT,
    key_enc    BLOB,
    key_hint   TEXT,
    updated_at INTEGER NOT NULL
);

CREATE TABLE llm_analyses (
    id            INTEGER PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          TEXT    NOT NULL,
    ref           TEXT    NOT NULL,
    provider      TEXT    NOT NULL,
    model         TEXT    NOT NULL,
    markdown      TEXT    NOT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);
CREATE INDEX llm_analyses_user ON llm_analyses (user_id, kind, ref, created_at DESC);

CREATE TABLE notification_targets (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL CHECK (kind IN ('discord', 'telegram', 'json')),
    label        TEXT    NOT NULL DEFAULT '',
    enabled      INTEGER NOT NULL DEFAULT 1,
    secret_enc   BLOB,
    hint         TEXT    NOT NULL DEFAULT '',
    last_status  TEXT,
    last_sent_at INTEGER,
    last_error   TEXT,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
CREATE INDEX notification_targets_user ON notification_targets (user_id);

-- Activities already present before V3 are never notified.
UPDATE activities SET notified_at = updated_at;

-- Per-activity efficiency / impact metrics, recomputed when the bundle or the formula version changes.
CREATE TABLE activity_metrics (
    activity_id INTEGER PRIMARY KEY REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    version     INTEGER NOT NULL,
    bundle_at   INTEGER NOT NULL,
    data        TEXT    NOT NULL
);
CREATE INDEX activity_metrics_user ON activity_metrics (user_id);
