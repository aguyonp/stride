// Package analysis calcule les métriques de séance (biomécanique, segments, dérive,
// zones), le contexte de récupération, la performance et les alertes, selon
// docs/API_CONTRACT.md. Les structures ci-dessous sont le contrat JSON (camelCase).
package analysis

// Mode d'analyse.
type Mode string

// Modes supportés.
const (
	ModeSFP         Mode = "sfp"
	ModeRecovery    Mode = "recovery"
	ModePerformance Mode = "performance"
)

// ParseMode valide un mode ("" → sfp).
func ParseMode(s string) (Mode, bool) {
	switch Mode(s) {
	case "", ModeSFP:
		return ModeSFP, true
	case ModeRecovery:
		return ModeRecovery, true
	case ModePerformance:
		return ModePerformance, true
	}
	return "", false
}

// ActivityListResponse : GET /api/activities.
type ActivityListResponse struct {
	Items   []ActivitySummary `json:"items"`
	HasMore bool              `json:"hasMore"`
}

// ActivitySummary : résumé d'activité.
type ActivitySummary struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	TypeLabel       string   `json:"typeLabel"`
	IsRunning       bool     `json:"isRunning"`
	StartLocal      string   `json:"startLocal"`
	Date            string   `json:"date"`
	DistanceM       *float64 `json:"distanceM"`
	DurationS       *float64 `json:"durationS"`
	AvgPaceSecPerKm *float64 `json:"avgPaceSecPerKm"`
	AvgHR           *float64 `json:"avgHR"`
	AvgPower        *float64 `json:"avgPower"`
	AvgCadenceSpm   *float64 `json:"avgCadenceSpm"`
	ElevGainM       *float64 `json:"elevGainM"`
	ElevLossM       *float64 `json:"elevLossM"`
	TrainingLoad    *float64 `json:"trainingLoad"`
}

// ActivityDetail : ActivitySummary + champs détaillés.
type ActivityDetail struct {
	ActivitySummary
	LocationName        *string  `json:"locationName"`
	MovingDurationS     *float64 `json:"movingDurationS"`
	MaxHR               *float64 `json:"maxHR"`
	NormPower           *float64 `json:"normPower"`
	MaxCadenceSpm       *float64 `json:"maxCadenceSpm"`
	AerobicTE           *float64 `json:"aerobicTE"`
	AnaerobicTE         *float64 `json:"anaerobicTE"`
	TrainingEffectLabel *string  `json:"trainingEffectLabel"`
	ImpactLoad          *float64 `json:"impactLoad"`
	Steps               *float64 `json:"steps"`
	Calories            *float64 `json:"calories"`
	BodyBatteryDelta    *float64 `json:"bodyBatteryDelta"`
}

// GctBalance : équilibre du temps de contact au sol.
type GctBalance struct {
	Available  bool     `json:"available"`
	LeftPct    *float64 `json:"leftPct"`
	RightPct   *float64 `json:"rightPct"`
	GapPct     *float64 `json:"gapPct"`
	WeakerSide *string  `json:"weakerSide"`
	Alert      *string  `json:"alert"`
	Reason     *string  `json:"reason"`
}

// Biomechanics : dynamique de course (course uniquement, hors marche/arrêt).
type Biomechanics struct {
	GctBalance            GctBalance `json:"gctBalance"`
	GroundContactTimeMs   *float64   `json:"groundContactTimeMs"`
	CadenceAvgSpm         *float64   `json:"cadenceAvgSpm"`
	CadenceMaxSpm         *float64   `json:"cadenceMaxSpm"`
	CadenceInTargetPct    *float64   `json:"cadenceInTargetPct"`
	CadenceBelow170Pct    *float64   `json:"cadenceBelow170Pct"`
	VerticalOscillationCm *float64   `json:"verticalOscillationCm"`
	VerticalRatioPct      *float64   `json:"verticalRatioPct"`
	StrideLengthM         *float64   `json:"strideLengthM"`
}

// Drift : dérive cardiaque.
type Drift struct {
	Method             string  `json:"method"` // "pw:hr" | "pace:hr"
	DecouplingPct      float64 `json:"decouplingPct"`
	FirstHalfHR        float64 `json:"firstHalfHR"`
	SecondHalfHR       float64 `json:"secondHalfHR"`
	LastThirdHrRiseBpm float64 `json:"lastThirdHrRiseBpm"`
	Verdict            string  `json:"verdict"` // stable | moderate | high
}

// Load : charge et fatigue.
type Load struct {
	ElevGainM    *float64  `json:"elevGainM"`
	ElevLossM    *float64  `json:"elevLossM"`
	Drift        *Drift    `json:"drift"`
	TrainingLoad *float64  `json:"trainingLoad"`
	AerobicTE    *float64  `json:"aerobicTE"`
	AnaerobicTE  *float64  `json:"anaerobicTE"`
	HrZonesS     []float64 `json:"hrZonesS"`
	PowerZonesS  []float64 `json:"powerZonesS"`
}

