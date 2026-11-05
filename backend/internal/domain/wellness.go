package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"stride/internal/analysis"
	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/garmin"
)

const wellnessTTL = 10 * time.Minute

var wellnessLabels = map[string]string{"sleep": "Sommeil", "hrv": "VFC", "daily_summary": "Résumés quotidiens", "user_summary": "Résumé du jour"}

var wellnessSources = map[string]string{"sleep": "sleep", "hrv": "hrv", "daily_summary": "dailySummaries", "user_summary": "userSummary"}

type wellEnvelope struct {
	T int64           `json:"t"`
	D json.RawMessage `json:"d"`
}

type wellnessResult struct {
	w        *analysis.Wellness
	warnings []string
	sources  []string
}

// wellness loads per-day Garmin wellness for [from, to] (range kinds) plus the user summary of
// userDays. Final days come from SQLite, recent days from the cache (10 min), the rest is fetched.
func (s *Services) wellness(ctx context.Context, uid int64, p athlete.Profile, from, to string, userDays []string, fr *fresh) *wellnessResult {
	res := &wellnessResult{w: analysis.NewWellness(to)}
	today := s.today(p)
	if to > today {
		to = today
	}
	g, err := s.garminClient(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrNotConnected) {
			res.warnings = append(res.warnings, "Bien-être (VFC, sommeil, Body Battery, pas, étages, stress) : Garmin non connecté")
		} else {
			res.warnings = append(res.warnings, "Bien-être indisponible ("+errText(err)+")")
		}
		return res
	}
	ctx = apimeter.WithUser(ctx, uid)
	payloads := map[string]map[string][]byte{}
	for _, kind := range []string{"sleep", "hrv", "daily_summary"} {
		days := analysis.DateRange(from, to)
		got, err := s.wellnessKind(ctx, uid, g, kind, days, today, fr)
		if err != nil {
			res.warnings = append(res.warnings, fmt.Sprintf("%s indisponible (%s)", wellnessLabels[kind], errText(err)))
		}
		payloads[kind] = got
	}
	var ud []string
	for _, d := range userDays {
		if d <= today {
			ud = append(ud, d)
		}
	}
	if len(ud) > 0 {
		got, err := s.wellnessKind(ctx, uid, g, "user_summary", ud, today, fr)
		if err != nil {
			res.warnings = append(res.warnings, fmt.Sprintf("%s indisponible (%s)", wellnessLabels["user_summary"], errText(err)))
		}
		payloads["user_summary"] = got
	}
	w := res.w
	for kind, days := range payloads {
		used := false
		for d, raw := range days {
			if len(raw) == 0 {
				continue
			}
			var err error
			switch kind {
			case "sleep":
				var v *garmin.DailySleep
				if v, err = garmin.Decode[garmin.DailySleep](raw); err == nil && v.HasData() {
					w.Sleep[d], used = v, true
				}
			case "hrv":
				var v *garmin.HRVSummary
				if v, err = garmin.Decode[garmin.HRVSummary](raw); err == nil {
					w.HRV[d], used = v, true
				}
			case "daily_summary":
				var v *garmin.DailySummary
				if v, err = garmin.Decode[garmin.DailySummary](raw); err == nil {
					w.Daily[d], used = v, true
				}
			case "user_summary":
				var v *garmin.UserSummary
				if v, err = garmin.Decode[garmin.UserSummary](raw); err == nil {
					w.User[d], used = v, true
				}
			}
		}
		if used {
			res.sources = append(res.sources, wellnessSources[kind])
		}
	}
	sort.Strings(res.sources)
	return res
}

func isFinalDay(d, today string, hasData bool) bool {
	return d < analysis.DateAdd(today, -1) && (hasData || d < analysis.DateAdd(today, -7))
}

