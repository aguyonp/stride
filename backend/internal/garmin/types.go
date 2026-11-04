package garmin

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// Les champs numériques optionnels sont des *float64 (nil = absent / null).

// TypeRef est un {typeKey} Garmin.
type TypeRef struct {
	TypeKey string `json:"typeKey"`
}

// ActivityListItem : élément de /activitylist-service/activities/search/activities.
type ActivityListItem struct {
	ActivityID       int64    `json:"activityId"`
	ActivityName     *string  `json:"activityName"`
	StartTimeLocal   string   `json:"startTimeLocal"` // "2026-11-02 18:30:12"
	StartTimeGMT     string   `json:"startTimeGMT"`
	ActivityType     TypeRef  `json:"activityType"`
	Distance         *float64 `json:"distance"`
	Duration         *float64 `json:"duration"`
	MovingDuration   *float64 `json:"movingDuration"`
	ElapsedDuration  *float64 `json:"elapsedDuration"`
	ElevationGain    *float64 `json:"elevationGain"`
	ElevationLoss    *float64 `json:"elevationLoss"`
	AverageSpeed     *float64 `json:"averageSpeed"`
	MaxSpeed         *float64 `json:"maxSpeed"`
	AverageHR        *float64 `json:"averageHR"`
	MaxHR            *float64 `json:"maxHR"`
	AvgCadenceSpm    *float64 `json:"averageRunningCadenceInStepsPerMinute"`
	MaxCadenceSpm    *float64 `json:"maxRunningCadenceInStepsPerMinute"`
	AvgPower         *float64 `json:"avgPower"`
	MaxPower         *float64 `json:"maxPower"`
	NormPower        *float64 `json:"normPower"`
	AvgVO            *float64 `json:"avgVerticalOscillation"` // cm
	AvgGCT           *float64 `json:"avgGroundContactTime"`   // ms
	AvgStrideLength  *float64 `json:"avgStrideLength"`        // cm
	AvgVerticalRatio *float64 `json:"avgVerticalRatio"`       // %
	AerobicTE        *float64 `json:"aerobicTrainingEffect"`
	AnaerobicTE      *float64 `json:"anaerobicTrainingEffect"`
	TELabel          *string  `json:"trainingEffectLabel"`
	TrainingLoad     *float64 `json:"activityTrainingLoad"`
	LapCount         *float64 `json:"lapCount"`
	LocationName     *string  `json:"locationName"`
	Steps            *float64 `json:"steps"`
	Calories         *float64 `json:"calories"`
	BodyBatteryDelta *float64 `json:"differenceBodyBattery"`
	HRZone1          *float64 `json:"hrTimeInZone_1"`
	HRZone2          *float64 `json:"hrTimeInZone_2"`
	HRZone3          *float64 `json:"hrTimeInZone_3"`
	HRZone4          *float64 `json:"hrTimeInZone_4"`
	HRZone5          *float64 `json:"hrTimeInZone_5"`
	PowerZone1       *float64 `json:"powerTimeInZone_1"`
	PowerZone2       *float64 `json:"powerTimeInZone_2"`
	PowerZone3       *float64 `json:"powerTimeInZone_3"`
	PowerZone4       *float64 `json:"powerTimeInZone_4"`
	PowerZone5       *float64 `json:"powerTimeInZone_5"`
	Fastest1000      *float64 `json:"fastestSplit_1000"`
	Fastest1609      *float64 `json:"fastestSplit_1609"`
	Fastest5000      *float64 `json:"fastestSplit_5000"`
	Fastest10000     *float64 `json:"fastestSplit_10000"`
	Fastest21098     *float64 `json:"fastestSplit_21098"`
	Fastest42195     *float64 `json:"fastestSplit_42195"`
}

// HRZones retourne [z1..z5] en secondes ou nil si absent.
func (a *ActivityListItem) HRZones() []float64 {
	return zones(a.HRZone1, a.HRZone2, a.HRZone3, a.HRZone4, a.HRZone5)
}

// PowerZones retourne [z1..z5] en secondes ou nil si absent.
func (a *ActivityListItem) PowerZones() []float64 {
	return zones(a.PowerZone1, a.PowerZone2, a.PowerZone3, a.PowerZone4, a.PowerZone5)
}

func zones(ps ...*float64) []float64 {
	out := make([]float64, len(ps))
	any := false
	for i, p := range ps {
		if p != nil {
			out[i] = *p
			any = true
		}
	}
	if !any {
		return nil
	}
	return out
}

