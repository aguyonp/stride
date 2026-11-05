package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"stride/internal/apimeter"
	"stride/internal/fitdata"
	"stride/internal/garmin"
	"stride/internal/provider/strava"
	"stride/internal/provider/weather"
	"stride/internal/view"
)

const (
	lvDB = iota
	lvCache
	lvLive
)

// fresh accumulates where the inputs of a response came from.
type fresh struct {
	mu    sync.Mutex
	level int
	at    time.Time
}

func (f *fresh) mark(level int, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if level > f.level {
		f.level = level
	}
	if at.After(f.at) {
		f.at = at
	}
}

func (f *fresh) view(now time.Time) view.Freshness {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := []string{view.SourceDB, view.SourceCache, view.SourceLive}[f.level]
	at := f.at
	if f.level == lvLive || at.IsZero() {
		at = now
	}
	return view.Freshness{Source: src, UpdatedAt: at.UTC().Format(time.RFC3339)}
}

// bundle is the decoded set of provider payloads of one activity.
type bundle struct {
	row       *activityRow
	listItem  *garmin.ActivityListItem
	detail    *garmin.ActivityDetail
	details   *garmin.ActivityDetails
	fit       []byte
	stravaAct *strava.Activity
	streams   *strava.Streams
	weather   *weather.Conditions
	imported  []byte // rescue import file (format in row.ImportFormat)
	sources   []string
	warnings  []string
}

var garminParts = []string{"garmin_detail", "garmin_details", "fit"}

const (
	finalAfter     = 2 * time.Hour
	nonFinalMaxAge = 10 * time.Minute
)

// ensureBundle fetches the missing parts of an activity bundle and marks it complete/final.
// Final bundles are never requested again.
func (s *Services) ensureBundle(ctx context.Context, uid int64, a *activityRow, fr *fresh) error {
	if a.Final {
		s.backfillWeather(ctx, uid, a, fr)
		return nil
	}
	_, err, _ := s.sf.Do(fmt.Sprintf("bundle:%d:%d", uid, a.ID), func() (any, error) {
		return nil, s.fetchBundle(ctx, uid, a, fr)
	})
	if err == nil {
		if fresh, ferr := s.activity(ctx, uid, a.ID); ferr == nil {
			*a = *fresh
		}
	}
	return err
}

