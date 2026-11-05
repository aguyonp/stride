package analysis

import "strings"

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

// TypeLabel returns the French label of a Garmin typeKey.
func TypeLabel(typeKey string) string {
	if l, ok := typeLabels[typeKey]; ok {
		return l
	}
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

func IsRunningType(typeKey string) bool {
	switch typeKey {
	case "virtual_run", "ultra_run", "obstacle_run":
		return true
	}
	return strings.HasSuffix(typeKey, "running")
}

// Sport maps a Garmin typeKey to the coarse sport of ActivitySummary.
func Sport(typeKey string) string {
	switch {
	case IsRunningType(typeKey):
		return "run"
	case strings.Contains(typeKey, "cycling") || strings.Contains(typeKey, "biking") || strings.Contains(typeKey, "ride") ||
		typeKey == "commuting" || typeKey == "bmx" || typeKey == "cyclocross" || strings.HasPrefix(typeKey, "e_bike"):
		return "bike"
	case strings.Contains(typeKey, "walk"):
		return "walk"
	case typeKey == "hiking" || typeKey == "mountaineering" || typeKey == "snowshoeing":
		return "hike"
	case strings.Contains(typeKey, "swim"):
		return "swim"
	case typeKey == "strength_training" || typeKey == "fitness_equipment" || typeKey == "hiit" || typeKey == "indoor_cardio":
		return "strength"
	}
	return "other"
}

var stravaTypes = map[string]string{
	"Run": "running", "TrailRun": "trail_running", "VirtualRun": "virtual_run",
	"Ride": "cycling", "MountainBikeRide": "mountain_biking", "GravelRide": "gravel_cycling", "EBikeRide": "e_bike_fitness",
	"EMountainBikeRide": "e_bike_mountain", "VirtualRide": "virtual_ride", "Velomobile": "cycling", "Handcycle": "cycling",
	"Walk": "walking", "Hike": "hiking", "Swim": "lap_swimming", "WeightTraining": "strength_training",
	"Crossfit": "hiit", "HighIntensityIntervalTraining": "hiit", "Workout": "fitness_equipment", "Yoga": "yoga",
	"Pilates": "pilates", "Elliptical": "elliptical", "StairStepper": "stair_climbing", "Rowing": "rowing",
	"Kayaking": "kayaking", "AlpineSki": "resort_skiing", "BackcountrySki": "backcountry_skiing",
	"NordicSki": "cross_country_skiing", "Snowboard": "snowboarding", "Snowshoe": "snowshoeing",
	"RockClimbing": "rock_climbing", "Tennis": "tennis", "Padel": "padel", "Soccer": "soccer", "Golf": "golf", "Surfing": "surfing",
}

// StravaTypeKey maps a Strava sport_type to the equivalent Garmin typeKey.
func StravaTypeKey(sportType string) string {
	if k, ok := stravaTypes[sportType]; ok {
		return k
	}
	var b strings.Builder
	for i, r := range sportType {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "other"
	}
	return b.String()
}

// CompatibleSports tells whether two activities of these sports can be the same recording.
func CompatibleSports(a, b string) bool {
	if a == b || a == "other" || b == "other" {
		return true
	}
	return (a == "walk" || a == "hike") && (b == "walk" || b == "hike")
}
