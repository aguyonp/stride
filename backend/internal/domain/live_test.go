//go:build live

// Live validation against the real Garmin Personal API. Run with:
//
//	STRIDE_LIVE_ENV=/path/to/.env go test -tags live -run TestLive -v ./internal/domain/
//
// It prints structure, field coverage, timings and call counts only: never values, ids, keys or tokens.
package domain_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"stride/internal/apimeter"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/provider/weather"
	"stride/internal/report"
	"stride/internal/secret"
	"stride/internal/store"
)

type countMeter struct {
	mu     sync.Mutex
	calls  map[string]int
	status map[int]int
	ms     map[string]time.Duration
}

func (m *countMeter) Acquire(context.Context, string, int64) (func(), error) { return func() {}, nil }
func (m *countMeter) Observe(c apimeter.Call) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[c.Endpoint]++
	m.status[c.Status]++
	m.ms[c.Endpoint] += c.Duration
}
func (m *countMeter) SetQuota(string, apimeter.Quota) {}
func (m *countMeter) Snapshot() apimeter.Snapshot     { return apimeter.Snapshot{} }
func (m *countMeter) total() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, v := range m.calls {
		n += v
	}
	return n
}

type liveWeather struct{}

func (liveWeather) At(context.Context, float64, float64, time.Time) (*weather.Conditions, error) {
	t, h, w, d := 10.0, 70.0, 12.0, 200.0
	return &weather.Conditions{Source: "fake", TemperatureC: &t, HumidityPct: &h, WindKph: &w, WindDirDeg: &d}, nil
}

func readKey(t *testing.T, path string) string {
	f, err := os.Open(path)
	if err != nil {
		t.Skip("STRIDE_LIVE_ENV not readable")
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export "), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "STRIDE_ADMIN_GARMIN_KEY", "GARMIN_API_TOKEN", "GARMIN_PERSONAL_API_KEY", "GARMIN_API_KEY":
			if v = strings.Trim(strings.TrimSpace(v), `"'`); v != "" {
				return v
			}
		}
	}
	t.Skip("no Garmin key in env file")
	return ""
}

// coverage walks a JSON document and returns the null and present leaf paths (arrays: first item).
func coverage(v any, path string, depth int, null, present *[]string) {
	switch x := v.(type) {
	case nil:
		*null = append(*null, path)
	case map[string]any:
		if depth == 0 {
			*present = append(*present, path)
			return
		}
		for k, val := range x {
			coverage(val, path+"."+k, depth-1, null, present)
		}
	case []any:
		if len(x) == 0 {
			*null = append(*null, path+"[] (empty)")
			return
		}
		if _, isObj := x[0].(map[string]any); isObj && depth > 0 {
			coverage(x[0], path+"[0]", depth-1, null, present)
			return
		}
		nn := 0
		for _, e := range x {
			if e != nil {
				nn++
			}
		}
		*present = append(*present, fmt.Sprintf("%s[%d, %d%% non-null]", path, len(x), nn*100/len(x)))
	default:
		*present = append(*present, path)
	}
}

func report2(t *testing.T, name string, v any) {
	raw, _ := json.Marshal(v)
	var doc any
	_ = json.Unmarshal(raw, &doc)
	var null, present []string
	coverage(doc, name, 4, &null, &present)
	sort.Strings(null)
	t.Logf("%s: %d fields present, %d null", name, len(present), len(null))
	t.Logf("%s null: %s", name, strings.Join(null, ", "))
}

func headers(md string) string {
	var out []string
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(l, "### ") {
			out = append(out, strings.TrimPrefix(l, "### "))
		}
	}
	return strings.Join(out, " | ")
}

