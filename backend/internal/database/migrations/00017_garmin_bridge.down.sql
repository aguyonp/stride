-- 4.7.0: revert the widened provider CHECK (§23, issue #147).
--
-- Deleting the bridge rows is the first step: the old table only accepts garmin/strava.
DELETE FROM provider_connections WHERE provider = 'garmin_web';

CREATE TABLE provider_connections_v46 (
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            TEXT    NOT NULL CHECK (provider IN ('garmin', 'strava')),
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

INSERT INTO provider_connections_v46 (
    user_id, provider, status, external_id, display_name, key_hint, secret_enc, token_enc,
    token_expires_at, scope, history_from, last_sync_at, last_live_at, last_new_activities,
    last_error, last_error_at, created_at, updated_at)
SELECT user_id, provider, status, external_id, display_name, key_hint, secret_enc, token_enc,
    token_expires_at, scope, history_from, last_sync_at, last_live_at, last_new_activities,
    last_error, last_error_at, created_at, updated_at
FROM provider_connections;

DROP TABLE provider_connections;
ALTER TABLE provider_connections_v46 RENAME TO provider_connections;
