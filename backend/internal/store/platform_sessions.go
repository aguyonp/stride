package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrUnknownSession = errors.New("store: unknown or expired session")

// Session rows only hold the SHA-256 of the cookie token.
type Session struct {
	TokenHash  []byte
	UserID     int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	UserAgent  string
}

func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)`, sess.TokenHash, sess.UserID, sess.CreatedAt.Unix(), sess.ExpiresAt.Unix(), sess.LastSeenAt.Unix(), sess.UserAgent)
	return err
}

// GetSession returns a non-expired session and its user.
func (s *Store) GetSession(ctx context.Context, hash []byte, now time.Time) (*Session, *User, error) {
	var (
		sess                   Session
		created, expires, seen int64
	)
	row := s.DB.QueryRowContext(ctx, `SELECT s.token_hash, s.user_id, s.created_at, s.expires_at, s.last_seen_at, s.user_agent,
		u.id, u.webauthn_user_id, u.display_name, u.role, u.status, u.onboarding_completed, u.created_at, u.last_login_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, hash, now.Unix())
	u, err := scanUser(row, []any{&sess.TokenHash, &sess.UserID, &created, &expires, &seen, &sess.UserAgent})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrUnknownSession
	}
	if err != nil {
		return nil, nil, err
	}
	sess.CreatedAt = time.Unix(created, 0).UTC()
	sess.ExpiresAt = time.Unix(expires, 0).UTC()
	sess.LastSeenAt = time.Unix(seen, 0).UTC()
	return &sess, u, nil
}

// TouchSession implements the sliding expiry.
func (s *Store) TouchSession(ctx context.Context, hash []byte, seen, expires time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`, seen.Unix(), expires.Unix(), hash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) PurgeExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
