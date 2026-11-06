DROP INDEX IF EXISTS activity_metrics_user;
DROP TABLE IF EXISTS activity_metrics;

DROP INDEX IF EXISTS notification_targets_user;
DROP TABLE IF EXISTS notification_targets;

DROP INDEX IF EXISTS llm_analyses_user;
DROP TABLE IF EXISTS llm_analyses;

DROP TABLE IF EXISTS llm_configs;

DROP INDEX IF EXISTS activity_blobs_user;
CREATE TABLE activity_blobs_v2 (
    activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL CHECK (kind IN ('garmin_list', 'garmin_detail', 'garmin_details', 'fit', 'strava_detail', 'strava_streams', 'weather')),
    data        BLOB    NOT NULL,
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (activity_id, kind)
);
INSERT INTO activity_blobs_v2 (activity_id, user_id, kind, data, fetched_at)
    SELECT activity_id, user_id, kind, data, fetched_at FROM activity_blobs
    WHERE kind IN ('garmin_list', 'garmin_detail', 'garmin_details', 'fit', 'strava_detail', 'strava_streams', 'weather');
DROP TABLE activity_blobs;
ALTER TABLE activity_blobs_v2 RENAME TO activity_blobs;
CREATE INDEX activity_blobs_user ON activity_blobs (user_id);

ALTER TABLE planned_sessions DROP COLUMN garmin_error;
ALTER TABLE planned_sessions DROP COLUMN garmin_pushed_at;
ALTER TABLE planned_sessions DROP COLUMN garmin_workout_id;
ALTER TABLE planned_sessions DROP COLUMN garmin_status;
ALTER TABLE planned_sessions DROP COLUMN source;
ALTER TABLE planned_sessions DROP COLUMN steps;

ALTER TABLE shoes DROP COLUMN strava_distance_m;

ALTER TABLE activity_notes DROP COLUMN shoe_auto;

ALTER TABLE activities DROP COLUMN notified_at;
ALTER TABLE activities DROP COLUMN import_format;
