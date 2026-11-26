package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"stride/internal/buildinfo"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/secret"
	"stride/internal/store"
	"stride/internal/updates"
)

// updatesEnv is the admin API wired with a fake GitHub: the release notice crosses the admin route,
// the domain write and the notification centre (§22, issue #143).
type updatesEnv struct {
	*testEnv
	ghCalls  *int32
	github   *httptest.Server
	dom      *domain.Services
	adminID  int64
	adminTok string
	userTok  string
}

// setupUpdates builds the harness with the domain and a fake update service. The first registered
// account of the installation is the administrator (§3), the second is a regular user.
func setupUpdates(t *testing.T, releaseBody string) *updatesEnv {
	t.Helper()
	var calls int32
	var built *domain.Services
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(releaseBody))
	}))
	t.Cleanup(gh.Close)

	e := setup(t, func(d *Deps) {
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		sealer, err := secret.New(make([]byte, 32), nil)
		if err != nil {
			t.Fatal(err)
		}
		dom, err := domain.New(domain.Deps{Env: env.Config{}, Store: d.Store, Cache: d.Cache, Sealer: sealer, Log: log})
		if err != nil {
			t.Fatal(err)
		}
		d.Domain = dom
		d.Updates = updates.New(gh.Client(), updates.WithBaseURL(gh.URL))
		built = dom
	})
	out := &updatesEnv{testEnv: e, ghCalls: &calls, github: gh, dom: built}
	out.adminID, out.adminTok = out.user("Ada")
	_, out.userTok = out.user("Bob")
	return out
}

// TestUpdatesRouteAdminOnly: the route is for administrators, the caller gets the entry of the
// version, and a second check adds nothing.
func TestUpdatesRouteAdminOnly(t *testing.T) {
	e := setupUpdates(t, `{"tag_name":"9.9.9","published_at":"2026-11-25T10:00:00Z"}`)
	buildinfo.Version = "4.7.0"
	t.Cleanup(func() { buildinfo.Version = "dev" })

	if code, _ := e.do("", "GET", "/api/updates", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
	if code, _ := e.do(e.userTok, "GET", "/api/updates", ""); code != http.StatusForbidden {
		t.Fatalf("non-admin: %d", code)
	}
	if n := atomic.LoadInt32(e.ghCalls); n != 0 {
		t.Fatalf("a non-admin triggered a GitHub call: %d", n)
	}

	code, body := e.do(e.adminTok, "GET", "/api/updates", "")
	if code != 200 {
		t.Fatalf("admin: %d %s", code, body)
	}
	var st updates.Status
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("json: %v %s", err, body)
	}
	if !st.Available || st.Current != "4.7.0" || st.Latest != "9.9.9" ||
		st.URL != "https://github.com/aguyonp/stride/releases/tag/9.9.9" || st.PublishedAt != "2026-11-25T10:00:00Z" {
		t.Fatalf("status: %+v", st)
	}
	if st.Notification == 0 || st.Read {
		t.Fatalf("notification: %+v", st)
	}
	var kind, ver, title string
	if err := e.st.R.QueryRow(`SELECT kind, code, title FROM inbox_entries WHERE id = ?`, st.Notification).Scan(&kind, &ver, &title); err != nil {
		t.Fatal(err)
	}
	if kind != domain.InboxKindUpdate || ver != "9.9.9" || title != "New version available" {
		t.Fatalf("entry: %s %s %s", kind, ver, title)
	}

	// One entry per admin and per version, however many checks.
	if code, _ := e.do(e.adminTok, "GET", "/api/updates", ""); code != 200 {
		t.Fatalf("second call: %d", code)
	}
	if n := e.count(`SELECT COUNT(*) FROM inbox_entries WHERE kind = 'update'`); n != 1 {
		t.Fatalf("update entries: %d", n)
	}

	// Reading the entry hides the banner on the next load; the history keeps it. The inbox route is
	// mounted by the domain API, so the harness marks it through the service.
	if _, err := e.dom.MarkInboxRead(context.Background(), e.adminID, []int64{st.Notification}); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	var after updates.Status
	if code, body := e.do(e.adminTok, "GET", "/api/updates", ""); code != 200 {
		t.Fatalf("third call: %d %s", code, body)
	} else if err := json.Unmarshal(body, &after); err != nil {
		t.Fatal(err)
	}
	if !after.Available || !after.Read || after.Notification != st.Notification {
		t.Fatalf("after read: %+v", after)
	}
}

