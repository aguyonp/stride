// Package view holds the JSON view models of the domain API (docs/SPEC.md §3.2), shared by the
// domain services and the report templates.
package view

import (
	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/provider/weather"
)

const (
	SourceDB    = "db"
	SourceCache = "cache"
	SourceLive  = "live"
)

type Freshness struct {
	Source    string `json:"source"`
	UpdatedAt string `json:"updatedAt"`
}

type ActivitySummary struct {
	ID              int64    `json:"id"`
	GarminID        *int64   `json:"garminId"`
	StravaID        *int64   `json:"stravaId"`
	Sources         []string `json:"sources"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	TypeLabel       string   `json:"typeLabel"`
	Sport           string   `json:"sport"`
	IsRunning       bool     `json:"isRunning"`
	StartLocal      string   `json:"startLocal"`
	Date            string   `json:"date"`
	DistanceM       *float64 `json:"distanceM"`
	DurationS       *float64 `json:"durationS"`
	AvgPaceSecPerKm *float64 `json:"avgPaceSecPerKm"`
	GapSecPerKm     *float64 `json:"gapSecPerKm"`
	AvgHR           *float64 `json:"avgHR"`
	AvgPower        *float64 `json:"avgPower"`
	AvgCadenceSpm   *float64 `json:"avgCadenceSpm"`
	ElevGainM       *float64 `json:"elevGainM"`
	ElevLossM       *float64 `json:"elevLossM"`
	TrainingLoad    *float64 `json:"trainingLoad"`
	PainDuring      *float64 `json:"painDuring"`
	HasNotes        bool     `json:"hasNotes"`
	PlanStatus      *string  `json:"planStatus"`
	// Kudos is the Strava kudos_count (null without Strava: Garmin likes are not exposed).
	Kudos *int `json:"kudos"`
}

type ActivityList struct {
	Items     []ActivitySummary `json:"items"`
	HasMore   bool              `json:"hasMore"`
	Freshness Freshness         `json:"freshness"`
}

type StravaInfo struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	PrivateNote    string   `json:"privateNote"`
	RPE            *float64 `json:"rpe"`
	RelativeEffort *float64 `json:"relativeEffort"`
	GearName       *string  `json:"gearName"`
}

type ActivityDetail struct {
	ActivitySummary
	LocationName        *string           `json:"locationName"`
	MovingDurationS     *float64          `json:"movingDurationS"`
	MaxHR               *float64          `json:"maxHR"`
	NormPower           *float64          `json:"normPower"`
	MaxCadenceSpm       *float64          `json:"maxCadenceSpm"`
	AerobicTE           *float64          `json:"aerobicTE"`
	AnaerobicTE         *float64          `json:"anaerobicTE"`
	TrainingEffectLabel *string           `json:"trainingEffectLabel"`
	ImpactLoad          *float64          `json:"impactLoad"`
	Steps               *float64          `json:"steps"`
	Calories            *float64          `json:"calories"`
	BodyBatteryDelta    *float64          `json:"bodyBatteryDelta"`
	Stamina             *analysis.Stamina `json:"stamina"`
	Strava              *StravaInfo       `json:"strava"`
	// GarminGear is always null: the Personal API does not expose gear (404), see GarminGearReason.
	GarminGear       *string `json:"garminGear"`
	GarminGearReason string  `json:"garminGearReason"`
}

type Weather struct {
	weather.Conditions
	WindDirLabel *string `json:"windDirLabel"`
}

type BikeLoad struct {
	Rides        int      `json:"rides"`
	DistanceKm   float64  `json:"distanceKm"`
	DurationS    float64  `json:"durationS"`
	AvgHR        *float64 `json:"avgHR"`
	TrainingLoad *float64 `json:"trainingLoad"`
}

type BikeDay struct {
	Date string `json:"date"`
	BikeLoad
}

type Floors struct {
	PrevDayUp   *float64 `json:"prevDayUp"`
	PrevDayDown *float64 `json:"prevDayDown"`
	DayUp       *float64 `json:"dayUp"`
	DayDown     *float64 `json:"dayDown"`
}

type Steps struct {
	PrevDay *float64 `json:"prevDay"`
	Day     *float64 `json:"day"`
}

// CrossLoad covers J-1 (whole day) and J until the activity start (whole day for the day view).
type CrossLoad struct {
	WindowStart     string            `json:"windowStart"`
	WindowEnd       string            `json:"windowEnd"`
	Bike            BikeLoad          `json:"bike"`
	BikeByDay       []BikeDay         `json:"bikeByDay"`
	Floors          Floors            `json:"floors"`
	Steps           Steps             `json:"steps"`
	StressPrevDay   *float64          `json:"stressPrevDay"`
	OtherActivities []ActivitySummary `json:"otherActivities"`
}

type HistoryRow struct {
	ActivityID      int64    `json:"activityId"`
	Date            string   `json:"date"`
	Type            string   `json:"type"`
	TypeLabel       string   `json:"typeLabel"`
	DistanceM       *float64 `json:"distanceM"`
	DurationS       *float64 `json:"durationS"`
	PaceSecPerKm    *float64 `json:"paceSecPerKm"`
	AvgCadenceSpm   *float64 `json:"avgCadenceSpm"`
	GctMs           *float64 `json:"gctMs"`
	ElevLossM       *float64 `json:"elevLossM"`
	TrainingLoad    *float64 `json:"trainingLoad"`
	ShoeName        *string  `json:"shoeName"`
	PainDuring      *float64 `json:"painDuring"`
	PainCold        *float64 `json:"painCold"`
	PainNextMorning *float64 `json:"painNextMorning"`
	// IsRunning drives the report (running-only history table).
	IsRunning bool `json:"-"`
}

type ActivityView struct {
	GeneratedAt string         `json:"generatedAt"`
	Freshness   Freshness      `json:"freshness"`
	SourceLabel string         `json:"sourceLabel"`
	Activity    ActivityDetail `json:"activity"`
	Weather     *Weather       `json:"weather"`
	// WeatherReason explains a null weather (indoor / no GPS, disabled, provider unavailable).
	WeatherReason *string                   `json:"weatherReason"`
	Biomechanics  analysis.Biomechanics     `json:"biomechanics"`
	Load          analysis.Load             `json:"load"`
	Segments      analysis.Segments         `json:"segments"`
	Series        analysis.Series           `json:"series"`
	CrossLoad     CrossLoad                 `json:"crossLoad"`
	Recovery      *analysis.RecoveryContext `json:"recovery"`
	Performance   *analysis.Performance     `json:"performance"`
	Plan          *athlete.PlanMatch        `json:"plan"`
	Notes         athlete.ActivityNotes     `json:"notes"`
	Goals         []athlete.Goal            `json:"goals"`
	Upcoming      []athlete.PlannedSession  `json:"upcoming"`
	History       []HistoryRow              `json:"history"`
	Alerts        []analysis.Alert          `json:"alerts"`
	Warnings      []string                  `json:"warnings"`
	DataSources   []string                  `json:"dataSources"`
	Efficiency    *analysis.Efficiency      `json:"efficiency"`
	Impact        *analysis.Impact          `json:"impact"`
	Readiness     analysis.ReadinessBanner  `json:"readiness"`
	// BodyStress carries active anatomical pain entries for prompt injection.
	BodyStress []analysis.BodyStressEntry `json:"bodyStress,omitempty"`
	// BodyCorrelations are the conclusive body map correlations (prompts only).
	BodyCorrelations []analysis.BodyCorrelationFactor `json:"-"`
	// PlannedToday is the planned session matched to (or scheduled on the day of) this activity.
	PlannedToday *athlete.PlannedSession `json:"-"`
	// Records feed the CONTEXTE & OBJECTIFS section of the prompts.
	Records []athlete.PersonalRecord `json:"-"`
}

type DayView struct {
	Date        string                    `json:"date"`
	GeneratedAt string                    `json:"generatedAt"`
	Freshness   Freshness                 `json:"freshness"`
	Recovery    *analysis.RecoveryContext `json:"recovery"`
	Day         analysis.DayInfo          `json:"day"`
	CrossLoad   CrossLoad                 `json:"crossLoad"`
	Activities  []ActivitySummary         `json:"activities"`
	Planned     []athlete.PlannedSession  `json:"planned"`
	Checkin     *athlete.DailyCheckin     `json:"checkin"`
	Goals       []athlete.Goal            `json:"goals"`
	Upcoming    []athlete.PlannedSession  `json:"upcoming"`
	Alerts      []analysis.Alert          `json:"alerts"`
	Warnings    []string                  `json:"warnings"`
	Readiness   analysis.ReadinessBanner  `json:"readiness"`
	// BodyStress carries active anatomical pain entries for prompt injection.
	BodyStress []analysis.BodyStressEntry `json:"bodyStress,omitempty"`
	// BodyCorrelations are the conclusive body map correlations (prompts only).
	BodyCorrelations []analysis.BodyCorrelationFactor `json:"-"`
	// History feeds the HISTORIQUE 14 JOURS table of the day report.
	History []HistoryRow             `json:"-"`
	Records []athlete.PersonalRecord `json:"-"`
}

type GarminIntegration struct {
	Connected      bool    `json:"connected"`
	Status         string  `json:"status"`
	KeyHint        *string `json:"keyHint"`
	TokenExpiresAt *string `json:"tokenExpiresAt"`
	LastSyncAt     *string `json:"lastSyncAt"`
	LastError      *string `json:"lastError"`
}

type StravaIntegration struct {
	Available   bool    `json:"available"`
	Connected   bool    `json:"connected"`
	Status      string  `json:"status"`
	AthleteName *string `json:"athleteName"`
	LastSyncAt  *string `json:"lastSyncAt"`
	LastError   *string `json:"lastError"`
	// Scopes granted to the stored token; MissingScopes lists those Stride needs but lacks.
	Scopes        []string `json:"scopes"`
	MissingScopes []string `json:"missingScopes"`
}

type WeatherIntegration struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
}

type Integrations struct {
	Garmin  GarminIntegration  `json:"garmin"`
	Strava  StravaIntegration  `json:"strava"`
	Weather WeatherIntegration `json:"weather"`
}

type ProviderSync struct {
	LastSyncAt    *string `json:"lastSyncAt"`
	NewActivities int     `json:"newActivities"`
	Error         *string `json:"error"`
}

type SyncStatus struct {
	Running       bool    `json:"running"`
	JobID         *string `json:"jobId,omitempty"`
	LastSyncAt    *string `json:"lastSyncAt"`
	LastLiveAt    *string `json:"lastLiveAt"`
	CooldownUntil *string `json:"cooldownUntil"`
	Providers     struct {
		Garmin *ProviderSync `json:"garmin"`
		Strava *ProviderSync `json:"strava"`
	} `json:"providers"`
}

type WeekDay struct {
	Date    string                   `json:"date"`
	Planned []athlete.PlannedSession `json:"planned"`
	Done    []ActivitySummary        `json:"done"`
}

type WeekTotals struct {
	PlannedDurationS float64 `json:"plannedDurationS"`
	DoneDurationS    float64 `json:"doneDurationS"`
	PlannedDistanceM float64 `json:"plannedDistanceM"`
	DoneDistanceM    float64 `json:"doneDistanceM"`
	SessionsPlanned  int     `json:"sessionsPlanned"`
	SessionsDone     int     `json:"sessionsDone"`
}

type WeekPlan struct {
	WeekStart string     `json:"weekStart"`
	Days      []WeekDay  `json:"days"`
	Totals    WeekTotals `json:"totals"`
}

type MultiExportResult struct {
	Markdown       string `json:"markdown"`
	TokensEstimate int    `json:"tokensEstimate"`
	Sessions       int    `json:"sessions"`
}

type AdminConnection struct {
	UserID      int64   `json:"userId"`
	Provider    string  `json:"provider"`
	Status      string  `json:"status"`
	LastSyncAt  *string `json:"lastSyncAt"`
	LastError   *string `json:"lastError"`
	LastErrorAt *string `json:"lastErrorAt"`
}

// MultiSession is one row of the multi-session export.
type MultiSession struct {
	Summary         ActivitySummary
	GctMs           *float64
	GctDriftPct     *float64
	ShoeName        *string
	PainDuring      *float64
	PainCold        *float64
	PainNextMorning *float64
}

type WeekTrend struct {
	WeekStart       string
	Sessions        int
	DistanceM       float64
	DurationS       float64
	Load            *float64
	LoadRatio       *float64
	CadenceAvg      *float64
	GctAvg          *float64
	EfAvg           *float64
	JointIndex      *float64
	HrvAvg          *float64
	SleepScoreAvg   *float64
	PainDuring      *float64
	PainCold        *float64
	PainNextMorning *float64
}

type PlanVsDone struct {
	Planned        int
	Done           int
	Partial        int
	Missed         int
	VolumeDeltaPct *float64
}

type AlertCount struct {
	Alert analysis.Alert
	Count int
}

// MultiExport is the input of the multi-session report (§6.4).
type MultiExport struct {
	From, To         string
	Weeks            int
	NextWeeks        int
	Goals            []athlete.Goal
	Sessions         []MultiSession
	Weekly           []WeekTrend
	Plan             PlanVsDone
	Alerts           []AlertCount
	Upcoming         []athlete.PlannedSession
	Unanalyzed       int
	Records          []athlete.PersonalRecord
	BodyStress       []analysis.BodyStressEntry
	BodyCorrelations []analysis.BodyCorrelationFactor
}

type EfficiencyPoint struct {
	Date         string   `json:"date"`
	ActivityID   int64    `json:"activityId"`
	EfSpeed      *float64 `json:"efSpeed"`
	EfPower      *float64 `json:"efPower"`
	AvgHR        *float64 `json:"avgHR"`
	PaceSecPerKm *float64 `json:"paceSecPerKm"`
}

type EfficiencyTrend struct {
	Points         []EfficiencyPoint `json:"points"`
	RollingEfSpeed []*float64        `json:"rollingEfSpeed"`
	ChangePct      *float64          `json:"changePct"`
}

type NotificationTarget struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	Label      string  `json:"label"`
	Enabled    bool    `json:"enabled"`
	Hint       string  `json:"hint"`
	LastStatus *string `json:"lastStatus"`
	LastSentAt *string `json:"lastSentAt"`
	LastError  *string `json:"lastError"`
}

type NotifyTestResult struct {
	OK      bool   `json:"ok"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type LLMConfig struct {
	Configured bool    `json:"configured"`
	Provider   *string `json:"provider"`
	Model      *string `json:"model"`
	BaseURL    *string `json:"baseUrl"`
	KeyHint    *string `json:"keyHint"`
	UpdatedAt  *string `json:"updatedAt"`
}

type LLMTestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int    `json:"latencyMs"`
	Message   string `json:"message"`
}

type LLMAnalysis struct {
	ID                   int64  `json:"id"`
	Kind                 string `json:"kind"`
	Ref                  string `json:"ref"`
	Provider             string `json:"provider"`
	Model                string `json:"model"`
	Markdown             string `json:"markdown"`
	PromptTokensEstimate int    `json:"promptTokensEstimate"`
	DurationMs           int64  `json:"durationMs"`
	CreatedAt            string `json:"createdAt"`
}

type CalendarDay struct {
	WeekDay
	Events []athlete.CalendarEvent `json:"events"`
}

type Calendar struct {
	Days []CalendarDay `json:"days"`
}

type ImportResult struct {
	ActivityID int64 `json:"activityId"`
	Merged     bool  `json:"merged"`
}

type TrackBounds struct {
	MinLat float64 `json:"minLat"`
	MaxLat float64 `json:"maxLat"`
	MinLon float64 `json:"minLon"`
	MaxLon float64 `json:"maxLon"`
}

// Track points are [lat, lon, tS, cadenceSpm|null, gradePct|null].
type Track struct {
	Points [][]*float64 `json:"points"`
	Bounds *TrackBounds `json:"bounds"`
}
