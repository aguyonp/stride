package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/view"
)

// ManualActivityInput is the body of POST/PATCH /api/activities (§12.4).
type ManualActivityInput struct {
	Date          string   `json:"date"`
	StartTime     *string  `json:"startTime"`
	Sport         string   `json:"sport"`
	Title         *string  `json:"title"`
	DistanceM     *float64 `json:"distanceM"`
	DurationS     *float64 `json:"durationS"`
	AvgHR         *float64 `json:"avgHR"`
	MaxHR         *float64 `json:"maxHR"`
	AvgCadenceSpm *float64 `json:"avgCadenceSpm"`
	ElevGainM     *float64 `json:"elevGainM"`
	ElevLossM     *float64 `json:"elevLossM"`
}

var manualFields = []string{"date", "startTime", "sport", "title", "distanceM", "durationS", "avgHR", "maxHR", "avgCadenceSpm", "elevGainM", "elevLossM"}

var manualTypeKeys = map[string]string{"run": "running", "bike": "cycling", "walk": "walking", "hike": "hiking", "swim": "lap_swimming",
	"strength": "strength_training", "other": "other"}

var errNotManual = &ConflictError{Msg: "Seules les séances saisies manuellement peuvent être modifiées ou supprimées."}

func inRange(p *float64, lo, hi float64, msg string) error {
	if p != nil && !(*p >= lo && *p <= hi) {
		return errors.New(msg)
	}
	return nil
}

// normalize validates the input and returns the activity columns (local time in loc).
func (in *ManualActivityInput) normalize(loc *time.Location, today string) (summaryCols, error) {
	in.Sport = strings.TrimSpace(in.Sport)
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		in.Title = &t
	}
	hm := "12:00"
	if in.StartTime != nil && strings.TrimSpace(*in.StartTime) != "" {
		hm = strings.TrimSpace(*in.StartTime)
	}
	var errs []error
	tk, ok := manualTypeKeys[in.Sport]
	if !ok {
		errs = append(errs, errors.New("sport invalide (run, bike, walk, hike, swim, strength, other)"))
	}
	if !athlete.ValidDate(in.Date) || in.Date > today || in.Date < "2000-01-01" {
		errs = append(errs, errors.New("date attendue au format AAAA-MM-JJ, au plus tard aujourd'hui"))
	}
	if len(hm) == 5 {
		hm += ":00"
	}
	if _, err := time.Parse("15:04:05", hm); err != nil {
		errs = append(errs, errors.New("startTime attendue au format HH:MM"))
	}
	if in.DurationS == nil {
		errs = append(errs, errors.New("durationS requise"))
	}
	if in.Title != nil && len([]rune(*in.Title)) > 120 {
		errs = append(errs, errors.New("titre trop long (120 caractères max)"))
	}
	errs = append(errs,
		inRange(in.DurationS, 60, 2*86400, "durationS hors limites [60 s, 48 h]"),
		inRange(in.DistanceM, 1, 1000000, "distanceM hors limites [1 m, 1000 km]"),
		inRange(in.AvgHR, 30, 250, "avgHR hors limites [30, 250]"),
		inRange(in.MaxHR, 30, 250, "maxHR hors limites [30, 250]"),
		inRange(in.AvgCadenceSpm, 20, 260, "avgCadenceSpm hors limites [20, 260]"),
		inRange(in.ElevGainM, 0, 20000, "elevGainM hors limites [0, 20000]"),
		inRange(in.ElevLossM, 0, 20000, "elevLossM hors limites [0, 20000]"))
	if in.AvgHR != nil && in.MaxHR != nil && *in.MaxHR < *in.AvgHR {
		errs = append(errs, errors.New("maxHR doit être ≥ avgHR"))
	}
	if err := errors.Join(errs...); err != nil {
		return summaryCols{}, err
	}
	start, err := time.ParseInLocation("2006-01-02T15:04:05", in.Date+"T"+hm, loc)
	if err != nil {
		return summaryCols{}, err
	}
	name := analysis.TypeLabel(tk)
	if in.Title != nil && *in.Title != "" {
		name = *in.Title
	}
	c := summaryCols{startUTC: start.Unix(), startLocal: in.Date + "T" + hm, date: in.Date, typ: tk, sport: analysis.Sport(tk), name: name,
		dist: in.DistanceM, dur: in.DurationS, moving: in.DurationS, elapsed: in.DurationS, hr: in.AvgHR, maxHR: in.MaxHR,
		cadence: in.AvgCadenceSpm, gain: in.ElevGainM, loss: in.ElevLossM}
	if in.DistanceM != nil && *in.DistanceM > 0 {
		c.speed = f64(*in.DistanceM / *in.DurationS)
	}
	return c, nil
}

