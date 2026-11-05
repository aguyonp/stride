package domain

import (
	"context"
	"encoding/json"
	"math"

	"stride/internal/analysis"
	"stride/internal/athlete"
)

// recordData is the per-activity best efforts persisted in activity_records (seconds by record key).
type recordData struct {
	Computed map[string]float64 `json:"computed,omitempty"`
	Strava   map[string]float64 `json:"strava,omitempty"`
}

var stravaEffortKeys = map[string]string{"1k": "1k", "5k": "5k", "10k": "10k", "Half-Marathon": "half", "Marathon": "marathon"}

func recordDistances() map[string]float64 {
	out := map[string]float64{}
	for _, d := range athlete.RecordDefs {
		if d.DistanceM > 0 {
			out[d.Key] = d.DistanceM
		}
	}
	return out
}

type storedRecord struct {
	version, bundleAt int64
	data              recordData
}

func (s *Services) storedRecords(ctx context.Context, uid int64) (map[int64]storedRecord, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT activity_id, version, bundle_at, data FROM activity_records WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]storedRecord{}
	for rows.Next() {
		var id int64
		var sr storedRecord
		var raw string
		if err := rows.Scan(&id, &sr.version, &sr.bundleAt, &raw); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &sr.data) == nil {
			out[id] = sr
		}
	}
	return out, rows.Err()
}

// activityBests computes the best efforts of a running activity from its stored bundle: rolling
// windows over the series (FIT, details, Strava streams, import), else the whole activity when
// its distance is just above a record distance, plus Strava best_efforts.
func (s *Services) activityBests(ctx context.Context, uid int64, a *activityRow) (recordData, error) {
	rd := recordData{Computed: map[string]float64{}, Strava: map[string]float64{}}
	b, err := s.loadBundle(ctx, uid, a)
	if err != nil {
		return rd, err
	}
	dists := recordDistances()
	if act := b.records(true); act != nil && len(act.Records) > 0 {
		rd.Computed = analysis.BestEfforts(act, dists)
	} else if a.DistanceM != nil && a.DurationS != nil {
		for k, d := range dists {
			if t, ok := analysis.WholeEffort(*a.DistanceM, *a.DurationS, d); ok {
				rd.Computed[k] = t
			}
		}
	}
	if b.stravaAct != nil {
		for _, e := range b.stravaAct.BestEfforts {
			if k, ok := stravaEffortKeys[e.Name]; ok && e.ElapsedS > 0 {
				rd.Strava[k] = e.ElapsedS
			}
		}
	}
	raw, _ := json.Marshal(rd)
	_, err = s.db().ExecContext(ctx, `INSERT INTO activity_records (activity_id, user_id, version, bundle_at, data) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (activity_id) DO UPDATE SET version = excluded.version, bundle_at = excluded.bundle_at, data = excluded.data`,
		a.ID, uid, analysis.RecordsVersion, a.BundleAt, string(raw))
	return rd, err
}

func paceOf(timeS, distM float64) *float64 {
	if timeS <= 0 || distM <= 0 {
		return nil
	}
	return analysis.Num(timeS/distM*1000, 0)
}

// records returns the personal records (manual wins, else the fastest of computed / Strava),
// recomputing only the activities whose bundle changed since the last computation.
func (s *Services) records(ctx context.Context, uid int64) ([]athlete.PersonalRecord, error) {
	rows, err := s.queryActivities(ctx, `WHERE user_id = ? ORDER BY start_utc`, uid)
	if err != nil {
		return nil, err
	}
	stored, err := s.storedRecords(ctx, uid)
	if err != nil {
		return nil, err
	}
	manual, err := s.repo().ManualRecords(ctx, uid)
	if err != nil {
		return nil, err
	}
	best := map[string]*athlete.PersonalRecord{}
	consider := func(key, source string, t float64, a *activityRow) {
		if cur := best[key]; cur == nil || t < *cur.TimeS {
			id, date := a.ID, a.LocalDate
			best[key] = &athlete.PersonalRecord{TimeS: &t, Date: &date, ActivityID: &id, Source: source}
		}
	}
	var longest *activityRow
	for _, a := range rows {
		if !a.isRunning() {
			continue
		}
		if a.DistanceM != nil && (longest == nil || *a.DistanceM > *longest.DistanceM) {
			longest = a
		}
		rd := stored[a.ID].data
		if sr, ok := stored[a.ID]; !ok || sr.version != analysis.RecordsVersion || sr.bundleAt != a.BundleAt {
			if rd, err = s.activityBests(ctx, uid, a); err != nil {
				return nil, err
			}
		}
		for k, t := range rd.Computed {
			consider(k, "computed", t, a)
		}
		for k, t := range rd.Strava {
			consider(k, "strava", t, a)
		}
	}
	out := make([]athlete.PersonalRecord, 0, len(athlete.RecordDefs))
	for _, d := range athlete.RecordDefs {
		r := athlete.PersonalRecord{Key: d.Key, Label: d.Label, DistanceM: d.DistanceM, Source: "computed"}
		if m, ok := manual[d.Key]; ok {
			r.DistanceM, r.TimeS, r.Date, r.Source = m.DistanceM, m.TimeS, m.Date, "manual"
		} else if d.Key == athlete.RecordLongest {
			if longest != nil {
				id, date := longest.ID, longest.LocalDate
				r.DistanceM, r.TimeS, r.Date, r.ActivityID = math.Round(*longest.DistanceM), analysis.PR(longest.DurationS, 0), &date, &id
			}
		} else if b := best[d.Key]; b != nil {
			r.TimeS, r.Date, r.ActivityID, r.Source = b.TimeS, b.Date, b.ActivityID, b.Source
		}
		if r.TimeS != nil {
			r.PaceSecPerKm = paceOf(*r.TimeS, r.DistanceM)
		}
		out = append(out, r)
	}
	return out, nil
}

// Records serves GET /api/records.
func (s *Services) Records(ctx context.Context, uid int64) (map[string][]athlete.PersonalRecord, error) {
	r, err := s.records(ctx, uid)
	if err != nil {
		return nil, err
	}
	return map[string][]athlete.PersonalRecord{"records": r}, nil
}

type RecordInput struct {
	TimeS     *float64 `json:"timeS"`
	DistanceM *float64 `json:"distanceM"`
	Date      *string  `json:"date"`
}

// PutRecord stores a manual record (PUT /api/records/{key}).
func (s *Services) PutRecord(ctx context.Context, uid int64, key string, in RecordInput) (athlete.PersonalRecord, error) {
	def, ok := athlete.RecordDefOf(key)
	if !ok {
		return athlete.PersonalRecord{}, ErrNotFound
	}
	m, err := athlete.NormalizeManualRecord(def, in.TimeS, in.DistanceM, in.Date)
	if err != nil {
		return athlete.PersonalRecord{}, validation(err)
	}
	if err := s.repo().PutManualRecord(ctx, uid, key, m); err != nil {
		return athlete.PersonalRecord{}, err
	}
	out := athlete.PersonalRecord{Key: key, Label: def.Label, DistanceM: m.DistanceM, TimeS: m.TimeS, Date: m.Date, Source: "manual"}
	if m.TimeS != nil {
		out.PaceSecPerKm = paceOf(*m.TimeS, m.DistanceM)
	}
	return out, nil
}

// DeleteRecord removes the manual value of a record (the computed one applies again).
func (s *Services) DeleteRecord(ctx context.Context, uid int64, key string) error {
	if _, ok := athlete.RecordDefOf(key); !ok {
		return ErrNotFound
	}
	return s.repo().DeleteManualRecord(ctx, uid, key)
}
