package analysis

import (
	"strings"

	"stride/internal/garmin"
)

var typeLabels = map[string]string{
	"running":                    "Course",
	"street_running":             "Course",
	"trail_running":              "Trail",
	"treadmill_running":          "Course tapis",
	"indoor_running":             "Course intérieure",
	"track_running":              "Piste",
	"virtual_run":                "Course virtuelle",
	"ultra_run":                  "Ultra",
	"obstacle_run":               "Course d'obstacles",
	"cycling":                    "Vélo",
	"road_biking":                "Vélo route",
	"mountain_biking":            "VTT",
	"gravel_cycling":             "Gravel",
	"indoor_cycling":             "Vélo intérieur",
	"virtual_ride":               "Vélo virtuel",
	"e_bike_fitness":             "Vélo électrique",
	"e_bike_mountain":            "VTT électrique",
	"bmx":                        "BMX",
	"cyclocross":                 "Cyclocross",
	"track_cycling":              "Vélo piste",
	"commuting":                  "Vélotaf",
	"walking":                    "Marche",
	"casual_walking":             "Marche",
	"speed_walking":              "Marche rapide",
	"hiking":                     "Randonnée",
	"strength_training":          "Renforcement",
	"fitness_equipment":          "Fitness",
	"indoor_cardio":              "Cardio",
	"hiit":                       "HIIT",
	"yoga":                       "Yoga",
	"pilates":                    "Pilates",
	"breathwork":                 "Respiration",
	"lap_swimming":               "Natation",
	"open_water_swimming":        "Eau libre",
	"swimming":                   "Natation",
	"elliptical":                 "Elliptique",
	"stair_climbing":             "Escaliers",
	"indoor_rowing":              "Rameur",
	"rowing":                     "Aviron",
	"multi_sport":                "Multisport",
	"other":                      "Autre",
	"kayaking":                   "Kayak",
	"whitewater_rafting":         "Rafting",
	"stand_up_paddleboarding":    "Paddle",
	"resort_skiing":              "Ski alpin",
	"resort_skiing_snowboarding": "Ski alpin",
	"backcountry_skiing":         "Ski de randonnée",
	"cross_country_skiing":       "Ski de fond",
	"skate_skiing":               "Ski skating",
	"snowboarding":               "Snowboard",
	"snowshoeing":                "Raquettes",
	"mountaineering":             "Alpinisme",
	"rock_climbing":              "Escalade",
	"bouldering":                 "Bloc (escalade)",
	"indoor_climbing":            "Escalade en salle",
	"tennis":                     "Tennis",
	"padel":                      "Padel",
	"soccer":                     "Football",
	"golf":                       "Golf",
	"surfing":                    "Surf",
	"mobility":                   "Mobilité",
}

