package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"stride/internal/applog"
	"stride/internal/store"
)

func TestAuditRecordsRequestsAndActions(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.DB.Exec(`INSERT INTO users (id, display_name, created_at) VALUES (3, 'Sam', 1)`)
	l := applog.New(st.DB, nil)

	mux := http.NewServeMux()
	authed := func(h http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			applog.SetUser(r.Context(), 3)
			h(w, r)
		})
	}
	mux.Handle("POST /api/sync", authed(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	mux.Handle("PUT /api/llm", authed(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	mux.Handle("GET /api/activities", authed(func(w http.ResponseWriter, _ *http.Request) {}))
	mux.HandleFunc("GET /api/public/share/{token}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	h := withAudit(l, mux)
	for _, req := range []*http.Request{
		httptest.NewRequest("GET", "/api/public/share/st4_SECRETtokenValue", nil),
		httptest.NewRequest("POST", "/api/sync", nil),
		httptest.NewRequest("PUT", "/api/llm", nil),
		httptest.NewRequest("GET", "/api/activities?limit=5", nil),
		httptest.NewRequest("GET", "/assets/app.js", nil),
	} {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	l.Close()

	req, _ := l.Query(ctx, applog.Filter{Category: applog.CategoryRequest})
	if len(req.Items) != 4 {
		t.Fatalf("requests: %d (static files must be skipped)", len(req.Items))
	}
	for _, it := range req.Items {
		if strings.Contains(it.Target, "SECRET") {
			t.Fatalf("share token stored in the audit log: %s", it.Target)
		}
	}
	acts, _ := l.Query(ctx, applog.Filter{Category: applog.CategoryAction})
	if len(acts.Items) != 2 || acts.Items[1].Action != "sync.start" || acts.Items[0].Action != "settings.llm" || acts.Items[0].Level != applog.LevelError {
		t.Fatalf("actions: %+v", acts.Items)
	}
	if acts.Items[1].UserID == nil || *acts.Items[1].UserID != 3 {
		t.Fatal("user not attributed")
	}
	if errs, _ := l.Query(ctx, applog.Filter{Level: applog.LevelError}); len(errs.Items) != 3 {
		t.Fatalf("error filter: %d", len(errs.Items))
	}
}

func TestLoggingRedactsShareTokens(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := withLogging(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	for _, path := range []string{"/api/public/share/st4_SECRETtokenValue", "/share/st4_SECRETtokenValue"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	if out := buf.String(); strings.Contains(out, "SECRET") || !strings.Contains(out, "/api/public/share/st4_SECR…") {
		t.Fatalf("share token logged: %s", out)
	}
}

// Portal writes are audited without a session: the actor is share:<prefix>, the target keeps the
// sub-path and never the token.
func TestAuditPortalWritesRedactToken(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	l := applog.New(st.DB, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/public/share/{token}/directive", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("DELETE /api/public/share/{token}/plan/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	h := withAudit(l, mux)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/public/share/st4_SECRETtokenValue/directive", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("DELETE", "/api/public/share/st4_SECRETtokenValue/plan/9", nil))
	l.Close()
	acts, _ := l.Query(ctx, applog.Filter{Category: applog.CategoryAction})
	if len(acts.Items) != 2 {
		t.Fatalf("actions: %+v", acts.Items)
	}
	del, put := acts.Items[0], acts.Items[1]
	if put.Action != "share.public_directive_put" || put.Message != "share:st4_SECR" || put.UserID != nil ||
		put.Target != "/api/public/share/st4_SECR…/directive" {
		t.Fatalf("put: %+v", put)
	}
	if del.Action != "share.public_plan_delete" || del.Level != applog.LevelError || del.Target != "/api/public/share/st4_SECR…/plan/9" {
		t.Fatalf("delete: %+v", del)
	}
	all, _ := l.Query(ctx, applog.Filter{})
	for _, it := range all.Items {
		if strings.Contains(it.Target, "SECRET") || strings.Contains(it.Message, "SECRET") {
			t.Fatalf("token stored: %+v", it)
		}
	}
}
