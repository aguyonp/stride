package strava

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"stride/internal/apimeter"
)

const fakeActivity = `{
  "id": 1001, "name": "Footing", "description": "Sortie facile", "private_note": "genou ok",
  "sport_type": "Run", "type": "Run",
  "start_date": "2026-11-03T06:30:00Z", "start_date_local": "2026-11-03T07:30:00Z", "timezone": "(GMT+01:00) Europe/Paris",
  "distance": 8012.5, "moving_time": 2700, "elapsed_time": 2760, "total_elevation_gain": 42.0, "average_speed": 2.97,
  "average_heartrate": 141.2, "max_heartrate": 160, "average_cadence": 85.5, "suffer_score": 37, "perceived_exertion": 4,
  "gear_id": "g42", "gear": {"id": "g42", "name": "Daily", "brand_name": "Brand", "model_name": "Model", "distance": 412000, "primary": true},
  "start_latlng": [45.0, 5.0], "device_name": "Watch", "external_id": "x.fit", "manual": false
}`

type fake struct {
	t        *testing.T
	mu       sync.Mutex
	hits     map[string]int
	fail5xx  atomic.Int32
	handlers map[string]http.HandlerFunc
}

func newFake(t *testing.T) (*fake, *httptest.Server) {
	f := &fake{t: t, hits: map[string]int{}, handlers: map[string]http.HandlerFunc{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		f.hits[key]++
		h := f.handlers[key]
		f.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fake) on(key string, h http.HandlerFunc) {
	f.mu.Lock()
	f.handlers[key] = h
	f.mu.Unlock()
}

func (f *fake) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[key]
}

func newTestClient(srv *httptest.Server, m apimeter.Meter) *Client {
	return NewClient(Config{ClientID: "123", ClientSecret: "app-secret", BaseURL: srv.URL, Meter: m, RetryBackoff: time.Millisecond})
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestAuthorizeURLAndTokenExchange(t *testing.T) {
	f, srv := newFake(t)
	c := newTestClient(srv, nil)
	u, _ := url.Parse(c.AuthorizeURL("st4te", "https://stride.example/api/integrations/strava/callback"))
	q := u.Query()
	if u.Path != "/oauth/authorize" || q.Get("client_id") != "123" || q.Get("scope") != DefaultScope || q.Get("state") != "st4te" || q.Get("response_type") != "code" {
		t.Fatalf("authorize url: %s", u)
	}
	f.on("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.RawQuery != "" || r.PostForm.Get("client_secret") != "app-secret" {
			t.Errorf("credentials must be in the body: %q", r.URL.RawQuery)
		}
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			if r.PostForm.Get("code") != "the-code" {
				t.Errorf("code %q", r.PostForm.Get("code"))
			}
			_, _ = io.WriteString(w, `{"token_type":"Bearer","access_token":"acc","refresh_token":"ref","expires_at":1800000000,"athlete":{"id":7,"firstname":"Test","lastname":"Runner"}}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"message":"Bad Request","errors":[{"resource":"RefreshToken","field":"refresh_token","code":"invalid"}]}`)
		}
	})
	tok, err := c.Exchange(context.Background(), "the-code")
	if err != nil || tok.AccessToken != "acc" || tok.RefreshToken != "ref" || tok.AthleteID != 7 || tok.AthleteName != "Test Runner" || tok.ExpiresAt.Unix() != 1800000000 {
		t.Fatalf("exchange: %+v %v", tok, err)
	}
	_, err = c.Refresh(context.Background(), "revoked")
	var ue *ErrUnauthorized
	if !errors.As(err, &ue) || strings.Contains(err.Error(), "revoked") {
		t.Fatalf("refresh with revoked token: %v", err)
	}
	if f.count("POST /oauth/token") != 2 {
		t.Fatalf("POST must not be retried")
	}
}

