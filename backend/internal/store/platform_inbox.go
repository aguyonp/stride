package store

import (
	"context"
	"database/sql"
	"strconv"
	"time"
)

// Notification-centre retention (issue #142): settings of the admin panel, applied by the
// domain on write and by PurgeInbox when the admin changes them.
const (
	SettingInboxRetentionDays = "inbox_retention_days"
	SettingInboxRetentionMax  = "inbox_retention_max"

	DefaultInboxRetentionDays = 10
	DefaultInboxRetentionMax  = 50

	// Bounds enforced server-side (clamped, never rejected): a retention under a day or over a
	// year makes no sense, and a per-user cap outside 1..200 is either useless or unbounded.
	MinInboxRetentionDays = 1
	MaxInboxRetentionDays = 365
	MinInboxRetentionMax  = 1
	MaxInboxRetentionMax  = 200
)

// ClampInboxRetention keeps the pair inside the supported bounds.
func ClampInboxRetention(days, max int) (int, int) {
	return clampInt(days, MinInboxRetentionDays, MaxInboxRetentionDays),
		clampInt(max, MinInboxRetentionMax, MaxInboxRetentionMax)
}

func clampInt(v, lo, hi int) int {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	default:
		return v
	}
}

// InboxRetention returns the stored retention, with the defaults when a key is missing or
// unreadable: the notification centre must never fail because of its settings.
func (s *Store) InboxRetention(ctx context.Context) (days, max int) {
	days, max = DefaultInboxRetentionDays, DefaultInboxRetentionMax
	if v, ok, err := s.Setting(ctx, SettingInboxRetentionDays); err == nil && ok {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	if v, ok, err := s.Setting(ctx, SettingInboxRetentionMax); err == nil && ok {
		if n, err := strconv.Atoi(v); err == nil {
			max = n
		}
	}
	return ClampInboxRetention(days, max)
}

// PurgeInbox applies the retention to every user (admin settings changed). It returns the
// number of deleted rows.
func (s *Store) PurgeInbox(ctx context.Context, days, max int, now time.Time) (int64, error) {
	return purgeInbox(ctx, s.DB, 0, days, max, now)
}

// PurgeUserInbox applies the retention to one user (after every write).
func (s *Store) PurgeUserInbox(ctx context.Context, uid int64, days, max int, now time.Time) (int64, error) {
	return purgeInbox(ctx, s.DB, uid, days, max, now)
}

// purgeInbox deletes the entries older than the window, then the ones beyond the per-user cap
// (the newest are kept). uid 0 means every user; any other value scopes both statements.
func purgeInbox(ctx context.Context, db *sql.DB, uid int64, days, max int, now time.Time) (int64, error) {
	days, max = ClampInboxRetention(days, max)
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
	scope, scopeArg := "", []any{}
	if uid > 0 {
		scope, scopeArg = " AND user_id = ?", []any{uid}
	}
	res, err := db.ExecContext(ctx, `DELETE FROM inbox_entries WHERE created_at < ?`+scope, append([]any{cutoff}, scopeArg...)...)
	if err != nil {
		return 0, err
	}
	deleted, _ := res.RowsAffected()

	// Rank per user, newest first, and drop everything past the cap.
	inner := "SELECT id, ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_at DESC, id DESC) AS rn FROM inbox_entries"
	capArgs := append([]any{}, scopeArg...)
	if uid > 0 {
		inner += " WHERE user_id = ?"
		capArgs = append(capArgs, uid)
	}
	capArgs = append(capArgs, max)
	res, err = db.ExecContext(ctx, `DELETE FROM inbox_entries WHERE 1 = 1`+scope+` AND id NOT IN (SELECT id FROM (`+inner+`) WHERE rn <= ?)`, capArgs...)
	if err != nil {
		return deleted, err
	}
	capped, _ := res.RowsAffected()
	return deleted + capped, nil
}