func (s *Services) fetchBundle(ctx context.Context, uid int64, a *activityRow, fr *fresh) error {
	kinds, err := s.blobKinds(ctx, uid, a.ID)
	if err != nil {
		return err
	}
	now := s.now()
	stale := func(kind string) bool {
		at, ok := kinds[kind]
		return !ok || (now.Sub(time.Unix(at, 0)) > nonFinalMaxAge && now.Sub(time.Unix(a.endUTC(), 0)) < finalAfter+nonFinalMaxAge)
	}
	ctx = apimeter.WithUser(ctx, uid)
	complete := true
	var firstErr error
	var mu sync.Mutex
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		complete = false
		if firstErr == nil {
			firstErr = err
		}
	}
	store := func(kind string, b []byte) {
		if err := s.putBlob(ctx, uid, a.ID, kind, b); err != nil {
			fail(err)
		}
		fr.mark(lvLive, now)
	}

	if a.GarminID != nil {
		g, err := s.garminClient(ctx, uid)
		switch {
		case errors.Is(err, ErrNotConnected):
			for _, k := range garminParts {
				if _, ok := kinds[k]; !ok {
					complete = false
				}
			}
		case err != nil:
			return err
		default:
			id := *a.GarminID
			fetchers := map[string]func(context.Context, int64) ([]byte, error){
				"garmin_detail": g.Activity, "garmin_details": g.ActivityDetails, "fit": g.ActivityFIT,
			}
			var eg errgroup.Group
			for _, k := range garminParts {
				if !stale(k) {
					continue
				}
				k, f := k, fetchers[k]
				eg.Go(func() error {
					b, err := f(ctx, id)
					switch {
					case err == nil:
						store(k, b)
					case garmin.IsNotFound(err) || errors.Is(err, garmin.ErrNoData):
						store(k, nil) // permanent absence (manual activity, no FIT…)
					default:
						fail(providerErr(pGarmin, err))
					}
					return nil
				})
			}
			_ = eg.Wait()
		}
	}

	if a.StravaID != nil {
		tok, err := s.stravaToken(ctx, uid)
		switch {
		case errors.Is(err, ErrNotConnected):
		case err != nil:
			fail(err)
		default:
			id := *a.StravaID
			if stale("strava_detail") {
				if act, err := s.deps.Strava.GetActivity(ctx, tok, id); err == nil {
					raw, _ := json.Marshal(act)
					store("strava_detail", raw)
					if act.KudosCount != nil {
						_, _ = s.db().ExecContext(ctx, `UPDATE activities SET kudos = ? WHERE id = ? AND user_id = ?`, *act.KudosCount, a.ID, uid)
					}
				} else {
					fail(providerErr(pStrava, err))
				}
			}
			if stale("strava_streams") {
				if st, err := s.deps.Strava.GetStreams(ctx, tok, id); err == nil {
					raw, _ := json.Marshal(st)
					store("strava_streams", raw)
				} else {
					fail(providerErr(pStrava, err))
				}
			}
		}
	}

	if s.deps.Weather != nil {
		if _, ok := kinds["weather"]; !ok {
			if err := s.fetchWeather(ctx, uid, a); err != nil {
				s.log.Warn("🌦️ Météo indisponible", "user", uid, "err", errText(providerErr("open-meteo", err)))
			} else {
				fr.mark(lvLive, now)
			}
		}
	}

	final := complete && now.Sub(time.Unix(a.endUTC(), 0)) > finalAfter
	_, err = s.db().ExecContext(ctx, `UPDATE activities SET complete = ?, final = ?, bundle_at = MAX(?, COALESCE(bundle_at, 0) + 1), updated_at = ? WHERE id = ? AND user_id = ?`,
		boolInt(complete), boolInt(final), now.Unix(), now.Unix(), a.ID, uid)
	if err != nil {
		return err
	}
	if !complete && firstErr != nil {
		kinds, _ = s.blobKinds(ctx, uid, a.ID)
		if len(kinds) <= 1 {
			return firstErr
		}
		s.log.Warn("⚠️ Bundle d'activité incomplet", "user", uid, "err", errText(firstErr))
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// startPosition returns the activity start coordinates from the payloads already stored.
func (s *Services) startPosition(ctx context.Context, uid int64, a *activityRow) (float64, float64, bool) {
	try := func(lat, lon *float64) (float64, float64, bool) {
		if lat == nil || lon == nil || (*lat == 0 && *lon == 0) || math.Abs(*lat) > 90 || math.Abs(*lon) > 180 {
			return 0, 0, false
		}
		return *lat, *lon, true
	}
	if b, _, ok, _ := s.blob(ctx, uid, a.ID, "garmin_detail"); ok && len(b) > 0 {
		if d, err := garmin.Decode[garmin.ActivityDetail](b); err == nil {
			if lat, lon, ok := try(d.Summary.StartLatitude, d.Summary.StartLongitude); ok {
				return lat, lon, true
			}
		}
	}
	if b, _, ok, _ := s.blob(ctx, uid, a.ID, "garmin_list"); ok && len(b) > 0 {
		if it, err := garmin.Decode[garmin.ActivityListItem](b); err == nil {
			if lat, lon, ok := try(it.StartLatitude, it.StartLongitude); ok {
				return lat, lon, true
			}
		}
	}
	if b, _, ok, _ := s.blob(ctx, uid, a.ID, "strava_detail"); ok && len(b) > 0 {
		var act strava.Activity
		if json.Unmarshal(b, &act) == nil && len(act.StartLatLng) == 2 {
			if lat, lon, ok := try(&act.StartLatLng[0], &act.StartLatLng[1]); ok {
				return lat, lon, true
			}
		}
	}
	if b, _, ok, _ := s.blob(ctx, uid, a.ID, "fit"); ok && len(b) > 0 {
		if fa, err := fitdata.Decode(b); err == nil {
			if lat, lon, ok := try(&fa.Session.StartLat, &fa.Session.StartLon); ok && !math.IsNaN(lat) {
				return lat, lon, true
			}
		}
	}
	if a.ImportFormat != nil {
		if b, err := s.loadBundle(ctx, uid, a); err == nil {
			if act := b.records(a.isRunning()); act != nil {
				for _, r := range act.Records {
					if lat, lon, ok := try(&r.Lat, &r.Lon); ok && !math.IsNaN(lat) && !math.IsNaN(lon) {
						return lat, lon, true
					}
				}
			}
		}
	}
	return 0, 0, false
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// backfillWeather fetches the weather missing from a final bundle (weather disabled or Open-Meteo
// unavailable when the bundle was completed). Indoor / no-GPS activities keep their empty marker.
func (s *Services) backfillWeather(ctx context.Context, uid int64, a *activityRow, fr *fresh) {
	if s.deps.Weather == nil {
		return
	}
	if _, _, ok, err := s.blob(ctx, uid, a.ID, "weather"); ok || err != nil {
		return
	}
	key := userKey(uid, "wx-retry", fmt.Sprint(a.ID))
	if _, wait := s.cacheGet(ctx, key); wait {
		return
	}
	wctx, cancel := context.WithTimeout(apimeter.WithUser(ctx, uid), weatherBackfillBudget)
	defer cancel()
	if err := s.fetchWeather(wctx, uid, a); err != nil {
		s.cacheSet(ctx, key, []byte("1"), weatherRetryAfter)
		s.log.Warn("🌦️ Météo indisponible", "user", uid, "err", errText(providerErr("open-meteo", err)))
		return
	}
	fr.mark(lvLive, s.now())
}

const (
	weatherBackfillBudget = 10 * time.Second
	weatherRetryAfter     = 30 * time.Minute

	reasonWeatherOff     = "Météo désactivée sur ce serveur"
	reasonWeatherIndoor  = "Séance en intérieur ou sans position GPS : pas de météo"
	reasonWeatherPending = "Météo momentanément indisponible (Open-Meteo), nouvelle tentative plus tard"
)

// weatherReason explains a null weather of a loaded bundle.
func (s *Services) weatherReason(ctx context.Context, uid int64, b *bundle) *string {
	r := reasonWeatherPending
	switch {
	case b.weather != nil:
		return nil
	case s.deps.Weather == nil:
		r = reasonWeatherOff
	default:
		if _, _, ok, _ := s.blob(ctx, uid, b.row.ID, "weather"); ok {
			r = reasonWeatherIndoor
		}
	}
	return &r
}

func (s *Services) fetchWeather(ctx context.Context, uid int64, a *activityRow) error {
	lat, lon, ok := s.startPosition(ctx, uid, a)
	if !ok {
		return s.putBlob(ctx, uid, a.ID, "weather", nil) // indoor / no GPS: nothing to look up
	}
	c, err := s.deps.Weather.At(ctx, round2(lat), round2(lon), time.Unix(a.StartUTC, 0).UTC())
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(c)
	return s.putBlob(ctx, uid, a.ID, "weather", raw)
}

// loadBundle decodes the stored payloads of an activity.
func (s *Services) loadBundle(ctx context.Context, uid int64, a *activityRow) (*bundle, error) {
	b := &bundle{row: a}
	get := func(kind string) []byte {
		data, _, ok, err := s.blob(ctx, uid, a.ID, kind)
		if err != nil || !ok || len(data) == 0 {
			return nil
		}
		return data
	}
	if raw := get("garmin_list"); raw != nil {
		b.listItem, _ = garmin.Decode[garmin.ActivityListItem](raw)
	}
	if raw := get("garmin_detail"); raw != nil {
		if d, err := garmin.Decode[garmin.ActivityDetail](raw); err == nil {
			b.detail = d
			b.sources = append(b.sources, "activity")
		}
	}
	if raw := get("garmin_details"); raw != nil {
		if d, err := garmin.Decode[garmin.ActivityDetails](raw); err == nil {
			b.details = d
		}
	}
	b.fit = get("fit")
	if f := a.ImportFormat; f != nil {
		b.imported = get("import_" + *f)
	}
	if raw := get("strava_detail"); raw != nil {
		var act strava.Activity
		if json.Unmarshal(raw, &act) == nil {
			b.stravaAct = &act
			b.sources = append(b.sources, "stravaActivity")
		}
	}
	if raw := get("strava_streams"); raw != nil {
		var st strava.Streams
		if json.Unmarshal(raw, &st) == nil {
			b.streams = &st
		}
	}
	if raw := get("weather"); raw != nil {
		var c weather.Conditions
		if json.Unmarshal(raw, &c) == nil {
			b.weather = &c
			b.sources = append(b.sources, "weather")
		}
	}
	return b, nil
}

// records decodes the time series: FIT (+ details GAP/stamina), details, then Strava streams.
func (b *bundle) records(isRunning bool) *fitdata.Activity {
	start := time.Unix(b.row.StartUTC, 0).UTC()
	if len(b.fit) > 0 {
		if a, err := fitdata.Decode(b.fit); err == nil && len(a.Records) > 0 {
			b.sources = append(b.sources, "fit")
			if b.details != nil {
				fitdata.MergeDetails(a, b.details)
				b.sources = append(b.sources, "details")
			}
			return a
		}
		b.warnings = append(b.warnings, "Fichier FIT illisible, repli sur les séries détaillées")
	}
	format := ""
	if b.row.ImportFormat != nil && len(b.imported) > 0 {
		format = *b.row.ImportFormat
	}
	if format == "fit" {
		if a, err := fitdata.Decode(b.imported); err == nil && len(a.Records) > 0 {
			b.sources = append(b.sources, "import")
			return a
		}
	}
	if b.details != nil && len(b.details.Metrics) > 0 {
		b.sources = append(b.sources, "details")
		return fitdata.FromDetails(b.details, start)
	}
	if format == "gpx" || format == "tcx" {
		if tr, err := fitdata.DecodeXML(b.imported); err == nil {
			b.sources = append(b.sources, "import")
			return tr.Act
		}
	}
	if b.streams != nil && len(b.streams.TimeS) > 0 {
		b.sources = append(b.sources, "stravaStreams")
		return fitdata.FromStrava(b.stravaAct, b.streams, isRunning)
	}
	return nil
}
