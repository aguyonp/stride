-- 4.3.0: usage management of the public MCP route. Single row (id = 1): the per-token and
-- per-IP rate limits of POST /mcp/{token} — requests per window, window in minutes —
-- tunable by an admin (GET/PUT /api/admin/mcp/quota). Defaults mirror the original constants.
CREATE TABLE mcp_quota (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    token_limit  INTEGER NOT NULL DEFAULT 100,
    token_window INTEGER NOT NULL DEFAULT 10,
    ip_limit     INTEGER NOT NULL DEFAULT 200,
    ip_window    INTEGER NOT NULL DEFAULT 10
);
INSERT INTO mcp_quota (id, token_limit, token_window, ip_limit, ip_window) VALUES (1, 100, 10, 200, 10);
