//go:build live

// 3.1.0 live validation: env seed (Garmin key + Strava tokens), Garmin sync, views, reports,
// records, Open-Meteo weather coverage, Garmin push capability and Strava /athlete + stats.
//
//	STRIDE_LIVE_ENV=/path/to/.env go test -tags live -run TestLive31 -v ./internal/domain/
//
// The env file is read in-process; values are never printed. Only structure, counts, coverage
// and timings are logged. The Strava token is refreshed only if Strava rejects it.
package domain_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/provider/strava"
	"stride/internal/provider/weather"
	"stride/internal/report"
	"stride/internal/secret"
	"stride/internal/store"
)

func readEnv(t *testing.T) map[string]string {
	path := os.Getenv("STRIDE_LIVE_ENV")
	if path == "" {
		path = "../../../.env"
	}
	f, err := os.Open(path)
	if err != nil {
		t.Skip("env file not readable")
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export "), "=")
		if ok && !strings.HasPrefix(strings.TrimSpace(k), "#") {
			out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return out
}

func TestLive31(t *testing.T) {
	kv := readEnv(t)
	garminKey := kv["STRIDE_ADMIN_GARMIN_KEY"]
	if garminKey == "" {
		garminKey = kv["GARMIN_API_TOKEN"]
	}
	if garminKey == "" {
		t.Skip("no Garmin key in env file")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/live31.db")
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
		WeatherProvider: "open-meteo", AdminGarminKey: garminKey,
		StravaClientID: kv["STRIDE_STRAVA_CLIENT_ID"], StravaClientSecret: kv["STRIDE_STRAVA_CLIENT_SECRET"],
		AdminStravaAccessToken: kv["STRIDE_STRAVA_ACCESS_TOKEN"], AdminStravaRefreshToken: kv["STRIDE_STRAVA_REFRESH_TOKEN"]}
	deps := domain.Deps{Env: cfg, Store: st, Cache: cache.NewMemory(), Sealer: sealer, Meter: meter,
		Weather: weather.NewOpenMeteo(weather.OpenMeteoOptions{Meter: meter}), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if cfg.StravaEnabled() {
		deps.Strava = strava.NewClient(strava.Config{ClientID: cfg.StravaClientID, ClientSecret: cfg.StravaClientSecret, Meter: meter})
	}
	svc, err := domain.New(deps)
	if err != nil {
		t.Fatal(err)
	}
	step := func(name string, fn func() error) {
		before := meter.total()
		t0 := time.Now()
		if err := fn(); err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		t.Logf("⏱ %-30s %7s  provider calls: %d", name, time.Since(t0).Round(time.Millisecond), meter.total()-before)
	}

	step("seed admin + first sync", func() error {
		if err := svc.OnUserCreated(ctx, 1, true); err != nil {
			return err
		}
		svc.Wait()
		in, err := svc.Integrations(ctx, 1)
		if err != nil {
			return err
		}
		t.Logf("integrations: garmin connected=%v status=%s | strava available=%v connected=%v status=%s scopes=%v missing=%v lastError=%v",
			in.Garmin.Connected, in.Garmin.Status, in.Strava.Available, in.Strava.Connected, in.Strava.Status, in.Strava.Scopes,
			in.Strava.MissingScopes, in.Strava.LastError != nil)
		if !in.Garmin.Connected {
			return errors.New("garmin not connected")
		}
		return nil
	})
	step("strava /athlete stats", func() error {
		s, err := svc.StravaStats(ctx, 1)
		if errors.Is(err, domain.ErrNotConnected) {
			t.Log("strava stats: not connected (no env tokens or Strava app)")
			return nil
		}
		if err != nil {
			return err
		}
		t.Logf("strava stats: recentRun.count=%d ytdRun.count=%d allRun.count=%d recentRide.count=%d (distances present: %v)",
			s.RecentRun.Count, s.YTDRun.Count, s.AllRun.Count, s.RecentRide.Count, s.YTDRun.DistanceM > 0)
		return nil
	})

	var ids []int64
	var runID int64
	step("list activities", func() error {
		l, err := svc.ListActivities(ctx, 1, 100, "", "", "", "")
		if err != nil {
			return err
		}
		sports := map[string]int{}
		kudos := 0
		for _, it := range l.Items {
			sports[it.Sport]++
			if it.Kudos != nil {
				kudos++
			}
			if len(ids) < 15 {
				ids = append(ids, it.ID)
			}
			if runID == 0 && it.IsRunning {
				runID = it.ID
			}
		}
		t.Logf("activities: %d by sport %v, with kudos %d", len(l.Items), sports, kudos)
		return nil
	})

	gps, gpsWeather, noGPS := 0, 0, 0
	reasons := map[string]int{}
	step(fmt.Sprintf("views + weather (%d recent)", len(ids)), func() error {
		for _, id := range ids {
			v, err := svc.ActivityView(ctx, 1, id)
			if err != nil {
				return err
			}
			tr, err := svc.Track(ctx, 1, id)
			if err != nil {
				return err
			}
			if len(tr.Points) > 0 {
				gps++
				if v.Weather != nil {
					gpsWeather++
				}
			} else {
				noGPS++
			}
			if v.WeatherReason != nil {
				reasons[*v.WeatherReason]++
			}
		}
		pct := 0
		if gps > 0 {
			pct = gpsWeather * 100 / gps
		}
		t.Logf("weather: GPS activities %d, with weather %d (%d%%); no-GPS %d; null reasons %v", gps, gpsWeather, pct, noGPS, reasons)
		return nil
	})
	step("views again (warm, backfill)", func() error {
		for _, id := range ids {
			if _, err := svc.ActivityView(ctx, 1, id); err != nil {
				return err
			}
		}
		return nil
	})

	for _, pass := range []string{"cold", "warm"} {
		step("records "+pass, func() error {
			r, err := svc.Records(ctx, 1)
			if err != nil {
				return err
			}
			var parts []string
			for _, rec := range r["records"] {
				parts = append(parts, fmt.Sprintf("%s:%s/%v", rec.Key, rec.Source, rec.TimeS != nil || (rec.Key == "longest" && rec.ActivityID != nil)))
			}
			t.Logf("records %s (key:source/known): %s", pass, strings.Join(parts, " "))
			return nil
		})
	}

	if runID != 0 {
		step("activity report", func() error {
			md, err := svc.ActivityReport(ctx, 1, runID, "")
			if err == nil {
				t.Logf("activity report ≈ %d tokens, records line: %v, N/D ×%d", report.EstimateTokens(md), strings.Contains(md, "Records personnels"), strings.Count(md, "N/D"))
			}
			return err
		})
	}
	today, _ := svc.ResolveDate(ctx, 1, "today")
	step("day report", func() error {
		md, err := svc.DayReport(ctx, 1, today, "")
		if err == nil {
			t.Logf("day report ≈ %d tokens, records line: %v", report.EstimateTokens(md), strings.Contains(md, "Records personnels"))
		}
		return err
	})
	step("goal prompt (phases)", func() error {
		target := time.Now().AddDate(0, 0, 70).Format("2006-01-02")
		if _, err := svc.CreateGoal(ctx, 1, []byte(`{"title":"10 km","kind":"race","targetDate":"`+target+`"}`)); err != nil {
			return err
		}
		res, err := svc.GeneratePlan(ctx, 1, domain.PlanGenerateInput{Mode: "prompt", Horizon: "goal"})
		if err != nil {
			return err
		}
		p := res.(*domain.PlanPromptResult)
		t.Logf("goal prompt ≈ %d tokens, contract v2: %v", p.TokensEstimate, strings.Contains(p.Prompt, `"version":2`))
		return nil
	})
	step("garmin push (expect unsupported)", func() error {
		date := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
		ps, err := svc.CreatePlanned(ctx, 1, []byte(`{"date":"`+date+`","sport":"run","title":"Test Stride","steps":[{"type":"warmup","durationS":600},{"type":"repeat","repeat":3,"steps":[{"type":"interval","durationS":60},{"type":"recovery","durationS":60}]},{"type":"cooldown","durationS":300}]}`))
		if err != nil {
			return err
		}
		_, err = svc.PushGarmin(ctx, 1, ps.ID)
		var pu *domain.PushUnsupportedError
		switch {
		case errors.As(err, &pu):
			t.Logf("garmin push: unsupported (status %s), FIT fallback message present: %v", pu.Session.Garmin.Status, strings.Contains(pu.Error(), ".FIT"))
		case err != nil:
			t.Logf("garmin push: error %v", err)
		default:
			t.Log("garmin push: ACCEPTED (Personal API now supports workouts)")
		}
		return nil
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
