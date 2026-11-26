package store

import (
	"context"
	"testing"
	"time"
)

// insertInbox writes an entry directly: the store tests only exercise the retention queries.
func insertInbox(t *testing.T, s *Store, uid int64, createdAt time.Time) {
	t.Helper()
	if _, err := s.DB.Exec(`INSERT INTO inbox_entries (user_id, kind, level, title, created_at) VALUES (?, 'alert', 'info', 'x', ?)`,
		uid, createdAt.Unix()); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func countInbox(t *testing.T, s *Store, uid int64) int {
	t.Helper()
	var n int
	if err := s.R.QueryRow(`SELECT COUNT(*) FROM inbox_entries WHERE user_id = ?`, uid).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestInboxRetentionDefaultsAndClamp(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)

	// Nothing stored yet: the compiled defaults.
	if days, max := s.InboxRetention(ctx); days != DefaultInboxRetentionDays || max != DefaultInboxRetentionMax {
		t.Fatalf("defaults: %d/%d", days, max)
	}
	// Stored values are clamped, never trusted as-is.
	if err := s.SetSetting(ctx, SettingInboxRetentionDays, "0", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, SettingInboxRetentionMax, "9999", time.Now()); err != nil {
		t.Fatal(err)
	}
	if days, max := s.InboxRetention(ctx); days != MinInboxRetentionDays || max != MaxInboxRetentionMax {
		t.Fatalf("clamped: %d/%d", days, max)
	}
	// An unparsable value falls back to the default, and never fails the reader.
	if err := s.SetSetting(ctx, SettingInboxRetentionDays, "beaucoup", time.Now()); err != nil {
		t.Fatal(err)
	}
	if days, _ := s.InboxRetention(ctx); days != DefaultInboxRetentionDays {
		t.Fatalf("garbage value: %d", days)
	}
	// The clamp has no "zero means default" magic: a stored 0 is pulled up to the minimum, the
	// admin handler is the one that keeps the current value when a field is omitted.
	if days, max := ClampInboxRetention(0, 0); days != MinInboxRetentionDays || max != MinInboxRetentionMax {
		t.Fatalf("zero clamps to the minimum: %d/%d", days, max)
	}
}

func TestPurgeInboxAgeAndCap(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	now := time.Now()
	for _, uid := range []int64{1, 2} {
		if _, err := s.DB.Exec(`INSERT INTO users (id, display_name, role, created_at) VALUES (?, ?, 'user', 1)`, uid, "u"); err != nil {
			t.Fatal(err)
		}
	}
	// 4 old entries (past the 10-day window), 6 fresh ones per user.
	for _, uid := range []int64{1, 2} {
		for i := 0; i < 4; i++ {
			insertInbox(t, s, uid, now.AddDate(0, 0, -11))
		}
		for i := 0; i < 6; i++ {
			insertInbox(t, s, uid, now.Add(-time.Duration(i)*time.Hour))
		}
	}

	deleted, err := s.PurgeUserInbox(ctx, 1, 10, 50, now)
	if err != nil {
		t.Fatalf("purge user: %v", err)
	}
	if deleted != 4 || countInbox(t, s, 1) != 6 {
		t.Fatalf("age purge: deleted=%d left=%d", deleted, countInbox(t, s, 1))
	}
	// The other user is untouched by a per-user purge.
	if countInbox(t, s, 2) != 10 {
		t.Fatalf("other user: %d", countInbox(t, s, 2))
	}

	// The cap keeps the newest entries only.
	if _, err := s.PurgeUserInbox(ctx, 1, 10, 3, now); err != nil {
		t.Fatalf("cap purge: %v", err)
	}
	if n := countInbox(t, s, 1); n != 3 {
		t.Fatalf("cap purge left %d", n)
	}
	var newest bool
	if err := s.R.QueryRow(`SELECT COUNT(*) = 0 FROM inbox_entries WHERE user_id = 1 AND created_at < ?`, now.Add(-3*time.Hour).Unix()).Scan(&newest); err != nil || !newest {
		t.Fatalf("kept the oldest entries instead of the newest: %v %v", newest, err)
	}

	// The global purge applies the same bounds to every user.
	if _, err := s.PurgeInbox(ctx, 10, 2, now); err != nil {
		t.Fatalf("global purge: %v", err)
	}
	for _, uid := range []int64{1, 2} {
		if n := countInbox(t, s, uid); n != 2 {
			t.Fatalf("global purge user %d: %d", uid, n)
		}
	}
}
