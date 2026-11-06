package domain

import (
	"bytes"
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/fitdata"
	"stride/internal/queue"
	"stride/internal/view"
)

const (
	MaxImportBytes = 25 << 20
	maxTrackPoints = 1500
)

func importFormat(name string, data []byte) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".fit":
		return "fit"
	case ".gpx":
		return "gpx"
	case ".tcx":
		return "tcx"
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	switch {
	case len(data) > 12 && string(data[8:12]) == ".FIT":
		return "fit"
	case bytes.Contains(head, []byte("<gpx")):
		return "gpx"
	case bytes.Contains(head, []byte("<TrainingCenterDatabase")):
		return "tcx"
	}
	return ""
}

var fitTypeKeys = map[string]string{"running": "running", "cycling": "cycling", "walking": "walking", "hiking": "hiking",
	"swimming": "lap_swimming", "training": "strength_training"}

func typeKeyOf(sport, sub string) string {
	tk := fitTypeKeys[strings.ToLower(sport)]
	switch {
	case tk == "running" && strings.Contains(sub, "trail"):
		return "trail_running"
	case tk == "running" && strings.Contains(sub, "treadmill"):
		return "treadmill_running"
	case tk == "":
		s := strings.ToLower(sport)
		switch {
		case strings.Contains(s, "run"):
			return "running"
		case strings.Contains(s, "bik") || strings.Contains(s, "cycl") || strings.Contains(s, "ride"):
			return "cycling"
		case strings.Contains(s, "walk"):
			return "walking"
		case strings.Contains(s, "hik"):
			return "hiking"
		}
		return "other"
	}
	return tk
}

// importSummary derives the activity columns from the decoded series.
func importSummary(act *fitdata.Activity, typeKey, name string, loc *time.Location) summaryCols {
	start := act.Start.UTC()
	local := start.In(loc).Format("2006-01-02T15:04:05")
	c := summaryCols{startUTC: start.Unix(), startLocal: local, date: local[:10], typ: typeKey, sport: analysis.Sport(typeKey), name: name}
	var moving, dist, hrW, hrS, pwW, pwS, cadW, cadS, gain, loss float64
	maxHR := math.NaN()
	lastAlt := math.NaN()
	for i, r := range act.Records {
		if !math.IsNaN(r.AltM) {
			if math.IsNaN(lastAlt) {
				lastAlt = r.AltM
			} else if d := r.AltM - lastAlt; math.Abs(d) >= 2 {
				if d > 0 {
					gain += d
				} else {
					loss -= d
				}
				lastAlt = r.AltM
			}
		}
		if !math.IsNaN(r.HR) && (math.IsNaN(maxHR) || r.HR > maxHR) {
			maxHR = r.HR
		}
		if i == 0 {
			continue
		}
		dt := r.T - act.Records[i-1].T
		if dt <= 0 || dt > 10 || math.IsNaN(r.SpeedMS) || r.SpeedMS < 1 {
			continue
		}
		moving += dt
		if !math.IsNaN(r.HR) {
			hrW, hrS = hrW+r.HR*dt, hrS+dt
		}
		if !math.IsNaN(r.PowerW) && r.PowerW > 0 {
			pwW, pwS = pwW+r.PowerW*dt, pwS+dt
		}
		if !math.IsNaN(r.CadenceSpm) && r.CadenceSpm > 0 {
			cadW, cadS = cadW+r.CadenceSpm*dt, cadS+dt
		}
	}
	last := act.Records[len(act.Records)-1]
	dist = act.Session.DistM
	if math.IsNaN(dist) {
		dist = last.DistM
	}
	elapsed := act.Session.ElapsedS
	if math.IsNaN(elapsed) {
		elapsed = last.T
	}
	opt := func(v float64) *float64 {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return nil
		}
		return &v
	}
	ratio := func(w, s float64) *float64 {
		if s < 60 {
			return nil
		}
		return opt(w / s)
	}
	c.dist, c.elapsed, c.moving = opt(dist), opt(elapsed), opt(moving)
	c.dur = c.moving
	if c.dur == nil {
		c.dur = c.elapsed
	}
	if moving > 0 && !math.IsNaN(dist) {
		c.speed = opt(dist / moving)
	}
	c.hr, c.maxHR, c.power, c.cadence = ratio(hrW, hrS), opt(maxHR), ratio(pwW, pwS), ratio(cadW, cadS)
	if c.hr == nil {
		c.hr = opt(act.Session.AvgHR)
	}
	c.gain, c.loss = opt(firstNum(act.Session.AscentM, gain)), opt(firstNum(act.Session.DescentM, loss))
	return c
}

