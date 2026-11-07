DROP INDEX IF EXISTS async_jobs_user;
DROP TABLE IF EXISTS async_jobs;

DROP INDEX IF EXISTS share_links_user;
DROP TABLE IF EXISTS share_links;

DROP INDEX IF EXISTS body_stress_entries_activity;
DROP INDEX IF EXISTS body_stress_entries_user_date;
DROP TABLE IF EXISTS body_stress_entries;

DROP INDEX IF EXISTS planned_sessions_week;
DROP INDEX IF EXISTS planned_sessions_plan;
DROP INDEX IF EXISTS planned_sessions_user_date;
CREATE TABLE planned_sessions_v31 (
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
    steps             TEXT    NOT NULL DEFAULT '[]',
    source            TEXT    NOT NULL DEFAULT 'manual',
    garmin_status     TEXT    NOT NULL DEFAULT 'none',
    garmin_workout_id INTEGER,
    garmin_pushed_at  INTEGER,
    garmin_error      TEXT,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
INSERT INTO planned_sessions_v31 (
    id, user_id, date, sport, title, target_duration_s, target_distance_m,
    target_pace_min, target_pace_max, target_cadence, instructions, steps,
    source, garmin_status, garmin_workout_id, garmin_pushed_at, garmin_error,
    created_at, updated_at
)
SELECT
    id, user_id, date, sport, title, target_duration_s, target_distance_m,
    target_pace_min, target_pace_max, target_cadence, instructions, steps,
    source, garmin_status, garmin_workout_id, garmin_pushed_at, garmin_error,
    created_at, updated_at
FROM planned_sessions;
DROP TABLE planned_sessions;
ALTER TABLE planned_sessions_v31 RENAME TO planned_sessions;
CREATE INDEX planned_sessions_user_date ON planned_sessions (user_id, date);

DROP INDEX IF EXISTS training_weeks_unique_standalone;
DROP INDEX IF EXISTS training_weeks_unique_plan;
DROP INDEX IF EXISTS training_weeks_plan;
DROP INDEX IF EXISTS training_weeks_user;
DROP TABLE IF EXISTS training_weeks;

DROP INDEX IF EXISTS plan_phases_plan;
DROP INDEX IF EXISTS plan_phases_user;
CREATE TABLE plan_phases_v31 (
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
INSERT INTO plan_phases_v31 (id, user_id, name, start_date, end_date, focus, weekly_volume_km, note, created_at, updated_at)
SELECT id, user_id, name, start_date, end_date, focus, weekly_volume_km, note, created_at, updated_at FROM plan_phases;
DROP TABLE plan_phases;
ALTER TABLE plan_phases_v31 RENAME TO plan_phases;
CREATE INDEX plan_phases_user ON plan_phases (user_id, start_date);

DROP INDEX IF EXISTS training_plans_user;
DROP TABLE IF EXISTS training_plans;
