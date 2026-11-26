-- 4.7.0: experimental Garmin Connect bridge (§23, issue #147).
--
-- The bridge tokens live beside the Personal API credentials, never in them: a dedicated provider
-- row, so the user switch is the presence of a row and the decommission is a DELETE. Widening the
-- CHECK is the only schema change; no new table.
CREATE TABLE provider_connections_v47 (
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            TEXT    NOT NULL CHECK (provider IN ('garmin', 'strava', 'garmin_web')),
    status              TEXT    NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'error', 'disconnected')),
    external_id         TEXT,
    display_name        TEXT,
    key_hint            TEXT,
    secret_enc          BLOB,
    token_enc           BLOB,
    token_expires_at    INTEGER,
    scope               TEXT,
    history_from        TEXT,
    last_sync_at        INTEGER,
    last_live_at        INTEGER,
    last_new_activities INTEGER NOT NULL DEFAULT 0,
    last_error          TEXT,
    last_error_at       INTEGER,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    PRIMARY KEY (user_id, provider)
);

INSERT INTO provider_connections_v47 (
    user_id, provider, status, external_id, display_name, key_hint, secret_enc, token_enc,
    token_expires_at, scope, history_from, last_sync_at, last_live_at, last_new_activities,
    last_error, last_error_at, created_at, updated_at)
SELECT user_id, provider, status, external_id, display_name, key_hint, secret_enc, token_enc,
    token_expires_at, scope, history_from, last_sync_at, last_live_at, last_new_activities,
    last_error, last_error_at, created_at, updated_at
FROM provider_connections;

DROP TABLE provider_connections;
ALTER TABLE provider_connections_v47 RENAME TO provider_connections;