// Segment : bloc course/marche ou lap.
type Segment struct {
	Index             int      `json:"index"`
	Kind              string   `json:"kind"`
	Label             string   `json:"label"`
	StartOffsetS      float64  `json:"startOffsetS"`
	DurationS         float64  `json:"durationS"`
	DistanceM         float64  `json:"distanceM"`
	PaceSecPerKm      *float64 `json:"paceSecPerKm"`
	AvgHR             *float64 `json:"avgHR"`
	MaxHR             *float64 `json:"maxHR"`
	AvgPower          *float64 `json:"avgPower"`
	AvgCadenceSpm     *float64 `json:"avgCadenceSpm"`
	GctMs             *float64 `json:"gctMs"`
	VoCm              *float64 `json:"voCm"`
	GctBalanceLeftPct *float64 `json:"gctBalanceLeftPct"`
	ElevGainM         *float64 `json:"elevGainM"`
	ElevLossM         *float64 `json:"elevLossM"`
}

// Segments : blocs et laps.
type Segments struct {
	Primary string    `json:"primary"` // "blocks" | "laps"
	Blocks  []Segment `json:"blocks"`
	Laps    []Segment `json:"laps"`
}

// Series : séries temporelles downsamplées (≤ 400 points, même longueur).
type Series struct {
	TS                []float64  `json:"tS"`
	HR                []*float64 `json:"hr"`
	CadenceSpm        []*float64 `json:"cadenceSpm"`
	PowerW            []*float64 `json:"powerW"`
	PaceSecPerKm      []*float64 `json:"paceSecPerKm"`
	ElevationM        []*float64 `json:"elevationM"`
	GctMs             []*float64 `json:"gctMs"`
	VoCm              []*float64 `json:"voCm"`
	GctBalanceLeftPct []*float64 `json:"gctBalanceLeftPct"`
}

// HRVContext : VFC de la nuit.
type HRVContext struct {
	LastNightAvgMs      *float64 `json:"lastNightAvgMs"`
	LastNight5MinHighMs *float64 `json:"lastNight5MinHighMs"`
	WeeklyAvgMs         *float64 `json:"weeklyAvgMs"`
	BaselineLowMs       *float64 `json:"baselineLowMs"`
	BaselineHighMs      *float64 `json:"baselineHighMs"`
	Status              *string  `json:"status"`
	StatusLabel         *string  `json:"statusLabel"`
	DeltaVsWeeklyPct    *float64 `json:"deltaVsWeeklyPct"`
}

// SleepContext : sommeil de la nuit.
type SleepContext struct {
	Score        *float64 `json:"score"`
	Qualifier    *string  `json:"qualifier"`
	DurationS    *float64 `json:"durationS"`
	DeepS        *float64 `json:"deepS"`
	RemS         *float64 `json:"remS"`
	LightS       *float64 `json:"lightS"`
	AwakeS       *float64 `json:"awakeS"`
	AvgSleepHR   *float64 `json:"avgSleepHR"`
	BedtimeLocal *string  `json:"bedtimeLocal"`
	WakeLocal    *string  `json:"wakeLocal"`
}

// RestingHR : FC de repos.
type RestingHR struct {
	Bpm   *float64 `json:"bpm"`
	Avg7d *float64 `json:"avg7d"`
	Delta *float64 `json:"delta"`
}

// BodyBattery : Body Battery du jour.
type BodyBattery struct {
	AtWake  *float64 `json:"atWake"`
	Max     *float64 `json:"max"`
	Min     *float64 `json:"min"`
	Charged *float64 `json:"charged"`
	Drained *float64 `json:"drained"`
}

// RecoveryContext : récupération de la nuit se terminant à `date`.
type RecoveryContext struct {
	Date        string        `json:"date"`
	HRV         *HRVContext   `json:"hrv"`
	Sleep       *SleepContext `json:"sleep"`
	RestingHR   *RestingHR    `json:"restingHR"`
	BodyBattery *BodyBattery  `json:"bodyBattery"`
	StressAvg   *float64      `json:"stressAvg"`
	Trend       []TrendDay    `json:"trend"`
}

// TrendDay : une ligne de la tendance 7 jours.
type TrendDay struct {
	Date           string   `json:"date"`
	HrvMs          *float64 `json:"hrvMs"`
	HrvStatus      *string  `json:"hrvStatus"`
	SleepScore     *float64 `json:"sleepScore"`
	SleepS         *float64 `json:"sleepS"`
	DeepS          *float64 `json:"deepS"`
	RestingHR      *float64 `json:"restingHR"`
	Steps          *float64 `json:"steps"`
	FloorsUp       *float64 `json:"floorsUp"`
	BodyBatteryMax *float64 `json:"bodyBatteryMax"`
	TrainingLoad   *float64 `json:"trainingLoad"`
}

