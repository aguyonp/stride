// Package strava is the Strava API v3 client (OAuth2, activities, streams, gear, webhooks).
// Types and the API interface are the contract consumed by the domain; the platform implements Client.
package strava

import (
	"context"
	"time"
)

type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scope        string
	AthleteID    int64
	AthleteName  string
}

type Gear struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	BrandName   string  `json:"brandName"`
	ModelName   string  `json:"modelName"`
	Description string  `json:"description"`
	DistanceM   float64 `json:"distanceM"`
	Retired     bool    `json:"retired"`
	Primary     bool    `json:"primary"`
	Kind        string  `json:"kind"` // shoe | bike
}

// Activity is a DetailedActivity subset. Pointers are nil when Strava omits the field.
type Activity struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	Description          string     `json:"description"`
	PrivateNote          string     `json:"privateNote"`
	SportType            string     `json:"sportType"`
	StartDate            time.Time  `json:"startDate"`
	StartDateLocal       string     `json:"startDateLocal"`
	Timezone             string     `json:"timezone"`
	DistanceM            float64    `json:"distanceM"`
	MovingTimeS          float64    `json:"movingTimeS"`
	ElapsedTimeS         float64    `json:"elapsedTimeS"`
	ElevationGainM       float64    `json:"elevationGainM"`
	AverageSpeedMS       float64    `json:"averageSpeedMS"`
	AverageHeartrate     *float64   `json:"averageHeartrate"`
	MaxHeartrate         *float64   `json:"maxHeartrate"`
	AverageCadenceRaw    *float64   `json:"averageCadenceRaw"` // Strava unit (rpm, one leg for runs)
	AverageWatts         *float64   `json:"averageWatts"`
	WeightedAverageWatts *float64   `json:"weightedAverageWatts"`
	SufferScore          *float64   `json:"sufferScore"`
	PerceivedExertion    *float64   `json:"perceivedExertion"`
	GearID               string     `json:"gearId"`
	Gear                 *Gear      `json:"gear"`
	StartLatLng          []float64  `json:"startLatLng"`
	DeviceName           string     `json:"deviceName"`
	ExternalID           string     `json:"externalId"`
	Manual               bool       `json:"manual"`
	FetchedAt            *time.Time `json:"fetchedAt"`
}

// Streams are aligned per second-ish samples; missing streams are nil.
type Streams struct {
	TimeS        []float64    `json:"timeS"`
	DistanceM    []float64    `json:"distanceM"`
	HeartRate    []float64    `json:"heartRate"`
	CadenceRaw   []float64    `json:"cadenceRaw"`
	Watts        []float64    `json:"watts"`
	AltitudeM    []float64    `json:"altitudeM"`
	VelocityMS   []float64    `json:"velocityMS"`
	GradePct     []float64    `json:"gradePct"`
	Moving       []bool       `json:"moving"`
	LatLng       [][2]float64 `json:"latLng"`
	TemperatureC []float64    `json:"temperatureC"`
}

type WebhookEvent struct {
	ObjectType     string            `json:"object_type"` // activity | athlete
	ObjectID       int64             `json:"object_id"`
	AspectType     string            `json:"aspect_type"` // create | update | delete
	OwnerID        int64             `json:"owner_id"`
	SubscriptionID int64             `json:"subscription_id"`
	EventTime      int64             `json:"event_time"`
	Updates        map[string]string `json:"updates"`
}

// API calls must be attributed with apimeter.WithUser(ctx, userID) and go through the meter.
type API interface {
	AuthorizeURL(state, redirectURI string) string
	Exchange(ctx context.Context, code string) (Tokens, error)
	Refresh(ctx context.Context, refreshToken string) (Tokens, error)
	Deauthorize(ctx context.Context, accessToken string) error
	ListActivities(ctx context.Context, accessToken string, after, before time.Time, page, perPage int) ([]Activity, error)
	GetActivity(ctx context.Context, accessToken string, id int64) (*Activity, error)
	GetStreams(ctx context.Context, accessToken string, id int64) (*Streams, error)
	GetGear(ctx context.Context, accessToken, gearID string) (*Gear, error)
	ListAthleteGear(ctx context.Context, accessToken string) ([]Gear, error)
	// EnsureSubscription creates the app-wide webhook subscription if missing and returns its id.
	EnsureSubscription(ctx context.Context, callbackURL, verifyToken string) (int64, error)
}

// ErrUnauthorized is returned when Strava rejects the token (revoked access).
type ErrUnauthorized struct{ Msg string }

func (e *ErrUnauthorized) Error() string { return "strava unauthorized: " + e.Msg }
