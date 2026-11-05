// Package config keeps the V1 settings store until the domain moves it; V1 env loading lives in internal/env now.
package config

// Config is the subset of the V1 configuration still read by settings.go; delete together with settings.go.
type Config struct {
	EnvAPIKey string
	Timezone  string
}

const DefaultTimezone = "Europe/Paris"
