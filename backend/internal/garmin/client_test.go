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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"stride/internal/apimeter"
)

const fakeKey = "cle-de-test-synthetique-0001"

func fakeJWT(exp time.Time) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	p := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	return h + "." + p + ".sig"
}

type recMeter struct {
	mu    sync.Mutex
	calls []apimeter.Call
}

func (m *recMeter) Acquire(context.Context, string, int64) (func(), error) { return func() {}, nil }
func (m *recMeter) Observe(c apimeter.Call) {
	m.mu.Lock()
	m.calls = append(m.calls, c)
	m.mu.Unlock()
}
func (m *recMeter) SetQuota(string, apimeter.Quota) {}
func (m *recMeter) Snapshot() apimeter.Snapshot     { return apimeter.Snapshot{} }

type fakeGarmin struct {
	tokens, data              atomic.Int32
	fail401, fail429, fail5xx atomic.Int32
}

func (f *fakeGarmin) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+tokenPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey "+fakeKey || r.Header.Get("Accept") != "*/*" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.tokens.Add(1)
		_, _ = io.WriteString(w, fakeJWT(time.Now().Add(24*time.Hour)))
	})
	mux.HandleFunc("GET /hrv-service/hrv/{date}", func(w http.ResponseWriter, r *http.Request) {
		f.data.Add(1)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.Header.Get("User-Agent") != userAgent {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch {
		case f.fail401.Load() > 0:
			f.fail401.Add(-1)
			w.WriteHeader(http.StatusUnauthorized)
		case f.fail429.Load() > 0:
			f.fail429.Add(-1)
			w.Header().Set("Retry-After", "42")
			w.WriteHeader(http.StatusTooManyRequests)
		case f.fail5xx.Load() > 0:
			f.fail5xx.Add(-1)
			w.WriteHeader(http.StatusBadGateway)
		case r.PathValue("date") == "2026-01-01":
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = io.WriteString(w, `{"hrvSummary":{"calendarDate":"`+r.PathValue("date")+`","lastNightAvg":45,"status":"BALANCED"}}`)
		}
	})
	mux.HandleFunc("GET /download-service/files/activity/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != fitAccept {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		fw, _ := zw.Create("x_ACTIVITY.fit")
		_, _ = fw.Write([]byte("0123456789ab-contenu-fit"))
		_ = zw.Close()
		_, _ = w.Write(buf.Bytes())
	})
	return mux
}

func newTestClient(url, key string, m apimeter.Meter, onTok func(string, time.Time)) *Client {
	return New(Options{BaseURL: url, UserID: 7, APIKey: key, Meter: m, OnToken: onTok,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Sleep: func(context.Context, time.Duration) error { return nil }})
}

func TestTokenRefreshOn401AndPersistence(t *testing.T) {
	fg := &fakeGarmin{}
	srv := httptest.NewServer(fg.handler())
	defer srv.Close()
	m := &recMeter{}
	saved := 0
	c := newTestClient(srv.URL, fakeKey, m, func(tok string, exp time.Time) {
		if strings.Count(tok, ".") == 2 && exp.After(time.Now()) {
			saved++
		}
	})
	fg.fail401.Store(1)
	b, err := c.HRVDay(context.Background(), "2026-11-03")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := Decode[HRVDay](b)
	if h.HrvSummary == nil || *h.HrvSummary.LastNightAvg != 45 {
		t.Fatalf("hrv: %+v", h)
	}
	if fg.tokens.Load() != 2 || fg.data.Load() != 2 || saved != 2 {
		t.Errorf("tokens=%d calls=%d saved=%d", fg.tokens.Load(), fg.data.Load(), saved)
	}
	if c.TokenExpiresAt() == nil {
		t.Error("token expiry expected")
	}
	for _, call := range m.calls {
		if call.Provider != apimeter.Garmin || call.UserID != 7 || strings.ContainsAny(call.Endpoint, "0123456789?") {
			t.Errorf("metered call leaks ids or query: %+v", call)
		}
	}
	// seeded token: no exchange
	c2 := New(Options{BaseURL: srv.URL, UserID: 7, APIKey: fakeKey, Token: fakeJWT(time.Now().Add(20 * time.Hour)), TokenExpiresAt: time.Now().Add(20 * time.Hour),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := c2.HRVDay(context.Background(), "2026-11-03"); err != nil || fg.tokens.Load() != 2 {
		t.Errorf("seeded token not reused: %v %d", err, fg.tokens.Load())
	}
}

func TestRateLimitAndRetries(t *testing.T) {
	fg := &fakeGarmin{}
	srv := httptest.NewServer(fg.handler())
	defer srv.Close()
	m := &recMeter{}
	c := newTestClient(srv.URL, fakeKey, m, nil)
	ctx := context.Background()
	fg.fail429.Store(1)
	_, err := c.HRVDay(ctx, "2026-11-03")
	if !IsRateLimited(err) || fg.data.Load() != 1 {
		t.Fatalf("429 must not be retried: %v calls=%d", err, fg.data.Load())
	}
	last := m.calls[len(m.calls)-1]
	if last.Status != 429 || last.RetryAfter != 42*time.Second {
		t.Errorf("observed 429: %+v", last)
	}
	fg.fail5xx.Store(2)
	if _, err := c.HRVDay(ctx, "2026-11-03"); err != nil {
		t.Fatalf("5xx should be retried twice: %v", err)
	}
	if _, err := c.HRVDay(ctx, "2026-01-01"); !errors.Is(err, ErrNoData) {
		t.Errorf("204 → ErrNoData, got %v", err)
	}
	bad := newTestClient(srv.URL, "mauvaise-cle-synthetique", m, nil)
	if _, err := bad.Refresh(ctx); !IsUnauthorized(err) {
		t.Errorf("bad key → unauthorized, got %v", err)
	}
	if _, err := newTestClient(srv.URL, "", m, nil).HRVDay(ctx, "2026-11-03"); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("no key → ErrNoAPIKey, got %v", err)
	}
}

func TestFITDownloadAcceptHeader(t *testing.T) {
	srv := httptest.NewServer((&fakeGarmin{}).handler())
	defer srv.Close()
	b, err := newTestClient(srv.URL, fakeKey, apimeter.Nop(), nil).ActivityFIT(context.Background(), 123)
	if err != nil || string(b) != "0123456789ab-contenu-fit" {
		t.Fatalf("fit: %q %v", b, err)
	}
}

func TestJWTExpiry(t *testing.T) {
	exp := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	if got, ok := jwtExpiry(fakeJWT(exp)); !ok || !got.Equal(exp) {
		t.Errorf("exp = %v %v", got, ok)
	}
	if _, ok := jwtExpiry("pas.un.jwt"); ok {
		t.Error("invalid JWT accepted")
	}
}

func TestDecoders(t *testing.T) {
	var ds DailySummary
	if err := json.Unmarshal([]byte(`{"movement":{"steps":{"goal":8000,"value":9000},"floorsAscended":12}}`), &ds); err != nil {
		t.Fatal(err)
	}
	if v, g := ds.StepsValue(); *v != 9000 || *g != 8000 || *ds.FloorsUp() != 12 {
		t.Error("GoalValue")
	}
	sl, err := Decode[DailySleep]([]byte(`{"dailySleepDTO":{"calendarDate":"2026-11-02","sleepTimeSeconds":100}}`))
	if err != nil || sl.CalendarDate != "2026-11-02" || !sl.HasData() {
		t.Errorf("sleep envelope: %+v %v", sl, err)
	}
	us, err := Decode[UserSummary]([]byte(`{"calendarDate":"2026-11-02","bodyBatteryAtWakeTime":81,"floorsAscended":10.5}`))
	if err != nil || *us.BodyBatteryAtWakeTime != 81 || *us.FloorsAscended != 10.5 {
		t.Errorf("user summary: %+v %v", us, err)
	}
	day := BodyBatteryDay{Descriptors: []BBDescriptor{{Index: 0, Key: "timestamp"}, {Index: 1, Key: "bodyBatteryLevel"}},
		Values: [][]any{{1.0, 50.0}, {2.0, nil}}}
	if pts := day.Points(); len(pts) != 1 || pts[0].Level != 50 {
		t.Errorf("bb points: %+v", pts)
	}
}
