package analysis

import (
	"fmt"
	"sort"

	"patella/internal/config"
	"patella/internal/frfmt"
)

// alertInput regroupe ce qui sert à évaluer les alertes (§5 du contrat).
type alertInput struct {
	isRunning bool
	bio       *Biomechanics
	elevLossM float64
	drift     *Drift
	fadePct   float64
	recovery  *RecoveryContext
	acwr      *ACWR
	floorsUp  float64 // mode jour uniquement
	dayMode   bool
}

var levelOrder = map[string]int{"danger": 0, "warn": 1, "info": 2, "ok": 3}

func sideFR(s string) string {
	if s == "right" {
		return "droite"
	}
	return "gauche"
}

// buildAlerts évalue les seuils du profil.
func buildAlerts(in alertInput, p config.Profile) []Alert {
	var out []Alert
	add := func(level, code, title, msg string) {
		out = append(out, Alert{Level: level, Code: code, Title: title, Message: msg})
	}

	if in.isRunning && in.bio != nil {
		b := in.bio
		if c := pf(b.CadenceAvgSpm); valid(c) {
			switch {
			case c < 170:
				add("danger", "CADENCE_LOW", "Cadence basse",
					fmt.Sprintf("%s spm < 170, risque d'overstriding.", frfmt.Int(c)))
			case c < p.CadenceTargetMin:
				add("warn", "CADENCE_LOW", "Cadence basse",
					fmt.Sprintf("%s spm < cible %s spm, raccourcir légèrement la foulée.", frfmt.Int(c), frfmt.Num(p.CadenceTargetMin, 1)))
			}
		}
		if vo := pf(b.VerticalOscillationCm); valid(vo) && vo > p.VerticalOscillationAlertCm {
			lvl := "warn"
			if vo > p.VerticalOscillationAlertCm+1 {
				lvl = "danger"
			}
			add(lvl, "VO_HIGH", "Oscillation verticale élevée",
				fmt.Sprintf("%s cm > %s cm, rebond excessif et impacts accrus sur la rotule.", frfmt.Dec(vo, 1), frfmt.Num(p.VerticalOscillationAlertCm, 1)))
		}
		g := b.GctBalance
		if g.Available && g.GapPct != nil {
			gap := *g.GapPct
			if gap > p.GctBalanceAlertPct {
				lvl := "warn"
				if gap > 2*p.GctBalanceAlertPct {
					lvl = "danger"
				}
				msg := fmt.Sprintf("Écart G/D de %s pt (%s %% G / %s %% D)", frfmt.Dec(gap, 1), frfmt.Dec(pf(g.LeftPct), 1), frfmt.Dec(pf(g.RightPct), 1))
				if g.WeakerSide != nil {
					if *g.WeakerSide == p.InjuredSide {
						msg += fmt.Sprintf(", contact plus court à %s = côté blessé (évitement probable).", sideFR(*g.WeakerSide))
					} else {
						msg += fmt.Sprintf(", contact plus court à %s (côté sain).", sideFR(*g.WeakerSide))
					}
				} else {
					msg += "."
				}
				add(lvl, "GCT_BALANCE", "Déséquilibre contact sol", msg)
			}
		} else if !g.Available {
			add("info", "GCT_BALANCE_NA", "Équilibre G/D non mesuré", "Nécessite HRM-Pro / HRM 600 / Running Dynamics Pod.")
		}
	}

	if valid(in.elevLossM) && in.elevLossM > p.DescentAlertM {
		lvl := "warn"
		if in.elevLossM > 2*p.DescentAlertM {
			lvl = "danger"
		}
		add(lvl, "DESCENT_HIGH", "Dénivelé négatif élevé",
			fmt.Sprintf("-%s m > %s m, les descentes sollicitent fortement l'articulation fémoro-patellaire.", frfmt.Int(in.elevLossM), frfmt.Num(p.DescentAlertM, 1)))
	}

	if d := in.drift; d != nil && d.DecouplingPct >= 5 {
		lvl := "warn"
		if d.DecouplingPct > 10 {
			lvl = "danger"
		}
		add(lvl, "DRIFT_HIGH", "Dérive cardiaque",
			fmt.Sprintf("Découplage %s %s %%, fatigue ou endurance aérobie insuffisante sur cette durée.", driftMethodLabel(d.Method), frfmt.Signed(d.DecouplingPct, 1)))
	}

	if valid(in.fadePct) {
		add("warn", "FADE_END", "Fin de séance en perte d'allure",
			fmt.Sprintf("Dernier bloc %s %% plus lent que les précédents avec une FC au moins égale (fatigue en fin de séance).", frfmt.Int(in.fadePct)))
	}

	if rc := in.recovery; rc != nil {
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
		if r := rc.RestingHR; r != nil && r.Delta != nil && *r.Delta >= p.RhrAlertDeltaBpm {
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
			v := *bb.AtWake
			switch {
			case v < 30:
				add("danger", "BB_LOW", "Body Battery très basse", fmt.Sprintf("%s/100 au réveil, réserves d'énergie faibles.", frfmt.Int(v)))
			case v < 50:
				add("warn", "BB_LOW", "Body Battery basse", fmt.Sprintf("%s/100 au réveil, réserves d'énergie limitées.", frfmt.Int(v)))
			}
		}
	}

	if in.dayMode && valid(in.floorsUp) && in.floorsUp > 20 {
		add("warn", "FLOORS_HIGH", "Beaucoup d'escaliers",
			fmt.Sprintf("%s étages montés (> 20), charge fémoro-patellaire accrue.", frfmt.Int(in.floorsUp)))
	}

	if a := in.acwr; a != nil && a.Ratio != nil {
		r := *a.Ratio
		switch {
		case r > 1.5:
			add("danger", "ACWR", "Charge aiguë excessive", fmt.Sprintf("ACWR %s (> 1,5), risque élevé de blessure, alléger les prochains jours.", frfmt.Dec(r, 2)))
		case r > 1.3:
			add("warn", "ACWR", "Charge aiguë élevée", fmt.Sprintf("ACWR %s (> 1,3), progression trop rapide.", frfmt.Dec(r, 2)))
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
		if in.dayMode {
			msg = "Aucun signal d'alerte sur cette journée."
		}
		add("ok", "ALL_GOOD", "RAS", msg)
	}
	sort.SliceStable(out, func(i, j int) bool { return levelOrder[out[i].Level] < levelOrder[out[j].Level] })
	if out == nil {
		out = []Alert{}
	}
	return out
}

func driftMethodLabel(m string) string {
	if m == "pw:hr" {
		return "Pw:Hr"
	}
	return "Pace:Hr"
}

// DriftMethodLabel exporte le libellé ("Pw:Hr" / "Pace:Hr").
func DriftMethodLabel(m string) string { return driftMethodLabel(m) }
