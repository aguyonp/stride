package analysis

import (
	"fmt"
	"math"
	"sort"

	"stride/internal/athlete"
	"stride/internal/frfmt"
)

// AlertInput gathers what the §5 alerts look at. Unknown numbers are NaN.
type AlertInput struct {
	DayMode         bool
	IsRunning       bool
	Bio             *Biomechanics
	ElevLossM       float64
	Drift           *Drift
	FadePct         float64
	Recovery        *RecoveryContext
	LoadRatio       *LoadRatio
	FloorsPrevDay   float64
	FloorsDay       float64
	Plans           []athlete.PlanMatch
	PainDuring      float64
	PainNextMorning float64
}

var levelOrder = map[string]int{"danger": 0, "warn": 1, "info": 2, "ok": 3}

func sideFR(s string) string {
	switch s {
	case "right":
		return "droite"
	case "both":
		return "des deux côtés"
	}
	return "gauche"
}

func f1(v float64) string { return frfmt.Num(v, 1) }

// BuildAlerts evaluates the profile thresholds; injury-specific alerts follow the focus preset.
func BuildAlerts(in AlertInput, p athlete.Profile) []Alert {
	th := p.Thresholds
	injury := p.Preset().InjuryAlerts
	var out []Alert
	add := func(level, code, title, msg string) {
		out = append(out, Alert{Level: level, Code: code, Title: title, Message: msg})
	}
	twoLevel := func(v, limit float64) string {
		if v > 2*limit {
			return "danger"
		}
		return "warn"
	}

	if in.IsRunning && in.Bio != nil {
		b := in.Bio
		if c := pf(b.CadenceAvgSpm); valid(c) {
			switch {
			case c < th.CadenceMinSpm:
				add("danger", "CADENCE_LOW", "Cadence basse",
					fmt.Sprintf("%s spm < %s spm, foulée trop longue (overstriding) et impact accru à chaque appui.", frfmt.Int(c), f1(th.CadenceMinSpm)))
			case c < th.CadenceTargetMin:
				add("warn", "CADENCE_LOW", "Cadence sous la cible",
					fmt.Sprintf("%s spm < cible %s-%s spm, raccourcir légèrement la foulée.", frfmt.Int(c), f1(th.CadenceTargetMin), f1(th.CadenceTargetMax)))
			}
		}
		if sl := b.Slope; injury && sl != nil && sl.DownhillCadenceDropSpm != nil && *sl.DownhillCadenceDropSpm > th.DownhillCadenceDropSpm {
			d := *sl.DownhillCadenceDropSpm
			add(twoLevel(d, th.DownhillCadenceDropSpm), "DOWNHILL_CADENCE_DROP", "Cadence qui chute en descente",
				fmt.Sprintf("-%s spm en descente vs plat (seuil %s spm), foulée qui s'allonge et freinage accru en descente.", frfmt.Int(d), f1(th.DownhillCadenceDropSpm)))
		}
		if fd := b.FatigueDrift; fd != nil && fd.GctDeltaPct != nil && *fd.GctDeltaPct > th.GctDriftMaxPct {
			d := *fd.GctDeltaPct
			add(twoLevel(d, th.GctDriftMaxPct), "GCT_DRIFT", "Dérive du temps de contact",
				fmt.Sprintf("GCT %s %% entre le début et la fin (seuil +%s %%), fatigue neuromusculaire et perte de raideur.", frfmt.Signed(d, 1), f1(th.GctDriftMaxPct)))
		}
		if vo := pf(b.VerticalOscillationCm); valid(vo) && vo > th.VerticalOscillationMaxCm {
			lvl := "warn"
			if vo > th.VerticalOscillationMaxCm+1 {
				lvl = "danger"
			}
			add(lvl, "VO_HIGH", "Oscillation verticale élevée",
				fmt.Sprintf("%s cm > %s cm, rebond excessif et impacts accrus.", frfmt.Dec(vo, 1), f1(th.VerticalOscillationMaxCm)))
		}
		g := b.GctBalance
		if g.Available && g.GapPct != nil {
			if gap := *g.GapPct; gap > th.GctBalanceGapPct {
				msg := fmt.Sprintf("Écart G/D de %s pt (%s %% G / %s %% D)", frfmt.Dec(gap, 1), frfmt.Dec(pf(g.LeftPct), 1), frfmt.Dec(pf(g.RightPct), 1))
				if g.WeakerSide != nil {
					if p.InjuredSide != nil && (*g.WeakerSide == *p.InjuredSide) {
						msg += fmt.Sprintf(", contact plus court à %s = côté surveillé (évitement probable).", sideFR(*g.WeakerSide))
					} else {
						msg += fmt.Sprintf(", contact plus court à %s.", sideFR(*g.WeakerSide))
					}
				} else {
					msg += "."
				}
				add(twoLevel(gap, th.GctBalanceGapPct), "GCT_BALANCE", "Déséquilibre contact sol", msg)
			}
		} else if !g.Available {
			reason := "Nécessite HRM-Pro / HRM 600 / Running Dynamics Pod."
			if g.Reason != nil && *g.Reason == NoGarminReason {
				reason = NoGarminReason + "."
			}
			add("info", "GCT_BALANCE_NA", "Équilibre G/D non mesuré", reason)
		}
	}

	if injury && valid(in.ElevLossM) && in.ElevLossM > th.DescentMaxM {
		add(twoLevel(in.ElevLossM, th.DescentMaxM), "DESCENT_HIGH", "Dénivelé négatif élevé",
			fmt.Sprintf("-%s m > %s m, les descentes augmentent fortement les contraintes excentriques (genoux, quadriceps).", frfmt.Int(in.ElevLossM), f1(th.DescentMaxM)))
	}
	if injury && valid(in.FloorsPrevDay) && in.FloorsPrevDay > th.FloorsPrevDayMax {
		add("warn", "FLOORS_PREV_DAY", "Escaliers la veille",
			fmt.Sprintf("%s étages montés la veille (> %s), charge cumulée avant la séance.", frfmt.Int(in.FloorsPrevDay), f1(th.FloorsPrevDayMax)))
	}
	if injury && in.DayMode && valid(in.FloorsDay) && in.FloorsDay > th.FloorsPrevDayMax {
		add("warn", "FLOORS_HIGH", "Beaucoup d'escaliers",
			fmt.Sprintf("%s étages montés (> %s), charge à prendre en compte pour la prochaine séance.", frfmt.Int(in.FloorsDay), f1(th.FloorsPrevDayMax)))
	}

	if d := in.Drift; d != nil && d.DecouplingPct >= 5 {
		lvl := "warn"
		if d.DecouplingPct > 10 {
			lvl = "danger"
		}
		add(lvl, "DRIFT_HIGH", "Dérive cardiaque",
			fmt.Sprintf("Découplage %s %s %%, fatigue ou endurance aérobie insuffisante sur cette durée.", DriftMethodLabel(d.Method), frfmt.Signed(d.DecouplingPct, 1)))
	}
	if valid(in.FadePct) {
		add("warn", "FADE_END", "Fin de séance en perte d'allure",
			fmt.Sprintf("Dernier bloc %s %% plus lent que les précédents avec une FC au moins égale (fatigue en fin de séance).", frfmt.Int(in.FadePct)))
	}

	if lr := in.LoadRatio; lr != nil && lr.Ratio != nil && *lr.Ratio > th.LoadRatioMax {
		lvl := "warn"
		if lr.Verdict == "danger" {
			lvl = "danger"
		}
		add(lvl, "LOAD_RATIO", "Ratio de charge élevé",
			fmt.Sprintf("Ratio aiguë/chronique %s (> %s, aiguë %s / chronique %s), progression trop rapide, alléger les prochains jours.",
				frfmt.Dec(*lr.Ratio, 2), frfmt.Num(th.LoadRatioMax, 2), frfmt.Int(lr.Acute), frfmt.Int(lr.Chronic)))
	}

	for _, m := range in.Plans {
		if m.Status != "done" && m.Status != "partial" {
			continue
		}
		dev := math.Max(math.Abs(first(pf(m.DurationDeltaPct), 0)), math.Abs(first(pf(m.DistanceDeltaPct), 0)))
		paceOut := m.PaceDeltaSecPerKm != nil && *m.PaceDeltaSecPerKm != 0
		if (valid(dev) && dev > th.PlanDeviationPct) || paceOut {
			add("warn", "PLAN_DEVIATION", "Écart prévu / réalisé", m.Summary)
		}
	}

	if pd := in.PainDuring; valid(pd) && pd >= 3 {
		lvl := "warn"
		if pd >= 5 {
			lvl = "danger"
		}
		add(lvl, "PAIN_DURING", "Douleur pendant l'effort",
			fmt.Sprintf("Gêne %s/10 pendant la séance (seuil 3/10), réduire volume et intensité.", frfmt.Num(pd, 1)))
	}
	if pn := in.PainNextMorning; valid(pn) && pn > 0 && (!valid(in.PainDuring) || pn > in.PainDuring) {
		lvl := "warn"
		if pn >= 5 {
			lvl = "danger"
		}
		add(lvl, "PAIN_NEXT_MORNING", "Douleur plus forte le lendemain",
			fmt.Sprintf("Gêne au réveil %s/10 supérieure à celle de l'effort, signe de surcharge tissulaire.", frfmt.Num(pn, 1)))
	}

	if rc := in.Recovery; rc != nil {
		if h := rc.HRV; h != nil {
			st := spv(h.Status)
			ln, low := pf(h.LastNightAvgMs), pf(h.BaselineLowMs)
			detail := ""
			if valid(ln) {
				detail = fmt.Sprintf(" (%s ms", frfmt.Int(ln))
				if valid(low) && valid(pf(h.BaselineHighMs)) {
					detail += fmt.Sprintf(" vs baseline %s-%s ms", frfmt.Int(low), frfmt.Int(pf(h.BaselineHighMs)))
				}
				detail += ")"
			}
			switch {
			case st == "LOW" || st == "POOR":
				add("danger", "HRV_LOW", "VFC basse", fmt.Sprintf("Statut %s%s, récupération incomplète, privilégier une séance facile.", HRVStatusLabel(st), detail))
			case st == "UNBALANCED":
				add("warn", "HRV_LOW", "VFC déséquilibrée", fmt.Sprintf("Statut %s%s, rester prudent sur l'intensité.", HRVStatusLabel(st), detail))
			case valid(ln) && valid(low) && ln < low:
				add("warn", "HRV_LOW", "VFC sous la baseline", fmt.Sprintf("VFC nocturne%s sous la borne basse, récupération incomplète.", detail))
			}
		}
		if r := rc.RestingHR; r != nil && r.Delta != nil && *r.Delta >= th.RhrDeltaMaxBpm {
			add("warn", "RHR_HIGH", "FC repos élevée",
				fmt.Sprintf("%s bpm vs moyenne 7 j (%s vs %s bpm), fatigue, stress ou début d'infection possible.", frfmt.Signed(*r.Delta, 0), frfmt.Int(pf(r.Bpm)), frfmt.Int(pf(r.Avg7d))))
		}
		if sl := rc.Sleep; sl != nil {
			if sc := pf(sl.Score); valid(sc) && sc < 60 {
				add("warn", "SLEEP_LOW", "Sommeil insuffisant", fmt.Sprintf("Score %s/100, récupération limitée.", frfmt.Int(sc)))
			}
			if dp := pf(sl.DeepS); valid(dp) && dp < 3600 {
				add("info", "SLEEP_LOW", "Sommeil profond court", fmt.Sprintf("Profond %s (< 1h), récupération tissulaire réduite.", frfmt.SleepHM(dp)))
			}
		}
		if bb := rc.BodyBattery; bb != nil && bb.AtWake != nil {
			switch v := *bb.AtWake; {
			case v < 30:
				add("danger", "BB_LOW", "Body Battery très basse", fmt.Sprintf("%s/100 au réveil, réserves d'énergie faibles.", frfmt.Int(v)))
			case v < 50:
				add("warn", "BB_LOW", "Body Battery basse", fmt.Sprintf("%s/100 au réveil, réserves d'énergie limitées.", frfmt.Int(v)))
			}
		}
	}

	serious := false
	for _, a := range out {
		if a.Level == "warn" || a.Level == "danger" {
			serious = true
			break
		}
	}
	if !serious {
		msg := "Aucun signal d'alerte sur cette séance."
		if in.DayMode {
			msg = "Aucun signal d'alerte sur cette journée."
		}
		add("ok", "ALL_GOOD", "RAS", msg)
	}
	sort.SliceStable(out, func(i, j int) bool { return levelOrder[out[i].Level] < levelOrder[out[j].Level] })
	return out
}

func DriftMethodLabel(m string) string {
	if m == "pw:hr" {
		return "Pw:Hr"
	}
	return "Pace:Hr"
}
