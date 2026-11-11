-- Columns used by a foreign key cannot be dropped: planned_sessions and calendar_events are rebuilt
-- to their 4.1 shape, every 4.1 column copied as is.
DROP INDEX IF EXISTS share_notes_remark;
DROP INDEX IF EXISTS share_notes_directive;
DROP INDEX IF EXISTS share_notes_activity;
DROP INDEX IF EXISTS share_notes_user;
DROP TABLE IF EXISTS share_notes;

DROP INDEX IF EXISTS planned_sessions_share;
DROP INDEX IF EXISTS planned_sessions_week;
DROP INDEX IF EXISTS planned_sessions_plan;
DROP INDEX IF EXISTS planned_sessions_user_date;
CREATE TABLE planned_sessions_v41 (
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
    updated_at        INTEGER NOT NULL,
    steps             TEXT    NOT NULL DEFAULT '[]',
    source            TEXT    NOT NULL DEFAULT 'manual',
    garmin_status     TEXT    NOT NULL DEFAULT 'none',
    garmin_workout_id INTEGER,
    garmin_pushed_at  INTEGER,
    garmin_error      TEXT,
    plan_id           INTEGER REFERENCES training_plans(id) ON DELETE SET NULL,
    phase_id          INTEGER REFERENCES plan_phases(id) ON DELETE SET NULL,
    week_id           INTEGER REFERENCES training_weeks(id) ON DELETE SET NULL
);
INSERT INTO planned_sessions_v41 (
    id, user_id, date, sport, title, target_duration_s, target_distance_m, target_pace_min, target_pace_max, target_cadence,
    instructions, created_at, updated_at, steps, source, garmin_status, garmin_workout_id, garmin_pushed_at, garmin_error,
    plan_id, phase_id, week_id
)
SELECT
    id, user_id, date, sport, title, target_duration_s, target_distance_m, target_pace_min, target_pace_max, target_cadence,
    instructions, created_at, updated_at, steps, CASE WHEN source = 'share' THEN 'manual' ELSE source END, garmin_status,
    garmin_workout_id, garmin_pushed_at, garmin_error, plan_id, phase_id, week_id
FROM planned_sessions;
DROP TABLE planned_sessions;
ALTER TABLE planned_sessions_v41 RENAME TO planned_sessions;
CREATE INDEX planned_sessions_user_date ON planned_sessions (user_id, date);
CREATE INDEX planned_sessions_plan ON planned_sessions (plan_id, date);
CREATE INDEX planned_sessions_week ON planned_sessions (week_id, date);

DROP INDEX IF EXISTS calendar_events_share;
DROP INDEX IF EXISTS calendar_events_user;
CREATE TABLE calendar_events_v41 (
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
INSERT INTO calendar_events_v41 (id, user_id, kind, start_date, end_date, severity, note, created_at, updated_at)
SELECT id, user_id, kind, start_date, end_date, severity, note, created_at, updated_at FROM calendar_events;
DROP TABLE calendar_events;
ALTER TABLE calendar_events_v41 RENAME TO calendar_events;
CREATE INDEX calendar_events_user ON calendar_events (user_id, start_date);

ALTER TABLE share_links DROP COLUMN allow_activity_details;
ALTER TABLE share_links DROP COLUMN allow_planning;
ALTER TABLE share_links DROP COLUMN allow_comments;
