package domainapi_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"stride/internal/domain"
	"stride/internal/provider/strava"
)

// gearStrava scripts the Strava gear endpoints on top of the shared fake.
type gearStrava struct {
	*fakeStrava
	mu     sync.Mutex
	scope  string
	list   []strava.Gear
	detail map[string]strava.Gear // missing id: 404 (deleted on Strava)
	gets   map[string]int
}

func (g *gearStrava) Exchange(ctx context.Context, code string) (strava.Tokens, error) {
	t, err := g.fakeStrava.Exchange(ctx, code)
	if g.scope != "" {
		t.Scope = g.scope
	}
	return t, err
}
func (g *gearStrava) Refresh(ctx context.Context, rt string) (strava.Tokens, error) {
	return g.Exchange(ctx, rt)
}
func (g *gearStrava) ListAthleteGear(context.Context, string) ([]strava.Gear, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.scope != "" && !strings.Contains(g.scope, "profile:read_all") {
		return nil, strava.ErrGearScope
	}
	return append([]strava.Gear(nil), g.list...), nil
}
func (g *gearStrava) GetGear(_ context.Context, _, id string) (*strava.Gear, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gets[id]++
	d, ok := g.detail[id]
	if !ok {
		return nil, &strava.APIError{Status: 404, Endpoint: "GET /gear/{id}", Message: "Record Not Found"}
	}
	return &d, nil
}
func (g *gearStrava) set(list ...strava.Gear) {
	g.mu.Lock()
	g.list = list
	g.mu.Unlock()
}
func (g *gearStrava) calls(id string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.gets[id]
}

func setupGear(t *testing.T, scope string) (*env2, *gearStrava) {
	gs := &gearStrava{scope: scope, detail: map[string]strava.Gear{}, gets: map[string]int{}}
	e := setupWith(t, nil, func(d *domain.Deps) {
		gs.fakeStrava = d.Strava.(*fakeStrava)
		d.Strava = gs
	})
	return e, gs
}

func shoeBy(t *testing.T, e *env2, uid int, key, val string) m {
	t.Helper()
	var shoes []m
	if code, body := e.do(uid, "GET", "/api/shoes", nil, &shoes); code != 200 {
		t.Fatalf("shoes: %d %s", code, body)
	}
	for _, s := range shoes {
		if s[key] == val {
			return s
		}
	}
	t.Fatalf("no shoe %s=%s in %v", key, val, shoes)
	return nil
}