func firstNum(vs ...float64) float64 {
	for _, v := range vs {
		if !math.IsNaN(v) {
			return v
		}
	}
	return math.NaN()
}

// ImportFile stores a rescue import (§11.7) through the sync worker pool: merged into a matching
// activity lacking a FIT, otherwise inserted as an import-only activity. It returns the
// *view.ImportResult, or the *queue.JobStatus when async or still running after the wait.
func (s *Services) ImportFile(ctx context.Context, uid int64, filename string, data []byte, async bool) (any, error) {
	if importFormat(filename, data) == "" {
		return nil, badRequest("Format non reconnu : fichiers .fit, .gpx ou .tcx uniquement.")
	}
	return awaitJob[view.ImportResult](ctx, s, queue.EnqueueRequest{
		UserID:  uid,
		Kind:    queue.KindImportActivity,
		Queue:   queue.QueueCritical,
		Label:   "Import de " + filepath.Base(filename),
		Payload: importJobPayload{Filename: filename, Data: data},
	}, async)
}

func (s *Services) importFileExec(ctx context.Context, uid int64, filename string, data []byte) (*view.ImportResult, error) {
	format := importFormat(filename, data)
	if format == "" {
		return nil, badRequest("Format non reconnu : fichiers .fit, .gpx ou .tcx uniquement.")
	}
	var act *fitdata.Activity
	typeKey, name := "", ""
	switch format {
	case "fit":
		a, err := fitdata.Decode(data)
		if err != nil {
			return nil, badRequest("Fichier FIT illisible.")
		}
		act, typeKey = a, typeKeyOf(a.Sport, a.SubSport)
	default:
		tr, err := fitdata.DecodeXML(data)
		if err != nil {
			return nil, badRequest("Fichier %s illisible : %s.", strings.ToUpper(format), err.Error())
		}
		act, typeKey, name = tr.Act, typeKeyOf(tr.Type, ""), tr.Name
	}
	if act == nil || act.Start.IsZero() || len(act.Records) < 10 {
		return nil, badRequest("Le fichier ne contient pas de séance exploitable (horodatage ou points manquants).")
	}
	if act.Start.After(s.now().Add(time.Hour)) || act.Start.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return nil, badRequest("Date de séance invalide dans le fichier.")
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = "Import " + strings.ToUpper(format)
	}
	c := importSummary(act, typeKey, name, p.Location())
	res := &view.ImportResult{}
	err = s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		now := s.now().Unix()
		id, err := s.matchCandidate(ctx, tx, uid, "", c)
		if err != nil {
			return err
		}
		if id != 0 {
			res.ActivityID, res.Merged = id, true
			var has int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM activity_blobs WHERE activity_id = ? AND
				((kind = 'fit' AND length(data) > 64) OR kind LIKE 'import_%')`, id).Scan(&has); err != nil || has > 0 {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE activities SET import_format = ?, distance_m = COALESCE(distance_m, ?), duration_s = COALESCE(duration_s, ?),
				moving_s = COALESCE(moving_s, ?), elapsed_s = COALESCE(elapsed_s, ?), avg_speed = COALESCE(avg_speed, ?), avg_hr = COALESCE(avg_hr, ?),
				max_hr = COALESCE(max_hr, ?), avg_power = COALESCE(avg_power, ?), avg_cadence = COALESCE(avg_cadence, ?), elev_gain = COALESCE(elev_gain, ?),
				elev_loss = COALESCE(elev_loss, ?), bundle_at = MAX(?, COALESCE(bundle_at, 0) + 1), updated_at = ? WHERE id = ? AND user_id = ?`, format, c.dist, c.dur, c.moving, c.elapsed,
				c.speed, c.hr, c.maxHR, c.power, c.cadence, c.gain, c.loss, now, now, id, uid); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM activity_metrics WHERE activity_id = ?`, id); err != nil {
				return err
			}
			return s.putBlobTx(ctx, tx, uid, id, "import_"+format, data)
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO activities (user_id, start_utc, start_local, local_date, type, sport, name, distance_m, duration_s,
			moving_s, elapsed_s, avg_speed, avg_hr, max_hr, avg_power, avg_cadence, elev_gain, elev_loss, import_format, notified_at, bundle_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, c.startUTC, c.startLocal, c.date, c.typ, c.sport, c.name,
			c.dist, c.dur, c.moving, c.elapsed, c.speed, c.hr, c.maxHR, c.power, c.cadence, c.gain, c.loss, format, now, now, now, now)
		if err != nil {
			return err
		}
		res.ActivityID, _ = r.LastInsertId()
		return s.putBlobTx(ctx, tx, uid, res.ActivityID, "import_"+format, data)
	})
	if err != nil {
		return nil, err
	}
	s.deps.Metrics.AddTelemetryPoints(format, len(act.Records))
	s.deps.Metrics.IncActivitySynced("import")
	if res.Merged {
		s.deps.Metrics.IncActivitySynced("fusion")
	}
	s.log.Info("📥 Import de secours enregistré", "user", uid, "format", format, "fusion", res.Merged)
	return res, nil
}

// Track serves GET /api/activities/{id}/track (≤ 1500 points, own activities only).
func (s *Services) Track(ctx context.Context, uid, id int64) (*view.Track, error) {
	a, err := s.activity(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	b, err := s.loadBundle(ctx, uid, a)
	if err != nil {
		return nil, err
	}
	out := &view.Track{Points: [][]*float64{}}
	act := positions(b, a.isRunning())
	if act == nil {
		return out, nil
	}
	var idx []int
	for i, r := range act.Records {
		if !math.IsNaN(r.Lat) && !math.IsNaN(r.Lon) && !(r.Lat == 0 && r.Lon == 0) {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return out, nil
	}
	step := (len(idx) + maxTrackPoints - 1) / maxTrackPoints
	bd := view.TrackBounds{MinLat: 90, MaxLat: -90, MinLon: 180, MaxLon: -180}
	num := func(v float64, n int) *float64 { return analysis.Num(v, n) }
	for k := 0; k < len(idx); k += step {
		i := idx[k]
		r := act.Records[i]
		var cad *float64
		if r.CadenceSpm > 0 {
			cad = num(r.CadenceSpm, 0)
		}
		grade := num(r.GradePct, 1)
		if grade == nil {
			grade = gradeAround(act.Records, i)
		}
		out.Points = append(out.Points, []*float64{num(r.Lat, 6), num(r.Lon, 6), num(math.Max(r.T, 0), 0), cad, grade})
		bd.MinLat, bd.MaxLat = math.Min(bd.MinLat, r.Lat), math.Max(bd.MaxLat, r.Lat)
		bd.MinLon, bd.MaxLon = math.Min(bd.MinLon, r.Lon), math.Max(bd.MaxLon, r.Lon)
	}
	out.Bounds = &bd
	return out, nil
}

// gradeAround estimates the slope (%) over about 50 m around record i.
func gradeAround(recs []fitdata.Record, i int) *float64 {
	lo, hi := i, i
	for lo > 0 && recs[i].DistM-recs[lo].DistM < 25 {
		lo--
	}
	for hi < len(recs)-1 && recs[hi].DistM-recs[i].DistM < 25 {
		hi++
	}
	dd, da := recs[hi].DistM-recs[lo].DistM, recs[hi].AltM-recs[lo].AltM
	if math.IsNaN(dd) || math.IsNaN(da) || dd < 20 {
		return nil
	}
	return analysis.Num(math.Max(-40, math.Min(40, da/dd*100)), 1)
}

func hasPositions(act *fitdata.Activity) bool {
	if act == nil {
		return false
	}
	for _, r := range act.Records {
		if !math.IsNaN(r.Lat) && !math.IsNaN(r.Lon) && !(r.Lat == 0 && r.Lon == 0) {
			return true
		}
	}
	return false
}

// positions returns the first series holding GPS positions: FIT, rescue import, then Strava streams.
func positions(b *bundle, isRunning bool) *fitdata.Activity {
	if len(b.fit) > 0 {
		if act, err := fitdata.Decode(b.fit); err == nil && hasPositions(act) {
			return act
		}
	}
	if b.row.ImportFormat != nil && len(b.imported) > 0 {
		var act *fitdata.Activity
		if *b.row.ImportFormat == "fit" {
			act, _ = fitdata.Decode(b.imported)
		} else if tr, err := fitdata.DecodeXML(b.imported); err == nil {
			act = tr.Act
		}
		if hasPositions(act) {
			return act
		}
	}
	if b.streams != nil && len(b.streams.LatLng) > 0 {
		if act := fitdata.FromStrava(b.stravaAct, b.streams, isRunning); hasPositions(act) {
			return act
		}
	}
	return nil
}
