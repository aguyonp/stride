package domain

import (
	"context"
	"database/sql"
	"strings"

	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/provider/strava"
)

// Strava gear sync (SPEC §14.5): a linked pair's total is Strava's all-time gear distance; a pair
// Strava no longer lists is checked with GET /gear/{id} and shown retired (retired or deleted there).

const (
	gearActive  = "active"
	gearRetired = "retired"
	gearDeleted = "deleted"
)

// errGearScope: the Strava token lacks profile:read_all, so Strava does not share the gear list.
var errGearScope = &ConflictError{Msg: "Strava ne partage pas ton équipement : reconnecte Strava (Réglages › Intégrations) en autorisant l’accès complet au profil (profile:read_all)."}

// ImportStravaShoes is the on-demand refresh: it also re-checks pairs already retired on Strava.
func (s *Services) ImportStravaShoes(ctx context.Context, uid int64) ([]athlete.Shoe, error) {
	return s.refreshStravaShoes(ctx, uid, true)
}

// refreshStravaShoes creates or updates local shoes from the athlete's Strava gear. Outside a full
// refresh, pairs already known as retired or deleted on Strava are not re-checked (quota).
func (s *Services) refreshStravaShoes(ctx context.Context, uid int64, full bool) ([]athlete.Shoe, error) {
	if cn, err := s.getConn(ctx, uid, pStrava); err == nil && cn.connected() && cn.Scope.Valid && !hasScope(cn.Scope.String, "profile:read_all") {
		return nil, errGearScope
	}
	tok, err := s.stravaToken(ctx, uid)
	if err != nil {
		return nil, err
	}
	ctx = apimeter.WithUser(ctx, uid)
	gear, err := s.deps.Strava.ListAthleteGear(ctx, tok)
	if strava.MissingScope(err) {
		return nil, errGearScope
	}
	if err != nil {
		return nil, providerErr(pStrava, err)
	}
	existing, err := s.repo().Shoes(ctx, uid)
	if err != nil {
		return nil, err
	}
	// detail calls GET /gear/{id} (brand, model, retired); gone is true on a 404 (deleted gear).
	// The first other failure (quota, cooldown, network) stops further detail calls this run.
	detailsOK := true
	detail := func(id string) (g *strava.Gear, gone bool) {
		if !detailsOK {
			return nil, false
		}
		g, err := s.deps.Strava.GetGear(ctx, tok, id)
		if strava.IsNotFound(err) {
			return nil, true
		}
		if err != nil {
			detailsOK = false
			s.log.Warn("⚠️ Détail d’équipement Strava indisponible", "user", uid, "err", errText(providerErr(pStrava, err)))
			return nil, false
		}
		return g, false
	}
	listed := map[string]bool{}
	for _, g := range gear {
		if g.Kind != "" && g.Kind != "shoe" {
			continue
		}
		listed[g.ID] = true
		cur := linkedShoe(existing, g.ID)
		if cur == nil || (cur.Brand == "" && cur.Model == "") {
			if d, _ := detail(g.ID); d != nil {
				g = mergeGear(g, *d)
			}
		}
		state := gearActive
		if g.Retired {
			state = gearRetired
		}
		if existing, err = s.applyGear(ctx, uid, existing, cur, g, state); err != nil {
			return nil, err
		}
	}
	// Linked pairs Strava no longer lists: retired there (Strava omits retired gear) or deleted.
	for _, sh := range existing {
		if sh.StravaGearID == nil || listed[*sh.StravaGearID] {
			continue
		}
		if !full && sh.StravaState != nil && *sh.StravaState != gearActive {
			continue
		}
		g := strava.Gear{ID: *sh.StravaGearID, Kind: "shoe", DistanceM: -1}
		d, gone := detail(g.ID)
		state := gearDeleted
		switch {
		case d != nil:
			g, state = mergeGear(g, *d), gearRetired
			g.DistanceM = d.DistanceM
			if !d.Retired {
				state = gearActive
			}
		case !gone:
			continue // unknown: keep the last known state
		}
		cur := linkedShoe(existing, g.ID)
		if existing, err = s.applyGear(ctx, uid, existing, cur, g, state); err != nil {
			return nil, err
		}
	}
	if err := s.autoAssignShoes(ctx, uid); err != nil {
		return nil, err
	}
	return s.repo().Shoes(ctx, uid)
}

