CREATE TABLE app_logs (
    id          INTEGER PRIMARY KEY,
    at_ms       INTEGER NOT NULL,
    category    TEXT    NOT NULL CHECK (category IN ('request', 'user_action')),
    level       TEXT    NOT NULL CHECK (level IN ('info', 'error')),
    user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    method      TEXT    NOT NULL DEFAULT '',
    target      TEXT    NOT NULL DEFAULT '',
    provider    TEXT    NOT NULL DEFAULT '',
    status      INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    action      TEXT    NOT NULL DEFAULT '',
    message     TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX app_logs_at ON app_logs (at_ms);
CREATE INDEX app_logs_filter ON app_logs (category, level, id);
CREATE INDEX app_logs_user ON app_logs (user_id, id);
