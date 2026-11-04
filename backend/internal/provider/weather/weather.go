// Package weather looks up historical conditions for an activity start (Open-Meteo, no API key).
// Types and Provider are the contract consumed by the domain; the platform implements OpenMeteo.
package weather

import (
	"context"
	"time"
)

type Conditions struct {
	Source       string    `json:"source"`
	TemperatureC *float64  `json:"temperatureC"`
	ApparentC    *float64  `json:"apparentC"`
	HumidityPct  *float64  `json:"humidityPct"`
	WindKph      *float64  `json:"windKph"`
	WindGustKph  *float64  `json:"windGustKph"`
	WindDirDeg   *float64  `json:"windDirDeg"`
	PrecipMm     *float64  `json:"precipMm"`
	WeatherCode  *int      `json:"weatherCode"`
	Description  string    `json:"description"`
	ObservedAt   time.Time `json:"observedAt"`
}

// Provider implementations must round coordinates to 2 decimals before any outbound call.
type Provider interface {
	At(ctx context.Context, lat, lon float64, t time.Time) (*Conditions, error)
}
