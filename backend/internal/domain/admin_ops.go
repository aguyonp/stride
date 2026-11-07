package domain

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"stride/internal/queue"
)

// Administration control center: operations an administrator runs on behalf of other users.
// Every query here is cross-user by design; the HTTP layer only exposes them behind RequireAdmin.

// Jobs exposes the job manager to the administration (listing, cancel, retry, pool load).
func (s *Services) Jobs() *queue.Manager { return s.queue }

// AdminSyncNow starts a provider sync for uid, skipping the lazy 5-minute cooldown. A sync already
// running is returned as is. ErrNotFound for unknown or disabled users, ErrNotConnected without provider.
func (s *Services) AdminSyncNow(ctx context.Context, uid int64) (*queue.JobStatus, error) {
	var status string
	err := s.db().QueryRowContext(ctx, `SELECT status FROM users WHERE id = ?`, uid).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, &ConflictError{Msg: "Compte désactivé : réactivez-le avant de lancer une synchronisation."}
	}
	g, gerr := s.getConn(ctx, uid, pGarmin)
	st, serr := s.getConn(ctx, uid, pStrava)
	if gerr != nil {
		return nil, gerr
	}
	if serr != nil {
		return nil, serr
	}
	if !g.connected() && !st.connected() {
		return nil, ErrNotConnected
	}
	jid := s.startSync(uid)
	if jid == "" {
		return nil, &ConflictError{Msg: "Synchronisation impossible à programmer pour le moment."}
	}
	return s.queue.Lookup(ctx, jid)
}