func linkedShoe(list []athlete.Shoe, gearID string) *athlete.Shoe {
	for i := range list {
		if list[i].StravaGearID != nil && *list[i].StravaGearID == gearID {
			return &list[i]
		}
	}
	return nil
}

// mergeGear completes a summary gear (from /athlete) with its detailed representation.
func mergeGear(g, d strava.Gear) strava.Gear {
	if d.Name != "" {
		g.Name = d.Name
	}
	if d.BrandName != "" {
		g.BrandName = d.BrandName
	}
	if d.ModelName != "" {
		g.ModelName = d.ModelName
	}
	if d.Description != "" {
		g.Description = d.Description
	}
	g.Retired = g.Retired || d.Retired
	return g
}

// gearLabel is the display name of a Strava pair: its name, else "brand model" (never its id).
func gearLabel(g strava.Gear) string {
	if n := strings.TrimSpace(g.Name); n != "" {
		return clip(n, 80)
	}
	if bm := strings.TrimSpace(g.BrandName + " " + g.ModelName); bm != "" {
		return clip(bm, 80)
	}
	return "Paire Strava"
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}

// applyGear links or creates the local pair of g and stores its Strava state. The Strava retired flag
// is applied when the Strava state changes (or on first link), so a local choice survives later syncs.
// g.DistanceM < 0 keeps the last known Strava distance (deleted gear).
func (s *Services) applyGear(ctx context.Context, uid int64, existing []athlete.Shoe, cur *athlete.Shoe, g strava.Gear, state string) ([]athlete.Shoe, error) {
	r := s.repo()
	gid := g.ID
	name := gearLabel(g)
	brand, model := clip(strings.TrimSpace(g.BrandName), 60), clip(strings.TrimSpace(g.ModelName), 80)
	if cur == nil && state != gearDeleted {
		for i := range existing {
			if existing[i].StravaGearID == nil && sameShoe(existing[i], name, brand, model) {
				cur = &existing[i]
				cur.StravaGearID = &gid
				break
			}
		}
	}
	switch {
	case cur == nil && state == gearDeleted:
		return existing, nil
	case cur == nil:
		sh := athlete.Shoe{Name: name, Brand: brand, Model: model, Type: "soft", StravaGearID: &gid, Retired: state != gearActive, Note: clip(strings.TrimSpace(g.Description), 1000)}
		if err := r.CreateShoe(ctx, uid, &sh); err != nil {
			return nil, err
		}
		existing = append(existing, sh)
		cur = &existing[len(existing)-1]
	default:
		if strings.TrimSpace(g.Name) != "" || (cur.Name == "" && name != "") {
			cur.Name = name
		}
		if brand != "" {
			cur.Brand = brand
		}
		if model != "" {
			cur.Model = model
		}
		if cur.StravaState == nil || *cur.StravaState != state {
			cur.Retired = state != gearActive
		}
		if err := r.UpdateShoe(ctx, uid, cur); err != nil {
			return nil, err
		}
	}
	dist := sql.NullFloat64{Float64: g.DistanceM, Valid: g.DistanceM >= 0}
	st := state
	cur.StravaState = &st
	_, err := s.db().ExecContext(ctx, `UPDATE shoes SET strava_distance_m = COALESCE(?, strava_distance_m), strava_primary = ?, strava_state = ?,
		strava_synced_at = ? WHERE id = ? AND user_id = ?`, dist, boolInt(g.Primary && state == gearActive), state, s.now().Unix(), cur.ID, uid)
	return existing, err
}
