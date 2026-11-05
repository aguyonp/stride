// Package analysis computes session insights (biomechanics, fatigue drift, slope, GAP, segments,
// cardiac drift, zones), recovery context, load and alerts. It is pure: inputs are decoded provider
// payloads, outputs are the JSON types of docs/V2_SPEC.md §3.2 (V1 §3 types extended).
package analysis

type GctBalance struct {
	Available  bool     `json:"available"`
	LeftPct    *float64 `json:"leftPct"`
	RightPct   *float64 `json:"rightPct"`
	GapPct     *float64 `json:"gapPct"`
	WeakerSide *string  `json:"weakerSide"`
	Alert      *string  `json:"alert"`
	Reason     *string  `json:"reason"`
}

type FatigueDrift struct {
	GctStartMs      *float64 `json:"gctStartMs"`
	GctEndMs        *float64 `json:"gctEndMs"`
	GctDeltaMs      *float64 `json:"gctDeltaMs"`
	GctDeltaPct     *float64 `json:"gctDeltaPct"`
	VoStartCm       *float64 `json:"voStartCm"`
	VoEndCm         *float64 `json:"voEndCm"`
	VoDeltaCm       *float64 `json:"voDeltaCm"`
	CadenceStartSpm *float64 `json:"cadenceStartSpm"`
	CadenceEndSpm   *float64 `json:"cadenceEndSpm"`
	CadenceDeltaSpm *float64 `json:"cadenceDeltaSpm"`
	Verdict         string   `json:"verdict"` // stable | moderate | high
}

type SlopeBucket struct {
	CadenceSpm   *float64 `json:"cadenceSpm"`
	StrideM      *float64 `json:"strideM"`
	StrideMaxM   *float64 `json:"strideMaxM"`
	PaceSecPerKm *float64 `json:"paceSecPerKm"`
	DurationS    float64  `json:"durationS"`
	DistanceM    float64  `json:"distanceM"`
}

type Slope struct {
	ThresholdPct           float64      `json:"thresholdPct"`
	Flat                   *SlopeBucket `json:"flat"`
	Downhill               *SlopeBucket `json:"downhill"`
	Uphill                 *SlopeBucket `json:"uphill"`
	DownhillCadenceDropSpm *float64     `json:"downhillCadenceDropSpm"`
}

type Biomechanics struct {
	GctBalance            GctBalance    `json:"gctBalance"`
	GroundContactTimeMs   *float64      `json:"groundContactTimeMs"`
	CadenceAvgSpm         *float64      `json:"cadenceAvgSpm"`
	CadenceMaxSpm         *float64      `json:"cadenceMaxSpm"`
	CadenceInTargetPct    *float64      `json:"cadenceInTargetPct"`
	CadenceBelow170Pct    *float64      `json:"cadenceBelow170Pct"`
	VerticalOscillationCm *float64      `json:"verticalOscillationCm"`
	VerticalRatioPct      *float64      `json:"verticalRatioPct"`
	StrideLengthM         *float64      `json:"strideLengthM"`
	FatigueDrift          *FatigueDrift `json:"fatigueDrift"`
	Slope                 *Slope        `json:"slope"`
}

type Drift struct {
	Method             string  `json:"method"` // pw:hr | pace:hr
	DecouplingPct      float64 `json:"decouplingPct"`
	FirstHalfHR        float64 `json:"firstHalfHR"`
	SecondHalfHR       float64 `json:"secondHalfHR"`
	LastThirdHrRiseBpm float64 `json:"lastThirdHrRiseBpm"`
	Verdict            string  `json:"verdict"` // stable | moderate | high
}

type Stamina struct {
	StartPct *float64 `json:"startPct"`
	EndPct   *float64 `json:"endPct"`
	MinPct   *float64 `json:"minPct"`
}

type LoadRatio struct {
	Acute   float64  `json:"acute"`
	Chronic float64  `json:"chronic"`
	Ratio   *float64 `json:"ratio"`
	Verdict string   `json:"verdict"` // low | optimal | high | danger
	Source  string   `json:"source"`  // computed
}

