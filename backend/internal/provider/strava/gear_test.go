package strava

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestGearMappingScopeAndDeleted(t *testing.T) {
	f, srv := newFake(t)
	c := newTestClient(srv, nil)
	ctx := context.Background()

	// detailed athlete: active shoes only (Strava leaves retired gear out), nickname as fallback name
	f.on("GET /api/v3/athlete", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer acc" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"id":7,"resource_state":3,"shoes":[
			{"id":"g101","primary":true,"name":"Hoka Clifton 9","resource_state":2,"retired":false,"distance":1065500.4},
			{"id":"g102","primary":false,"name":"","nickname":"Fractionné","resource_state":2,"retired":false,"distance":312000}]}`)
	})
	gear, err := c.ListAthleteGear(ctx, "acc")
	if err != nil || len(gear) != 2 {
		t.Fatalf("gear: %+v %v", gear, err)
	}
	if g := gear[0]; g.ID != "g101" || g.Kind != "shoe" || !g.Primary || g.DistanceM != 1065500.4 || g.Name != "Hoka Clifton 9" {
		t.Fatalf("shoe 0: %+v", g)
	}
	if gear[1].Name != "Fractionné" || gear[1].Primary {
		t.Fatalf("nickname fallback: %+v", gear[1])
	}

	// detailed gear: brand and model, retired flag
	f.on("GET /api/v3/gear/g103", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"g103","primary":false,"name":"Vieille paire","brand_name":"Saucony","model_name":"Ride 16","distance":842100,"retired":true,"resource_state":3}`)
	})
	g, err := c.GetGear(ctx, "acc", "g103")
	if err != nil || g.Kind != "shoe" || !g.Retired || g.BrandName != "Saucony" || g.ModelName != "Ride 16" || g.DistanceM != 842100 {
		t.Fatalf("detail: %+v %v", g, err)
	}

	// gear deleted on Strava: 404 Record Not Found
	f.on("GET /api/v3/gear/g104", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Record Not Found","errors":[{"resource":"Gear","field":"id","code":"invalid"}]}`)
	})
	if _, err := c.GetGear(ctx, "acc", "g104"); !IsNotFound(err) {
		t.Fatalf("deleted gear: %v", err)
	}

	// token without profile:read_all: summary athlete, no gear list
	f.on("GET /api/v3/athlete", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":7,"resource_state":2,"firstname":"Alex"}`)
	})
	if _, err := c.ListAthleteGear(ctx, "acc"); !MissingScope(err) || TokenRejected(err) {
		t.Fatalf("summary athlete must report the missing scope: %v", err)
	}
}
