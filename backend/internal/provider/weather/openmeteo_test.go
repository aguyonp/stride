package weather

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"stride/internal/apimeter"
)

// hourlyDay builds a synthetic Open-Meteo hourly payload for the given UTC day.
func hourlyDay(day time.Time, nullAt int) []byte {
	var h struct {
		Time   []int64    `json:"time"`
		Temp   []*float64 `json:"temperature_2m"`
		App    []*float64 `json:"apparent_temperature"`
		Hum    []*float64 `json:"relative_humidity_2m"`
		Wind   []*float64 `json:"wind_speed_10m"`
		Gust   []*float64 `json:"wind_gusts_10m"`
		Dir    []*float64 `json:"wind_direction_10m"`
		Precip []*float64 `json:"precipitation"`
		Code   []*float64 `json:"weather_code"`
	}
	for i := 0; i < 24; i++ {
		v := func(x float64) *float64 { return &x }
		h.Time = append(h.Time, day.Add(time.Duration(i)*time.Hour).Unix())
		if i == nullAt {
			h.Temp = append(h.Temp, nil)
		} else {
			h.Temp = append(h.Temp, v(float64(i)))
		}
		h.App = append(h.App, v(float64(i)-1))
		h.Hum = append(h.Hum, v(80))
		h.Wind = append(h.Wind, v(12.5))
		h.Gust = append(h.Gust, nil)
		h.Dir = append(h.Dir, v(225))
		h.Precip = append(h.Precip, v(0.2))
		h.Code = append(h.Code, v(61))
	}
	b, _ := json.Marshal(map[string]any{"latitude": 45, "longitude": 5, "hourly_units": map[string]string{"time": "unixtime"}, "hourly": h})
	return b
}

type recorder struct {
	mu   sync.Mutex
	reqs []url.Values
	path []string
}

func fakeOM(t *testing.T, status func(n int) int) (*httptest.Server, *recorder) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, r.URL.Query())
		rec.path = append(rec.path, r.URL.Path)
		n := len(rec.reqs)
		rec.mu.Unlock()
		if st := status(n); st != 200 {
			w.WriteHeader(st)
			_, _ = io.WriteString(w, `{"error":true,"reason":"Parameter error"}`)
			return
		}
		start, _ := time.Parse("2006-01-02", r.URL.Query().Get("start_date"))
		end, _ := time.Parse("2006-01-02", r.URL.Query().Get("end_date"))
		var all map[string]any
		var merged struct {
			Hourly map[string][]any `json:"hourly"`
		}
		merged.Hourly = map[string][]any{}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			_ = json.Unmarshal(hourlyDay(d, 7), &all)
			for k, v := range all["hourly"].(map[string]any) {
				merged.Hourly[k] = append(merged.Hourly[k], v.([]any)...)
			}
		}
		_ = json.NewEncoder(w).Encode(merged)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func provider(srv *httptest.Server, now time.Time, m apimeter.Meter) *OpenMeteo {
	return NewOpenMeteo(OpenMeteoOptions{ForecastURL: srv.URL + "/v1/forecast", ArchiveURL: srv.URL + "/v1/archive",
		Meter: m, Now: func() time.Time { return now }, RetryBackoff: time.Millisecond})
}

