package analysis

import (
	"strings"

	"patella/internal/config"
	"patella/internal/frfmt"
	"patella/internal/garmin"
)

// rangeLabel : "3:35–3:40".
func rangeLabel(minSec, maxSec float64) string {
	return frfmt.PaceShort(minSec) + "–" + frfmt.PaceShort(maxSec)
}

// computeACWR : charge aiguë (7 j) / chronique (moyenne hebdo sur 28 j), activités
// démarrées jusqu'à `untilLocal` inclus.
func computeACWR(acts []garmin.ActivityListItem, date, untilLocal string) *ACWR {
	from7, from28 := dateAdd(date, -6), dateAdd(date, -27)
	acute, total28 := 0.0, 0.0
	for i := range acts {
		a := &acts[i]
		if a.TrainingLoad == nil {
			continue
		}
		d := a.LocalDate()
		if d > date || d < from28 {
			continue
		}
		if untilLocal != "" && garmin.NormalizeLocal(a.StartTimeLocal) > untilLocal {
			continue
		}
		total28 += *a.TrainingLoad
		if d >= from7 {
			acute += *a.TrainingLoad
		}
	}
	chronic := total28 / 4
	if chronic <= 0 {
		return nil
	}
	ratio := acute / chronic
	verdict := "optimal"
	switch {
	case ratio > 1.5:
		verdict = "danger"
	case ratio > 1.3:
		verdict = "high"
	case ratio < 0.8:
		verdict = "low"
	}
	return &ACWR{Acute7d: roundTo(acute, 0), Chronic28d: roundTo(chronic, 0), Ratio: num(ratio, 2), Verdict: verdict}
}

// bestEfforts depuis les champs fastestSplit_* de la liste.
func bestEfforts(it *garmin.ActivityListItem) []BestEffort {
	out := []BestEffort{}
	if it == nil {
		return out
	}
	add := func(label string, p *float64) {
		if p != nil && *p > 0 {
			out = append(out, BestEffort{Label: label, Seconds: roundTo(*p, 0)})
		}
	}
	add("1 km", it.Fastest1000)
	add("1 mile", it.Fastest1609)
	add("5 km", it.Fastest5000)
	add("10 km", it.Fastest10000)
	add("Semi-marathon", it.Fastest21098)
	add("Marathon", it.Fastest42195)
	return out
}

// pickZones choisit la config de zones FC la plus pertinente (course, sinon défaut).
func pickZones(zs []garmin.HRZoneConfig) []float64 {
	var def []float64
	for i := range zs {
		f := zs[i].Floors()
		if f == nil {
			continue
		}
		sport := strings.ToUpper(spv(zs[i].Sport))
		if sport == "RUNNING" {
			return f
		}
		if def == nil || sport == "DEFAULT" {
			def = f
		}
	}
	return def
}

func buildPerformance(s *session, det *garmin.ActivityDetail, it *garmin.ActivityListItem, w *wellness, p config.Profile, hrZones []float64, untilLocal string) *Performance {
	perf := &Performance{TargetPaces: []TargetPace{}, BestEfforts: bestEfforts(it)}
	var sm *garmin.ActivitySumDTO
	if det != nil {
		sm = &det.Summary
	}
	avgP, normP := nan, nan
	if sm != nil {
		avgP, normP = pf(sm.AveragePower), pf(sm.NormalizedPower)
	}
	if s != nil {
		avgP = first(avgP, s.act.Session.AvgPower)
		normP = first(normP, s.act.Session.NormPower)
	}
	if it != nil {
		avgP = first(avgP, pf(it.AvgPower))
		normP = first(normP, pf(it.NormPower))
	}
	perf.AvgPower, perf.NormPower = num(avgP, 0), num(normP, 0)
	perf.TimeInZonesS = hrZones
	if len(hrZones) == 5 {
		perf.Z4Z5S = num(hrZones[3]+hrZones[4], 0)
	}
	targets := []struct {
		label    string
		min, max float64
	}{
		{"Allure 10 km", p.Pace10kMinSecPerKm, p.Pace10kMaxSecPerKm},
		{"Allure marathon", p.PaceMarathonMinSecPerKm, p.PaceMarathonMaxSecPerKm},
	}
	for _, t := range targets {
		secs, dist := targetPaceTime(s, t.min, t.max)
		perf.TargetPaces = append(perf.TargetPaces, TargetPace{
			Label: t.label, RangeLabel: rangeLabel(t.min, t.max), Seconds: roundTo(secs, 0), DistanceM: roundTo(dist, 0),
		})
	}
	if w != nil && w.actsOK {
		perf.ACWR = computeACWR(w.acts, w.date, untilLocal)
	}
	if w != nil {
		perf.HrZoneFloorsBpm = pickZones(w.zones)
	}
	return perf
}
