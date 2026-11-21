// Package mcp embeds a per-user Model Context Protocol server in Stride: the public route
// POST /mcp/{token} (Streamable HTTP, stateless) plus the /api/mcp* management endpoints.
// The token is the only credential: its SHA-256 is stored, the plaintext is returned once at
// generation and never logged.
package mcp

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned for an unknown, revoked or disabled token: the public route answers
// 404 for all three so the states are indistinguishable (same policy as share links).
var ErrNotFound = errors.New("mcp: unknown token")

// Access is the stored MCP access of a user (one row in mcp_access).
type Access struct {
	UserID      int64
	Enabled     bool
	TokenHash   []byte // nil until a token is generated
	TokenPrefix string // shown to the owner, e.g. "sm4_ab12"
	CreatedAt   *time.Time
	LastUsedAt  *time.Time
}

// Resolved is the outcome of resolving a token from the public route.
type Resolved struct {
	UserID      int64
	DisplayName string
	TokenPrefix string
}

func mcpGet(ctx context.Context, db *sql.DB, uid int64) (*Access, error) {
	var (
		a   Access
		ena int
		cre sql.NullInt64
		lst sql.NullInt64
	)
	hash := new([]byte)
	err := db.QueryRowContext(ctx, `SELECT enabled, token_hash, token_prefix, created_at, last_used_at FROM mcp_access WHERE user_id = ?`, uid).
		Scan(&ena, hash, &a.TokenPrefix, &cre, &lst)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return &Access{UserID: uid}, nil
	case err != nil:
		return nil, err
	}
	a.UserID = uid
	a.Enabled = ena != 0
	a.TokenHash = *hash
	if a.TokenHash != nil && len(a.TokenHash) == 0 {
		a.TokenHash = nil
	}
	if cre.Valid {
		t := time.Unix(cre.Int64, 0).UTC()
		a.CreatedAt = &t
	}
	if lst.Valid {
		t := time.Unix(lst.Int64, 0).UTC()
		a.LastUsedAt = &t
	}
	return &a, nil
}

func mcpSetEnabled(ctx context.Context, db *sql.DB, uid int64, enabled bool) error {
	_, err := db.ExecContext(ctx, `INSERT INTO mcp_access (user_id, enabled) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET enabled = excluded.enabled`, uid, enabled)
	return err
}

func mcpSetToken(ctx context.Context, db *sql.DB, uid int64, hash []byte, prefix string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO mcp_access (user_id, enabled, token_hash, token_prefix, created_at)
		VALUES (?, 1, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET enabled = 1, token_hash = excluded.token_hash,
			token_prefix = excluded.token_prefix, created_at = excluded.created_at`, uid, hash, prefix, now.Unix())
	return err
}

func mcpClearToken(ctx context.Context, db *sql.DB, uid int64) error {
	_, err := db.ExecContext(ctx, `UPDATE mcp_access SET token_hash = NULL, token_prefix = '', created_at = NULL WHERE user_id = ?`, uid)
	return err
}

// mcpResolve returns the owner of a token: unknown, revoked or disabled tokens all yield ErrNotFound.
func mcpResolve(ctx context.Context, db *sql.DB, hash []byte) (*Resolved, error) {
	var (
		uid    int64
		ena    int
		prefix string
		name   string
	)
	err := db.QueryRowContext(ctx, `SELECT m.user_id, m.enabled, m.token_prefix, COALESCE(u.display_name, '')
		FROM mcp_access m JOIN users u ON u.id = m.user_id WHERE m.token_hash = ?`, hash).
		Scan(&uid, &ena, &prefix, &name)
	switch {
	case errors.Is(err, sql.ErrNoRows), ena == 0:
		return nil, ErrNotFound
	case err != nil:
		return nil, err
	}
	return &Resolved{UserID: uid, DisplayName: name, TokenPrefix: prefix}, nil
}

// mcpTouchLastUsed records the last use at most once per refresh interval (one conditional
// update, a no-op outside it), so a steady stream of calls never hammers the write pool.
func mcpTouchLastUsed(ctx context.Context, db *sql.DB, uid int64, now time.Time, refresh time.Duration) error {
	_, err := db.ExecContext(ctx, `UPDATE mcp_access SET last_used_at = ?
		WHERE user_id = ? AND (last_used_at IS NULL OR last_used_at < ?)`,
		now.Unix(), uid, now.Add(-refresh).Unix())
	return err
}