type Load struct {
	ElevGainM            *float64   `json:"elevGainM"`
	ElevLossM            *float64   `json:"elevLossM"`
	Drift                *Drift     `json:"drift"`
	TrainingLoad         *float64   `json:"trainingLoad"`
	AerobicTE            *float64   `json:"aerobicTE"`
	AnaerobicTE          *float64   `json:"anaerobicTE"`
	HrZonesS             []float64  `json:"hrZonesS"`
	PowerZonesS          []float64  `json:"powerZonesS"`
	ImpactLoad           *float64   `json:"impactLoad"`
	Stamina              *Stamina   `json:"stamina"`
	LoadRatio            *LoadRatio `json:"loadRatio"`
	StravaRelativeEffort *float64   `json:"stravaRelativeEffort"`
}

type Segment struct {
	Index             int      `json:"index"`
	Kind              string   `json:"kind"`
	Label             string   `json:"label"`
	StartOffsetS      float64  `json:"startOffsetS"`
	DurationS         float64  `json:"durationS"`
	DistanceM         float64  `json:"distanceM"`
	PaceSecPerKm      *float64 `json:"paceSecPerKm"`
	GapSecPerKm       *float64 `json:"gapSecPerKm"`
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

type Segments struct {
	Primary string    `json:"primary"` // blocks | laps
	Blocks  []Segment `json:"blocks"`
	Laps    []Segment `json:"laps"`
}

func (sg Segments) PrimaryList() []Segment {
	if sg.Primary == "blocks" {
		return sg.Blocks
	}
	return sg.Laps
}

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
	GapSecPerKm       []*float64 `json:"gapSecPerKm"`
	StaminaPct        []*float64 `json:"staminaPct"`
}

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

type RestingHR struct {
	Bpm   *float64 `json:"bpm"`
	Avg7d *float64 `json:"avg7d"`
	Delta *float64 `json:"delta"`
}

type BodyBattery struct {
	AtWake  *float64 `json:"atWake"`
	Max     *float64 `json:"max"`
	Min     *float64 `json:"min"`
	Charged *float64 `json:"charged"`
	Drained *float64 `json:"drained"`
}

type Readiness struct {
	Score  *float64 `json:"score"`
	Status *string  `json:"status"`
	Source *string  `json:"source"` // manual | null
	Reason *string  `json:"reason"`
}

type RecoveryContext struct {
	Date        string        `json:"date"`
	HRV         *HRVContext   `json:"hrv"`
	Sleep       *SleepContext `json:"sleep"`
	RestingHR   *RestingHR    `json:"restingHR"`
	BodyBattery *BodyBattery  `json:"bodyBattery"`
	StressAvg   *float64      `json:"stressAvg"`
	Trend       []TrendDay    `json:"trend"`
	Readiness   Readiness     `json:"readiness"`
}

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
	// PainNextMorning is filled by the domain for the day report only.
	PainNextMorning *float64 `json:"-"`
}

type TargetPace struct {
	Label      string  `json:"label"`
	RangeLabel string  `json:"rangeLabel"`
	Seconds    float64 `json:"seconds"`
	DistanceM  float64 `json:"distanceM"`
}

type ACWR struct {
	Acute7d    float64  `json:"acute7d"`
	Chronic28d float64  `json:"chronic28d"`
	Ratio      *float64 `json:"ratio"`
	Verdict    string   `json:"verdict"`
}

type BestEffort struct {
	Label   string  `json:"label"`
	Seconds float64 `json:"seconds"`
}

type Performance struct {
	AvgPower        *float64     `json:"avgPower"`
	NormPower       *float64     `json:"normPower"`
	TimeInZonesS    []float64    `json:"timeInZonesS"`
	Z4Z5S           *float64     `json:"z4z5S"`
	TargetPaces     []TargetPace `json:"targetPaces"`
	ACWR            *ACWR        `json:"acwr"`
	BestEfforts     []BestEffort `json:"bestEfforts"`
	HrZoneFloorsBpm []float64    `json:"hrZoneFloorsBpm"`
}

type Alert struct {
	Level   string `json:"level"` // info | warn | danger | ok
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

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
