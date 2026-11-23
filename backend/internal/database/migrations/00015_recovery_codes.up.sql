-- 4.4.0: one-time recovery codes (issue #56). One row per code: only the SHA-256 of the
-- normalized code is stored; the plaintext is shown to the user once, at generation, and
-- is never recoverable. used_at NULL = unused; the atomic UPDATE ... WHERE used_at IS NULL
-- in the store layer makes each code strictly single-use.
CREATE TABLE recovery_codes (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  BLOB NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    used_at    INTEGER
);
CREATE INDEX recovery_codes_user_id ON recovery_codes(user_id);
