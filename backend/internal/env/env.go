package env

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr string
	// MetricsAddr serves GET /metrics on a separate listener; empty disables it.
	MetricsAddr string
	DataDir     string
	DBPath      string
	ValkeyURL   string

	MasterKey         []byte
	MasterKeyPrevious []byte

	PublicURL        string
	RPID             string
	RPOrigins        []string
	CookieSecure     bool
	SessionTTL       time.Duration
	RegistrationMode string

	Timezone  string
	LogLevel  string
	LogFormat string

	GarminBaseURL  string
	AdminGarminKey string

	StravaClientID     string
	StravaClientSecret string
	StravaWebhookToken string
	// One-time seed of the first admin's Strava connection (refreshed tokens live in the DB).
	AdminStravaAccessToken  string
	AdminStravaRefreshToken string

	WeatherProvider string

	// AllowPrivateEgress lets user-supplied URLs (LLM base URL, webhooks) reach private networks.
	AllowPrivateEgress bool

	SyncHour      int
	SyncJitterMin time.Duration
	SyncJitterMax time.Duration

	// S3 seeds the backup target once, when the admin settings hold none (§15.4).
	S3 S3Seed
}

// S3Seed is the STRIDE_S3_* environment (empty Endpoint and Bucket: nothing to seed).
type S3Seed struct {
	Endpoint, Region, Bucket, Prefix, AccessKeyID, SecretAccessKey string
	PathStyle, AutoUpload                                          bool
	RetentionDays                                                  int
}

func (c Config) StravaEnabled() bool { return c.StravaClientID != "" && c.StravaClientSecret != "" }

