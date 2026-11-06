-- Owned by the platform workstream. Domain tables reference users(id) only.
CREATE TABLE users (
    id                   INTEGER PRIMARY KEY,
    webauthn_user_id     BLOB    UNIQUE,
    display_name         TEXT    NOT NULL,
    role                 TEXT    NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    status               TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    onboarding_completed INTEGER NOT NULL DEFAULT 0,
    created_at           INTEGER NOT NULL,
    last_login_at        INTEGER
);

CREATE TABLE webauthn_credentials (
    id              BLOB    PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT    NOT NULL,
    credential      BLOB    NOT NULL,
    backup_eligible INTEGER NOT NULL DEFAULT 0,
    backup_state    INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER
);
CREATE INDEX webauthn_credentials_user ON webauthn_credentials(user_id);

CREATE TABLE sessions (
    token_hash   BLOB    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    user_agent   TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE INDEX sessions_expires ON sessions(expires_at);

CREATE TABLE invitations (
    id          INTEGER PRIMARY KEY,
    code_hash   BLOB    NOT NULL UNIQUE,
    code_prefix TEXT    NOT NULL,
    label       TEXT    NOT NULL DEFAULT '',
    max_uses    INTEGER NOT NULL DEFAULT 1 CHECK (max_uses >= 1),
    uses        INTEGER NOT NULL DEFAULT 0,
    expires_at  INTEGER,
    revoked     INTEGER NOT NULL DEFAULT 0,
    created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  INTEGER NOT NULL
);

CREATE TABLE app_settings (
    key        TEXT    PRIMARY KEY,
    value      TEXT    NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE api_events (
    id          INTEGER PRIMARY KEY,
    at_ms       INTEGER NOT NULL,
    provider    TEXT    NOT NULL,
    user_id     INTEGER,
    endpoint    TEXT    NOT NULL,
    status      INTEGER NOT NULL,
    message     TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX api_events_at ON api_events(at_ms);
