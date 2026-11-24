package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"time"

	"stride/internal/auth"
	"stride/internal/store"
)

// TokenPrefix marks a MCP token (its first characters are the only part shown again).
const TokenPrefix = "sm4_"

const lastUsedRefresh = 5 * time.Minute

// Service manages the per-user MCP access (switch, token lifecycle) and resolves public tokens.
type Service struct {
	db   *sql.DB
	read *sql.DB
	log  *slog.Logger
	now  func() time.Time
}

// Status is what the account screen shows.
type Status struct {
	Enabled     bool       `json:"enabled"`
	HasToken    bool       `json:"hasToken"`
	TokenPrefix string     `json:"tokenPrefix"`
	CreatedAt   *time.Time `json:"createdAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	// The effective public-route quota, so the account card quotes the real limits instead
	// of a hardcoded sentence (issue #136).
	Quota Quota `json:"quota"`
}

func NewService(st *store.Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: st.DB, read: st.R, log: log, now: time.Now}
}

// GetQuota loads the stored public-route quota (defaults when the row is missing).
func (s *Service) GetQuota(ctx context.Context) (Quota, error) {
	return mcpGetQuota(ctx, s.read)
}

// Status returns the MCP access state of a user, with the effective public-route quota
// (defaults when the row is unreadable, like Server.New).
func (s *Service) Status(ctx context.Context, uid int64) (Status, error) {
	a, err := mcpGet(ctx, s.read, uid)
	if err != nil {
		return Status{}, err
	}
	q, qerr := s.GetQuota(ctx)
	if qerr != nil {
		q = DefaultQuota
	}
	return Status{Enabled: a.Enabled, HasToken: a.TokenHash != nil, TokenPrefix: a.TokenPrefix,
		CreatedAt: a.CreatedAt, LastUsedAt: a.LastUsedAt, Quota: q}, nil
}

// SetEnabled turns the MCP access on or off. Disabling keeps the token: re-enabling reuses it.
func (s *Service) SetEnabled(ctx context.Context, uid int64, enabled bool) error {
	return mcpSetEnabled(ctx, s.db, uid, enabled)
}

// Generate creates (or replaces) the user's token and enables MCP. The plaintext is returned
// exactly once; only the hash is kept.
func (s *Service) Generate(ctx context.Context, uid int64) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	hash := sha256.Sum256([]byte(token))
	if err := mcpSetToken(ctx, s.db, uid, hash[:], token[:len(TokenPrefix)+4], s.now()); err != nil {
		return "", err
	}
	return token, nil
}

// Revoke deletes the token; the master switch is left as is.
func (s *Service) Revoke(ctx context.Context, uid int64) error {
	return mcpClearToken(ctx, s.db, uid)
}

// Resolve maps a public token to its owner. Unknown, revoked and disabled tokens all yield
// ErrNotFound (the route answers a uniform 404).
func (s *Service) Resolve(ctx context.Context, token string) (Resolved, auth.User, error) {
	hash := sha256.Sum256([]byte(token))
	r, err := mcpResolve(ctx, s.read, hash[:])
	if err != nil {
		return Resolved{}, auth.User{}, err
	}
	return *r, auth.User{ID: r.UserID, DisplayName: r.DisplayName, Role: auth.RoleUser}, nil
}

// Touch records that the token was used (throttled to lastUsedRefresh).
func (s *Service) Touch(ctx context.Context, uid int64) {
	if err := mcpTouchLastUsed(ctx, s.db, uid, s.now(), lastUsedRefresh); err != nil {
		s.log.Warn("⚠️ MCP : dernier usage non consigné", "err", err.Error())
	}
}