func TestShoeGearMileageRetiredAndIsolation(t *testing.T) {
	e, gs := setupGear(t, "")
	ctx := context.Background()
	hoka := strava.Gear{ID: "g101", Name: "Hoka Clifton 9", DistanceM: 1065500, Primary: true, Kind: "shoe"}
	peg := strava.Gear{ID: "g102", Name: "Fractionné", DistanceM: 312000, Kind: "shoe"}
	ride := strava.Gear{ID: "g103", Name: "Saucony Ride 16", DistanceM: 842100, Kind: "shoe"}
	gone := strava.Gear{ID: "g104", Name: "Brooks Ghost 15", DistanceM: 701000, Kind: "shoe"}
	gs.detail["g101"] = strava.Gear{ID: "g101", Name: "Hoka Clifton 9", BrandName: "Hoka", ModelName: "Clifton 9", DistanceM: 1065500, Primary: true}
	gs.detail["g102"] = strava.Gear{ID: "g102", Name: "Fractionné", BrandName: "Nike", ModelName: "Pegasus 41", DistanceM: 312000}
	gs.detail["g103"] = strava.Gear{ID: "g103", Name: "Saucony Ride 16", BrandName: "Saucony", ModelName: "Ride 16", DistanceM: 842100}
	gs.detail["g104"] = strava.Gear{ID: "g104", Name: "Brooks Ghost 15", BrandName: "Brooks", ModelName: "Ghost 15", DistanceM: 701000}
	gs.set(hoka, peg, ride, gone)

	// user 2 holds a pair with the same gear id but no Strava connection: never touched
	if _, err := e.st.DB.Exec(`INSERT INTO shoes (user_id, name, brand, model, strava_gear_id, created_at, updated_at) VALUES (2, 'Clifton à moi', 'Hoka', 'Clifton 9', 'g101', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	connectStrava(t, e, 1)
	// Stride holds only part of the history: 8 km on g101, 6 km on g103, 10 km on a manual pair
	for i, a := range []struct {
		gear string
		m    float64
	}{{"g101", 8000}, {"g103", 6000}, {"", 10000}} {
		var gear any
		if a.gear != "" {
			gear = a.gear
		}
		ts := e.fx.runStart.Unix() - int64(86400*(i+3))
		if _, err := e.st.DB.Exec(`INSERT INTO activities (user_id, strava_id, start_utc, start_local, local_date, type, sport, name, distance_m, strava_gear, created_at, updated_at)
			VALUES (1, ?, ?, '2026-10-01T07:00:00', '2026-10-01', 'running', 'run', 'Sortie', ?, ?, 1, 1)`, 70001+i, ts, a.m, gear); err != nil {
			t.Fatal(err)
		}
	}
	var manual m
	if code, body := e.do(1, "POST", "/api/shoes", m{"name": "Route", "brand": "Kiprun", "model": "KD900"}, &manual); code != 201 {
		t.Fatalf("manual shoe: %d %s", code, body)
	}
	manualAct := e.id(`SELECT id FROM activities WHERE strava_id = 70003`)
	e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", manualAct), m{"shoeId": manual["id"]}, nil)

	if code, body := e.do(1, "POST", "/api/shoes/import-strava", nil, nil); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	h := shoeBy(t, e, 1, "stravaGearId", "g101")
	if h["distanceKm"] != 1065.5 || h["analyzedKm"] != 8.0 || h["distanceSource"] != "strava" || h["stravaPrimary"] != true ||
		h["brand"] != "Hoka" || h["model"] != "Clifton 9" || h["retired"] != false || h["stravaState"] != "active" || h["stravaSyncedAt"] == nil {
		t.Fatalf("strava total must win: %v", h)
	}
	if p := shoeBy(t, e, 1, "stravaGearId", "g102"); p["name"] != "Fractionné" || p["brand"] != "Nike" || p["stravaPrimary"] != false {
		t.Fatalf("nickname + brand/model: %v", p)
	}
	if r := shoeBy(t, e, 1, "name", "Route"); r["distanceKm"] != 10.0 || r["analyzedKm"] != 10.0 || r["distanceSource"] != "stride" || r["stravaState"] != nil {
		t.Fatalf("manual pair falls back to the local sum: %v", r)
	}

	// g103 retired on Strava (left out of /athlete, detail retired), g104 deleted (404)
	gs.detail["g103"] = strava.Gear{ID: "g103", BrandName: "Saucony", ModelName: "Ride 16", DistanceM: 850300, Retired: true}
	delete(gs.detail, "g104")
	gs.set(hoka, peg)
	if code, body := e.do(1, "POST", "/api/shoes/import-strava", nil, nil); code != 200 {
		t.Fatalf("re-import: %d %s", code, body)
	}
	ret := shoeBy(t, e, 1, "stravaGearId", "g103")
	if ret["retired"] != true || ret["stravaState"] != "retired" || ret["distanceKm"] != 850.3 || ret["analyzedKm"] != 6.0 {
		t.Fatalf("retired on Strava: %v", ret)
	}
	del := shoeBy(t, e, 1, "stravaGearId", "g104")
	if del["retired"] != true || del["stravaState"] != "deleted" || del["distanceKm"] != 701.0 || del["name"] != "Brooks Ghost 15" {
		t.Fatalf("deleted on Strava keeps its last total: %v", del)
	}
	// history kept: the activity still points to the retired pair
	var linked int64
	_ = e.st.DB.QueryRow(`SELECT shoe_id FROM activity_notes WHERE activity_id = ?`, e.id(`SELECT id FROM activities WHERE strava_id = 70002`)).Scan(&linked)
	if linked != int64(ret["id"].(float64)) {
		t.Fatalf("retired pair lost its activity: %d", linked)
	}

	// a local choice survives syncs while the Strava state is unchanged
	e.do(1, "PATCH", fmt.Sprintf("/api/shoes/%d", int64(shoeBy(t, e, 1, "stravaGearId", "g102")["id"].(float64))), m{"retired": true}, nil)
	// the periodic sync does not re-check pairs already known as retired or deleted (quota)
	before := gs.calls("g103") + gs.calls("g104")
	if err := e.svc.MorningSync(ctx, 1); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	if after := gs.calls("g103") + gs.calls("g104"); after != before {
		t.Fatalf("periodic sync re-checked retired pairs: %d -> %d", before, after)
	}
	if p := shoeBy(t, e, 1, "stravaGearId", "g102"); p["retired"] != true {
		t.Fatalf("local retire overridden: %v", p)
	}
	// back in the athlete's active gear on Strava: active again
	ride.DistanceM = 851000
	gs.set(hoka, peg, ride)
	e.do(1, "POST", "/api/shoes/import-strava", nil, nil)
	if r := shoeBy(t, e, 1, "stravaGearId", "g103"); r["retired"] != false || r["stravaState"] != "active" || r["distanceKm"] != 851.0 {
		t.Fatalf("un-retired on Strava: %v", r)
	}

	// isolation: user 2 sees only their pair, untouched by user 1's sync
	var other []m
	e.do(2, "GET", "/api/shoes", nil, &other)
	if len(other) != 1 || other[0]["name"] != "Clifton à moi" || other[0]["distanceSource"] != "stride" || other[0]["stravaState"] != nil || other[0]["distanceKm"] != 0.0 {
		t.Fatalf("user 2 shoes: %v", other)
	}
	if code, _ := e.do(2, "POST", "/api/shoes/import-strava", nil, nil); code != 412 {
		t.Fatalf("user 2 import without Strava: %d", code)
	}
	// read-only fields are ignored on PATCH
	if code, body := e.do(1, "PATCH", fmt.Sprintf("/api/shoes/%d", int64(h["id"].(float64))), m{"distanceKm": 1, "distanceSource": "stride", "stravaState": "deleted"}, nil); code != 200 {
		t.Fatalf("patch read-only: %d %s", code, body)
	}
	if h2 := shoeBy(t, e, 1, "stravaGearId", "g101"); h2["distanceKm"] != 1065.5 || h2["stravaState"] != "active" {
		t.Fatalf("read-only fields changed: %v", h2)
	}
}

func TestShoeGearMissingScope(t *testing.T) {
	e, _ := setupGear(t, "read,activity:read_all")
	connectStrava(t, e, 1)
	code, body := e.do(1, "POST", "/api/shoes/import-strava", nil, nil)
	if code != 409 || !strings.Contains(body, "profile:read_all") || !strings.Contains(body, "reconnecte Strava") {
		t.Fatalf("missing scope must ask to reconnect: %d %s", code, body)
	}
	var integ m
	e.do(1, "GET", "/api/integrations", nil, &integ)
	if miss := fmt.Sprint(integ["strava"].(m)["missingScopes"]); !strings.Contains(miss, "profile:read_all") {
		t.Fatalf("integrations missingScopes: %s", miss)
	}
}
