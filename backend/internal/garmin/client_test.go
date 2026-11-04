package garmin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeKey = "cle-de-test-synthetique-0001"

var tokenBase = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func fakeJWT(exp time.Time) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	p := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	return h + "." + p + ".sig"
}

type fakeGarmin struct {
	tokens, data, fit atomic.Int32
	fail401, fail429  atomic.Int32
	lastAccept        atomic.Value
}

func (f *fakeGarmin) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+tokenPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey "+fakeKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		n := f.tokens.Add(1)
		_, _ = io.WriteString(w, fakeJWT(tokenBase.Add(24*time.Hour).Add(time.Duration(n)*time.Second)))
	})
	mux.HandleFunc("GET /hrv-service/hrv/{date}", func(w http.ResponseWriter, r *http.Request) {
		f.data.Add(1)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.Header.Get("User-Agent") != userAgent {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.fail401.Load() > 0 {
			f.fail401.Add(-1)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.fail429.Load() > 0 {
			f.fail429.Add(-1)
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.PathValue("date") == "2026-01-01" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, `{"hrvSummary":{"calendarDate":"`+r.PathValue("date")+`","lastNightAvg":45,"status":"BALANCED"}}`)
	})
	mux.HandleFunc("GET /download-service/files/activity/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.fit.Add(1)
		f.lastAccept.Store(r.Header.Get("Accept"))
		if r.Header.Get("Accept") != fitAccept {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		fw, _ := zw.Create(r.PathValue("id") + "_ACTIVITY.fit")
		_, _ = fw.Write([]byte("0123456789ab-contenu-fit"))
		_ = zw.Close()
		_, _ = w.Write(buf.Bytes())
	})
	mux.HandleFunc("GET /activity-service/activity/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	return mux
}

func newTestClient(t *testing.T, srvURL, dir string, key string, now time.Time) *Client {
	t.Helper()
	return New(Options{
		BaseURL: srvURL, DataDir: dir, Key: func() string { return key },
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Location: func() *time.Location { return time.UTC },
		Now:      func() time.Time { return now },
	})
}

func TestTokenRetryAndCache(t *testing.T) {
	fg := &fakeGarmin{}
	srv := httptest.NewServer(fg.handler(t))
	defer srv.Close()
	dir := t.TempDir()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	c := newTestClient(t, srv.URL, dir, fakeKey, now)
	ctx := context.Background()

	// 401 une fois → refresh forcé du jeton → succès
	fg.fail401.Store(1)
	h, err := c.HRVDay(ctx, "2026-11-03")
	if err != nil || h.HrvSummary == nil || pf(h.HrvSummary.LastNightAvg) != 45 {
		t.Fatalf("HRVDay : %v %+v", err, h)
	}
	if fg.tokens.Load() != 2 || fg.data.Load() != 2 {
		t.Errorf("jetons=%d appels=%d (attendu 2/2)", fg.tokens.Load(), fg.data.Load())
	}
	// cache mémoire (date = aujourd'hui)
	if _, err := c.HRVDay(ctx, "2026-11-03"); err != nil || fg.data.Load() != 2 {
		t.Errorf("cache mémoire attendu (appels=%d)", fg.data.Load())
	}
	// 429 → backoff → succès ; date ancienne → cache disque
	fg.fail429.Store(1)
	if _, err := c.HRVDay(ctx, "2026-10-20"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "hrv", "2026-10-20.json")); err != nil {
		t.Errorf("cache disque attendu : %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "hrv", "2026-11-03.json")); err == nil {
		t.Error("pas de cache disque attendu pour aujourd'hui")
	}
	// 204 → ErrNoData
	if _, err := c.HRVDay(ctx, "2026-01-01"); !errors.Is(err, ErrNoData) {
		t.Errorf("ErrNoData attendu, obtenu %v", err)
	}
	// 404 → IsNotFound
	if _, err := c.Activity(ctx, 42); !IsNotFound(err) {
		t.Errorf("404 attendu, obtenu %v", err)
	}

	// token.json : 0600, empreinte de la clé, jamais la clé
	b, err := os.ReadFile(filepath.Join(dir, "token.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), fakeKey) {
		t.Fatal("la clé ne doit jamais être écrite dans token.json")
	}
	var ts tokenState
	_ = json.Unmarshal(b, &ts)
	if ts.KeySHA256 != keyHash(fakeKey) || ts.Token == "" {
		t.Errorf("token.json inattendu")
	}
	if st, _ := os.Stat(filepath.Join(dir, "token.json")); st.Mode().Perm() != 0o600 {
		t.Errorf("permissions token.json = %v", st.Mode().Perm())
	}

	// un nouveau client relit le jeton disque (pas de nouvel échange)
	c2 := newTestClient(t, srv.URL, dir, fakeKey, now)
	if _, err := c2.Token(ctx); err != nil || fg.tokens.Load() != 2 {
		t.Errorf("jeton disque non réutilisé (échanges=%d, err=%v)", fg.tokens.Load(), err)
	}
	if c2.TokenExpiresAt() == nil {
		t.Error("expiration attendue")
	}
	// clé différente → jeton invalide → nouvel échange (refusé ici)
	c3 := newTestClient(t, srv.URL, dir, "autre-cle-synthetique-0002", now)
	if _, err := c3.Token(ctx); !IsUnauthorized(err) {
		t.Errorf("401 attendu pour une autre clé, obtenu %v", err)
	}
	// renouvellement si < 1 h restante
	c4 := newTestClient(t, srv.URL, dir, fakeKey, now.Add(23*time.Hour+30*time.Minute))
	before := fg.tokens.Load()
	if _, err := c4.Token(ctx); err != nil || fg.tokens.Load() != before+1 {
		t.Errorf("renouvellement attendu (échanges %d → %d)", before, fg.tokens.Load())
	}
}

func TestFITDownload(t *testing.T) {
	fg := &fakeGarmin{}
	srv := httptest.NewServer(fg.handler(t))
	defer srv.Close()
	dir := t.TempDir()
	c := newTestClient(t, srv.URL, dir, fakeKey, tokenBase)
	b, err := c.ActivityFIT(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "0123456789ab-contenu-fit" {
		t.Errorf("contenu extrait : %q", b)
	}
	if fg.lastAccept.Load() != fitAccept {
		t.Errorf("Accept = %v", fg.lastAccept.Load())
	}
	st, err := os.Stat(filepath.Join(dir, "cache", "fit", "123.fit"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("cache FIT : %v", err)
	}
	if _, err := c.ActivityFIT(context.Background(), 123); err != nil || fg.fit.Load() != 1 {
		t.Errorf("FIT attendu depuis le cache disque (téléchargements=%d)", fg.fit.Load())
	}
}

func TestNoKey(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:1", t.TempDir(), "", time.Now())
	if _, err := c.HRVDay(context.Background(), "2026-11-03"); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("ErrNoAPIKey attendu, obtenu %v", err)
	}
	if c.Configured() {
		t.Error("Configured() attendu false")
	}
}

func TestJWTExpiry(t *testing.T) {
	exp := time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC)
	got, ok := jwtExpiry(fakeJWT(exp))
	if !ok || !got.Equal(exp) {
		t.Errorf("exp = %v %v", got, ok)
	}
	if _, ok := jwtExpiry("pas.un.jwt"); ok {
		t.Error("JWT invalide accepté")
	}
}

func TestGoalValueAndBBPoints(t *testing.T) {
	var d DailySummary
	raw := `{"calendarDate":"2026-11-03","movement":{"steps":{"goal":8000,"value":8432},"floorsAscended":12,"floorsDescended":{}}}`
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	v, g := d.StepsValue()
	if pf(v) != 8432 || pf(g) != 8000 || pf(d.FloorsUp()) != 12 || d.FloorsDown() != nil {
		t.Errorf("movement : %v %v %v %v", pf(v), pf(g), pf(d.FloorsUp()), d.FloorsDown())
	}
	var bb []BodyBatteryDay
	raw = `[{"date":"2026-11-03","bodyBatteryValueDescriptorDTOList":[{"bodyBatteryValueDescriptorIndex":0,"bodyBatteryValueDescriptorKey":"timestamp"},{"bodyBatteryValueDescriptorIndex":1,"bodyBatteryValueDescriptorKey":"bodyBatteryLevel"}],"bodyBatteryValuesArray":[[1762150000000,82],[1762160000000,null]]}]`
	if err := json.Unmarshal([]byte(raw), &bb); err != nil {
		t.Fatal(err)
	}
	pts := bb[0].Points()
	if len(pts) != 1 || pts[0].Level != 82 {
		t.Errorf("points BB : %+v", pts)
	}
}

func pf(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}
