-- 4.0.0: cycle-based planning engine (macro/meso/micro), body stress map, secure share links, async job status.

CREATE TABLE training_plans (
    id                 INTEGER PRIMARY KEY,
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    goal_id            INTEGER REFERENCES goals(id) ON DELETE SET NULL,
    name               TEXT    NOT NULL,
    sport              TEXT    NOT NULL DEFAULT 'run' CHECK (sport IN ('run', 'bike', 'other')),
    start_date         TEXT    NOT NULL,
    end_date           TEXT    NOT NULL,
    target_event_label TEXT    NOT NULL DEFAULT '',
    status             TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'archived')),
    note               TEXT    NOT NULL DEFAULT '',
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);
CREATE INDEX training_plans_user ON training_plans (user_id, status, start_date DESC);

ALTER TABLE plan_phases ADD COLUMN plan_id INTEGER REFERENCES training_plans(id) ON DELETE CASCADE;
ALTER TABLE plan_phases ADD COLUMN phase_type TEXT NOT NULL DEFAULT 'build';
ALTER TABLE plan_phases ADD COLUMN order_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE plan_phases ADD COLUMN target_weekly_load REAL;
ALTER TABLE plan_phases ADD COLUMN target_weekly_duration_s REAL;
CREATE INDEX plan_phases_plan ON plan_phases (plan_id, order_index);

CREATE TABLE training_weeks (
    id                INTEGER PRIMARY KEY,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id           INTEGER REFERENCES training_plans(id) ON DELETE CASCADE,
    phase_id          INTEGER REFERENCES plan_phases(id) ON DELETE SET NULL,
    week_start        TEXT    NOT NULL,
    week_end          TEXT    NOT NULL,
    week_number       INTEGER NOT NULL DEFAULT 1,
    is_assimilation   INTEGER NOT NULL DEFAULT 0,
    target_volume_km  REAL,
    target_load       REAL,
    target_duration_s REAL,
    note              TEXT    NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX training_weeks_user ON training_weeks (user_id, week_start);
CREATE INDEX training_weeks_plan ON training_weeks (plan_id, week_start);
CREATE UNIQUE INDEX training_weeks_unique_plan ON training_weeks (user_id, plan_id, week_start) WHERE plan_id IS NOT NULL;
CREATE UNIQUE INDEX training_weeks_unique_standalone ON training_weeks (user_id, week_start) WHERE plan_id IS NULL;

ALTER TABLE planned_sessions ADD COLUMN plan_id INTEGER REFERENCES training_plans(id) ON DELETE SET NULL;
ALTER TABLE planned_sessions ADD COLUMN phase_id INTEGER REFERENCES plan_phases(id) ON DELETE SET NULL;
ALTER TABLE planned_sessions ADD COLUMN week_id INTEGER REFERENCES training_weeks(id) ON DELETE SET NULL;
CREATE INDEX planned_sessions_plan ON planned_sessions (plan_id, date);
CREATE INDEX planned_sessions_week ON planned_sessions (week_id, date);

CREATE TABLE body_stress_entries (
    id                  INTEGER PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date                TEXT    NOT NULL,
    activity_id         INTEGER REFERENCES activities(id) ON DELETE SET NULL,
    zone                TEXT    NOT NULL CHECK (zone IN ('knee_pfps', 'achilles', 'plantar_fascia', 'shin_splints', 'iliotibial_band', 'hamstrings_calves')),
    side                TEXT    NOT NULL DEFAULT 'both' CHECK (side IN ('left', 'right', 'both')),
    pain_score          REAL    NOT NULL CHECK (pain_score >= 0 AND pain_score <= 10),
    cold_next_day_score REAL    CHECK (cold_next_day_score IS NULL OR (cold_next_day_score >= 0 AND cold_next_day_score <= 10)),
    note                TEXT    NOT NULL DEFAULT '',
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);
CREATE INDEX body_stress_entries_user_date ON body_stress_entries (user_id, date DESC, id DESC);
CREATE INDEX body_stress_entries_activity ON body_stress_entries (activity_id) WHERE activity_id IS NOT NULL;

CREATE TABLE share_links (
    id               INTEGER PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label            TEXT    NOT NULL DEFAULT '',
    token_hash       BLOB    NOT NULL UNIQUE,
    token_prefix     TEXT    NOT NULL,
    url_ciphertext   BLOB    NOT NULL,
    expires_at       INTEGER,
    revoked_at       INTEGER,
    access_count     INTEGER NOT NULL DEFAULT 0,
    last_accessed_at INTEGER,
    created_at       INTEGER NOT NULL
);
CREATE INDEX share_links_user ON share_links (user_id, created_at DESC);

CREATE TABLE async_jobs (
    id           TEXT    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL,
    queue        TEXT    NOT NULL CHECK (queue IN ('critical', 'default', 'low')),
    state        TEXT    NOT NULL CHECK (state IN ('queued', 'running', 'retrying', 'completed', 'failed')),
    label        TEXT    NOT NULL DEFAULT '',
    progress_pct REAL,
    processed    INTEGER,
    total        INTEGER,
    attempts     INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    error        TEXT,
    result_json  TEXT,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
CREATE INDEX async_jobs_user ON async_jobs (user_id, created_at DESC);
