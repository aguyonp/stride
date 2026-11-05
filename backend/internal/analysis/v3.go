package analysis

import (
	"fmt"
	"math"
	"strings"

	"stride/internal/fitdata"
	"stride/internal/frfmt"
)

// MetricsVersion is bumped whenever the efficiency / impact formulas change (persisted metrics).
const MetricsVersion = 1

const (
	efMinMovingS   = 20 * 60
	movingMinSpeed = 1.0  // m/s; slower samples are pauses
	maxSampleGapS  = 10.0 // larger recording gaps are pauses
	steadyMaxCV    = 0.25 // coefficient of variation of the 60 s speed above which the effort is interval-like
)

type Efficiency struct {
	Eligible       bool     `json:"eligible"`
	Reason         *string  `json:"reason"`
	EfSpeed        *float64 `json:"efSpeed"` // m/min per bpm
	EfPower        *float64 `json:"efPower"` // W per bpm
	TrendChangePct *float64 `json:"trendChangePct"`
}

type Impact struct {
	Steps         *float64 `json:"steps"`
	StepsPerKm    *float64 `json:"stepsPerKm"`
	VoFactor      *float64 `json:"voFactor"`
	DescentFactor *float64 `json:"descentFactor"`
	JointIndex    *float64 `json:"jointIndex"`
	JointIndex7d  *float64 `json:"jointIndex7d"`
}

// Metrics is the persisted per-activity part (trend and 7-day sums are added by the domain).
type Metrics struct {
	Efficiency   Efficiency `json:"efficiency"`
	Impact       *Impact    `json:"impact"`
	AvgHR        *float64   `json:"avgHR"`
	PaceSecPerKm *float64   `json:"paceSecPerKm"`
}

type MetricsInput struct {
	Sport                                                                             string // run | bike | …
	IsRunning                                                                         bool
	Act                                                                               *fitdata.Activity
	DistanceM, MovingS, DurationS, AvgHR, AvgPower, AvgCadenceSpm, ElevLossM, AvgVOCm *float64
}

type movingStats struct {
	movingS, distM, hrW, hrS, pwW, pwS, steps, stepsS, voW, voS float64
	speeds                                                      []float64
}

func scanMoving(act *fitdata.Activity) movingStats {
	var ms movingStats
	if act == nil {
		return ms
	}
	recs := act.Records
	var bucket, bucketT float64
	for i := 1; i < len(recs); i++ {
		r := recs[i]
		dt := r.T - recs[i-1].T
		if !valid(dt) || dt <= 0 || dt > maxSampleGapS {
			continue
		}
		sp := r.SpeedMS
		if !valid(sp) && valid(r.DistM) && valid(recs[i-1].DistM) {
			sp = (r.DistM - recs[i-1].DistM) / dt
		}
		if !valid(sp) || sp < movingMinSpeed {
			continue
		}
		ms.movingS += dt
		ms.distM += sp * dt
		if valid(r.HR) && r.HR > 0 {
			ms.hrW += r.HR * dt
			ms.hrS += dt
		}
		if valid(r.PowerW) && r.PowerW > 0 {
			ms.pwW += r.PowerW * dt
			ms.pwS += dt
		}
		if valid(r.CadenceSpm) && r.CadenceSpm > 0 {
			ms.steps += r.CadenceSpm / 60 * dt
			ms.stepsS += dt
		}
		if valid(r.VOCm) && r.VOCm > 0 {
			ms.voW += r.VOCm * dt
			ms.voS += dt
		}
		g := r.GAPSpeedMS
		if !valid(g) || g <= 0 {
			g = sp
		}
		bucket += g * dt
		bucketT += dt
		if bucketT >= 60 {
			ms.speeds = append(ms.speeds, bucket/bucketT)
			bucket, bucketT = 0, 0
		}
	}
	return ms
}

func cv(xs []float64) float64 {
	if len(xs) < 5 {
		return 0
	}
	m := 0.0
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	v := 0.0
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return math.Sqrt(v/float64(len(xs))) / m
}