func TestNearestHourRoundingAndEndpoints(t *testing.T) {
	srv, rec := fakeOM(t, func(int) int { return 200 })
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	m := apimeter.NewLocal(apimeter.Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	p := provider(srv, now, m)

	c, err := p.At(context.Background(), 45.123456, 5.987654, time.Date(2026, 11, 3, 9, 40, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if *c.TemperatureC != 10 || c.ObservedAt.Hour() != 10 || *c.WeatherCode != 61 || c.Description != "Pluie faible" ||
		c.WindGustKph != nil || *c.WindKph != 12.5 || *c.HumidityPct != 80 || c.Source != "open-meteo" {
		t.Fatalf("conditions: %+v", c)
	}
	q := rec.reqs[0]
	if rec.path[0] != "/v1/forecast" || q.Get("latitude") != "45.12" || q.Get("longitude") != "5.99" || q.Get("wind_speed_unit") != "kmh" ||
		q.Get("hourly") != hourlyVars || q.Get("start_date") != "2026-11-03" || q.Get("timeformat") != "unixtime" {
		t.Fatalf("query: %s %v", rec.path[0], q)
	}

	// late start: the window spans two days; hour 07 has a null temperature and is skipped
	c, err = p.At(context.Background(), 45.1, 5.1, time.Date(2026, 9, 1, 7, 10, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rec.path[1] != "/v1/archive" || c.ObservedAt.Hour() != 8 {
		t.Fatalf("archive / null skip: %s %+v", rec.path[1], c)
	}
	c, err = p.At(context.Background(), 45.1, 5.1, time.Date(2026, 11, 2, 23, 50, 0, 0, time.UTC))
	if err != nil || rec.reqs[2].Get("end_date") != "2026-11-03" || !c.ObservedAt.Equal(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("midnight: %v %+v %v", err, c, rec.reqs[2])
	}
	if s := m.Snapshot().Providers[2]; s.Requests24h != 3 {
		t.Fatalf("meter: %+v", s)
	}
}

func TestErrorsAndRetries(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	srv, rec := fakeOM(t, func(n int) int {
		if n <= 2 {
			return http.StatusServiceUnavailable
		}
		return 200
	})
	p := provider(srv, now, nil)
	if _, err := p.At(context.Background(), 45, 5, now.Add(-time.Hour)); err != nil || len(rec.reqs) != 3 {
		t.Fatalf("5xx retried: %v (%d calls)", err, len(rec.reqs))
	}
	bad, _ := fakeOM(t, func(int) int { return http.StatusBadRequest })
	if _, err := provider(bad, now, nil).At(context.Background(), 45, 5, now.Add(-time.Hour)); err == nil {
		t.Fatal("400 must fail")
	}
	if _, err := p.At(context.Background(), 0, 0, now); !errors.Is(err, ErrInvalidCoordinate) {
		t.Fatalf("null island: %v", err)
	}
	if _, err := p.At(context.Background(), 95, 5, now); !errors.Is(err, ErrInvalidCoordinate) {
		t.Fatalf("lat range: %v", err)
	}
	if _, err := p.At(context.Background(), 45, 5, now.Add(48*time.Hour)); !errors.Is(err, ErrNoData) {
		t.Fatalf("future: %v", err)
	}
}

func TestLabels(t *testing.T) {
	for deg, want := range map[float64]string{0: "N", 44: "NE", 90: "E", 180: "S", 225: "SO", 270: "O", 337.6: "N", -45: "NO", 359: "N"} {
		if got := WindDirLabel(deg); got != want {
			t.Errorf("%v° → %s, want %s", deg, got, want)
		}
	}
	if Describe(3) != "Couvert" || Describe(1234) != "Conditions inconnues" || Round2(2.349) != 2.35 || Round2(-0.005) != -0.01 {
		t.Fatal("describe / round")
	}
}

func TestForecastFutureSlot(t *testing.T) {
	srv, rec := fakeOM(t, func(int) int { return 200 })
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	p := provider(srv, now, apimeter.Nop())
	var _ Forecaster = p
	c, err := p.Forecast(context.Background(), 45.123456, 5.987654, time.Date(2026, 11, 5, 17, 50, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rec.path[0] != "/v1/forecast" || rec.reqs[0].Get("latitude") != "45.12" || *c.TemperatureC != 18 || c.Source != "open-meteo-forecast" {
		t.Fatalf("forecast: %s %v %+v", rec.path[0], rec.reqs[0], c)
	}
	for _, at := range []time.Time{now.Add(-3 * time.Hour), now.AddDate(0, 0, 20)} {
		if _, err := p.Forecast(context.Background(), 45, 5, at); !errors.Is(err, ErrNoData) {
			t.Fatalf("%s: %v", at, err)
		}
	}
	if _, err := p.Forecast(context.Background(), 0, 0, now.Add(time.Hour)); !errors.Is(err, ErrInvalidCoordinate) {
		t.Fatal("null island accepted")
	}
}
