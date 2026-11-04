package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"stride/internal/analysis"
	"stride/internal/config"
	"stride/internal/fitdata"
	"stride/internal/garmin"
	"stride/internal/report"
	"stride/internal/web"
)

func newServer(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	dir := t.TempDir()
	st, err := config.NewStore(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := garmin.New(garmin.Options{BaseURL: "http://127.0.0.1:1", DataDir: dir, Key: func() string { k, _ := st.APIKey(); return k }, Logger: log})
	svc := &analysis.Service{G: g, FIT: fitdata.NewStore(g, log), Settings: st, Reporter: report.Reporter{}, Log: log}
	s := &Server{Cfg: cfg, Settings: st, G: g, Svc: svc, Log: log, Version: "test", Static: web.New()}
	return s.Handler()
}

func do(h http.Handler, method, path string, body string, user, pass string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthAndErrors(t *testing.T) {
	h := newServer(t, config.Config{})
	rec := do(h, "GET", "/api/health", "", "", "")
	var health map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &health)
	if rec.Code != 200 || health["status"] != "ok" || health["garminConfigured"] != false || health["version"] != "test" {
		t.Fatalf("health : %d %v", rec.Code, health)
	}
	cases := []struct {
		path string
		code int
		ec   string
	}{
		{"/api/activities", 412, "NO_API_KEY"},
		{"/api/activities?limit=0", 400, "BAD_REQUEST"},
		{"/api/activities/abc/analysis", 400, "BAD_REQUEST"},
		{"/api/activities/1/analysis?mode=x", 400, "BAD_REQUEST"},
		{"/api/days/2999-01-01/analysis", 400, "BAD_REQUEST"},
		{"/api/days/2026-11-02/analysis", 412, "NO_API_KEY"},
		{"/api/inconnu", 404, "NOT_FOUND"},
	}
	for _, c := range cases {
		rec := do(h, "GET", c.path, "", "", "")
		var e struct{ Error, Code string }
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != c.code || e.Code != c.ec || e.Error == "" {
			t.Errorf("%s : %d %+v", c.path, rec.Code, e)
		}
	}
}

func TestSettingsWithoutKey(t *testing.T) {
	h := newServer(t, config.Config{Timezone: "Europe/Paris"})
	rec := do(h, "GET", "/api/settings", "", "", "")
	var s map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s["apiKeyConfigured"] != false || s["apiKeySource"] != "none" || s["apiKeyHint"] != nil || s["timezone"] != "Europe/Paris" {
		t.Errorf("settings : %v", s)
	}
	rec = do(h, "PUT", "/api/settings", `{"profile":{"cadenceTargetMin":176},"timezone":"Europe/Berlin"}`, "", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	prof := s["profile"].(map[string]any)
	if rec.Code != 200 || prof["cadenceTargetMin"] != 176.0 || s["timezone"] != "Europe/Berlin" {
		t.Errorf("PUT settings : %d %v", rec.Code, s)
	}
	rec = do(h, "PUT", "/api/settings", `{"profile":{"injuredSide":"milieu"}}`, "", "")
	if rec.Code != 400 {
		t.Errorf("profil invalide accepté : %d", rec.Code)
	}
	rec = do(h, "POST", "/api/settings/test", "", "", "")
	if rec.Code != 412 {
		t.Errorf("test sans clé : %d", rec.Code)
	}
}

func TestBasicAuth(t *testing.T) {
	h := newServer(t, config.Config{AuthUser: "moi", AuthPassword: "secret-de-test"})
	if rec := do(h, "GET", "/api/health", "", "", ""); rec.Code != 200 {
		t.Errorf("health doit rester public : %d", rec.Code)
	}
	rec := do(h, "GET", "/api/settings", "", "", "")
	if rec.Code != 401 || !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Errorf("401 attendu : %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/settings", "", "moi", "faux"); rec.Code != 401 {
		t.Errorf("mauvais mot de passe accepté : %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/settings", "", "moi", "secret-de-test"); rec.Code != 200 {
		t.Errorf("bons identifiants refusés : %d", rec.Code)
	}
	if rec := do(h, "GET", "/", "", "", ""); rec.Code != 401 {
		t.Errorf("SPA doit être protégé : %d", rec.Code)
	}
}

func TestKeyHint(t *testing.T) {
	if h := keyHint("abcdefghijkl-a1b2"); h == nil || *h != "••••a1b2" {
		t.Errorf("hint : %v", h)
	}
	if h := keyHint("court"); h == nil || *h != "••••" {
		t.Errorf("hint clé courte : %v", *h)
	}
	if keyHint("") != nil {
		t.Error("hint nil attendu sans clé")
	}
}