func (s *Services) wellnessKind(ctx context.Context, uid int64, g *garmin.Client, kind string, days []string, today string, fr *fresh) (map[string][]byte, error) {
	out := map[string][]byte{}
	if len(days) == 0 {
		return out, nil
	}
	rows, err := s.db().QueryContext(ctx, `SELECT date, data, fetched_at FROM wellness_days WHERE user_id = ? AND kind = ? AND date >= ? AND date <= ? AND final = 1`,
		uid, kind, days[0], days[len(days)-1])
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d string
		var data []byte
		var at int64
		if err := rows.Scan(&d, &data, &at); err != nil {
			rows.Close()
			return nil, err
		}
		if b, err := gunzip(data); err == nil {
			out[d] = b
			fr.mark(lvDB, time.Unix(at, 0))
		}
	}
	rows.Close()
	var missing []string
	for _, d := range days {
		if _, ok := out[d]; ok {
			continue
		}
		if raw, ok := s.cacheGet(ctx, userKey(uid, "well", kind, d)); ok {
			var env wellEnvelope
			if json.Unmarshal(raw, &env) == nil {
				if len(env.D) > 0 && string(env.D) != "null" {
					out[d] = env.D
				} else {
					out[d] = nil
				}
				fr.mark(lvCache, time.Unix(env.T, 0))
				continue
			}
		}
		missing = append(missing, d)
	}
	if len(missing) == 0 {
		return out, nil
	}
	fetched, err := s.fetchWellness(ctx, g, kind, missing)
	if err != nil {
		return out, providerErr(pGarmin, err)
	}
	fr.mark(lvLive, s.now())
	for _, d := range missing {
		raw := fetched[d]
		out[d] = raw
		if isFinalDay(d, today, len(raw) > 0) {
			_, err := s.db().ExecContext(ctx, `INSERT INTO wellness_days (user_id, date, kind, data, final, fetched_at) VALUES (?, ?, ?, ?, 1, ?)
				ON CONFLICT (user_id, kind, date) DO UPDATE SET data = excluded.data, final = 1, fetched_at = excluded.fetched_at`,
				uid, d, kind, gz(raw), s.now().Unix())
			if err != nil {
				return out, err
			}
			continue
		}
		env, _ := json.Marshal(wellEnvelope{T: s.now().Unix(), D: raw})
		s.cacheSet(ctx, userKey(uid, "well", kind, d), env, wellnessTTL)
	}
	return out, nil
}

// fetchWellness calls the provider for the missing days (ranges in chunks of 28 days).
func (s *Services) fetchWellness(ctx context.Context, g *garmin.Client, kind string, days []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	if kind == "user_summary" {
		for _, d := range days {
			b, err := g.UserSummaryDaily(ctx, d)
			if err != nil && !errors.Is(err, garmin.ErrNoData) {
				return out, err
			}
			out[d] = b
		}
		return out, nil
	}
	first, last := days[0], days[len(days)-1]
	for start := first; start <= last; start = analysis.DateAdd(start, 28) {
		end := analysis.DateAdd(start, 27)
		if end > last {
			end = last
		}
		var b []byte
		var err error
		switch kind {
		case "sleep":
			b, err = g.SleepRange(ctx, start, end)
		case "hrv":
			b, err = g.HRVRange(ctx, start, end)
		case "daily_summary":
			b, err = g.DailySummaries(ctx, start, end)
		}
		if errors.Is(err, garmin.ErrNoData) {
			continue
		}
		if err != nil {
			return out, err
		}
		items, err := splitByDate(kind, b)
		if err != nil {
			return out, err
		}
		for d, raw := range items {
			out[d] = raw
		}
	}
	return out, nil
}

func splitByDate(kind string, b []byte) (map[string][]byte, error) {
	var items []json.RawMessage
	switch kind {
	case "hrv":
		var env struct {
			S []json.RawMessage `json:"hrvSummaries"`
		}
		if err := json.Unmarshal(b, &env); err != nil {
			return nil, fmt.Errorf("garmin hrv range: %w", err)
		}
		items = env.S
	case "daily_summary":
		var env struct {
			D []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(b, &env); err != nil {
			return nil, fmt.Errorf("garmin daily summaries: %w", err)
		}
		items = env.D
	default:
		if err := json.Unmarshal(b, &items); err != nil {
			return nil, fmt.Errorf("garmin sleep range: %w", err)
		}
	}
	out := map[string][]byte{}
	for _, it := range items {
		var head struct {
			CalendarDate string          `json:"calendarDate"`
			DTO          json.RawMessage `json:"dailySleepDTO"`
		}
		if json.Unmarshal(it, &head) != nil {
			continue
		}
		d := head.CalendarDate
		if d == "" && len(head.DTO) > 0 {
			var inner struct {
				CalendarDate string `json:"calendarDate"`
			}
			_ = json.Unmarshal(head.DTO, &inner)
			d = inner.CalendarDate
		}
		if len(d) >= 10 {
			out[d[:10]] = it
		}
	}
	return out, nil
}
