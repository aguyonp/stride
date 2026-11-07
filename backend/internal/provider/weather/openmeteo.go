package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"stride/internal/apimeter"
)

const (
	DefaultForecastURL = "https://api.open-meteo.com/v1/forecast"
	DefaultArchiveURL  = "https://archive-api.open-meteo.com/v1/archive"
	hourlyVars         = "temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,wind_gusts_10m,wind_direction_10m,precipitation,weather_code"
)

var (
	ErrNoData            = errors.New("weather: no data for this time and place")
	ErrInvalidCoordinate = errors.New("weather: invalid coordinates")
)

type OpenMeteoOptions struct {
	ForecastURL string
	ArchiveURL  string
	HTTP        *http.Client
	Meter       apimeter.Meter
	Now         func() time.Time
	// RecentDays: younger start times use the forecast API (past data), older ones the archive (default 30).
	RecentDays   int
	RetryBackoff time.Duration
}

// OpenMeteo implements Provider with the keyless Open-Meteo APIs.
type OpenMeteo struct {
	o OpenMeteoOptions
}

var _ Provider = (*OpenMeteo)(nil)

func NewOpenMeteo(o OpenMeteoOptions) *OpenMeteo {
	if o.ForecastURL == "" {
		o.ForecastURL = DefaultForecastURL
	}
	if o.ArchiveURL == "" {
		o.ArchiveURL = DefaultArchiveURL
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 20 * time.Second}
	}
	if o.Meter == nil {
		o.Meter = apimeter.Nop()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.RecentDays <= 0 {
		o.RecentDays = 30
	}
	if o.RetryBackoff <= 0 {
		o.RetryBackoff = time.Second
	}
	return &OpenMeteo{o: o}
}

// Round2 rounds a coordinate to 2 decimals (~1 km) before it leaves the server.
func Round2(v float64) float64 { return math.Round(v*100) / 100 }

type omResponse struct {
	Hourly struct {
		Time        []int64    `json:"time"`
		Temperature []*float64 `json:"temperature_2m"`
		Apparent    []*float64 `json:"apparent_temperature"`
		Humidity    []*float64 `json:"relative_humidity_2m"`
		Wind        []*float64 `json:"wind_speed_10m"`
		Gusts       []*float64 `json:"wind_gusts_10m"`
		WindDir     []*float64 `json:"wind_direction_10m"`
		Precip      []*float64 `json:"precipitation"`
		Code        []*float64 `json:"weather_code"`
	} `json:"hourly"`
}

func (p *OpenMeteo) At(ctx context.Context, lat, lon float64, t time.Time) (*Conditions, error) {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.Abs(lat) > 90 || math.Abs(lon) > 180 || (lat == 0 && lon == 0) {
		return nil, ErrInvalidCoordinate
	}
	lat, lon = Round2(lat), Round2(lon)
	t = t.UTC()
	now := p.o.Now().UTC()
	if t.After(now.Add(time.Hour)) {
		return nil, ErrNoData
	}
	base, endpoint := p.o.ForecastURL, "GET /v1/forecast"
	if now.Sub(t) > time.Duration(p.o.RecentDays)*24*time.Hour {
		base, endpoint = p.o.ArchiveURL, "GET /v1/archive"
	}
	return p.hourly(ctx, base, endpoint, lat, lon, t)
}

// forecastHorizon is how far ahead Open-Meteo forecasts hourly conditions.
const forecastHorizon = 15 * 24 * time.Hour

// Forecast returns the forecast conditions at a future time (Smart Workout Morphing, §15.5).
func (p *OpenMeteo) Forecast(ctx context.Context, lat, lon float64, t time.Time) (*Conditions, error) {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.Abs(lat) > 90 || math.Abs(lon) > 180 || (lat == 0 && lon == 0) {
		return nil, ErrInvalidCoordinate
	}
	t = t.UTC()
	if now := p.o.Now().UTC(); t.Before(now.Add(-time.Hour)) || t.After(now.Add(forecastHorizon)) {
		return nil, ErrNoData
	}
	c, err := p.hourly(ctx, p.o.ForecastURL, "GET /v1/forecast", Round2(lat), Round2(lon), t)
	if c != nil {
		c.Source = "open-meteo-forecast"
	}
	return c, err
}