// TestUpdatesRouteDisabled: the admin switch stops the call, the entry and the banner.
func TestUpdatesRouteDisabled(t *testing.T) {
	e := setupUpdates(t, `{"tag_name":"9.9.9"}`)
	buildinfo.Version = "4.7.0"
	t.Cleanup(func() { buildinfo.Version = "dev" })

	if code, body := e.do(e.adminTok, "PUT", "/api/admin/settings", `{"updateCheckEnabled":false}`); code != 200 ||
		!strings.Contains(string(body), `"updateCheckEnabled":false`) {
		t.Fatalf("disable: %d %s", code, body)
	}
	var st updates.Status
	if code, body := e.do(e.adminTok, "GET", "/api/updates", ""); code != 200 {
		t.Fatalf("disabled: %d %s", code, body)
	} else if err := json.Unmarshal(body, &st); err != nil {
		t.Fatal(err)
	}
	if st.Available || !st.Disabled || st.Notification != 0 {
		t.Fatalf("disabled status: %+v", st)
	}
	if n := atomic.LoadInt32(e.ghCalls); n != 0 {
		t.Fatalf("a call went out while disabled: %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM inbox_entries WHERE kind = 'update'`); n != 0 {
		t.Fatalf("entries while disabled: %d", n)
	}

	// Re-enabled with GitHub down: no error for the caller, no entry.
	if code, _ := e.do(e.adminTok, "PUT", "/api/admin/settings", `{"updateCheckEnabled":true}`); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	e.github.Close()
	if code, body := e.do(e.adminTok, "GET", "/api/updates", ""); code != 200 || strings.Contains(string(body), `"available":true`) {
		t.Fatalf("github down: %d %s", code, body)
	}
}

// TestUpdatesRouteDevBuild: a dev build reports the release and announces nothing.
func TestUpdatesRouteDevBuild(t *testing.T) {
	e := setupUpdates(t, `{"tag_name":"9.9.9"}`)
	buildinfo.Version = "dev"
	var st updates.Status
	if code, body := e.do(e.adminTok, "GET", "/api/updates", ""); code != 200 {
		t.Fatalf("dev: %d %s", code, body)
	} else if err := json.Unmarshal(body, &st); err != nil {
		t.Fatal(err)
	}
	if st.Available || st.Notification != 0 || st.Latest != "9.9.9" {
		t.Fatalf("dev status: %+v", st)
	}
	if n := e.count(`SELECT COUNT(*) FROM inbox_entries WHERE kind = 'update'`); n != 0 {
		t.Fatalf("dev entries: %d", n)
	}
}

// TestUpdatesNoticeForEveryAdmin: the notice is written for every admin, not only the caller.
func TestUpdatesNoticeForEveryAdmin(t *testing.T) {
	e := setupUpdates(t, `{"tag_name":"9.9.9"}`)
	buildinfo.Version = "4.7.0"
	t.Cleanup(func() { buildinfo.Version = "dev" })
	second, _ := e.user("Grace")
	role := store.RoleAdmin
	if err := e.st.UpdateUserAccess(context.Background(), second, &role, nil); err != nil {
		t.Fatal(err)
	}
	if code, body := e.do(e.adminTok, "GET", "/api/updates", ""); code != 200 {
		t.Fatalf("check: %d %s", code, body)
	}
	if n := e.count(`SELECT COUNT(*) FROM inbox_entries WHERE kind = 'update'`); n != 2 {
		t.Fatalf("entries: %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM inbox_entries WHERE kind = 'update' AND user_id = ` + strconv.FormatInt(second, 10)); n != 1 {
		t.Fatal("the second admin has no notice")
	}
}
