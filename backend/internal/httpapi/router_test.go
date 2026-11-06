package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stride/internal/admin"
	"stride/internal/apimeter"
	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/observability"
	"stride/internal/sse"
	"stride/internal/store"
	"stride/internal/web"
)

type downCache struct{ *cache.Memory }

func (downCache) Ping(context.Context) error { return errors.New("connection refused") }

func newRouter(t *testing.T, c cache.Cache, logOut io.Writer) http.Handler {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if logOut == nil {
		logOut = io.Discard
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelDebug}))
	a, err := auth.New(auth.Options{Env: env.Config{RPID: "localhost", RPOrigins: []string{"http://localhost:8080"}, SessionTTL: time.Hour}, Store: st, Cache: c, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	metrics := observability.New()
	hub := sse.NewHub(sse.Options{Metrics: metrics, Log: log})
	t.Cleanup(hub.Close)
	dom, _ := domain.New(domain.Deps{Store: st, Cache: c, Log: log, SSE: hub, Metrics: metrics})
	return New(Deps{
		Store: st, Cache: c, Auth: a, Domain: dom, Log: log, Logs: applog.New(st.DB, log), HTTPS: true, Static: web.New(),
		Admin:   admin.New(admin.Deps{Store: st, Cache: c, Meter: apimeter.Nop(), Auth: a, Log: log}),
		Metrics: metrics,
		SSE:     hub,
	})
}

func call(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthAndReadiness(t *testing.T) {
	h := newRouter(t, cache.NewMemory(), nil)
	if rec := call(h, "GET", "/health", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health: %d %s", rec.Code, rec.Body)
	}
	rec := call(h, "GET", "/health/ready", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"checks":{"cache":"ok","db":"ok"}`) {
		t.Fatalf("ready: %d %s", rec.Code, rec.Body)
	}
	down := newRouter(t, downCache{cache.NewMemory()}, nil)
	rec = call(down, "GET", "/health/ready", nil)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"cache":"error: connection refused"`) {
		t.Fatalf("not ready: %d %s", rec.Code, rec.Body)
	}
}

func TestCSRFGuard(t *testing.T) {
	h := newRouter(t, cache.NewMemory(), nil)
	rec := call(h, "POST", "/api/auth/logout", nil)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "FORBIDDEN") {
		t.Fatalf("missing header: %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, "POST", "/api/auth/logout", map[string]string{"X-Stride-CSRF": "1"}); rec.Code != http.StatusNoContent {
		t.Fatalf("with header: %d", rec.Code)
	}
	if rec := call(h, "DELETE", "/api/me", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE without header: %d", rec.Code)
	}
	if rec := call(h, "POST", "/api/webhooks/strava", nil); rec.Code == http.StatusForbidden {
		t.Fatal("the Strava webhook must not require the CSRF header")
	}
	if rec := call(h, "GET", "/api/me", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET is not CSRF-guarded: %d", rec.Code)
	}
}

func TestRoutingHeadersAndSPA(t *testing.T) {
	var logs bytes.Buffer
	h := newRouter(t, cache.NewMemory(), &logs)
	rec := call(h, "GET", "/api/nope", nil)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "NOT_FOUND") {
		t.Fatalf("unknown api: %d %s", rec.Code, rec.Body)
	}
	rec = call(h, "GET", "/register?invite=SECRET-CODE", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("spa: %d", rec.Code)
	}
	for k, want := range map[string]string{
		"Content-Security-Policy":   "default-src 'self'",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "no-referrer",
		"Permissions-Policy":        "publickey-credentials-get=(self)",
		"Strict-Transport-Security": "max-age",
	} {
		if !strings.Contains(rec.Header().Get(k), want) {
			t.Errorf("%s = %q", k, rec.Header().Get(k))
		}
	}
	call(h, "GET", "/api/integrations/strava/callback?code=oauth-secret&state=s1&scope=read", nil)
	if out := logs.String(); strings.Contains(out, "oauth-secret") || strings.Contains(out, "SECRET-CODE") || !strings.Contains(out, "scope=read") {
		t.Fatalf("request log must mask secrets: %s", out)
	}
	rec = call(h, "GET", "/api/auth/state", map[string]string{"Accept-Encoding": "gzip"})
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("state: %d %v", rec.Code, rec.Header())
	}
}

func TestRecoverAndBodyLimit(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := withRecover(log, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	if rec := call(h, "GET", "/x", nil); rec.Code != 500 || !strings.Contains(rec.Body.String(), "INTERNAL") {
		t.Fatalf("recover: %d %s", rec.Code, rec.Body)
	}
	h = withBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	req := httptest.NewRequest("POST", "/api/x", bytes.NewReader(make([]byte, maxBodyBytes+1)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body limit: %d", rec.Code)
	}
}

func TestPlatformEndpoints(t *testing.T) {
	h := newRouter(t, cache.NewMemory(), nil)

	// GET /version
	rec := call(h, "GET", "/version", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("version code: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"version":"4.0.0"`) {
		t.Fatalf("version body: %s", rec.Body.String())
	}

	// GET /metrics
	rec = call(h, "GET", "/metrics", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics code: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "stride_") {
		t.Fatalf("metrics body: %s", rec.Body.String())
	}

	for _, path := range []string{"/openapi.json", "/openapi.yaml"} {
		if rec = call(h, "GET", path, nil); rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "openapi") {
			t.Fatalf("%s must not be served", path)
		}
	}
}