func (s *Services) summaryOf(ctx context.Context, uid, id int64) (view.ActivitySummary, error) {
	a, err := s.activity(ctx, uid, id)
	if err != nil {
		return view.ActivitySummary{}, err
	}
	sums, err := s.summaries(ctx, uid, []*activityRow{a})
	if err != nil {
		return view.ActivitySummary{}, err
	}
	return sums[0], nil
}

// CreateManualActivity stores an activity entered by the athlete (works without any provider).
func (s *Services) CreateManualActivity(ctx context.Context, uid int64, body []byte) (view.ActivitySummary, error) {
	var in ManualActivityInput
	if err := patch(&in, body, manualFields, nil); err != nil {
		return view.ActivitySummary{}, err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return view.ActivitySummary{}, err
	}
	c, err := in.normalize(p.Location(), s.today(p))
	if err != nil {
		return view.ActivitySummary{}, validation(err)
	}
	now := s.now().Unix()
	res, err := s.db().ExecContext(ctx, `INSERT INTO activities (user_id, start_utc, start_local, local_date, type, sport, name, distance_m, duration_s,
		moving_s, elapsed_s, avg_speed, avg_hr, max_hr, avg_cadence, elev_gain, elev_loss, complete, final, manual, notified_at, bundle_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1, 1, ?, ?, ?, ?)`, uid, c.startUTC, c.startLocal, c.date, c.typ, c.sport, c.name,
		c.dist, c.dur, c.moving, c.elapsed, c.speed, c.hr, c.maxHR, c.cadence, c.gain, c.loss, now, now, now, now)
	if err != nil {
		return view.ActivitySummary{}, err
	}
	id, _ := res.LastInsertId()
	s.log.Info("✍️ Séance manuelle enregistrée", "user", uid)
	return s.summaryOf(ctx, uid, id)
}

func (s *Services) manualActivity(ctx context.Context, uid, id int64) (*activityRow, error) {
	a, err := s.activity(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if !a.Manual {
		return nil, errNotManual
	}
	return a, nil
}

// PatchManualActivity updates a manual activity (409 for provider activities).
func (s *Services) PatchManualActivity(ctx context.Context, uid, id int64, body []byte) (view.ActivitySummary, error) {
	a, err := s.manualActivity(ctx, uid, id)
	if err != nil {
		return view.ActivitySummary{}, err
	}
	hm := a.StartLocal[11:16]
	in := ManualActivityInput{Date: a.LocalDate, StartTime: &hm, Sport: a.Sport, Title: &a.Name, DistanceM: a.DistanceM, DurationS: a.DurationS,
		AvgHR: a.AvgHR, MaxHR: a.MaxHR, AvgCadenceSpm: a.AvgCadence, ElevGainM: a.ElevGain, ElevLossM: a.ElevLoss}
	if _, ok := manualTypeKeys[in.Sport]; !ok {
		in.Sport = "other"
	}
	if err := patch(&in, body, manualFields, []string{"id"}); err != nil {
		return view.ActivitySummary{}, err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return view.ActivitySummary{}, err
	}
	c, err := in.normalize(p.Location(), s.today(p))
	if err != nil {
		return view.ActivitySummary{}, validation(err)
	}
	now := s.now().Unix()
	// bundle_at changes so that metrics and records are recomputed
	_, err = s.db().ExecContext(ctx, `UPDATE activities SET start_utc = ?, start_local = ?, local_date = ?, type = ?, sport = ?, name = ?, distance_m = ?,
		duration_s = ?, moving_s = ?, elapsed_s = ?, avg_speed = ?, avg_hr = ?, max_hr = ?, avg_cadence = ?, elev_gain = ?, elev_loss = ?,
		bundle_at = MAX(?, COALESCE(bundle_at, 0) + 1), updated_at = ? WHERE id = ? AND user_id = ? AND manual = 1`, c.startUTC, c.startLocal, c.date,
		c.typ, c.sport, c.name, c.dist, c.dur, c.moving, c.elapsed, c.speed, c.hr, c.maxHR, c.cadence, c.gain, c.loss, now, now, id, uid)
	if err != nil {
		return view.ActivitySummary{}, err
	}
	return s.summaryOf(ctx, uid, id)
}

// DeleteManualActivity removes a manual activity with its notes (409 for provider activities).
func (s *Services) DeleteManualActivity(ctx context.Context, uid, id int64) error {
	if _, err := s.manualActivity(ctx, uid, id); err != nil {
		return err
	}
	if _, err := s.db().ExecContext(ctx, `DELETE FROM activities WHERE id = ? AND user_id = ? AND manual = 1`, id, uid); err != nil {
		return fmt.Errorf("delete manual activity: %w", err)
	}
	return nil
}