// TypeLabel retourne le libellé FR d'un typeKey Garmin.
func TypeLabel(typeKey string) string {
	if l, ok := typeLabels[typeKey]; ok {
		return l
	}
	// variantes Garmin : "kayaking_v2", "resort_skiing_snowboarding_ws"…
	for _, suf := range []string{"_v2", "_v3", "_ws"} {
		if base := strings.TrimSuffix(typeKey, suf); base != typeKey {
			return TypeLabel(base)
		}
	}
	switch {
	case IsRunningType(typeKey):
		return "Course"
	case strings.Contains(typeKey, "cycling") || strings.Contains(typeKey, "biking"):
		return "Vélo"
	case strings.Contains(typeKey, "walk"):
		return "Marche"
	case strings.Contains(typeKey, "swim"):
		return "Natation"
	case typeKey == "":
		return "Activité"
	}
	s := strings.ReplaceAll(typeKey, "_", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// IsRunningType indique si le typeKey est une course à pied.
func IsRunningType(typeKey string) bool {
	switch typeKey {
	case "virtual_run", "ultra_run", "obstacle_run":
		return true
	}
	return strings.HasSuffix(typeKey, "running")
}

// SummaryFromList convertit un élément de liste Garmin.
func SummaryFromList(it *garmin.ActivityListItem) ActivitySummary {
	tk := it.ActivityType.TypeKey
	s := ActivitySummary{
		ID:            it.ActivityID,
		Name:          spv(it.ActivityName),
		Type:          tk,
		TypeLabel:     TypeLabel(tk),
		IsRunning:     IsRunningType(tk),
		StartLocal:    garmin.NormalizeLocal(it.StartTimeLocal),
		Date:          it.LocalDate(),
		DistanceM:     pr(it.Distance, 1),
		DurationS:     pr(it.Duration, 1),
		AvgHR:         pr(it.AverageHR, 0),
		AvgPower:      pr(it.AvgPower, 0),
		AvgCadenceSpm: pr(it.AvgCadenceSpm, 1),
		ElevGainM:     pr(it.ElevationGain, 1),
		ElevLossM:     pr(it.ElevationLoss, 1),
		TrainingLoad:  pr(it.TrainingLoad, 1),
	}
	s.AvgPaceSecPerKm = num(paceFromSpeed(pf(it.AverageSpeed)), 0)
	if s.AvgPaceSecPerKm == nil && valid(pf(it.Distance)) && pf(it.Distance) > 0 && valid(pf(it.Duration)) {
		s.AvgPaceSecPerKm = num(pf(it.Duration)/pf(it.Distance)*1000, 0)
	}
	return s
}

// DetailFromAPI construit ActivityDetail depuis le détail Garmin (et l'élément de liste si connu).
func DetailFromAPI(d *garmin.ActivityDetail, it *garmin.ActivityListItem) ActivityDetail {
	sm := &d.Summary
	tk := d.ActivityTypeDTO.TypeKey
	if tk == "" && it != nil {
		tk = it.ActivityType.TypeKey
	}
	s := ActivitySummary{
		ID:            d.ActivityID,
		Name:          spv(d.ActivityName),
		Type:          tk,
		TypeLabel:     TypeLabel(tk),
		IsRunning:     IsRunningType(tk),
		StartLocal:    d.LocalStart(),
		Date:          d.LocalDate(),
		DistanceM:     pr(sm.Distance, 1),
		DurationS:     pr(sm.Duration, 1),
		AvgHR:         pr(sm.AverageHR, 0),
		AvgPower:      pr(sm.AveragePower, 0),
		AvgCadenceSpm: pr(sm.AverageRunCadence, 1),
		ElevGainM:     pr(sm.ElevationGain, 1),
		ElevLossM:     pr(sm.ElevationLoss, 1),
		TrainingLoad:  pr(sm.ActivityTrainingLoad, 1),
	}
	s.AvgPaceSecPerKm = num(paceFromSpeed(pf(sm.AverageSpeed)), 0)
	if s.AvgPaceSecPerKm == nil && valid(pf(sm.Distance)) && pf(sm.Distance) > 0 && valid(pf(sm.Duration)) {
		s.AvgPaceSecPerKm = num(pf(sm.Duration)/pf(sm.Distance)*1000, 0)
	}
	out := ActivityDetail{
		ActivitySummary:     s,
		LocationName:        d.LocationName,
		MovingDurationS:     pr(sm.MovingDuration, 1),
		MaxHR:               pr(sm.MaxHR, 0),
		NormPower:           pr(sm.NormalizedPower, 0),
		MaxCadenceSpm:       pr(sm.MaxRunCadence, 0),
		AerobicTE:           pr(sm.TrainingEffect, 1),
		AnaerobicTE:         pr(sm.AnaerobicTE, 1),
		TrainingEffectLabel: sm.TrainingEffectLabel,
		ImpactLoad:          pr(sm.ImpactLoad, 0),
		Steps:               pr(sm.Steps, 0),
		Calories:            pr(sm.Calories, 0),
		BodyBatteryDelta:    pr(sm.DifferenceBodyBattery, 0),
	}
	if it != nil {
		if out.TrainingLoad == nil {
			out.TrainingLoad = pr(it.TrainingLoad, 1)
		}
		if out.AvgCadenceSpm == nil {
			out.AvgCadenceSpm = pr(it.AvgCadenceSpm, 1)
		}
		if out.NormPower == nil {
			out.NormPower = pr(it.NormPower, 0)
		}
		if out.AerobicTE == nil {
			out.AerobicTE = pr(it.AerobicTE, 1)
		}
		if out.AnaerobicTE == nil {
			out.AnaerobicTE = pr(it.AnaerobicTE, 1)
		}
		if out.LocationName == nil {
			out.LocationName = it.LocationName
		}
	}
	return out
}

// historyRow convertit un élément de liste en ligne d'historique.
func historyRow(it *garmin.ActivityListItem) HistoryRow {
	s := SummaryFromList(it)
	return HistoryRow{
		Date:          s.Date,
		Type:          s.Type,
		TypeLabel:     s.TypeLabel,
		DistanceM:     s.DistanceM,
		DurationS:     s.DurationS,
		PaceSecPerKm:  s.AvgPaceSecPerKm,
		AvgHR:         s.AvgHR,
		AvgCadenceSpm: s.AvgCadenceSpm,
		ElevLossM:     s.ElevLossM,
		TrainingLoad:  s.TrainingLoad,
	}
}
