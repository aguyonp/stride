-- 4.1.0: health status (golden source of the profile focus) and share link scopes.

CREATE TABLE health_status (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    zones      TEXT    NOT NULL DEFAULT '[]',
    updated_at INTEGER NOT NULL
);

ALTER TABLE share_links ADD COLUMN scope_json TEXT NOT NULL DEFAULT '{}';