// hourly reads the hourly conditions around t from base (coordinates already rounded).
func (p *OpenMeteo) hourly(ctx context.Context, base, endpoint string, lat, lon float64, t time.Time) (*Conditions, error) {
	q := url.Values{
		"latitude":        {strconv.FormatFloat(lat, 'f', 2, 64)},
		"longitude":       {strconv.FormatFloat(lon, 'f', 2, 64)},
		"hourly":          {hourlyVars},
		"wind_speed_unit": {"kmh"},
		"timezone":        {"GMT"},
		"timeformat":      {"unixtime"},
		"start_date":      {t.Add(-time.Hour).Format("2006-01-02")},
		"end_date":        {t.Add(time.Hour).Format("2006-01-02")},
	}
	var resp omResponse
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(p.o.RetryBackoff * time.Duration(1<<(attempt-1)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		var retry bool
		if retry, err = p.fetch(ctx, base+"?"+q.Encode(), endpoint, &resp); err == nil || !retry {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return resp.nearest(t)
}

func (p *OpenMeteo) fetch(ctx context.Context, u, endpoint string, out *omResponse) (retry bool, err error) {
	uid := apimeter.UserFrom(ctx)
	release, err := p.o.Meter.Acquire(ctx, apimeter.OpenMeteo, uid)
	if err != nil {
		return false, err
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	start := p.o.Now()
	res, err := p.o.HTTP.Do(req)
	call := apimeter.Call{Provider: apimeter.OpenMeteo, UserID: uid, Endpoint: endpoint, Duration: p.o.Now().Sub(start)}
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		err = fmt.Errorf("open-meteo %s: %w", endpoint, err)
		call.Err = err
		p.o.Meter.Observe(call)
		var ne net.Error
		return ctx.Err() == nil && (errors.As(err, &ne) || errors.Is(err, io.ErrUnexpectedEOF)), err
	}
	defer res.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	call.Status = res.StatusCode
	if res.StatusCode == http.StatusTooManyRequests {
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 {
			call.RetryAfter = time.Duration(s) * time.Second
		}
	}
	p.o.Meter.Observe(call)
	if res.StatusCode != http.StatusOK {
		var e struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(body, &e)
		return res.StatusCode >= 500, fmt.Errorf("open-meteo %s: HTTP %d %s", endpoint, res.StatusCode, e.Reason)
	}
	if readErr != nil {
		return true, fmt.Errorf("open-meteo %s: read body: %w", endpoint, readErr)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, fmt.Errorf("open-meteo %s: decode: %w", endpoint, err)
	}
	return false, nil
}

func at(v []*float64, i int) *float64 {
	if i < len(v) && v[i] != nil {
		x := *v[i]
		return &x
	}
	return nil
}

func (r omResponse) nearest(t time.Time) (*Conditions, error) {
	h := r.Hourly
	best, bestDiff := -1, int64(math.MaxInt64)
	for i, ts := range h.Time {
		d := ts - t.Unix()
		if d < 0 {
			d = -d
		}
		if d < bestDiff && at(h.Temperature, i) != nil {
			best, bestDiff = i, d
		}
	}
	if best < 0 || bestDiff > 3600 {
		return nil, ErrNoData
	}
	c := &Conditions{
		Source:       "open-meteo",
		TemperatureC: at(h.Temperature, best),
		ApparentC:    at(h.Apparent, best),
		HumidityPct:  at(h.Humidity, best),
		WindKph:      at(h.Wind, best),
		WindGustKph:  at(h.Gusts, best),
		WindDirDeg:   at(h.WindDir, best),
		PrecipMm:     at(h.Precip, best),
		ObservedAt:   time.Unix(h.Time[best], 0).UTC(),
	}
	if code := at(h.Code, best); code != nil {
		n := int(*code)
		c.WeatherCode = &n
		c.Description = Describe(n)
	}
	return c, nil
}

var wmo = map[int]string{
	0: "Ciel dégagé", 1: "Plutôt dégagé", 2: "Partiellement nuageux", 3: "Couvert",
	45: "Brouillard", 48: "Brouillard givrant",
	51: "Bruine légère", 53: "Bruine modérée", 55: "Bruine dense",
	56: "Bruine verglaçante légère", 57: "Bruine verglaçante dense",
	61: "Pluie faible", 63: "Pluie modérée", 65: "Pluie forte",
	66: "Pluie verglaçante faible", 67: "Pluie verglaçante forte",
	71: "Neige faible", 73: "Neige modérée", 75: "Neige forte", 77: "Grains de neige",
	80: "Averses faibles", 81: "Averses modérées", 82: "Averses violentes",
	85: "Averses de neige faibles", 86: "Averses de neige fortes",
	95: "Orage", 96: "Orage avec grêle faible", 99: "Orage avec grêle forte",
}

// Describe returns the French label of a WMO weather code.
func Describe(code int) string {
	if s, ok := wmo[code]; ok {
		return s
	}
	return "Conditions inconnues"
}

// WindDirLabel converts a direction in degrees (where the wind comes from) to a French 8-point label.
func WindDirLabel(deg float64) string {
	labels := [...]string{"N", "NE", "E", "SE", "S", "SO", "O", "NO"}
	d := math.Mod(math.Mod(deg, 360)+360, 360)
	return labels[int(math.Round(d/45))%8]
}
