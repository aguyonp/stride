-- Owned by the platform workstream. Domain tables reference users(id) only.
CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    display_name  TEXT    NOT NULL,
    role          TEXT    NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    status        TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at    INTEGER NOT NULL,
    last_login_at INTEGER
);