// intervalStructured detects interval sessions from workout laps (recovery or active steps).
func intervalStructured(act *fitdata.Activity) bool {
	if act == nil {
		return false
	}
	rest, active := 0, 0
	for _, l := range act.Laps {
		switch {
		case l.Intensity == "rest" || l.Intensity == "recovery":
			rest++
		case l.HasWorkoutStep && (l.Intensity == "interval" || l.Intensity == "active"):
			active++
		}
	}
	return rest >= 2 || active >= 3
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// ComputeMetrics derives aerobic efficiency (§11.4: speed m/min ÷ HR and power ÷ HR on moving
// time) and the cumulative impact (steps × VO factor × descent factor / 1000).
func ComputeMetrics(in MetricsInput) Metrics {
	ms := scanMoving(in.Act)
	movingS, distM := ms.movingS, ms.distM
	if movingS < 60 {
		movingS = first(pf(in.MovingS), pf(in.DurationS))
		distM = pf(in.DistanceM)
	}
	hr := pf(in.AvgHR)
	if ms.hrS >= 60 {
		hr = ms.hrW / ms.hrS
	}
	pw := pf(in.AvgPower)
	if ms.pwS >= 60 {
		pw = ms.pwW / ms.pwS
	}
	out := Metrics{AvgHR: num(hr, 0)}
	if valid(distM) && distM > 0 && valid(movingS) && movingS > 0 {
		out.PaceSecPerKm = num(movingS/distM*1000, 0)
	}

	ef := Efficiency{}
	reason := ""
	switch {
	case in.Sport != "run" && in.Sport != "bike":
		reason = "Efficience calculée pour la course et le vélo uniquement"
	case !valid(movingS) || movingS < efMinMovingS:
		reason = "Séance trop courte (moins de 20 min en mouvement)"
	case !valid(hr) || hr <= 0:
		reason = "Fréquence cardiaque indisponible"
	case intervalStructured(in.Act) || cv(ms.speeds) > steadyMaxCV:
		reason = "Séance fractionnée : efficience calculée sur les efforts continus uniquement"
	}
	if reason == "" && valid(distM) && distM > 0 {
		ef.Eligible = true
		ef.EfSpeed = num(distM/movingS*60/hr, 3)
		if valid(pw) && pw > 0 {
			ef.EfPower = num(pw/hr, 3)
		}
	} else if reason == "" {
		reason = "Distance indisponible"
	}
	if reason != "" {
		ef.Reason = &reason
	}
	out.Efficiency = ef

	if !in.IsRunning {
		return out
	}
	steps := math.NaN()
	if ms.stepsS >= 60 {
		steps = ms.steps * movingS / ms.stepsS
	} else if c := pf(in.AvgCadenceSpm); valid(c) && c > 0 && valid(movingS) {
		steps = c / 60 * movingS
	}
	if !valid(steps) {
		return out
	}
	imp := &Impact{Steps: num(steps, 0)}
	vo := pf(in.AvgVOCm)
	if ms.voS >= 60 {
		vo = ms.voW / ms.voS
	}
	if in.Act != nil && !valid(vo) {
		vo = in.Act.Session.AvgVOCm
	}
	voFactor := 1.0
	if valid(vo) && vo > 0 {
		voFactor = clamp(vo/8, 0.75, 1.5)
		imp.VoFactor = num(voFactor, 2)
	}
	descentFactor := 1.0
	if km := distM / 1000; valid(km) && km > 0 {
		imp.StepsPerKm = num(steps/km, 0)
		loss := pf(in.ElevLossM)
		if !valid(loss) && in.Act != nil {
			loss = in.Act.Session.DescentM
			if !valid(loss) && len(in.Act.Records) > 1 {
				loss = newSession(in.Act, true).window(math.Inf(-1), math.Inf(1), false).descent
			}
		}
		if valid(loss) && loss >= 0 {
			descentFactor = 1 + 0.01*(loss/km)
			imp.DescentFactor = num(descentFactor, 2)
		}
	}
	imp.JointIndex = num(steps*voFactor*descentFactor/1000, 1)
	out.Impact = imp
	return out
}

// EfficiencyChangePct compares the mean EF of the last 4 weeks to the first 4 weeks of a window
// (dated points, oldest first); nil without 2 points on each side.
func EfficiencyChangePct(dates []string, values []float64, from, to string) *float64 {
	early, late := DateAdd(from, 27), DateAdd(to, -27)
	var a, b []float64
	for i, d := range dates {
		if d < from || d > to {
			continue
		}
		if d <= early {
			a = append(a, values[i])
		}
		if d >= late {
			b = append(b, values[i])
		}
	}
	if len(a) < 2 || len(b) < 2 {
		return nil
	}
	ma, mb := mean(a), mean(b)
	if ma <= 0 {
		return nil
	}
	return num((mb-ma)/ma*100, 1)
}

func mean(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// ---------- readiness banner ----------

type ReadinessBanner struct {
	Level               string   `json:"level"` // green | orange | red | unknown
	Title               string   `json:"title"`
	Reasons             []string `json:"reasons"`
	Hrv7dAvg            *float64 `json:"hrv7dAvg"`
	HrvBaselineLow      *float64 `json:"hrvBaselineLow"`
	NightsBelowBaseline *int     `json:"nightsBelowBaseline"`
	Sleep7dAvg          *float64 `json:"sleep7dAvg"`
	LoadRatio           *float64 `json:"loadRatio"`
}

var readinessTitles = map[string]string{
	"green":   "Feu vert : préparation correcte",
	"orange":  "Vigilance : signaux de fatigue",
	"red":     "Alerte : récupération insuffisante sous charge élevée",
	"unknown": "Préparation : données insuffisantes",
}

const loadRatioAlert = 1.3

// BuildReadinessBanner applies §11.4 on the 7 days ending at w.Date.
func BuildReadinessBanner(w *Wellness, lr *LoadRatio, rc *RecoveryContext) ReadinessBanner {
	b := ReadinessBanner{Reasons: []string{}}
	var hrvs []float64
	var lastNight, low float64 = math.NaN(), math.NaN()
	nights, known := 0, 0
	days := dateRange(dateAdd(w.Date, -6), w.Date)
	for _, d := range days {
		if h := w.HRV[d]; h != nil && h.Baseline != nil && valid(pf(h.Baseline.BalancedLow)) {
			low = pf(h.Baseline.BalancedLow)
		}
	}
	var sleeps []float64
	for _, d := range days {
		if h := w.HRV[d]; h != nil && valid(pf(h.LastNightAvg)) {
			v := pf(h.LastNightAvg)
			hrvs = append(hrvs, v)
			lastNight = v
			dl := low
			if h.Baseline != nil && valid(pf(h.Baseline.BalancedLow)) {
				dl = pf(h.Baseline.BalancedLow)
			}
			if valid(dl) {
				known++
				if v < dl {
					nights++
				}
			}
		}
		if sl := w.sleepFor(d); sl != nil {
			if sc, _ := sl.Score(); valid(pf(sc)) {
				sleeps = append(sleeps, pf(sc))
			}
		}
	}
	if len(hrvs) > 0 {
		b.Hrv7dAvg = num(mean(hrvs), 0)
	}
	b.HrvBaselineLow = num(low, 0)
	if known > 0 {
		b.NightsBelowBaseline = &nights
	}
	if len(sleeps) > 0 {
		b.Sleep7dAvg = num(mean(sleeps), 0)
	}
	ratio := math.NaN()
	if lr != nil {
		ratio = pf(lr.Ratio)
		b.LoadRatio = num(ratio, 2)
	}
	hrvAvgLow := b.Hrv7dAvg != nil && valid(low) && *b.Hrv7dAvg < low
	lastLow := valid(lastNight) && valid(low) && lastNight < low
	manyNights := nights >= 3
	highLoad := valid(ratio) && ratio > loadRatioAlert
	poorSleep := b.Sleep7dAvg != nil && *b.Sleep7dAvg < 60
	rhrUp := rc != nil && rc.RestingHR != nil && rc.RestingHR.Delta != nil && *rc.RestingHR.Delta >= 5

	if hrvAvgLow {
		b.Reasons = append(b.Reasons, fmt.Sprintf("VFC moyenne 7 j (%s ms) sous la ligne de base basse (%s ms)", frfmt.Int(*b.Hrv7dAvg), frfmt.Int(low)))
	}
	if manyNights {
		b.Reasons = append(b.Reasons, fmt.Sprintf("%d nuits sur 7 sous la ligne de base VFC", nights))
	}
	if lastLow && !hrvAvgLow {
		b.Reasons = append(b.Reasons, fmt.Sprintf("VFC de la nuit dernière (%s ms) sous la ligne de base basse (%s ms)", frfmt.Int(lastNight), frfmt.Int(low)))
	}
	if highLoad {
		b.Reasons = append(b.Reasons, fmt.Sprintf("Ratio de charge élevé (%s > %s)", frfmt.Dec(ratio, 2), frfmt.Dec(loadRatioAlert, 1)))
	}
	if poorSleep {
		b.Reasons = append(b.Reasons, fmt.Sprintf("Sommeil moyen 7 j faible (%s/100)", frfmt.Int(*b.Sleep7dAvg)))
	}
	if rhrUp {
		b.Reasons = append(b.Reasons, fmt.Sprintf("FC de repos +%s bpm vs moyenne 7 j", frfmt.Int(*rc.RestingHR.Delta)))
	}
	switch {
	case (hrvAvgLow || manyNights) && highLoad:
		b.Level = "red"
	case hrvAvgLow || lastLow || manyNights || highLoad || poorSleep || rhrUp:
		b.Level = "orange"
	case len(hrvs) == 0 && len(sleeps) == 0 && !valid(ratio) && (rc == nil || rc.RestingHR == nil):
		b.Level = "unknown"
		b.Reasons = append(b.Reasons, "Ni VFC, ni sommeil, ni ratio de charge disponibles")
	default:
		b.Level = "green"
		b.Reasons = append(b.Reasons, "Aucun signal de fatigue détecté")
	}
	b.Title = readinessTitles[b.Level]
	return b
}

// ReadinessEmoji is the level marker used in prompts and notifications.
func ReadinessEmoji(level string) string {
	return map[string]string{"green": "🟢", "orange": "🟠", "red": "🔴"}[strings.ToLower(level)]
}
