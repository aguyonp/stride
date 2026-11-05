-- 3.1.0: kudos, manual activities, personal records, calendar events, plan phases (§12).

ALTER TABLE activities ADD COLUMN kudos INTEGER;
ALTER TABLE activities ADD COLUMN manual INTEGER NOT NULL DEFAULT 0;

-- Per-activity best efforts (computed windows + Strava best_efforts), recomputed when the bundle or the version changes.
CREATE TABLE activity_records (
    activity_id INTEGER PRIMARY KEY REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    version     INTEGER NOT NULL,
    bundle_at   INTEGER NOT NULL,
    data        TEXT    NOT NULL
);
CREATE INDEX activity_records_user ON activity_records (user_id);

-- Manual overrides of personal records.
CREATE TABLE personal_records (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT    NOT NULL CHECK (key IN ('1k', '5k', '10k', 'half', 'marathon', 'longest')),
    distance_m REAL    NOT NULL,
    time_s     REAL,
    date       TEXT,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, key)
);

CREATE TABLE calendar_events (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL CHECK (kind IN ('fatigue', 'pain', 'illness', 'unavailable', 'forced_rest')),
    start_date TEXT    NOT NULL,
    end_date   TEXT    NOT NULL,
    severity   INTEGER NOT NULL DEFAULT 2 CHECK (severity BETWEEN 1 AND 3),
    note       TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX calendar_events_user ON calendar_events (user_id, start_date);

CREATE TABLE plan_phases (
    id               INTEGER PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             TEXT    NOT NULL,
    start_date       TEXT    NOT NULL,
    end_date         TEXT    NOT NULL,
    focus            TEXT    NOT NULL DEFAULT '',
    weekly_volume_km REAL,
    note             TEXT    NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
);
CREATE INDEX plan_phases_user ON plan_phases (user_id, start_date);