// LocalDate retourne la date locale "YYYY-MM-DD" de l'activité.
func (a *ActivityListItem) LocalDate() string { return datePart(a.StartTimeLocal) }

// ActivityDetail : /activity-service/activity/{id}.
type ActivityDetail struct {
	ActivityID      int64          `json:"activityId"`
	ActivityName    *string        `json:"activityName"`
	ActivityTypeDTO TypeRef        `json:"activityTypeDTO"`
	LocationName    *string        `json:"locationName"`
	Summary         ActivitySumDTO `json:"summaryDTO"`
	SplitSummaries  []SplitSummary `json:"splitSummaries"`
}

// ActivitySumDTO : summaryDTO du détail d'activité.
type ActivitySumDTO struct {
	StartTimeLocal        string   `json:"startTimeLocal"` // "2026-11-02T18:30:12.0"
	StartTimeGMT          string   `json:"startTimeGMT"`
	Distance              *float64 `json:"distance"`
	Duration              *float64 `json:"duration"`
	MovingDuration        *float64 `json:"movingDuration"`
	ElapsedDuration       *float64 `json:"elapsedDuration"`
	ElevationGain         *float64 `json:"elevationGain"`
	ElevationLoss         *float64 `json:"elevationLoss"`
	AverageSpeed          *float64 `json:"averageSpeed"`
	AverageMovingSpeed    *float64 `json:"averageMovingSpeed"`
	MaxSpeed              *float64 `json:"maxSpeed"`
	AverageHR             *float64 `json:"averageHR"`
	MaxHR                 *float64 `json:"maxHR"`
	MinHR                 *float64 `json:"minHR"`
	AverageRunCadence     *float64 `json:"averageRunCadence"` // spm
	MaxRunCadence         *float64 `json:"maxRunCadence"`
	AveragePower          *float64 `json:"averagePower"`
	MaxPower              *float64 `json:"maxPower"`
	NormalizedPower       *float64 `json:"normalizedPower"`
	GroundContactTime     *float64 `json:"groundContactTime"`   // ms
	StrideLength          *float64 `json:"strideLength"`        // cm
	VerticalOscillation   *float64 `json:"verticalOscillation"` // cm
	VerticalRatio         *float64 `json:"verticalRatio"`       // %
	TrainingEffect        *float64 `json:"trainingEffect"`
	AnaerobicTE           *float64 `json:"anaerobicTrainingEffect"`
	TrainingEffectLabel   *string  `json:"trainingEffectLabel"`
	ActivityTrainingLoad  *float64 `json:"activityTrainingLoad"`
	ImpactLoad            *float64 `json:"impactLoad"`
	Steps                 *float64 `json:"steps"`
	Calories              *float64 `json:"calories"`
	AvgGradeAdjustedSpeed *float64 `json:"avgGradeAdjustedSpeed"`
	DifferenceBodyBattery *float64 `json:"differenceBodyBattery"`
	BeginPotentialStamina *float64 `json:"beginPotentialStamina"`
	EndPotentialStamina   *float64 `json:"endPotentialStamina"`
	ModerateIntensityMins *float64 `json:"moderateIntensityMinutes"`
	VigorousIntensityMins *float64 `json:"vigorousIntensityMinutes"`
	AerobicTEMessage      *string  `json:"aerobicTrainingEffectMessage"`
	AnaerobicTEMessage    *string  `json:"anaerobicTrainingEffectMessage"`
}

// SplitSummary : élément de splitSummaries.
type SplitSummary struct {
	SplitType         *string  `json:"splitType"`
	NoOfSplits        *float64 `json:"noOfSplits"`
	Distance          *float64 `json:"distance"`
	Duration          *float64 `json:"duration"`
	AverageSpeed      *float64 `json:"averageSpeed"`
	AverageHR         *float64 `json:"averageHR"`
	AverageRunCadence *float64 `json:"averageRunCadence"`
	AveragePower      *float64 `json:"averagePower"`
	ElevationGain     *float64 `json:"elevationGain"`
	ElevationLoss     *float64 `json:"elevationLoss"`
}

// LocalDate retourne la date locale "YYYY-MM-DD".
func (d *ActivityDetail) LocalDate() string { return datePart(d.Summary.StartTimeLocal) }

