DROP TABLE IF EXISTS daily_checkins;

DROP INDEX IF EXISTS activity_notes_shoe;
DROP INDEX IF EXISTS activity_notes_user;
DROP TABLE IF EXISTS activity_notes;

DROP INDEX IF EXISTS shoes_user_gear;
DROP INDEX IF EXISTS shoes_user;
DROP TABLE IF EXISTS shoes;

DROP INDEX IF EXISTS planned_sessions_user_date;
DROP TABLE IF EXISTS planned_sessions;

DROP INDEX IF EXISTS goals_user;
DROP TABLE IF EXISTS goals;

DROP TABLE IF EXISTS athlete_profiles;

DROP INDEX IF EXISTS wellness_days_user_date;
DROP TABLE IF EXISTS wellness_days;

DROP INDEX IF EXISTS activity_blobs_user;
DROP TABLE IF EXISTS activity_blobs;

DROP INDEX IF EXISTS activities_user_utc;
DROP INDEX IF EXISTS activities_user_date;
DROP INDEX IF EXISTS activities_user_start;
DROP INDEX IF EXISTS activities_user_strava;
DROP INDEX IF EXISTS activities_user_garmin;
DROP TABLE IF EXISTS activities;

DROP INDEX IF EXISTS provider_connections_external;
DROP TABLE IF EXISTS provider_connections;