// AdminRetryJob replays a failed sync job; provider syncs go through the regular sync entry point so
// the running state stays consistent. queue.ErrNotReplayable for other kinds or states.
func (s *Services) AdminRetryJob(ctx context.Context, id string) (*queue.JobStatus, error) {
	j, err := s.queue.Lookup(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.State != queue.StateFailed || !queue.Replayable(j.Kind) {
		return nil, queue.ErrNotReplayable
	}
	if j.Kind == queue.KindSyncProvider {
		return s.AdminSyncNow(ctx, j.UserID)
	}
	return s.queue.Retry(ctx, id)
}

// AdminShareLink is a share link seen by an administrator: never the URL nor the token, only its prefix.
type AdminShareLink struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"userId"`
	UserName       string  `json:"userName"`
	Label          string  `json:"label"`
	TokenPrefix    string  `json:"tokenPrefix"`
	CreatedAt      string  `json:"createdAt"`
	ExpiresAt      *string `json:"expiresAt"`
	RevokedAt      *string `json:"revokedAt"`
	AccessCount    int     `json:"accessCount"`
	LastAccessedAt *string `json:"lastAccessedAt"`
	State          string  `json:"state"` // active | expired | revoked
	ShareScope
}

// AdminShareLinks lists the share links of every user, newest first (active only unless all).
func (s *Services) AdminShareLinks(ctx context.Context, all bool) ([]AdminShareLink, error) {
	q := `SELECT l.id, l.user_id, u.display_name, l.label, l.token_prefix, l.created_at, l.expires_at, l.revoked_at,
		l.access_count, l.last_accessed_at, l.scope_json FROM share_links l JOIN users u ON u.id = l.user_id`
	args := []any{}
	if !all {
		q += ` WHERE l.revoked_at IS NULL AND (l.expires_at IS NULL OR l.expires_at > ?)`
		args = append(args, s.now().Unix())
	}
	rows, err := s.db().QueryContext(ctx, q+` ORDER BY l.created_at DESC, l.id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := s.now().Unix()
	out := []AdminShareLink{}
	for rows.Next() {
		var l AdminShareLink
		var created int64
		var exp, rev, last sql.NullInt64
		var scope string
		if err := rows.Scan(&l.ID, &l.UserID, &l.UserName, &l.Label, &l.TokenPrefix, &created, &exp, &rev, &l.AccessCount, &last, &scope); err != nil {
			return nil, err
		}
		l.CreatedAt, l.ExpiresAt, l.RevokedAt, l.LastAccessedAt = *rfc3339(created), nullTime(exp), nullTime(rev), nullTime(last)
		l.ShareScope = parseScope(scope)
		switch {
		case rev.Valid:
			l.State = "revoked"
		case exp.Valid && exp.Int64 <= now:
			l.State = "expired"
		default:
			l.State = "active"
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// AdminRevokeShareLink revokes any user's link at once and drops its cached public view.
// It returns the owner id (for the audit trail); ErrNotFound for an unknown link.
func (s *Services) AdminRevokeShareLink(ctx context.Context, id int64) (int64, error) {
	var uid int64
	err := s.db().QueryRowContext(ctx, `SELECT user_id FROM share_links WHERE id = ?`, id).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return uid, s.DeleteShareLink(ctx, uid, id)
}

// AdminUserStats is the activity footprint of one user.
type AdminUserStats struct {
	LastSeenAt   *string  `json:"lastSeenAt"`
	Sessions     int      `json:"sessions"`
	Activities   int      `json:"activities"`
	StorageBytes int64    `json:"storageBytes"`
	Integrations []string `json:"integrations"`
}

// AdminUsersStats returns the footprint of every user: last session activity, live sessions,
// activities, stored blob bytes and connected integrations (garmin, strava, llm).
func (s *Services) AdminUsersStats(ctx context.Context) (map[int64]AdminUserStats, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT u.id,
		(SELECT MAX(last_seen_at) FROM sessions se WHERE se.user_id = u.id),
		(SELECT COUNT(*) FROM sessions se WHERE se.user_id = u.id AND se.expires_at > ?),
		(SELECT COUNT(*) FROM activities a WHERE a.user_id = u.id),
		(SELECT COALESCE(SUM(LENGTH(b.data)), 0) FROM activity_blobs b WHERE b.user_id = u.id)
			+ (SELECT COALESCE(SUM(LENGTH(w.data)), 0) FROM wellness_days w WHERE w.user_id = u.id),
		(SELECT GROUP_CONCAT(pc.provider) FROM provider_connections pc
			WHERE pc.user_id = u.id AND pc.status <> 'disconnected' AND LENGTH(pc.secret_enc) > 0),
		(SELECT COUNT(*) FROM llm_configs l WHERE l.user_id = u.id)
		FROM users u`, s.now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]AdminUserStats{}
	for rows.Next() {
		var id int64
		var seen sql.NullInt64
		var providers sql.NullString
		var llm int
		var st AdminUserStats
		if err := rows.Scan(&id, &seen, &st.Sessions, &st.Activities, &st.StorageBytes, &providers, &llm); err != nil {
			return nil, err
		}
		st.LastSeenAt = nullTime(seen)
		st.Integrations = []string{}
		for _, p := range []string{pGarmin, pStrava} {
			if providers.Valid && strings.Contains(","+providers.String+",", ","+p+",") {
				st.Integrations = append(st.Integrations, p)
			}
		}
		if llm > 0 {
			st.Integrations = append(st.Integrations, "llm")
		}
		out[id] = st
	}
	return out, rows.Err()
}

// AdminCounts are instance-wide totals for the control center overview.
type AdminCounts struct {
	Users            int `json:"users"`
	ActiveUsers      int `json:"activeUsers"`
	Admins           int `json:"admins"`
	ConnectedUsers   int `json:"connectedUsers"`
	LLMUsers         int `json:"llmUsers"`
	Activities       int `json:"activities"`
	ActiveShareLinks int `json:"activeShareLinks"`
	Sessions         int `json:"sessions"`
	FeedbackNew      int `json:"feedbackNew"`
}

func (s *Services) AdminCounts(ctx context.Context) (AdminCounts, error) {
	var c AdminCounts
	now := s.now().Unix()
	err := s.rdb().QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM users),
		(SELECT COUNT(*) FROM users WHERE status = 'active'),
		(SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active'),
		(SELECT COUNT(DISTINCT user_id) FROM provider_connections WHERE status <> 'disconnected' AND LENGTH(secret_enc) > 0),
		(SELECT COUNT(*) FROM llm_configs),
		(SELECT COUNT(*) FROM activities),
		(SELECT COUNT(*) FROM share_links WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)),
		(SELECT COUNT(*) FROM sessions WHERE expires_at > ?),
		(SELECT COUNT(*) FROM feedback WHERE status = 'new')`, now, now).
		Scan(&c.Users, &c.ActiveUsers, &c.Admins, &c.ConnectedUsers, &c.LLMUsers, &c.Activities, &c.ActiveShareLinks, &c.Sessions, &c.FeedbackNew)
	return c, err
}
