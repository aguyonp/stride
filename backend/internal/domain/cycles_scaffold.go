package domain

import (
	"context"
	"database/sql"
	"math"
)

// defaultPeakKm is the peak weekly volume of a scaffolded plan when none is given.
const defaultPeakKm = 50.0

type scaffoldWeek struct {
	start, end string
	number     int
	assim      bool
	volumeKm   float64
	load       *float64
	note       string
}

type scaffoldPhase struct {
	name, focus string
	phaseType   PhaseType
	start, end  string
	volumeKm    float64
	load        *float64
	weeks       []scaffoldWeek
}

type phaseSpec struct {
	name      string
	phaseType PhaseType
	focus     string
	share     float64
	volFactor float64
}

var (
	longPlanSpecs = []phaseSpec{
		{"Fondation aérobie", PhaseFoundation, "Endurance fondamentale & régularité", 0.34, 0.78},
		{"Développement & Seuil", PhaseBuild, "Volume progressif & tempo contrôlé", 0.34, 0.92},
		{"Spécifique objectif", PhaseSpecific, "Allure cible & économie de course", 0.22, 1.0},
		{"Affûtage", PhaseTaper, "Fraîcheur neuromusculaire avant course", 0.10, 0.62},
	}
	shortPlanSpecs = []phaseSpec{
		{"Bloc principal", PhaseSpecific, "Travail spécifique objectif", 0.75, 0.95},
		{"Affûtage", PhaseTaper, "Assimilation pré-compétition", 0.25, 0.65},
	}
)

// scaffoldPhases splits [start, end] into contiguous Monday-based phases (4 from 6 weeks on, else 2)
// with weekly targets ramping towards peakKm; every 4th week outside the taper is an assimilation
// week at 74 %. Load targets are set only when peakLoad is given.
func scaffoldPhases(start, end string, peakKm float64, peakLoad *float64) []scaffoldPhase {
	first := weekStart(start)
	total := max(1, diffDays(first, weekStart(end))/7+1)
	specs := longPlanSpecs
	if total < 6 {
		specs = shortPlanSpecs
	}
	var out []scaffoldPhase
	cursor := 0
	for idx, sp := range specs {
		remaining := total - cursor
		if remaining <= 0 {
			break
		}
		count := remaining
		if idx < len(specs)-1 {
			count = max(1, min(remaining-(len(specs)-1-idx), int(math.Round(float64(total)*sp.share))))
		}
		ph := scaffoldPhase{name: sp.name, focus: sp.focus, phaseType: sp.phaseType, start: addDays(first, cursor*7),
			volumeKm: math.Round(peakKm * sp.volFactor)}
		ph.end = addDays(ph.start, count*7-1)
		if peakLoad != nil {
			ph.load = f64(math.Round(*peakLoad * sp.volFactor))
		}
		for w := range count {
			wk := scaffoldWeek{start: addDays(ph.start, w*7), number: cursor + w + 1}
			wk.end = addDays(wk.start, 6)
			wk.assim = sp.phaseType != PhaseTaper && wk.number%4 == 0
			factor := 0.94 + math.Min(0.10, float64(w)*0.04)
			if wk.assim {
				factor, wk.note = 0.74, "Semaine d’assimilation (-25 % volume)"
			}
			wk.volumeKm = math.Round(ph.volumeKm*factor*10) / 10
			if ph.load != nil {
				wk.load = f64(math.Round(*ph.load * factor))
			}
			ph.weeks = append(ph.weeks, wk)
		}
		out = append(out, ph)
		cursor += count
	}
	return out
}

func insertScaffold(ctx context.Context, tx *sql.Tx, uid, planID, now int64, phases []scaffoldPhase) error {
	for idx, ph := range phases {
		res, err := tx.ExecContext(ctx, `INSERT INTO plan_phases (user_id, plan_id, name, phase_type, order_index, start_date, end_date, focus,
			weekly_volume_km, target_weekly_load, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
			uid, planID, ph.name, string(ph.phaseType), idx, ph.start, ph.end, ph.focus, ph.volumeKm, ph.load, now, now)
		if err != nil {
			return err
		}
		phaseID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, w := range ph.weeks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO training_weeks (user_id, plan_id, phase_id, week_start, week_end, week_number,
				is_assimilation, target_volume_km, target_load, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				uid, planID, phaseID, w.start, w.end, w.number, boolInt(w.assim), w.volumeKm, w.load, w.note, now, now); err != nil {
				return err
			}
		}
	}
	return nil
}