// Load reads ENV_FILE (or ./.env) without overriding variables already set, then the process environment.
func Load() (Config, error) {
	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}
	if err := loadDotEnv(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("env file %s: %w", envFile, err)
	}

	c := Config{
		ListenAddr:              get("STRIDE_LISTEN_ADDR", ":8080"),
		MetricsAddr:             os.Getenv("STRIDE_METRICS_ADDR"),
		DataDir:                 get("STRIDE_DATA_DIR", "./data"),
		ValkeyURL:               os.Getenv("STRIDE_VALKEY_URL"),
		PublicURL:               strings.TrimRight(get("STRIDE_PUBLIC_URL", "http://localhost:8080"), "/"),
		RegistrationMode:        get("STRIDE_REGISTRATION", "invite"),
		Timezone:                get("TZ", "Europe/Paris"),
		LogLevel:                get("LOG_LEVEL", "info"),
		LogFormat:               get("LOG_FORMAT", "pretty"),
		GarminBaseURL:           strings.TrimRight(get("STRIDE_GARMIN_BASE_URL", "https://personal.connectapi.garmin.com"), "/"),
		AdminGarminKey:          first(os.Getenv("STRIDE_ADMIN_GARMIN_KEY"), os.Getenv("GARMIN_API_TOKEN")),
		StravaClientID:          os.Getenv("STRIDE_STRAVA_CLIENT_ID"),
		StravaClientSecret:      os.Getenv("STRIDE_STRAVA_CLIENT_SECRET"),
		StravaWebhookToken:      os.Getenv("STRIDE_STRAVA_WEBHOOK_TOKEN"),
		AdminStravaAccessToken:  os.Getenv("STRIDE_STRAVA_ACCESS_TOKEN"),
		AdminStravaRefreshToken: os.Getenv("STRIDE_STRAVA_REFRESH_TOKEN"),
		WeatherProvider:         get("STRIDE_WEATHER", "open-meteo"),
	}
	c.DBPath = get("STRIDE_DB_PATH", filepath.Join(c.DataDir, "stride.db"))

	var err error
	if c.MasterKey, err = decodeKey("STRIDE_MASTER_KEY", true); err != nil {
		return Config{}, err
	}
	if c.MasterKeyPrevious, err = decodeKey("STRIDE_MASTER_KEY_PREVIOUS", false); err != nil {
		return Config{}, err
	}

	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" {
		return Config{}, fmt.Errorf("STRIDE_PUBLIC_URL invalid: %q", c.PublicURL)
	}
	c.CookieSecure = u.Scheme == "https"
	c.RPID = get("STRIDE_WEBAUTHN_RP_ID", u.Hostname())
	c.RPOrigins = splitList(get("STRIDE_WEBAUTHN_ORIGINS", c.PublicURL))

	if c.RegistrationMode != "invite" && c.RegistrationMode != "open" {
		return Config{}, fmt.Errorf("STRIDE_REGISTRATION must be invite|open, got %q", c.RegistrationMode)
	}
	if c.SessionTTL, err = time.ParseDuration(get("STRIDE_SESSION_TTL", "720h")); err != nil {
		return Config{}, fmt.Errorf("STRIDE_SESSION_TTL: %w", err)
	}
	if c.SyncHour, err = strconv.Atoi(get("STRIDE_SYNC_HOUR", "7")); err != nil || c.SyncHour < 0 || c.SyncHour > 23 {
		return Config{}, fmt.Errorf("STRIDE_SYNC_HOUR must be 0-23")
	}
	if c.SyncJitterMin, err = time.ParseDuration(get("STRIDE_SYNC_JITTER_MIN", "30s")); err != nil {
		return Config{}, fmt.Errorf("STRIDE_SYNC_JITTER_MIN: %w", err)
	}
	if c.SyncJitterMax, err = time.ParseDuration(get("STRIDE_SYNC_JITTER_MAX", "90s")); err != nil || c.SyncJitterMax < c.SyncJitterMin {
		return Config{}, fmt.Errorf("STRIDE_SYNC_JITTER_MAX must be >= STRIDE_SYNC_JITTER_MIN")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return Config{}, fmt.Errorf("TZ: %w", err)
	}
	if c.AllowPrivateEgress, err = strconv.ParseBool(get("STRIDE_ALLOW_PRIVATE_EGRESS", "false")); err != nil {
		return Config{}, fmt.Errorf("STRIDE_ALLOW_PRIVATE_EGRESS must be true|false")
	}
	c.S3 = S3Seed{Endpoint: os.Getenv("STRIDE_S3_ENDPOINT"), Region: get("STRIDE_S3_REGION", "us-east-1"), Bucket: os.Getenv("STRIDE_S3_BUCKET"),
		Prefix: os.Getenv("STRIDE_S3_PREFIX"), AccessKeyID: os.Getenv("STRIDE_S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("STRIDE_S3_SECRET_ACCESS_KEY")}
	if c.S3.PathStyle, err = strconv.ParseBool(get("STRIDE_S3_PATH_STYLE", "false")); err != nil {
		return Config{}, fmt.Errorf("STRIDE_S3_PATH_STYLE must be true|false")
	}
	if c.S3.AutoUpload, err = strconv.ParseBool(get("STRIDE_S3_AUTO_UPLOAD", "false")); err != nil {
		return Config{}, fmt.Errorf("STRIDE_S3_AUTO_UPLOAD must be true|false")
	}
	if c.S3.RetentionDays, err = strconv.Atoi(get("STRIDE_S3_RETENTION_DAYS", "30")); err != nil || c.S3.RetentionDays < 0 || c.S3.RetentionDays > 3650 {
		return Config{}, fmt.Errorf("STRIDE_S3_RETENTION_DAYS must be 0-3650")
	}
	return c, nil
}

func decodeKey(name string, required bool) ([]byte, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		if required {
			return nil, fmt.Errorf("%s is required (generate one with: openssl rand -base64 32)", name)
		}
		return nil, nil
	}
	k, err := base64.StdEncoding.DecodeString(v)
	if err != nil || len(k) != 32 {
		return nil, fmt.Errorf("%s must be 32 bytes base64-encoded (openssl rand -base64 32)", name)
	}
	return k, nil
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}

func get(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func first(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimRight(strings.TrimSpace(p), "/"); p != "" {
			out = append(out, p)
		}
	}
	return out
}
