package analysis

import (
	"math"

	"stride/internal/athlete"
	"stride/internal/fitdata"
	"stride/internal/garmin"
)

// Version is bumped whenever Core changes; it is part of the analysis cache key.
const Version = 2

// Inputs of the per-activity computation. Detail/ListItem are nil for Strava-only activities.
type Inputs struct {
	TypeKey         string
	Detail          *garmin.ActivityDetail
	ListItem        *garmin.ActivityListItem
	Act             *fitdata.Activity
	Thresholds      athlete.Thresholds
	PaceTargets     []athlete.PaceTarget
	HRZoneFloors    []float64
	GarminConnected bool
}

// Core is the cacheable part of an activity analysis (depends only on the bundle and the profile).
type Core struct {
	Biomechanics Biomechanics `json:"biomechanics"`
	Segments     Segments     `json:"segments"`
	Series       Series       `json:"series"`
	Drift        *Drift       `json:"drift"`
	HrZonesS     []float64    `json:"hrZonesS"`
	PowerZonesS  []float64    `json:"powerZonesS"`
	Performance  Performance  `json:"performance"`
	GapSecPerKm  *float64     `json:"gapSecPerKm"`
	Stamina      *Stamina     `json:"stamina"`
	ElevGainM    *float64     `json:"elevGainM"`
	ElevLossM    *float64     `json:"elevLossM"`
	FadePct      *float64     `json:"fadePct"`
	Warnings     []string     `json:"warnings"`
}

const (
	warnGAPModel   = "GAP calculée avec un modèle de coût de pente (Minetti), non fournie par Garmin"
	warnStrideCalc = "Longueur de foulée calculée (vitesse / cadence), non mesurée par un capteur"
	warnNoDynamics = "Dynamique de course (GCT, oscillation verticale, équilibre G/D) et Stamina : Garmin non connecté"
)

func Compute(in Inputs) *Core {
	isRun := IsRunningType(in.TypeKey)
	var s *session
	if in.Act != nil && len(in.Act.Records) > 0 {
		s = newSession(in.Act, isRun)
	}
	c := &Core{Warnings: []string{}}
	c.Biomechanics = computeBiomech(s, in.Detail, in.Thresholds, isRun, in.GarminConnected)
	if isRun && s != nil {
		c.Biomechanics.FatigueDrift = s.fatigueDrift()
		c.Biomechanics.Slope = s.slope()
	}
	c.Segments = buildSegments(s)
	c.Series = buildSeries(s)
	c.Drift = computeDrift(s)
	c.FadePct = num(fadeEnd(c.Segments), 1)

	var hrZones, pwZones []float64
	if in.ListItem != nil {
		hrZones, pwZones = in.ListItem.HRZones(), in.ListItem.PowerZones()
	}
	if hrZones == nil && in.Act != nil {
		hrZones = in.Act.HRZonesS
	}
	if pwZones == nil && in.Act != nil {
		pwZones = in.Act.PowerZonesS
	}
	if hrZones == nil {
		hrZones = zonesFromRecords(s, in.HRZoneFloors)
	}
	c.HrZonesS, c.PowerZonesS = roundZones(hrZones), roundZones(pwZones)
	c.Performance = buildPerformance(s, in.Detail, in.ListItem, in.PaceTargets, c.HrZonesS, in.HRZoneFloors)

	gain, loss := nan, nan
	if in.Detail != nil {
		gain, loss = pf(in.Detail.Summary.ElevationGain), pf(in.Detail.Summary.ElevationLoss)
	}
	if in.ListItem != nil {
		gain, loss = first(gain, pf(in.ListItem.ElevationGain)), first(loss, pf(in.ListItem.ElevationLoss))
	}
	if in.Act != nil {
		gain, loss = first(gain, in.Act.Session.AscentM), first(loss, in.Act.Session.DescentM)
	}
	if s != nil && (!valid(gain) || !valid(loss)) {
		ws := s.window(math.Inf(-1), math.Inf(1), false)
		gain, loss = first(gain, ws.ascent), first(loss, ws.descent)
	}
	c.ElevGainM, c.ElevLossM = num(gain, 0), num(loss, 0)

	if isRun {
		gapSpeed := nan
		if in.Detail != nil {
			gapSpeed = pf(in.Detail.Summary.AvgGradeAdjustedSpeed)
		}
		if in.ListItem != nil {
			gapSpeed = first(gapSpeed, pf(in.ListItem.AvgGradeAdjustedSpeed))
		}
		if !valid(gapSpeed) && s != nil {
			var acc wacc
			for i := range s.act.Records {
				if s.run[i] {
					acc.add(s.gap[i], s.w[i])
				}
			}
			gapSpeed = acc.mean()
		}
		c.GapSecPerKm = num(paceFromSpeed(gapSpeed), 0)
		if s != nil && s.gapModeled && !s.gapRecorded {
			c.Warnings = append(c.Warnings, warnGAPModel)
		}
		if s != nil && c.Biomechanics.StrideLengthM != nil && !in.Act.Has(func(r fitdata.Record) float64 { return r.StepM }) &&
			(in.Detail == nil || in.Detail.Summary.StrideLength == nil) {
			c.Warnings = append(c.Warnings, warnStrideCalc)
		}
	}

	if in.Detail != nil {
		sm := in.Detail.Summary
		c.Stamina = staminaFrom(sm.BeginPotentialStamina, sm.EndPotentialStamina, sm.MinAvailableStamina, in.Act)
	} else if in.Act != nil && in.GarminConnected {
		c.Stamina = staminaFrom(nil, nil, nil, in.Act)
	}
	if !in.GarminConnected && isRun {
		c.Warnings = append(c.Warnings, warnNoDynamics)
	}
	return c
}

func roundZones(z []float64) []float64 {
	if z == nil {
		return nil
	}
	out := make([]float64, len(z))
	for i, v := range z {
		out[i] = roundTo(v, 0)
	}
	return out
}
