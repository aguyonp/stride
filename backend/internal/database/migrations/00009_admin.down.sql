DROP TABLE IF EXISTS feedback;
DROP INDEX IF EXISTS async_jobs_state;
ALTER TABLE async_jobs DROP COLUMN payload_json;