func TestActivityMappingQuotaAndUnauthorized(t *testing.T) {
	f, srv := newFake(t)
	m := apimeter.NewLocal(apimeter.Options{Log: quiet()})
	c := newTestClient(srv, m)
	f.on("GET /api/v3/activities/1001", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer acc" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"Authorization Error","errors":[{"resource":"Athlete","field":"access_token","code":"invalid"}]}`)
			return
		}
		w.Header().Set("X-RateLimit-Limit", "200,2000")
		w.Header().Set("X-RateLimit-Usage", "10,100")
		w.Header().Set("X-ReadRateLimit-Limit", "100,1000")
		w.Header().Set("X-ReadRateLimit-Usage", "9,99")
		_, _ = io.WriteString(w, fakeActivity)
	})
	ctx := apimeter.WithUser(context.Background(), 5)
	a, err := c.GetActivity(ctx, "acc", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if a.PrivateNote != "genou ok" || a.Description != "Sortie facile" || *a.PerceivedExertion != 4 || *a.SufferScore != 37 ||
		a.Gear == nil || a.Gear.Kind != "shoe" || a.Gear.BrandName != "Brand" || a.GearID != "g42" ||
		a.StartDateLocal != "2026-11-03T07:30:00" || a.Timezone != "Europe/Paris" || *a.AverageCadenceRaw != 85.5 ||
		a.StartDate.Hour() != 6 || len(a.StartLatLng) != 2 || a.FetchedAt == nil || a.MaxHeartrate == nil || a.AverageWatts != nil {
		t.Fatalf("mapping: %+v", a)
	}
	q := m.Snapshot().Providers[1].Quota
	if q == nil || q.Limit15m != 100 || q.Usage15m != 9 || q.LimitDaily != 1000 {
		t.Fatalf("quota must follow the tighter read limit: %+v", q)
	}
	_, err = c.GetActivity(ctx, "bad", 1001)
	var ue *ErrUnauthorized
	if !errors.As(err, &ue) || !strings.Contains(ue.Msg, "Authorization Error") {
		t.Fatalf("401: %v", err)
	}
	if f.count("GET /api/v3/activities/1001") != 2 {
		t.Fatal("401 must not be retried")
	}
	snap := m.Snapshot()
	if len(snap.RecentErrors) != 1 || snap.RecentErrors[0].Endpoint != "GET /activities/{id}" || snap.RecentErrors[0].UserID != 5 {
		t.Fatalf("meter error event: %+v", snap.RecentErrors)
	}
	if _, err := c.GetActivity(ctx, "acc", 404); !IsNotFound(err) {
		t.Fatalf("404: %v", err)
	}
}

func TestListStreamsAndGear(t *testing.T) {
	f, srv := newFake(t)
	c := newTestClient(srv, nil)
	f.on("GET /api/v3/athlete/activities", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("after") != "1762128000" || q.Get("before") != "" || q.Get("page") != "2" || q.Get("per_page") != "200" {
			t.Errorf("list query %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `[`+fakeActivity+`,{"id":1002,"type":"Ride","start_date":"2026-11-02T10:00:00Z"}]`)
	})
	list, err := c.ListActivities(context.Background(), "acc", time.Unix(1762128000, 0), time.Time{}, 2, 500)
	if err != nil || len(list) != 2 || list[1].SportType != "Ride" || list[1].Gear != nil {
		t.Fatalf("list: %+v %v", list, err)
	}

	f.on("GET /api/v3/activities/1001/streams", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key_by_type") != "true" || !strings.Contains(r.URL.Query().Get("keys"), "grade_smooth") {
			t.Errorf("streams query %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"time":{"data":[0,1,2]},"distance":{"data":[0,2.9,5.8]},"cadence":{"data":[84,85,86]},
			"moving":{"data":[true,true,false]},"latlng":{"data":[[45.0,5.0],[45.00001,5.00001],[45.00002,5.00002]]},
			"grade_smooth":{"data":[0.5,-2.0,1.0]},"temp":{"data":[12,12,12]},"heartrate":{"data":[120,130,140]}}`)
	})
	s, err := c.GetStreams(context.Background(), "acc", 1001)
	if err != nil || len(s.TimeS) != 3 || s.CadenceRaw[2] != 86 || s.Moving[2] || s.LatLng[1][0] != 45.00001 || s.GradePct[1] != -2 || s.Watts != nil || len(s.TemperatureC) != 3 {
		t.Fatalf("streams: %+v %v", s, err)
	}

	f.on("GET /api/v3/athlete", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":7,"shoes":[{"id":"g1","name":"Daily","distance":1000,"primary":true}],"bikes":[{"id":"b2","name":"Road","distance":5000}]}`)
	})
	gear, err := c.ListAthleteGear(context.Background(), "acc")
	if err != nil || len(gear) != 2 || gear[0].Kind != "shoe" || gear[1].Kind != "bike" || !gear[0].Primary {
		t.Fatalf("gear: %+v %v", gear, err)
	}
	f.on("GET /api/v3/gear/b2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"b2","name":"Road","brand_name":"B","model_name":"M","frame_type":3,"retired":true}`)
	})
	g, err := c.GetGear(context.Background(), "acc", "b2")
	if err != nil || g.Kind != "bike" || !g.Retired || g.ModelName != "M" {
		t.Fatalf("get gear: %+v %v", g, err)
	}
}

