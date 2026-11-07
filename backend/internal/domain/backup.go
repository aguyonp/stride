package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"stride/internal/athlete"
	"stride/internal/workout"
)

type activityRef struct {
	GarminID *int64  `json:"garminId"`
	StravaID *int64  `json:"stravaId"`
	StartUTC *string `json:"startUtc"`
}

type backupGoal struct {
	Title       string   `json:"title"`
	Kind        string   `json:"kind"`
	TargetValue *float64 `json:"targetValue"`
	TargetUnit  *string  `json:"targetUnit"`
	TargetDate  *string  `json:"targetDate"`
	Status      string   `json:"status"`
	Note        string   `json:"note"`
}

type backupPlanned struct {
	Date                  string         `json:"date"`
	Sport                 string         `json:"sport"`
	Title                 string         `json:"title"`
	TargetDurationS       *float64       `json:"targetDurationS"`
	TargetDistanceM       *float64       `json:"targetDistanceM"`
	TargetPaceMinSecPerKm *float64       `json:"targetPaceMinSecPerKm"`
	TargetPaceMaxSecPerKm *float64       `json:"targetPaceMaxSecPerKm"`
	TargetCadenceSpm      *float64       `json:"targetCadenceSpm"`
	Instructions          string         `json:"instructions"`
	Steps                 []workout.Step `json:"steps,omitempty"`
	Source                string         `json:"source,omitempty"`
}

type backupShoe struct {
	Name         string   `json:"name"`
	Brand        string   `json:"brand"`
	Model        string   `json:"model"`
	DropMm       *float64 `json:"dropMm"`
	Type         string   `json:"type"`
	StravaGearID *string  `json:"stravaGearId"`
	Retired      bool     `json:"retired"`
	Note         string   `json:"note"`
}

type backupCheckin struct {
	Date              string   `json:"date"`
	TrainingReadiness *float64 `json:"trainingReadiness"`
	ReadinessStatus   *string  `json:"readinessStatus"`
	Note              string   `json:"note"`
}

type backupNotes struct {
	ActivityRef     activityRef `json:"activityRef"`
	PainDuring      *float64    `json:"painDuring"`
	PainCold        *float64    `json:"painCold"`
	PainNextMorning *float64    `json:"painNextMorning"`
	RPE             *float64    `json:"rpe"`
	Tags            []string    `json:"tags"`
	Note            string      `json:"note"`
	ShoeRef         *string     `json:"shoeRef"`
}

// Backup is the stride-backup v1 document: manual data only, no internal ids, no secrets.
type Backup struct {
	Format        string           `json:"format"`
	Version       int              `json:"version"`
	ExportedAt    string           `json:"exportedAt"`
	Profile       *athlete.Profile `json:"profile"`
	Goals         []backupGoal     `json:"goals"`
	Plan          []backupPlanned  `json:"plan"`
	Shoes         []backupShoe     `json:"shoes"`
	Checkins      []backupCheckin  `json:"checkins"`
	ActivityNotes []backupNotes    `json:"activityNotes"`
}

type ImportCounts struct {
	Goals    int `json:"goals"`
	Plan     int `json:"plan"`
	Shoes    int `json:"shoes"`
	Notes    int `json:"notes"`
	Checkins int `json:"checkins"`
	Profile  int `json:"profile"`
}