// TargetPace : temps passé dans une plage d'allure cible.
type TargetPace struct {
	Label      string  `json:"label"`
	RangeLabel string  `json:"rangeLabel"`
	Seconds    float64 `json:"seconds"`
	DistanceM  float64 `json:"distanceM"`
}

// ACWR : ratio charge aiguë / chronique.
type ACWR struct {
	Acute7d    float64  `json:"acute7d"`
	Chronic28d float64  `json:"chronic28d"`
	Ratio      *float64 `json:"ratio"`
	Verdict    string   `json:"verdict"` // low | optimal | high | danger
}

// BestEffort : meilleur temps sur une distance.
type BestEffort struct {
	Label   string  `json:"label"`
	Seconds float64 `json:"seconds"`
}

// Performance : calculé seulement en mode performance.
type Performance struct {
	AvgPower     *float64     `json:"avgPower"`
	NormPower    *float64     `json:"normPower"`
	TimeInZonesS []float64    `json:"timeInZonesS"`
	Z4Z5S        *float64     `json:"z4z5S"`
	TargetPaces  []TargetPace `json:"targetPaces"`
	ACWR         *ACWR        `json:"acwr"`
	BestEfforts  []BestEffort `json:"bestEfforts"`
	// Extension : planchers des zones FC Garmin (Z1..Z5), si disponibles.
	HrZoneFloorsBpm []float64 `json:"hrZoneFloorsBpm"`
}

// HistoryRow : ligne d'historique compact.
type HistoryRow struct {
	Date          string   `json:"date"`
	Type          string   `json:"type"`
	TypeLabel     string   `json:"typeLabel"` // extension
	DistanceM     *float64 `json:"distanceM"`
	DurationS     *float64 `json:"durationS"`
	PaceSecPerKm  *float64 `json:"paceSecPerKm"`
	AvgHR         *float64 `json:"avgHR"`
	AvgCadenceSpm *float64 `json:"avgCadenceSpm"`
	ElevLossM     *float64 `json:"elevLossM"`
	TrainingLoad  *float64 `json:"trainingLoad"`
}

// Alert : alerte affichée et reprise dans le rapport.
type Alert struct {
	Level   string `json:"level"` // info | warn | danger | ok
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// ActivityAnalysis : GET /api/activities/{id}/analysis.
type ActivityAnalysis struct {
	Mode                 Mode             `json:"mode"`
	GeneratedAt          string           `json:"generatedAt"`
	Activity             ActivityDetail   `json:"activity"`
	Biomechanics         Biomechanics     `json:"biomechanics"`
	Load                 Load             `json:"load"`
	Segments             Segments         `json:"segments"`
	Series               Series           `json:"series"`
	Recovery             *RecoveryContext `json:"recovery"`
	Performance          *Performance     `json:"performance"`
	History              []HistoryRow     `json:"history"`
	Alerts               []Alert          `json:"alerts"`
	Report               string           `json:"report"`
	ReportTokensEstimate int              `json:"reportTokensEstimate"`
	DataSources          []string         `json:"dataSources"`
	Warnings             []string         `json:"warnings"`
	// HistoryDays : nb de jours d'historique demandés (non sérialisé).
	HistoryDays int `json:"-"`
}

// DayInfo : activité de la journée (Mode 2).
type DayInfo struct {
	Steps                *float64 `json:"steps"`
	StepGoal             *float64 `json:"stepGoal"`
	FloorsUp             *float64 `json:"floorsUp"`
	FloorsDown           *float64 `json:"floorsDown"`
	IntensityModerateMin *float64 `json:"intensityModerateMin"`
	IntensityVigorousMin *float64 `json:"intensityVigorousMin"`
	ActiveCalories       *float64 `json:"activeCalories"`
	StressAvg            *float64 `json:"stressAvg"`
	BodyBatteryCharged   *float64 `json:"bodyBatteryCharged"`
	BodyBatteryDrained   *float64 `json:"bodyBatteryDrained"`
}

// DayAnalysis : GET /api/days/{date}/analysis.
type DayAnalysis struct {
	Mode                 Mode              `json:"mode"`
	Date                 string            `json:"date"`
	GeneratedAt          string            `json:"generatedAt"`
	Recovery             RecoveryContext   `json:"recovery"`
	Day                  DayInfo           `json:"day"`
	Activities           []ActivitySummary `json:"activities"`
	History              []HistoryRow      `json:"history"`
	Alerts               []Alert           `json:"alerts"`
	Report               string            `json:"report"`
	ReportTokensEstimate int               `json:"reportTokensEstimate"`
	Warnings             []string          `json:"warnings"`
	// DataSources : extension (sources utilisées), comme pour l'analyse d'activité.
	DataSources []string `json:"dataSources"`
	HistoryDays int      `json:"-"`
}
