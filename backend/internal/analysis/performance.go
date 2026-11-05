package analysis

import (
	"math"
	"strings"

	"stride/internal/athlete"
	"stride/internal/frfmt"
	"stride/internal/garmin"
)

func rangeLabel(minSec, maxSec float64) string {
	return frfmt.PaceShort(minSec) + "–" + frfmt.PaceShort(maxSec)
}

// DatedLoad is the training load of one activity.
type DatedLoad struct {
	Date       string // local date
	StartLocal string
	Load       float64
}

// ComputeLoadRatio returns acute (7 d) / chronic (28 d ÷ 4) load for activities started up to
// untilLocal (inclusive; "" = whole day). Nil when the chronic load is zero.
func ComputeLoadRatio(loads []DatedLoad, date, untilLocal string, maxRatio float64) *LoadRatio {
	from7, from28 := dateAdd(date, -6), dateAdd(date, -27)
	acute, total28 := 0.0, 0.0
	for _, l := range loads {
		if l.Date > date || l.Date < from28 {
			continue
		}
		if untilLocal != "" && l.StartLocal > untilLocal {
			continue
		}
		total28 += l.Load
		if l.Date >= from7 {
			acute += l.Load
		}
	}
	chronic := total28 / 4
	if chronic <= 0 {
		return nil
	}
	ratio := acute / chronic
	if maxRatio <= 0 {
		maxRatio = 1.3
	}
	verdict := "optimal"
	switch {
	case ratio > math.Max(1.5, maxRatio+0.2):
		verdict = "danger"
	case ratio > maxRatio:
		verdict = "high"
	case ratio < 0.8:
		verdict = "low"
	}
	return &LoadRatio{Acute: roundTo(acute, 0), Chronic: roundTo(chronic, 0), Ratio: num(ratio, 2), Verdict: verdict, Source: "computed"}
}

// ACWRFrom exposes a load ratio in the V1 Performance.acwr shape.
func ACWRFrom(lr *LoadRatio) *ACWR {
	if lr == nil {
		return nil
	}
	return &ACWR{Acute7d: lr.Acute, Chronic28d: lr.Chronic, Ratio: lr.Ratio, Verdict: lr.Verdict}
}

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

// PickZones chooses the running HR zones (DEFAULT otherwise).
func PickZones(zs []garmin.HRZoneConfig) []float64 {
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

func buildPerformance(s *session, det *garmin.ActivityDetail, it *garmin.ActivityListItem, targets []athlete.PaceTarget, hrZones, zoneFloors []float64) Performance {
	perf := Performance{TargetPaces: []TargetPace{}, BestEfforts: bestEfforts(it), HrZoneFloorsBpm: zoneFloors}
	avgP, normP := nan, nan
	if det != nil {
		avgP, normP = pf(det.Summary.AveragePower), pf(det.Summary.NormalizedPower)
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
	for _, t := range targets {
		secs, dist := targetPaceTime(s, t.MinSecPerKm, t.MaxSecPerKm)
		perf.TargetPaces = append(perf.TargetPaces, TargetPace{
			Label: "Allure " + t.Label, RangeLabel: rangeLabel(t.MinSecPerKm, t.MaxSecPerKm), Seconds: roundTo(secs, 0), DistanceM: roundTo(dist, 0),
		})
	}
	return perf
}