func (s *Services) Backup(ctx context.Context, uid int64) (*Backup, error) {
	r := s.readRepo()
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	b := &Backup{Format: "stride-backup", Version: 1, ExportedAt: s.now().UTC().Format(time.RFC3339), Profile: &p,
		Goals: []backupGoal{}, Plan: []backupPlanned{}, Shoes: []backupShoe{}, Checkins: []backupCheckin{}, ActivityNotes: []backupNotes{}}
	goals, err := r.Goals(ctx, uid, false)
	if err != nil {
		return nil, err
	}
	for _, g := range goals {
		b.Goals = append(b.Goals, backupGoal{g.Title, g.Kind, g.TargetValue, g.TargetUnit, g.TargetDate, g.Status, g.Note})
	}
	plan, err := r.Plan(ctx, uid, "", "")
	if err != nil {
		return nil, err
	}
	for _, ps := range plan {
		b.Plan = append(b.Plan, backupPlanned{ps.Date, ps.Sport, ps.Title, ps.TargetDurationS, ps.TargetDistanceM,
			ps.TargetPaceMinSecPerKm, ps.TargetPaceMaxSecPerKm, ps.TargetCadenceSpm, ps.Instructions, ps.Steps, ps.Source})
	}
	shoes, err := r.Shoes(ctx, uid)
	if err != nil {
		return nil, err
	}
	shoeName := map[int64]string{}
	for _, sh := range shoes {
		shoeName[sh.ID] = sh.Name
		b.Shoes = append(b.Shoes, backupShoe{sh.Name, sh.Brand, sh.Model, sh.DropMm, sh.Type, sh.StravaGearID, sh.Retired, sh.Note})
	}
	cks, err := r.Checkins(ctx, uid, "0000-00-00", "9999-99-99")
	if err != nil {
		return nil, err
	}
	for _, d := range sortedCheckinDates(cks) {
		c := cks[d]
		b.Checkins = append(b.Checkins, backupCheckin{c.Date, c.TrainingReadiness, c.ReadinessStatus, c.Note})
	}
	rows, err := s.rdb().QueryContext(ctx, `SELECT a.id, a.garmin_id, a.strava_id, a.start_utc FROM activity_notes n
		JOIN activities a ON a.id = n.activity_id WHERE n.user_id = ? AND a.user_id = ? ORDER BY a.start_utc`, uid, uid)
	if err != nil {
		return nil, err
	}
	type ref struct {
		id  int64
		ref activityRef
	}
	var refs []ref
	for rows.Next() {
		var id, start int64
		var gid, sid sql.NullInt64
		if err := rows.Scan(&id, &gid, &sid, &start); err != nil {
			rows.Close()
			return nil, err
		}
		ar := activityRef{StartUTC: rfc3339(start)}
		if gid.Valid {
			ar.GarminID = &gid.Int64
		}
		if sid.Valid {
			ar.StravaID = &sid.Int64
		}
		refs = append(refs, ref{id, ar})
	}
	rows.Close()
	ids := make([]int64, len(refs))
	for i, x := range refs {
		ids[i] = x.id
	}
	notes, err := r.Notes(ctx, uid, ids)
	if err != nil {
		return nil, err
	}
	for _, x := range refs {
		n := notes[x.id]
		bn := backupNotes{ActivityRef: x.ref, PainDuring: n.PainDuring, PainCold: n.PainCold, PainNextMorning: n.PainNextMorning,
			RPE: n.RPE, Tags: n.Tags, Note: n.Note}
		if n.ShoeID != nil {
			if name, ok := shoeName[*n.ShoeID]; ok {
				bn.ShoeRef = &name
			}
		}
		b.ActivityNotes = append(b.ActivityNotes, bn)
	}
	return b, nil
}

