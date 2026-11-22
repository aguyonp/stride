-- 4.3.0: per-user MCP (Model Context Protocol) access. One row per user: the master switch
-- (enabled), the SHA-256 of the active token (NULL until generated) and its short prefix,
-- which is the only form of the token ever stored or shown again.
CREATE TABLE mcp_access (
    user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled      INTEGER NOT NULL DEFAULT 0,
    token_hash   BLOB,
    token_prefix TEXT NOT NULL DEFAULT '',
    created_at   INTEGER,
    last_used_at INTEGER
);
