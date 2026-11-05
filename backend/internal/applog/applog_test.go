package applog_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"stride/internal/applog"
	"stride/internal/store"
)

func TestAddQueryFilterPurge(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB.Exec(`INSERT INTO users (id, display_name, created_at) VALUES (7, 'Alex', 1)`); err != nil {
		t.Fatal(err)
	}
	l := applog.New(st.DB, nil)
	uid := int64(7)
	old := time.Now().Add(-applog.Retention - time.Hour)
	l.Add(applog.Entry{At: old, Category: applog.CategoryRequest, Method: "GET", Target: "/api/old", Status: 200})
	l.Add(applog.Entry{Category: applog.CategoryRequest, UserID: &uid, Method: "GET", Target: "/api/activities", Status: 200})
	l.Add(applog.Entry{Category: applog.CategoryRequest, Level: applog.LevelError, Method: "OUT", Provider: "garmin", Target: "/activity-service", Status: 503})
	l.Add(applog.Entry{Category: applog.CategoryAction, UserID: &uid, Action: "sync.start", Status: 200})
	l.Close()

	page, err := l.Query(ctx, applog.Filter{})
	if err != nil || len(page.Items) != 4 || page.Items[0].Action != "sync.start" {
		t.Fatalf("all: %+v %v", page.Items, err)
	}
	if p, _ := l.Query(ctx, applog.Filter{Level: applog.LevelError}); len(p.Items) != 1 || p.Items[0].Provider != "garmin" {
		t.Fatalf("errors: %+v", p.Items)
	}
	p, _ := l.Query(ctx, applog.Filter{Category: applog.CategoryAction, UserID: 7})
	if len(p.Items) != 1 || p.Items[0].UserName == nil || *p.Items[0].UserName != "Alex" {
		t.Fatalf("actions of user: %+v", p.Items)
	}
	p, _ = l.Query(ctx, applog.Filter{Limit: 2})
	if len(p.Items) != 2 || p.NextBefore == nil {
		t.Fatalf("pagination: %+v", p)
	}
	if p2, _ := l.Query(ctx, applog.Filter{Limit: 2, Before: *p.NextBefore}); len(p2.Items) != 2 || p2.NextBefore != nil {
		t.Fatalf("second page: %+v", p2)
	}
	if n, err := l.Purge(ctx, time.Now()); err != nil || n != 1 {
		t.Fatalf("purge removed %d (%v)", n, err)
	}
}