func TestLive(t *testing.T) {
	key := readKey(t, os.Getenv("STRIDE_LIVE_ENV"))
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/live.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mk := make([]byte, 32)
	_, _ = rand.Read(mk)
	sealer, _ := secret.New(mk, nil)
	if _, err := st.DB.Exec(`INSERT INTO users (id, display_name, role, created_at) VALUES (1, 'live', 'admin', 1)`); err != nil {
		t.Fatal(err)
	}
	meter := &countMeter{calls: map[string]int{}, status: map[int]int{}, ms: map[string]time.Duration{}}
	cfg := env.Config{GarminBaseURL: "https://personal.connectapi.garmin.com", Timezone: "Europe/Paris", PublicURL: "http://localhost:8080",
		WeatherProvider: "fake", AdminGarminKey: key}
	svc, err := domain.New(domain.Deps{Env: cfg, Store: st, Cache: cache.NewMemory(), Sealer: sealer, Meter: meter, Weather: liveWeather{},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	step := func(name string, fn func() error) {
		before := meter.total()
		t0 := time.Now()
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("⏱ %-28s %6s  provider calls: %d", name, time.Since(t0).Round(time.Millisecond), meter.total()-before)
	}
	step("connect + first sync", func() error {
		if err := svc.OnUserCreated(ctx, 1, true); err != nil {
			return err
		}
		svc.Wait()
		in, err := svc.Integrations(ctx, 1)
		if err == nil && !in.Garmin.Connected {
			err = fmt.Errorf("garmin not connected after OnUserCreated")
		}
		return err
	})
	var runID int64
	step("list (db)", func() error {
		l, err := svc.ListActivities(ctx, 1, 100, "", "", "", "")
		if err != nil {
			return err
		}
		sports := map[string]int{}
		for _, it := range l.Items {
			sports[it.Sport]++
			if runID == 0 && it.IsRunning {
				runID = it.ID
			}
		}
		t.Logf("activities listed: %d by sport %v, freshness %s", len(l.Items), sports, l.Freshness.Source)
		return nil
	})
	if runID == 0 {
		t.Skip("no running activity in the last 56 days")
	}
	for _, pass := range []string{"cold", "warm"} {
		step("activity view "+pass, func() error {
			v, err := svc.ActivityView(ctx, 1, runID)
			if err != nil {
				return err
			}
			t.Logf("activity view %s: freshness=%s source=%s dataSources=%v", pass, v.Freshness.Source, v.SourceLabel, v.DataSources)
			if pass == "cold" {
				report2(t, "activity", v)
				var codes []string
				for _, a := range v.Alerts {
					codes = append(codes, a.Level+":"+a.Code)
				}
				t.Logf("alerts: %v", codes)
				t.Logf("warnings: %v", v.Warnings)
				t.Logf("segments: primary=%s blocks=%d laps=%d series=%d points", v.Segments.Primary, len(v.Segments.Blocks), len(v.Segments.Laps), len(v.Series.TS))
			}
			return nil
		})
	}
	today := time.Now().In(time.FixedZone("CET", 3600)).Format("2006-01-02")
	if d, err := svc.ResolveDate(ctx, 1, "today"); err == nil {
		today = d
	}
	for _, pass := range []string{"cold", "warm"} {
		step("day view "+pass, func() error {
			v, err := svc.DayView(ctx, 1, today)
			if err != nil {
				return err
			}
			t.Logf("day view %s: freshness=%s activities=%d warnings=%v", pass, v.Freshness.Source, len(v.Activities), v.Warnings)
			if pass == "cold" {
				report2(t, "day", v)
			}
			return nil
		})
	}
	step("activity report", func() error {
		md, err := svc.ActivityReport(ctx, 1, runID, "")
		if err == nil {
			t.Logf("activity report ≈ %d tokens, N/D ×%d, sections: %s", report.EstimateTokens(md), strings.Count(md, "N/D"), headers(md))
		}
		return err
	})
	step("day report", func() error {
		md, err := svc.DayReport(ctx, 1, today, "")
		if err == nil {
			t.Logf("day report ≈ %d tokens, N/D ×%d, sections: %s", report.EstimateTokens(md), strings.Count(md, "N/D"), headers(md))
		}
		return err
	})
	step("multi export 2 weeks", func() error {
		r, err := svc.MultiExport(ctx, 1, nil, 2, nil)
		if err == nil {
			t.Logf("multi export: %d sessions ≈ %d tokens, N/D ×%d, sections: %s", r.Sessions, r.TokensEstimate, strings.Count(r.Markdown, "N/D"), headers(r.Markdown))
		}
		return err
	})
	var eps []string
	for ep, n := range meter.calls {
		eps = append(eps, fmt.Sprintf("%s ×%d (avg %s)", ep, n, (meter.ms[ep]/time.Duration(n)).Round(time.Millisecond)))
	}
	sort.Strings(eps)
	t.Logf("provider calls total %d, statuses %v", meter.total(), meter.status)
	for _, e := range eps {
		t.Logf("  %s", e)
	}
}