func TestRetriesAndRateLimitCooldown(t *testing.T) {
	f, srv := newFake(t)
	m := apimeter.NewLocal(apimeter.Options{Log: quiet()})
	c := newTestClient(srv, m)
	var calls atomic.Int32
	f.on("GET /api/v3/athlete", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `{"shoes":[]}`)
	})
	if _, err := c.ListAthleteGear(context.Background(), "acc"); err != nil {
		t.Fatalf("5xx must be retried twice: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}

	f.on("GET /api/v3/activities/7", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "200,2000")
		w.Header().Set("X-RateLimit-Usage", "201,500")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"message":"Rate Limit Exceeded"}`)
	})
	_, err := c.GetActivity(context.Background(), "acc", 7)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 429 {
		t.Fatalf("429: %v", err)
	}
	st := m.Snapshot().Providers[1]
	if st.CooldownUntil == nil || st.CooldownUntil.Sub(time.Now()) > 15*time.Minute || st.RateLimited24h != 1 {
		t.Fatalf("cooldown until next quarter hour: %+v", st)
	}
	if _, err := c.GetActivity(context.Background(), "acc", 7); !errors.Is(err, apimeter.ErrCooldown) {
		t.Fatalf("cooldown must fail fast: %v", err)
	}
	if f.count("GET /api/v3/activities/7") != 1 {
		t.Fatal("no request may be sent while cooling down")
	}
}

func TestRetryAfterComputation(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 7, 30, 0, time.UTC)
	h := http.Header{}
	if d := retryAfter(h, window{100, 100, true}, window{1000, 10, true}, now); d != 7*time.Minute+30*time.Second {
		t.Fatalf("15-min window: %s", d)
	}
	if d := retryAfter(h, window{100, 5, true}, window{1000, 1000, true}, now); d != 13*time.Hour+52*time.Minute+30*time.Second {
		t.Fatalf("daily window: %s", d)
	}
	h.Set("Retry-After", "42")
	if d := retryAfter(h, window{}, window{}, now); d != 42*time.Second {
		t.Fatalf("header: %s", d)
	}
}

func TestEnsureSubscription(t *testing.T) {
	f, srv := newFake(t)
	c := newTestClient(srv, nil)
	existing := `[{"id":11,"callback_url":"https://old.example/api/webhooks/strava"}]`
	f.on("GET /api/v3/push_subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("client_id") != "123" {
			t.Errorf("list query %s", r.URL.Query().Get("client_id"))
		}
		_, _ = io.WriteString(w, existing)
	})
	f.on("DELETE /api/v3/push_subscriptions/11", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	f.on("POST /api/v3/push_subscriptions", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("callback_url") != "https://stride.example/api/webhooks/strava" || r.PostForm.Get("verify_token") != "vt" {
			t.Errorf("create form %v", r.PostForm.Get("callback_url"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":12}`)
	})
	id, err := c.EnsureSubscription(context.Background(), "https://stride.example/api/webhooks/strava", "vt")
	if err != nil || id != 12 || f.count("DELETE /api/v3/push_subscriptions/11") != 1 {
		t.Fatalf("create: %d %v", id, err)
	}
	existing = `[{"id":12,"callback_url":"https://stride.example/api/webhooks/strava"}]`
	id, err = c.EnsureSubscription(context.Background(), "https://stride.example/api/webhooks/strava", "vt")
	if err != nil || id != 12 || f.count("POST /api/v3/push_subscriptions") != 1 {
		t.Fatalf("reuse: %d %v", id, err)
	}
}

func TestAthleteStatsKudosAndScopeErrors(t *testing.T) {
	f, srv := newFake(t)
	c := newTestClient(srv, nil)
	ctx := context.Background()
	f.on("GET /api/v3/athlete", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"Authorization Error","errors":[{"resource":"AccessToken","field":"access_token","code":"invalid"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id": 77, "firstname": "Prénom", "lastname": "Nom", "resource_state": 2}`)
	})
	f.on("GET /api/v3/athletes/77/stats", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"recent_run_totals":{"count":3,"distance":25000.5,"moving_time":9000,"elevation_gain":120},
			"ytd_run_totals":{"count":40,"distance":400000},"all_run_totals":{"count":100},"recent_ride_totals":{"count":1},"ytd_ride_totals":{}}`)
	})
	f.on("GET /api/v3/athlete/activities", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Authorization Error","errors":[{"resource":"AccessToken","field":"activity:read_permission","code":"missing"}]}`)
	})
	f.on("GET /api/v3/activities/1001", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.TrimSuffix(fakeActivity, "}")+`, "kudos_count": 4,
			"best_efforts": [{"name": "1k", "distance": 1000, "elapsed_time": 271}, {"name": "5k", "distance": 5000, "elapsed_time": 1402}]}`)
	})
	a, err := c.GetAthlete(ctx, "tok")
	if err != nil || a.ID != 77 || a.Name != "Prénom Nom" || a.Detailed {
		t.Fatalf("athlete: %+v %v", a, err)
	}
	if _, err := c.GetAthlete(ctx, "expired"); !TokenRejected(err) || MissingScope(err) {
		t.Fatalf("expired token: %v", err)
	}
	st, err := c.GetAthleteStats(ctx, "tok", 77)
	if err != nil || st.RecentRun.Count != 3 || st.RecentRun.DistanceM != 25000.5 || st.YTDRun.Count != 40 || st.AllRun.Count != 100 || st.RecentRide.Count != 1 {
		t.Fatalf("stats: %+v %v", st, err)
	}
	if _, err := c.ListActivities(ctx, "tok", time.Time{}, time.Time{}, 1, 1); !MissingScope(err) || TokenRejected(err) {
		t.Fatalf("missing scope: %v", err)
	}
	act, err := c.GetActivity(ctx, "tok", 1001)
	if err != nil || act.KudosCount == nil || *act.KudosCount != 4 || len(act.BestEfforts) != 2 || act.BestEfforts[1].ElapsedS != 1402 {
		t.Fatalf("activity: %+v %v", act, err)
	}
	if MissingScope(errors.New("x")) || TokenRejected(nil) {
		t.Fatal("non-strava errors")
	}
}
