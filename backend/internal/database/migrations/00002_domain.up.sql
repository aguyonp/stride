-- Owned by the domain workstream (providers, activities, wellness, athlete data).
-- Timestamps are unix seconds (UTC); dates are YYYY-MM-DD in the athlete's timezone.

CREATE TABLE provider_connections (
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            TEXT    NOT NULL CHECK (provider IN ('garmin', 'strava')),
    status              TEXT    NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'error', 'disconnected')),
    external_id         TEXT,
    display_name        TEXT,
    key_hint            TEXT,
    secret_enc          BLOB,
    token_enc           BLOB,
    token_expires_at    INTEGER,
    scope               TEXT,
    history_from        TEXT,
    last_sync_at        INTEGER,
    last_live_at        INTEGER,
    last_new_activities INTEGER NOT NULL DEFAULT 0,
    last_error          TEXT,
    last_error_at       INTEGER,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    PRIMARY KEY (user_id, provider)
);
CREATE UNIQUE INDEX provider_connections_external ON provider_connections (provider, external_id) WHERE external_id IS NOT NULL;

CREATE TABLE activities (
    id            INTEGER PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    garmin_id     INTEGER,
    strava_id     INTEGER,
    start_utc     INTEGER NOT NULL,
    start_local   TEXT    NOT NULL,
    local_date    TEXT    NOT NULL,
    type          TEXT    NOT NULL,
    sport         TEXT    NOT NULL,
    name          TEXT    NOT NULL DEFAULT '',
    distance_m    REAL,
    duration_s    REAL,
    moving_s      REAL,
    elapsed_s     REAL,
    avg_speed     REAL,
    gap_speed     REAL,
    avg_hr        REAL,
    max_hr        REAL,
    avg_power     REAL,
    avg_cadence   REAL,
    avg_gct       REAL,
    elev_gain     REAL,
    elev_loss     REAL,
    training_load REAL,
    strava_gear   TEXT,
    complete      INTEGER NOT NULL DEFAULT 0,
    final         INTEGER NOT NULL DEFAULT 0,
    bundle_at     INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
CREATE UNIQUE INDEX activities_user_garmin ON activities (user_id, garmin_id) WHERE garmin_id IS NOT NULL;
CREATE UNIQUE INDEX activities_user_strava ON activities (user_id, strava_id) WHERE strava_id IS NOT NULL;
CREATE INDEX activities_user_start ON activities (user_id, start_local DESC);
CREATE INDEX activities_user_date ON activities (user_id, local_date);
CREATE INDEX activities_user_utc ON activities (user_id, start_utc);

CREATE TABLE activity_blobs (
    activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL CHECK (kind IN ('garmin_list', 'garmin_detail', 'garmin_details', 'fit', 'strava_detail', 'strava_streams', 'weather')),
    data        BLOB    NOT NULL,
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (activity_id, kind)
);
CREATE INDEX activity_blobs_user ON activity_blobs (user_id);

CREATE TABLE wellness_days (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date       TEXT    NOT NULL,
    kind       TEXT    NOT NULL CHECK (kind IN ('sleep', 'hrv', 'daily_summary', 'user_summary')),
    data       BLOB    NOT NULL,
    final      INTEGER NOT NULL DEFAULT 0,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, kind, date)
);
CREATE INDEX wellness_days_user_date ON wellness_days (user_id, date);

CREATE TABLE athlete_profiles (
    user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    focus_area   TEXT    NOT NULL DEFAULT 'none',
    focus_label  TEXT    NOT NULL DEFAULT '',
    injured_side TEXT,
    timezone     TEXT    NOT NULL,
    overrides    TEXT    NOT NULL DEFAULT '{}',
    pace_targets TEXT    NOT NULL DEFAULT '[]',
    updated_at   INTEGER NOT NULL
);

CREATE TABLE goals (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        TEXT    NOT NULL,
    kind         TEXT    NOT NULL,
    target_value REAL,
    target_unit  TEXT,
    target_date  TEXT,
    status       TEXT    NOT NULL DEFAULT 'active',
    note         TEXT    NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
CREATE INDEX goals_user ON goals (user_id, status);

CREATE TABLE planned_sessions (
    id                INTEGER PRIMARY KEY,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date              TEXT    NOT NULL,
    sport             TEXT    NOT NULL,
    title             TEXT    NOT NULL DEFAULT '',
    target_duration_s REAL,
    target_distance_m REAL,
    target_pace_min   REAL,
    target_pace_max   REAL,
    target_cadence    REAL,
    instructions      TEXT    NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX planned_sessions_user_date ON planned_sessions (user_id, date);

CREATE TABLE shoes (
    id             INTEGER PRIMARY KEY,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name           TEXT    NOT NULL,
    brand          TEXT    NOT NULL DEFAULT '',
    model          TEXT    NOT NULL DEFAULT '',
    drop_mm        REAL,
    type           TEXT    NOT NULL DEFAULT 'soft',
    strava_gear_id TEXT,
    retired        INTEGER NOT NULL DEFAULT 0,
    note           TEXT    NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);
CREATE INDEX shoes_user ON shoes (user_id);
CREATE UNIQUE INDEX shoes_user_gear ON shoes (user_id, strava_gear_id) WHERE strava_gear_id IS NOT NULL;

CREATE TABLE activity_notes (
    activity_id       INTEGER PRIMARY KEY REFERENCES activities(id) ON DELETE CASCADE,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pain_during       REAL,
    pain_cold         REAL,
    pain_next_morning REAL,
    rpe               REAL,
    shoe_id           INTEGER REFERENCES shoes(id) ON DELETE SET NULL,
    tags              TEXT    NOT NULL DEFAULT '[]',
    note              TEXT    NOT NULL DEFAULT '',
    updated_at        INTEGER NOT NULL
);
CREATE INDEX activity_notes_user ON activity_notes (user_id);
CREATE INDEX activity_notes_shoe ON activity_notes (shoe_id);

CREATE TABLE daily_checkins (
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date               TEXT    NOT NULL,
    training_readiness REAL,
    readiness_status   TEXT,
    note               TEXT    NOT NULL DEFAULT '',
    updated_at         INTEGER NOT NULL,
    PRIMARY KEY (user_id, date)
);
