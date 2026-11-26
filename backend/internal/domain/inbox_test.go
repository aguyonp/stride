package domain

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"stride/internal/cache"
	"stride/internal/env"
	"stride/internal/secret"
	"stride/internal/store"
)

// inboxServices builds a real store (migrations included) with two users and no provider.
func inboxServices(t *testing.T) *Services {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/inbox.db")
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	sealer, err := secret.New(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Deps{Env: env.Config{Timezone: "Europe/Paris"}, Store: st, Cache: cache.NewMemory(), Sealer: sealer,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close(ctx)
		st.Close()
	})
	for _, uid := range []int64{1, 2} {
		if _, err := st.DB.Exec(`INSERT INTO users (id, display_name, role, created_at) VALUES (?, ?, 'user', 1)`, uid, "u"); err != nil {
			t.Fatal(err)
		}
	}
	s.Now = func() time.Time { return time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC) }
	return s
}

func TestEmitAlertFeedsTheInboxOnce(t *testing.T) {
	ctx := context.Background()
	s := inboxServices(t)

	s.emitAlert(ctx, 1, nil, "2026-11-24", "warn", "CADENCE_LOW", "Cadence basse", "168 spm sur 20 min")
	page, err := s.Inbox(ctx, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Unread != 1 {
		t.Fatalf("inbox after one alert: %+v", page)
	}
	e := page.Items[0]
	if e.Kind != InboxKindAlert || e.Level != "warn" || e.Title != "Cadence basse" || e.Detail != "168 spm sur 20 min" {
		t.Fatalf("entry: %+v", e)
	}
	if e.Date == nil || *e.Date != "2026-11-24" || e.ActivityID != nil || e.ReadAt != nil {
		t.Fatalf("entry target: %+v", e)
	}

	// Same alert again inside the 6 h window: the SSE dedup keeps the toast away and the centre
	// does not grow.
	s.emitAlert(ctx, 1, nil, "2026-11-24", "warn", "CADENCE_LOW", "Cadence basse", "168 spm sur 20 min")
	if page, _ = s.Inbox(ctx, 1, 0, 0); len(page.Items) != 1 {
		t.Fatalf("duplicate alert: %+v", page.Items)
	}

	// Another code on the same day is a distinct entry, and the level mapping is applied.
	s.emitAlert(ctx, 1, nil, "2026-11-24", "danger", "LOAD_RATIO", "Charge élevée", "ACWR 1,6")
	if page, _ = s.Inbox(ctx, 1, 0, 0); len(page.Items) != 2 || page.Items[0].Level != "error" {
		t.Fatalf("second alert: %+v", page.Items)
	}

	// Another user stays empty.
	if other, _ := s.Inbox(ctx, 2, 0, 0); len(other.Items) != 0 || other.Unread != 0 {
		t.Fatalf("isolation: %+v", other)
	}
}

func TestWriteInboxRefreshesTheSameTarget(t *testing.T) {
	ctx := context.Background()
	s := inboxServices(t)
	date := "2026-11-23"
	aid := int64(42)

	first := s.writeInbox(ctx, 1, inboxInput{Kind: InboxKindShare, Level: "info", Title: shareInboxTitle("remark"),
		Code: "remark", Detail: "Kiné", ActivityID: &aid, Date: &date, DedupKey: shareInboxKey(7, aid, "remark")})
	if first == nil || first.ID == 0 {
		t.Fatalf("first write: %+v", first)
	}
	if _, err := s.MarkAllInboxRead(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// Editing the same remark again re-surfaces the same row instead of stacking a second one.
	later := time.Date(2026, 11, 24, 11, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return later }
	second := s.writeInbox(ctx, 1, inboxInput{Kind: InboxKindShare, Level: "info", Title: shareInboxTitle("remark"),
		Code: "remark", Detail: "Kiné (modifié)", ActivityID: &aid, Date: &date, DedupKey: shareInboxKey(7, aid, "remark")})
	if second == nil || second.ID != first.ID || second.CreatedAt != later.UTC().Format(time.RFC3339) {
		t.Fatalf("refreshed: %+v (first %+v)", second, first)
	}
	page, err := s.Inbox(ctx, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Unread != 1 || !strings.Contains(page.Items[0].Detail, "modifié") {
		t.Fatalf("after refresh: %+v", page)
	}
	// A different target is a different entry.
	other := int64(43)
	s.writeInbox(ctx, 1, inboxInput{Kind: InboxKindShare, Level: "info", Title: shareInboxTitle("remark"),
		Code: "remark", ActivityID: &other, DedupKey: shareInboxKey(7, other, "remark")})
	if page, _ = s.Inbox(ctx, 1, 0, 0); len(page.Items) != 2 {
		t.Fatalf("second target: %+v", page.Items)
	}
}

func TestInboxMarkReadAndPagination(t *testing.T) {
	ctx := context.Background()
	s := inboxServices(t)
	for i := 0; i < 5; i++ {
		s.Now = func() time.Time { return time.Date(2026, 11, 24, 8, i, 0, 0, time.UTC) }
		s.writeInbox(ctx, 1, inboxInput{Kind: InboxKindAlert, Level: "info", Title: "T" + string(rune('a'+i))})
	}

	page, err := s.Inbox(ctx, 1, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore || page.Unread != 5 {
		t.Fatalf("first page: %+v", page)
	}
	if page.Items[0].Title != "Te" || page.Items[1].Title != "Td" {
		t.Fatalf("newest first: %+v", page.Items)
	}
	next, err := s.Inbox(ctx, 1, 2, page.Items[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 2 || next.Items[0].Title != "Tc" || !next.HasMore {
		t.Fatalf("second page: %+v", next.Items)
	}

	// Marking one entry read leaves the others unread; another user's ids change nothing.
	unread, err := s.MarkInboxRead(ctx, 1, []int64{page.Items[0].ID})
	if err != nil || unread != 4 {
		t.Fatalf("mark one: %d %v", unread, err)
	}
	unread, err = s.MarkInboxRead(ctx, 2, []int64{page.Items[1].ID})
	if err != nil || unread != 0 {
		t.Fatalf("mark for another user: %d %v", unread, err)
	}
	if unread, _ = s.UnreadInbox(ctx, 1); unread != 4 {
		t.Fatalf("isolation of the read flag: %d", unread)
	}
	// Marking twice is harmless; read-all empties the badge.
	if unread, _ = s.MarkInboxRead(ctx, 1, []int64{page.Items[0].ID}); unread != 4 {
		t.Fatalf("idempotent mark: %d", unread)
	}
	if unread, err = s.MarkAllInboxRead(ctx, 1); err != nil || unread != 0 {
		t.Fatalf("read all: %d %v", unread, err)
	}
	// The history is kept.
	if page, _ = s.Inbox(ctx, 1, 0, 0); len(page.Items) != 5 || page.Unread != 0 {
		t.Fatalf("after read all: %+v", page)
	}
	if _, err := s.MarkInboxRead(ctx, 1, []int64{0}); err == nil {
		t.Fatal("invalid id accepted")
	}
}

func TestInboxRetentionOnWrite(t *testing.T) {
	ctx := context.Background()
	s := inboxServices(t)
	// 2 entries max, 1 day of history.
	for _, kv := range [][2]string{{store.SettingInboxRetentionMax, "2"}, {store.SettingInboxRetentionDays, "1"}} {
		if err := s.deps.Store.SetSetting(ctx, kv[0], kv[1], time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		s.Now = func() time.Time { return time.Date(2026, 11, 24, 8, i, 0, 0, time.UTC) }
		s.writeInbox(ctx, 1, inboxInput{Kind: InboxKindAlert, Level: "info", Title: "T" + string(rune('a'+i))})
	}
	page, err := s.Inbox(ctx, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Title != "Tc" {
		t.Fatalf("cap on write: %+v", page.Items)
	}

	// An entry older than the window is dropped by the next write.
	if _, err := s.deps.Store.DB.Exec(`INSERT INTO inbox_entries (user_id, kind, level, title, created_at) VALUES (1, 'alert', 'info', 'old', ?)`,
		time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC).Unix()); err != nil {
		t.Fatal(err)
	}
	s.writeInbox(ctx, 1, inboxInput{Kind: InboxKindAlert, Level: "info", Title: "Td"})
	var old int
	if err := s.deps.Store.R.QueryRow(`SELECT COUNT(*) FROM inbox_entries WHERE user_id = 1 AND title = 'old'`).Scan(&old); err != nil || old != 0 {
		t.Fatalf("age purge on write: %d %v", old, err)
	}
	if deleted, err := s.PurgeInbox(ctx); err != nil {
		t.Fatalf("admin purge: %v", err)
	} else {
		_ = deleted
	}
}

func TestShareInboxTitlesAndKeys(t *testing.T) {
	// Every portal kind has an English fallback title, stored once for all languages.
	for kind, want := range map[string]string{
		"directive": "Physio/coach instruction updated",
		"remark":    "New remark on a session",
		"plan":      "Session prescribed by your physio or coach",
		"event":     "Constraint added to your plan",
	} {
		if got := shareInboxTitle(kind); got != want {
			t.Fatalf("%s: %q", kind, got)
		}
	}
	if shareInboxTitle("other") == "" {
		t.Fatal("empty title for an unknown kind")
	}

	// Keys are stable per target, and a remark is per session.
	if shareInboxKey(7, 42, "remark") != shareInboxKey(7, 42, "remark") {
		t.Fatal("unstable key")
	}
	if shareInboxKey(7, 42, "remark") == shareInboxKey(7, 43, "remark") || shareInboxKey(7, 42, "remark") == shareInboxKey(8, 42, "remark") {
		t.Fatal("key collision")
	}
	if alertInboxKey("2026-11-24", nil, "X") == alertInboxKey("2026-11-24", ip64(9), "X") {
		t.Fatal("day and session alerts share a key")
	}
}

func ip64(v int64) *int64 { return &v }
