package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"stride/internal/garminbridge"
	"stride/internal/store"
)

// apiFake is the Connect web API as the bridge sees it: the workout service only. The login flow has
// its own tests in internal/garminbridge; here the tokens are seeded, so no SSO is involved.
type apiFake struct {
	mu        sync.Mutex
	requests  []string
	workouts  []map[string]any
	nextID    int64
	blockAPIs bool // answers 429 on everything, as Garmin does when it throttles
	createN   int
	putN      int
}

func (f *apiFake) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, s)
}

func (f *apiFake) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func (f *apiFake) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}

func newAPIFake(t *testing.T) (*apiFake, *httptest.Server) {
	t.Helper()
	f := &apiFake{nextID: 500}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /workout-service/workouts", func(w http.ResponseWriter, r *http.Request) {
		f.record("GET workouts")
		f.mu.Lock()
		blocked, rows := f.blockAPIs, f.workouts
		f.mu.Unlock()
		if blocked {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(rows)
	})
	mux.HandleFunc("POST /workout-service/workout", func(w http.ResponseWriter, r *http.Request) {
		f.record("POST workout")
		f.mu.Lock()
		f.createN++
		f.nextID++
		id := f.nextID
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"workoutId": id})
	})
	mux.HandleFunc("PUT /workout-service/workout/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.record("PUT workout")
		f.mu.Lock()
		f.putN++
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /workout-service/schedule/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.record("POST schedule")
		f.mu.Lock()
		blocked := f.blockAPIs
		f.mu.Unlock()
		if blocked {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

// bridgeDomain builds a domain with the experimental bridge wired to the fake API.
func bridgeDomain(t *testing.T) (*Services, *apiFake, *garminbridge.Service) {
	t.Helper()
	s := inboxServices(t) // real store, two users, no provider
	f, srv := newAPIFake(t)
	bridge := garminbridge.New(garminbridge.Options{
		HTTP: srv.Client(), Store: s.BridgeTokenStore(), SSOBase: srv.URL, APIBase: srv.URL,
		Now: func() time.Time { return time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC) },
	})
	s.SetGarminBridge(bridge)
	return s, f, bridge
}

// enableBridge flips the admin setting (the env switch is the wiring's decision).
func enableBridge(t *testing.T, s *Services, on bool) {
	t.Helper()
	v := "false"
	if on {
		v = "true"
	}
	if err := s.deps.Store.SetSetting(context.Background(), store.SettingGarminBridgeEnabled, v, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// connectBridge stores tokens for a user: the presence of tokens is the user switch.
func connectBridge(t *testing.T, s *Services, uid int64) {
	t.Helper()
	err := s.BridgeTokenStore().Save(context.Background(), uid, garminbridge.Tokens{
		AccessToken: "bearer-1", RefreshToken: "oauth1-secret",
		CreatedAt:   time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC),
		ExpiresAt:   time.Date(2027, 11, 26, 8, 0, 0, 0, time.UTC),
		DisplayName: "Antoine",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newSession(t *testing.T, s *Services, date string) int64 {
	t.Helper()
	ps, err := s.CreatePlanned(context.Background(), 1, []byte(`{"date":"`+date+`","sport":"run","title":"Footing","steps":[{"type":"warmup","durationS":600},{"type":"interval","durationS":300}]}`))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return ps.ID
}

func garminState(t *testing.T, s *Services, id int64) (string, int64) {
	t.Helper()
	var status string
	var workout *int64
	if err := s.db().QueryRow(`SELECT garmin_status, garmin_workout_id FROM planned_sessions WHERE id = ?`, id).Scan(&status, &workout); err != nil {
		t.Fatal(err)
	}
	if workout == nil {
		return status, 0
	}
	return status, *workout
}

func TestBridgeKillSwitch(t *testing.T) {
	s, f, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, false)
	connectBridge(t, s, 1)

	if s.BridgeAvailable(ctx) {
		t.Fatal("bridge reported available with the setting off")
	}
	// Not available, but the tokens are still there: switching the bridge off hides it, it does not
	// disconnect anyone. The card is hidden, the state stays readable.
	if st := s.BridgeStatus(ctx, 1); st.State != garminbridge.StateDisconnected || st.Available {
		t.Fatalf("status: %+v", st)
	}
	if _, err := s.BridgeLogin(ctx, 1, "a@b.c", "pw"); err != garminbridge.ErrDisabled {
		t.Fatalf("login: %v", err)
	}
	// The manual push falls back to the Personal API, which is not connected here.
	id := newSession(t, s, "2026-11-26")
	if _, err := s.PushGarmin(ctx, 1, id); err == nil {
		t.Fatal("push succeeded without any Garmin connection")
	}
	if n := f.count("GET workouts"); n != 0 {
		t.Fatalf("the gateway was called with the bridge off: %d", n)
	}
}

// Available is the flag the settings card reads: false keeps it hidden, true offers the flow.
func TestBridgeAvailability(t *testing.T) {
	s, _, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, true)
	if st := s.BridgeStatus(ctx, 1); !st.Available || st.State != garminbridge.StateDisconnected {
		t.Fatalf("available: %+v", st)
	}
	connectBridge(t, s, 1)
	if st := s.BridgeStatus(ctx, 1); !st.Available || st.State != garminbridge.StateConnected {
		t.Fatalf("connected: %+v", st)
	}
	enableBridge(t, s, false)
	if st := s.BridgeStatus(ctx, 1); st.Available {
		t.Fatalf("hidden: %+v", st)
	}
}

func TestBridgePublisherIsUsedWhenConnected(t *testing.T) {
	s, f, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, true)

	if s.BridgeAvailable(ctx) != true {
		t.Fatal("bridge not available with the setting on")
	}
	// No tokens yet: the Personal API stays the publisher (and fails, which is not our concern here).
	id := newSession(t, s, "2026-11-26")
	if _, err := s.PushGarmin(ctx, 1, id); err == nil || f.count("POST workout") != 0 {
		t.Fatalf("personal path used the bridge: %v", err)
	}

	connectBridge(t, s, 1)
	ps, err := s.PushGarmin(ctx, 1, id)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if ps.Garmin.Status != "pushed" || ps.Garmin.WorkoutID == nil {
		t.Fatalf("session: %+v", ps.Garmin)
	}
	if f.count("POST workout") != 1 || f.count("POST schedule") != 1 {
		t.Fatalf("calls: %v", f.requests)
	}
}

func TestPushIsIdempotent(t *testing.T) {
	s, f, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, true)
	connectBridge(t, s, 1)
	id := newSession(t, s, "2026-11-26")

	if _, err := s.PushGarmin(ctx, 1, id); err != nil {
		t.Fatal(err)
	}
	f.reset()
	// The second push reuses the stored workout id: an update, never a duplicate.
	if _, err := s.PushGarmin(ctx, 1, id); err != nil {
		t.Fatal(err)
	}
	if f.count("POST workout") != 0 || f.count("PUT workout") != 1 {
		t.Fatalf("second push: %v", f.requests)
	}
	if st, wid := garminState(t, s, id); st != "pushed" || wid == 0 {
		t.Fatalf("state: %s %d", st, wid)
	}
}

func TestPushTodaySkipsAndReports(t *testing.T) {
	s, f, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, true)
	connectBridge(t, s, 1)
	today := "2026-11-26"

	first := newSession(t, s, today)
	todo := newSession(t, s, today)
	// One already pushed, one to send, one without steps (skipped).
	if _, err := s.PushGarmin(ctx, 1, first); err != nil {
		t.Fatal(err)
	}
	plain, err := s.CreatePlanned(ctx, 1, []byte(`{"date":"`+today+`","sport":"run","title":"Sans structure"}`))
	if err != nil {
		t.Fatal(err)
	}
	f.reset()

	results, err := s.PushToday(ctx, 1, today)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results: %+v", results)
	}
	byID := map[int64]PushTodayResult{}
	for _, r := range results {
		byID[r.SessionID] = r
	}
	if byID[first].Status != "skipped" || !strings.Contains(byID[first].Message, "Déjà envoyée") {
		t.Fatalf("already pushed: %+v", byID[first])
	}
	if byID[todo].Status != "pushed" || byID[todo].WorkoutID == 0 {
		t.Fatalf("to send: %+v", byID[todo])
	}
	if byID[plain.ID].Status != "skipped" {
		t.Fatalf("without steps: %+v", byID[plain.ID])
	}
	if f.count("POST workout") != 1 {
		t.Fatalf("one workout expected: %v", f.requests)
	}
}

