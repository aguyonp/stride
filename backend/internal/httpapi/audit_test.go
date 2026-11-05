package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	h := withAudit(l, mux)
	for _, req := range []*http.Request{
		httptest.NewRequest("POST", "/api/sync", nil),
		httptest.NewRequest("PUT", "/api/llm", nil),
		httptest.NewRequest("GET", "/api/activities?limit=5", nil),
		httptest.NewRequest("GET", "/assets/app.js", nil),
	} {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	l.Close()

	req, _ := l.Query(ctx, applog.Filter{Category: applog.CategoryRequest})
	if len(req.Items) != 3 {
		t.Fatalf("requests: %d (static files must be skipped)", len(req.Items))
	}
	acts, _ := l.Query(ctx, applog.Filter{Category: applog.CategoryAction})
	if len(acts.Items) != 2 || acts.Items[1].Action != "sync.start" || acts.Items[0].Action != "settings.llm" || acts.Items[0].Level != applog.LevelError {
		t.Fatalf("actions: %+v", acts.Items)
	}
	if acts.Items[1].UserID == nil || *acts.Items[1].UserID != 3 {
		t.Fatal("user not attributed")
	}
	if errs, _ := l.Query(ctx, applog.Filter{Level: applog.LevelError}); len(errs.Items) != 2 {
		t.Fatalf("error filter: %d", len(errs.Items))
	}
}