func sortedCheckinDates(m map[string]athlete.DailyCheckin) []string {
	out := make([]string, 0, len(m))
	for d := range m {
		out = append(out, d)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Restore imports a backup. merge upserts by natural keys; replace wipes the manual data first.
func (s *Services) Restore(ctx context.Context, uid int64, body []byte, mode string) (*ImportCounts, error) {
	if mode == "" {
		mode = "merge"
	}
	if mode != "merge" && mode != "replace" {
		return nil, badRequest("mode doit valoir merge ou replace.")
	}
	var b Backup
	dec := json.NewDecoder(strings.NewReader(string(body)))
	if err := dec.Decode(&b); err != nil {
		return nil, badRequest("Sauvegarde illisible (JSON invalide).")
	}
	if b.Format != "stride-backup" || b.Version != 1 {
		return nil, badRequest("Format de sauvegarde non reconnu (stride-backup v1 attendu).")
	}
	counts := &ImportCounts{}
	err := s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		r := athlete.Repo{DB: tx, Now: s.Now}
		if mode == "replace" {
			if err := r.WipeManual(ctx, uid); err != nil {
				return err
			}
		}
		if b.Profile != nil {
			if err := r.PutProfile(ctx, uid, *b.Profile); err != nil {
				return validation(err)
			}
			counts.Profile = 1
		}
		goals, err := r.Goals(ctx, uid, false)
		if err != nil {
			return err
		}
		for _, bg := range b.Goals {
			g := athlete.Goal{Title: bg.Title, Kind: bg.Kind, TargetValue: bg.TargetValue, TargetUnit: bg.TargetUnit, TargetDate: bg.TargetDate, Status: bg.Status, Note: bg.Note}
			found := false
			for _, ex := range goals {
				if strings.EqualFold(ex.Title, strings.TrimSpace(g.Title)) && ex.Kind == g.Kind {
					g.ID, found = ex.ID, true
				}
			}
			if found {
				err = r.UpdateGoal(ctx, uid, &g)
			} else {
				err = r.CreateGoal(ctx, uid, &g)
			}
			if err != nil {
				return validation(err)
			}
			counts.Goals++
		}
		plan, err := r.Plan(ctx, uid, "", "")
		if err != nil {
			return err
		}
		for _, bp := range b.Plan {
			ps := athlete.PlannedSession{Date: bp.Date, Sport: bp.Sport, Title: bp.Title, TargetDurationS: bp.TargetDurationS, TargetDistanceM: bp.TargetDistanceM,
				TargetPaceMinSecPerKm: bp.TargetPaceMinSecPerKm, TargetPaceMaxSecPerKm: bp.TargetPaceMaxSecPerKm, TargetCadenceSpm: bp.TargetCadenceSpm, Instructions: bp.Instructions,
				Steps: bp.Steps, Source: bp.Source}
			found := false
			for _, ex := range plan {
				if ex.Date == ps.Date && ex.Sport == ps.Sport && strings.EqualFold(ex.Title, strings.TrimSpace(ps.Title)) {
					ps.ID, found = ex.ID, true
				}
			}
			if found {
				err = r.UpdatePlanned(ctx, uid, &ps)
			} else {
				err = r.CreatePlanned(ctx, uid, &ps)
			}
			if err != nil {
				return validation(err)
			}
			counts.Plan++
		}
		shoes, err := r.Shoes(ctx, uid)
		if err != nil {
			return err
		}
		for _, bs := range b.Shoes {
			sh := athlete.Shoe{Name: bs.Name, Brand: bs.Brand, Model: bs.Model, DropMm: bs.DropMm, Type: bs.Type, StravaGearID: bs.StravaGearID, Retired: bs.Retired, Note: bs.Note}
			found := false
			for _, ex := range shoes {
				if strings.EqualFold(ex.Name, strings.TrimSpace(sh.Name)) || (ex.StravaGearID != nil && sh.StravaGearID != nil && *ex.StravaGearID == *sh.StravaGearID) {
					sh.ID, found = ex.ID, true
				}
			}
			if found {
				err = r.UpdateShoe(ctx, uid, &sh)
			} else {
				err = r.CreateShoe(ctx, uid, &sh)
			}
			if err != nil {
				return validation(err)
			}
			counts.Shoes++
		}
		for _, bc := range b.Checkins {
			c := athlete.DailyCheckin{Date: bc.Date, TrainingReadiness: bc.TrainingReadiness, ReadinessStatus: bc.ReadinessStatus, Note: bc.Note}
			if !athlete.ValidDate(c.Date) {
				return badRequest("Check-in avec une date invalide.")
			}
			if err := r.PutCheckin(ctx, uid, &c); err != nil {
				return validation(err)
			}
			counts.Checkins++
		}
		shoes, err = r.Shoes(ctx, uid)
		if err != nil {
			return err
		}
		for _, bn := range b.ActivityNotes {
			aid, err := findActivity(ctx, tx, uid, bn.ActivityRef)
			if err != nil {
				return err
			}
			if aid == 0 {
				continue
			}
			n := athlete.ActivityNotes{PainDuring: bn.PainDuring, PainCold: bn.PainCold, PainNextMorning: bn.PainNextMorning, RPE: bn.RPE, Tags: bn.Tags, Note: bn.Note}
			if bn.ShoeRef != nil {
				for _, sh := range shoes {
					if strings.EqualFold(sh.Name, *bn.ShoeRef) {
						id := sh.ID
						n.ShoeID = &id
					}
				}
			}
			if err := r.PutNotes(ctx, uid, aid, &n); err != nil {
				return validation(err)
			}
			counts.Notes++
		}
		return nil
	})
	if err != nil {
		var br *BadRequestError
		if errors.As(err, &br) {
			return nil, err
		}
		return nil, err
	}
	_ = s.PurgeUserCache(ctx, uid)
	s.log.Info("💾 Sauvegarde restaurée", "user", uid, "mode", mode, "objectifs", counts.Goals, "plan", counts.Plan,
		"chaussures", counts.Shoes, "notes", counts.Notes, "checkins", counts.Checkins)
	return counts, nil
}

// findActivity resolves an activity reference (garmin id, strava id, then start ± 120 s).
func findActivity(ctx context.Context, tx *sql.Tx, uid int64, ref activityRef) (int64, error) {
	var id int64
	try := func(q string, arg any) (bool, error) {
		err := tx.QueryRowContext(ctx, `SELECT id FROM activities WHERE user_id = ? AND `+q+` LIMIT 1`, uid, arg).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil, err
	}
	if ref.GarminID != nil {
		if ok, err := try("garmin_id = ?", *ref.GarminID); ok || err != nil {
			return id, err
		}
	}
	if ref.StravaID != nil {
		if ok, err := try("strava_id = ?", *ref.StravaID); ok || err != nil {
			return id, err
		}
	}
	if ref.StartUTC != nil {
		if t, err := time.Parse(time.RFC3339, *ref.StartUTC); err == nil {
			err := tx.QueryRowContext(ctx, `SELECT id FROM activities WHERE user_id = ? AND start_utc BETWEEN ? AND ? ORDER BY ABS(start_utc - ?) LIMIT 1`,
				uid, t.Unix()-matchWindowS, t.Unix()+matchWindowS, t.Unix()).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return 0, nil
			}
			return id, err
		}
	}
	return 0, nil
}
