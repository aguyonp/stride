DROP INDEX IF EXISTS plan_phases_user;
DROP TABLE IF EXISTS plan_phases;

DROP INDEX IF EXISTS calendar_events_user;
DROP TABLE IF EXISTS calendar_events;

DROP TABLE IF EXISTS personal_records;

DROP INDEX IF EXISTS activity_records_user;
DROP TABLE IF EXISTS activity_records;

ALTER TABLE activities DROP COLUMN manual;
ALTER TABLE activities DROP COLUMN kudos;