func TestBridgeBreakerAfterRepeatedBlocks(t *testing.T) {
	s, f, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, true)
	connectBridge(t, s, 1)
	f.blockAPIs = true

	ids := []int64{newSession(t, s, "2026-11-26"), newSession(t, s, "2026-11-26"), newSession(t, s, "2026-11-26")}
	for _, id := range ids {
		if _, err := s.PushGarmin(ctx, 1, id); err == nil {
			t.Fatal("push succeeded while Garmin blocks")
		}
	}
	if !s.BridgeLocked(1) {
		t.Fatal("breaker not tripped")
	}
	before := len(f.requests)
	extra := newSession(t, s, "2026-11-26")
	_, err := s.PushGarmin(ctx, 1, extra)
	if err == nil || !strings.Contains(errText(err), "bloqué") {
		t.Fatalf("locked push: %v", err)
	}
	if len(f.requests) != before {
		t.Fatalf("the breaker still called Garmin: %v", f.requests[before:])
	}
	// Disconnecting clears it.
	if err := s.BridgeDisconnect(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if s.BridgeLocked(1) {
		t.Fatal("breaker survived a disconnect")
	}
}

func TestExpiredBridgeNotifiesAndFallsBackToFIT(t *testing.T) {
	s, _, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, true)
	// Expired tokens without a refresh secret: the push can only fail.
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.BridgeTokenStore().Save(ctx, 1, garminbridge.Tokens{AccessToken: "old", CreatedAt: past, ExpiresAt: past})
	id := newSession(t, s, "2026-11-26")

	if _, err := s.PushGarmin(ctx, 1, id); err == nil {
		t.Fatal("expired bridge pushed")
	}
	if st, _ := garminState(t, s, id); st != "failed" {
		t.Fatalf("status: %s", st)
	}
	page, err := s.Inbox(ctx, 1, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range page.Items {
		if e.Kind == InboxKindGarminBridge && e.Code == "GARMIN_BRIDGE_EXPIRED" && e.Level == "warn" && e.URL == "/settings#integrations" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no bridge notification: %+v", page.Items)
	}
}

// The fallback stays useful without the bridge: the file name is what the import checklist names.
func TestWorkoutFITName(t *testing.T) {
	s, _, _ := bridgeDomain(t)
	ctx := context.Background()
	id := newSession(t, s, "2026-11-26")
	_, name, err := s.WorkoutFIT(ctx, 1, id)
	if err != nil {
		t.Fatal(err)
	}
	if name != "stride-2026-11-26-run.fit" {
		t.Fatalf("name: %q", name)
	}
}

func TestBridgeDisconnectRemovesTokens(t *testing.T) {
	s, _, _ := bridgeDomain(t)
	ctx := context.Background()
	enableBridge(t, s, true)
	connectBridge(t, s, 1)
	if st := s.BridgeStatus(ctx, 1); st.State != garminbridge.StateConnected {
		t.Fatalf("status: %+v", st)
	}
	if err := s.BridgeDisconnect(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if st := s.BridgeStatus(ctx, 1); st.State != garminbridge.StateDisconnected {
		t.Fatalf("after disconnect: %+v", st)
	}
}