// LocalStart retourne "YYYY-MM-DDTHH:MM:SS" (heure locale, sans offset).
func (d *ActivityDetail) LocalStart() string { return NormalizeLocal(d.Summary.StartTimeLocal) }

// StartGMT parse startTimeGMT.
func (d *ActivityDetail) StartGMT() (time.Time, bool) { return parseGarminTime(d.Summary.StartTimeGMT) }

// NormalizeLocal convertit "2026-11-02 18:30:12" ou "2026-11-02T18:30:12.0" en "2026-11-02T18:30:12".
func NormalizeLocal(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Replace(s, " ", "T", 1)
	if i := strings.IndexByte(s, '.'); i > 0 {
		s = s[:i]
	}
	if len(s) > 19 {
		s = s[:19]
	}
	return s
}

func datePart(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func parseGarminTime(s string) (time.Time, bool) {
	s = NormalizeLocal(s)
	t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ActivityDetails : /activity-service/activity/{id}/details (time-series).
type ActivityDetails struct {
	MetricDescriptors []MetricDescriptor `json:"metricDescriptors"`
	Metrics           []struct {
		Metrics []*float64 `json:"metrics"`
	} `json:"activityDetailMetrics"`
}

// MetricDescriptor décrit une colonne des time-series.
type MetricDescriptor struct {
	MetricsIndex int    `json:"metricsIndex"`
	Key          string `json:"key"`
	Unit         struct {
		Key    string  `json:"key"`
		Factor float64 `json:"factor"`
	} `json:"unit"`
}

// Column retourne l'index de la métrique key (ou -1).
func (d *ActivityDetails) Column(key string) int {
	for _, m := range d.MetricDescriptors {
		if m.Key == key {
			return m.MetricsIndex
		}
	}
	return -1
}

// ScoreValue : {value, qualifierKey}.
type ScoreValue struct {
	Value        *float64 `json:"value"`
	QualifierKey *string  `json:"qualifierKey"`
}

// DailySleep : /sleep-service/sleep/dailySleep (objet à plat).
type DailySleep struct {
	CalendarDate             string   `json:"calendarDate"`
	SleepTimeSeconds         *float64 `json:"sleepTimeSeconds"`
	NapTimeSeconds           *float64 `json:"napTimeSeconds"`
	DeepSleepSeconds         *float64 `json:"deepSleepSeconds"`
	LightSleepSeconds        *float64 `json:"lightSleepSeconds"`
	RemSleepSeconds          *float64 `json:"remSleepSeconds"`
	AwakeSleepSeconds        *float64 `json:"awakeSleepSeconds"`
	AwakeCount               *float64 `json:"awakeCount"`
	AvgHeartRate             *float64 `json:"avgHeartRate"`
	AvgSleepStress           *float64 `json:"avgSleepStress"`
	AverageRespirationValue  *float64 `json:"averageRespirationValue"`
	SleepStartTimestampGMT   *float64 `json:"sleepStartTimestampGMT"`
	SleepEndTimestampGMT     *float64 `json:"sleepEndTimestampGMT"`
	SleepStartTimestampLocal *float64 `json:"sleepStartTimestampLocal"`
	SleepEndTimestampLocal   *float64 `json:"sleepEndTimestampLocal"`
	SleepScores              *struct {
		Overall        *ScoreValue `json:"overall"`
		DeepPercentage *ScoreValue `json:"deepPercentage"`
		RemPercentage  *ScoreValue `json:"remPercentage"`
	} `json:"sleepScores"`
	SleepScoreFeedback *string `json:"sleepScoreFeedback"`
	SleepNeed          *struct {
		Actual   *float64 `json:"actual"`
		Baseline *float64 `json:"baseline"`
	} `json:"sleepNeed"`
}

// HasData indique si la nuit contient une durée de sommeil.
func (s *DailySleep) HasData() bool {
	return s != nil && s.SleepTimeSeconds != nil && *s.SleepTimeSeconds > 0
}

// Score retourne le score global (ou nil).
func (s *DailySleep) Score() (*float64, *string) {
	if s == nil || s.SleepScores == nil || s.SleepScores.Overall == nil {
		return nil, nil
	}
	return s.SleepScores.Overall.Value, s.SleepScores.Overall.QualifierKey
}

// HRVSummary : hrvSummary.
type HRVSummary struct {
	CalendarDate      string   `json:"calendarDate"`
	WeeklyAvg         *float64 `json:"weeklyAvg"`
	LastNightAvg      *float64 `json:"lastNightAvg"`
	LastNight5MinHigh *float64 `json:"lastNight5MinHigh"`
	Baseline          *struct {
		LowUpper      *float64 `json:"lowUpper"`
		BalancedLow   *float64 `json:"balancedLow"`
		BalancedUpper *float64 `json:"balancedUpper"`
		MarkerValue   *float64 `json:"markerValue"`
	} `json:"baseline"`
	Status         *string `json:"status"`
	FeedbackPhrase *string `json:"feedbackPhrase"`
}

// HRVDay : /hrv-service/hrv/{date}.
type HRVDay struct {
	HrvSummary *HRVSummary `json:"hrvSummary"`
}

// HRVRange : /hrv-service/hrv/daily/{start}/{end}.
type HRVRange struct {
	HrvSummaries []HRVSummary `json:"hrvSummaries"`
}

// BodyBatteryDay : élément de /wellness-service/wellness/bodyBattery/reports/daily.
type BodyBatteryDay struct {
	Date        string            `json:"date"`
	Charged     *float64          `json:"charged"`
	Drained     *float64          `json:"drained"`
	Values      [][]any           `json:"bodyBatteryValuesArray"`
	Descriptors []BBDescriptor    `json:"bodyBatteryValueDescriptorDTOList"`
	Events      []BodyBatteryEvnt `json:"bodyBatteryActivityEvent"`
}

// BBDescriptor décrit une colonne de bodyBatteryValuesArray.
type BBDescriptor struct {
	Index int    `json:"bodyBatteryValueDescriptorIndex"`
	Key   string `json:"bodyBatteryValueDescriptorKey"`
}

// BodyBatteryEvnt : événement (SLEEP, ACTIVITY, STRESS, RECOVERY…).
type BodyBatteryEvnt struct {
	EventType         string   `json:"eventType"`
	EventStartTimeGmt string   `json:"eventStartTimeGmt"`
	DurationMs        *float64 `json:"durationInMilliseconds"`
	Impact            *float64 `json:"bodyBatteryImpact"`
}

// BBPoint : point (horodatage ms, niveau).
type BBPoint struct {
	TsMs  float64
	Level float64
}

// Points extrait les points (timestamp, niveau) en s'appuyant sur les descripteurs
// (sonde 2026-11-03 : [timestamp, bodyBatteryLevel]).
func (d *BodyBatteryDay) Points() []BBPoint {
	tsIdx, lvlIdx := 0, 1
	for _, desc := range d.Descriptors {
		switch strings.ToLower(desc.Key) {
		case "timestamp":
			tsIdx = desc.Index
		case "bodybatterylevel":
			lvlIdx = desc.Index
		}
	}
	var out []BBPoint
	for _, row := range d.Values {
		if tsIdx >= len(row) || lvlIdx >= len(row) {
			continue
		}
		ts, ok1 := row[tsIdx].(float64)
		lv, ok2 := row[lvlIdx].(float64)
		if ok1 && ok2 {
			out = append(out, BBPoint{TsMs: ts, Level: lv})
		}
	}
	return out
}

// GoalValue : {goal, value, distanceInMeters} — accepte aussi un nombre nu.
type GoalValue struct {
	Goal             *float64 `json:"goal"`
	Value            *float64 `json:"value"`
	DistanceInMeters *float64 `json:"distanceInMeters"`
}

// UnmarshalJSON accepte un objet ou un nombre.
func (g *GoalValue) UnmarshalJSON(b []byte) error {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || string(t) == "null" {
		return nil
	}
	if t[0] != '{' {
		var f float64
		if err := json.Unmarshal(t, &f); err != nil {
			return nil // tolérant
		}
		g.Value = &f
		return nil
	}
	type plain GoalValue
	var p plain
	if err := json.Unmarshal(t, &p); err != nil {
		return err
	}
	*g = GoalValue(p)
	return nil
}

// DailySummary : élément de /usersummary-service/v2/dailysummaries (data[]).
type DailySummary struct {
	CalendarDate string `json:"calendarDate"`
	Movement     *struct {
		Steps           *GoalValue `json:"steps"`
		FloorsAscended  *GoalValue `json:"floorsAscended"`
		FloorsDescended *GoalValue `json:"floorsDescended"`
	} `json:"movement"`
	HeartRate *struct {
		MinValue     *float64 `json:"minValue"`
		MaxValue     *float64 `json:"maxValue"`
		RestingValue *float64 `json:"restingValue"`
	} `json:"heartRate"`
	IntensityMinutes *struct {
		Goal     *float64 `json:"goal"`
		Moderate *float64 `json:"moderate"`
		Vigorous *float64 `json:"vigorous"`
	} `json:"intensityMinutes"`
	Stress *struct {
		AvgLevel  *float64 `json:"avgLevel"`
		MaxLevel  *float64 `json:"maxLevel"`
		Qualifier *string  `json:"qualifier"`
	} `json:"stress"`
	BodyBattery *struct {
		MinValue     *float64 `json:"minValue"`
		MaxValue     *float64 `json:"maxValue"`
		ChargedValue *float64 `json:"chargedValue"`
		DrainedValue *float64 `json:"drainedValue"`
		LatestValue  *float64 `json:"latestValue"`
	} `json:"bodyBattery"`
	Calories *struct {
		BurnedResting *float64 `json:"burnedResting"`
		BurnedActive  *float64 `json:"burnedActive"`
		BurnedTotal   *float64 `json:"burnedTotal"`
	} `json:"calories"`
	Respiration *struct {
		AvgValue *float64 `json:"avgValue"`
	} `json:"respiration"`
}

// Steps / FloorsUp / FloorsDown : accès tolérants.
func (d *DailySummary) StepsValue() (*float64, *float64) {
	if d == nil || d.Movement == nil || d.Movement.Steps == nil {
		return nil, nil
	}
	return d.Movement.Steps.Value, d.Movement.Steps.Goal
}

// FloorsUp retourne les étages montés.
func (d *DailySummary) FloorsUp() *float64 {
	if d == nil || d.Movement == nil || d.Movement.FloorsAscended == nil {
		return nil
	}
	return d.Movement.FloorsAscended.Value
}

// FloorsDown retourne les étages descendus.
func (d *DailySummary) FloorsDown() *float64 {
	if d == nil || d.Movement == nil || d.Movement.FloorsDescended == nil {
		return nil
	}
	return d.Movement.FloorsDescended.Value
}

// DailySummaries : enveloppe {data:[…]}.
type DailySummaries struct {
	Data []DailySummary `json:"data"`
}

// StepsDay : élément de /usersummary-service/stats/steps/daily.
type StepsDay struct {
	CalendarDate  string   `json:"calendarDate"`
	TotalSteps    *float64 `json:"totalSteps"`
	TotalDistance *float64 `json:"totalDistance"`
	StepGoal      *float64 `json:"stepGoal"`
}

// DailyHeartRate : /wellness-service/wellness/dailyHeartRate.
type DailyHeartRate struct {
	CalendarDate        string   `json:"calendarDate"`
	RestingHeartRate    *float64 `json:"restingHeartRate"`
	LastSevenDaysAvgRHR *float64 `json:"lastSevenDaysAvgRestingHeartRate"`
	MinHeartRate        *float64 `json:"minHeartRate"`
	MaxHeartRate        *float64 `json:"maxHeartRate"`
}

// HRZoneConfig : élément de /biometric-service/heartRateZones/v2.
type HRZoneConfig struct {
	Sport                *string  `json:"sport"`
	TrainingMethod       *string  `json:"trainingMethod"`
	Zone1Floor           *float64 `json:"zone1Floor"`
	Zone2Floor           *float64 `json:"zone2Floor"`
	Zone3Floor           *float64 `json:"zone3Floor"`
	Zone4Floor           *float64 `json:"zone4Floor"`
	Zone5Floor           *float64 `json:"zone5Floor"`
	MaxHeartRateUsed     *float64 `json:"maxHeartRateUsed"`
	LactateThresholdUsed *float64 `json:"lactateThresholdHeartRateUsed"`
	RestingHRUsed        *float64 `json:"restingHeartRateUsed"`
}

// Floors retourne [z1..z5] planchers + max, ou nil.
func (z *HRZoneConfig) Floors() []float64 {
	ps := []*float64{z.Zone1Floor, z.Zone2Floor, z.Zone3Floor, z.Zone4Floor, z.Zone5Floor}
	out := make([]float64, 0, 5)
	for _, p := range ps {
		if p == nil {
			return nil
		}
		out = append(out, *p)
	}
	return out
}
