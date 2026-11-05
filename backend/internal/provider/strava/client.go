package strava

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"stride/internal/apimeter"
)

const (
	DefaultBaseURL = "https://www.strava.com"
	DefaultScope   = "read,activity:read_all,profile:read_all"
	streamKeys     = "time,distance,heartrate,cadence,watts,altitude,velocity_smooth,grade_smooth,moving,latlng,temp"
	maxBody        = 32 << 20
)

type Config struct {
	ClientID     string
	ClientSecret string
	// BaseURL overrides https://www.strava.com (tests).
	BaseURL string
	HTTP    *http.Client
	Meter   apimeter.Meter
	Now     func() time.Time
	// RetryBackoff is the base delay between retries of idempotent calls (default 1 s).
	RetryBackoff time.Duration
}

// Client implements API. Calls are attributed to apimeter.UserFrom(ctx).
type Client struct {
	cfg   Config
	base  string
	hc    *http.Client
	meter apimeter.Meter
	now   func() time.Time
}

var _ API = (*Client)(nil)

func NewClient(cfg Config) *Client {
	c := &Client{cfg: cfg, base: strings.TrimRight(cfg.BaseURL, "/"), hc: cfg.HTTP, meter: cfg.Meter, now: cfg.Now}
	if c.base == "" {
		c.base = DefaultBaseURL
	}
	if c.hc == nil {
		c.hc = &http.Client{Timeout: 60 * time.Second}
	}
	if c.meter == nil {
		c.meter = apimeter.Nop()
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.cfg.RetryBackoff <= 0 {
		c.cfg.RetryBackoff = time.Second
	}
	return c
}

// APIError is any non-2xx Strava answer other than an authorization failure.
type APIError struct {
	Status   int
	Endpoint string
	Message  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("strava %s: HTTP %d %s", e.Endpoint, e.Status, e.Message)
}

func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

type request struct {
	method   string
	endpoint string
	path     string
	query    url.Values
	form     url.Values
	token    string
	oauth    bool
}

func (c *Client) do(ctx context.Context, req request, out any) error {
	attempts := 1
	if req.method == http.MethodGet {
		attempts = 3
	}
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			t := time.NewTimer(c.cfg.RetryBackoff * time.Duration(1<<(i-1)))
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
		var retry bool
		if retry, err = c.once(ctx, req, out); err == nil || !retry {
			return err
		}
	}
	return err
}

func (c *Client) once(ctx context.Context, req request, out any) (retry bool, err error) {
	uid := apimeter.UserFrom(ctx)
	release, err := c.meter.Acquire(ctx, apimeter.Strava, uid)
	if err != nil {
		return false, err
	}
	defer release()

	u := c.base + req.path
	if len(req.query) > 0 {
		u += "?" + req.query.Encode()
	}
	var body io.Reader
	if req.form != nil {
		body = strings.NewReader(req.form.Encode())
	}
	hr, err := http.NewRequestWithContext(ctx, req.method, u, body)
	if err != nil {
		return false, err
	}
	hr.Header.Set("Accept", "application/json")
	if req.form != nil {
		hr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if req.token != "" {
		hr.Header.Set("Authorization", "Bearer "+req.token)
	}

	start := c.now()
	resp, err := c.hc.Do(hr)
	call := apimeter.Call{Provider: apimeter.Strava, UserID: uid, Endpoint: req.endpoint, Duration: c.now().Sub(start)}
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		err = fmt.Errorf("strava %s: %w", req.endpoint, err)
		call.Err = err
		c.meter.Observe(call)
		return ctx.Err() == nil && isTransient(err), err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	call.Status = resp.StatusCode
	q15, qDay := parseQuota(resp.Header)
	if q := pickQuota(q15, qDay); q != nil {
		q.UpdatedAt = c.now().UTC()
		c.meter.SetQuota(apimeter.Strava, *q)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		call.RetryAfter = retryAfter(resp.Header, q15, qDay, c.now())
	}
	if readErr != nil && resp.StatusCode < 300 {
		call.Err = fmt.Errorf("strava %s: read body: %w", req.endpoint, readErr)
	}
	c.meter.Observe(call)
	if call.Err != nil {
		return true, call.Err
	}

	switch st := resp.StatusCode; {
	case st == http.StatusUnauthorized, req.oauth && st == http.StatusBadRequest:
		return false, &ErrUnauthorized{Msg: apiMessage(data, st)}
	case st >= 500:
		return true, &APIError{Status: st, Endpoint: req.endpoint, Message: apiMessage(data, st)}
	case st >= 300:
		return false, &APIError{Status: st, Endpoint: req.endpoint, Message: apiMessage(data, st)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return false, fmt.Errorf("strava %s: decode: %w", req.endpoint, err)
		}
	}
	return false, nil
}

func isTransient(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

func apiMessage(body []byte, status int) string {
	var m struct {
		Message string `json:"message"`
		Errors  []struct {
			Resource string `json:"resource"`
			Field    string `json:"field"`
			Code     string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		msg := m.Message
		for _, e := range m.Errors {
			msg += fmt.Sprintf(" (%s.%s: %s)", e.Resource, e.Field, e.Code)
		}
		return msg
	}
	return http.StatusText(status)
}

type window struct {
	limit, usage int
	ok           bool
}

// parseQuota reads "15min,daily" pairs, keeping the most constrained of the overall and read limits.
func parseQuota(h http.Header) (q15, qDay window) {
	for _, prefix := range []string{"X-RateLimit-", "X-ReadRateLimit-"} {
		l := splitPair(h.Get(prefix + "Limit"))
		u := splitPair(h.Get(prefix + "Usage"))
		if l == nil || u == nil {
			continue
		}
		if w := (window{l[0], u[0], true}); tighter(w, q15) {
			q15 = w
		}
		if w := (window{l[1], u[1], true}); tighter(w, qDay) {
			qDay = w
		}
	}
	return q15, qDay
}

func tighter(a, b window) bool {
	if !b.ok {
		return a.ok && a.limit > 0
	}
	return a.limit > 0 && float64(a.limit-a.usage)/float64(a.limit) < float64(b.limit-b.usage)/float64(b.limit)
}

func splitPair(v string) []int {
	a, b, ok := strings.Cut(v, ",")
	if !ok {
		return nil
	}
	x, err1 := strconv.Atoi(strings.TrimSpace(a))
	y, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil {
		return nil
	}
	return []int{x, y}
}

func pickQuota(q15, qDay window) *apimeter.Quota {
	if !q15.ok && !qDay.ok {
		return nil
	}
	return &apimeter.Quota{Limit15m: q15.limit, Usage15m: q15.usage, LimitDaily: qDay.limit, UsageDaily: qDay.usage}
}

// retryAfter honours Retry-After, else waits for the exhausted Strava window (quarter hour or UTC midnight).
func retryAfter(h http.Header, q15, qDay window, now time.Time) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			return time.Duration(s) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil && t.After(now) {
			return t.Sub(now)
		}
	}
	now = now.UTC()
	if qDay.ok && qDay.usage >= qDay.limit {
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Sub(now)
	}
	if q15.ok && q15.usage >= q15.limit {
		return now.Truncate(15 * time.Minute).Add(15 * time.Minute).Sub(now)
	}
	return 0
}

func (c *Client) AuthorizeURL(state, redirectURI string) string {
	q := url.Values{
		"client_id":       {c.cfg.ClientID},
		"redirect_uri":    {redirectURI},
		"response_type":   {"code"},
		"approval_prompt": {"auto"},
		"scope":           {DefaultScope},
		"state":           {state},
	}
	return c.base + "/oauth/authorize?" + q.Encode()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Scope        string `json:"scope"`
	Athlete      *struct {
		ID        int64  `json:"id"`
		Firstname string `json:"firstname"`
		Lastname  string `json:"lastname"`
	} `json:"athlete"`
}

func (t tokenResponse) tokens() Tokens {
	out := Tokens{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, Scope: t.Scope}
	if t.ExpiresAt > 0 {
		out.ExpiresAt = time.Unix(t.ExpiresAt, 0).UTC()
	}
	if t.Athlete != nil {
		out.AthleteID = t.Athlete.ID
		out.AthleteName = strings.TrimSpace(t.Athlete.Firstname + " " + t.Athlete.Lastname)
	}
	return out
}

func (c *Client) token(ctx context.Context, form url.Values) (Tokens, error) {
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	var tr tokenResponse
	err := c.do(ctx, request{method: http.MethodPost, endpoint: "POST /oauth/token", path: "/oauth/token", form: form, oauth: true}, &tr)
	if err != nil {
		return Tokens{}, err
	}
	if tr.AccessToken == "" {
		return Tokens{}, errors.New("strava POST /oauth/token: empty access token")
	}
	return tr.tokens(), nil
}

func (c *Client) Exchange(ctx context.Context, code string) (Tokens, error) {
	return c.token(ctx, url.Values{"code": {code}, "grant_type": {"authorization_code"}})
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	return c.token(ctx, url.Values{"refresh_token": {refreshToken}, "grant_type": {"refresh_token"}})
}

func (c *Client) Deauthorize(ctx context.Context, accessToken string) error {
	return c.do(ctx, request{method: http.MethodPost, endpoint: "POST /oauth/deauthorize", path: "/oauth/deauthorize",
		form: url.Values{"access_token": {accessToken}}, oauth: true}, nil)
}

type apiGear struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	BrandName   string  `json:"brand_name"`
	ModelName   string  `json:"model_name"`
	Description string  `json:"description"`
	Distance    float64 `json:"distance"`
	Retired     bool    `json:"retired"`
	Primary     bool    `json:"primary"`
	FrameType   *int    `json:"frame_type"`
}

func (g apiGear) gear(kind string) Gear {
	if kind == "" {
		kind = "shoe"
		if g.FrameType != nil || strings.HasPrefix(g.ID, "b") {
			kind = "bike"
		}
	}
	return Gear{ID: g.ID, Name: g.Name, BrandName: g.BrandName, ModelName: g.ModelName, Description: g.Description,
		DistanceM: g.Distance, Retired: g.Retired, Primary: g.Primary, Kind: kind}
}

type apiActivity struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	Description          *string   `json:"description"`
	PrivateNote          *string   `json:"private_note"`
	SportType            string    `json:"sport_type"`
	Type                 string    `json:"type"`
	StartDate            time.Time `json:"start_date"`
	StartDateLocal       string    `json:"start_date_local"`
	Timezone             string    `json:"timezone"`
	Distance             float64   `json:"distance"`
	MovingTime           float64   `json:"moving_time"`
	ElapsedTime          float64   `json:"elapsed_time"`
	TotalElevationGain   float64   `json:"total_elevation_gain"`
	AverageSpeed         float64   `json:"average_speed"`
	AverageHeartrate     *float64  `json:"average_heartrate"`
	MaxHeartrate         *float64  `json:"max_heartrate"`
	AverageCadence       *float64  `json:"average_cadence"`
	AverageWatts         *float64  `json:"average_watts"`
	WeightedAverageWatts *float64  `json:"weighted_average_watts"`
	SufferScore          *float64  `json:"suffer_score"`
	PerceivedExertion    *float64  `json:"perceived_exertion"`
	GearID               *string   `json:"gear_id"`
	Gear                 *apiGear  `json:"gear"`
	StartLatlng          []float64 `json:"start_latlng"`
	DeviceName           string    `json:"device_name"`
	ExternalID           *string   `json:"external_id"`
	Manual               bool      `json:"manual"`
	KudosCount           *int      `json:"kudos_count"`
	BestEfforts          []struct {
		Name        string  `json:"name"`
		Distance    float64 `json:"distance"`
		ElapsedTime float64 `json:"elapsed_time"`
	} `json:"best_efforts"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// activity maps the Strava payload; StartDateLocal drops Strava's misleading "Z" and Timezone keeps the IANA name.
func (a apiActivity) activity() Activity {
	out := Activity{
		ID: a.ID, Name: a.Name, Description: str(a.Description), PrivateNote: str(a.PrivateNote),
		SportType: a.SportType, StartDate: a.StartDate.UTC(), StartDateLocal: strings.TrimSuffix(a.StartDateLocal, "Z"),
		Timezone: a.Timezone, DistanceM: a.Distance, MovingTimeS: a.MovingTime, ElapsedTimeS: a.ElapsedTime,
		ElevationGainM: a.TotalElevationGain, AverageSpeedMS: a.AverageSpeed, AverageHeartrate: a.AverageHeartrate,
		MaxHeartrate: a.MaxHeartrate, AverageCadenceRaw: a.AverageCadence, AverageWatts: a.AverageWatts,
		WeightedAverageWatts: a.WeightedAverageWatts, SufferScore: a.SufferScore, PerceivedExertion: a.PerceivedExertion,
		GearID: str(a.GearID), DeviceName: a.DeviceName, ExternalID: str(a.ExternalID), Manual: a.Manual, KudosCount: a.KudosCount,
	}
	for _, e := range a.BestEfforts {
		out.BestEfforts = append(out.BestEfforts, Effort{Name: e.Name, DistanceM: e.Distance, ElapsedS: e.ElapsedTime})
	}
	if out.SportType == "" {
		out.SportType = a.Type
	}
	if _, name, ok := strings.Cut(a.Timezone, ") "); ok {
		out.Timezone = name
	}
	if len(a.StartLatlng) == 2 {
		out.StartLatLng = a.StartLatlng
	}
	if a.Gear != nil {
		g := a.Gear.gear("")
		out.Gear = &g
	}
	return out
}

func (c *Client) ListActivities(ctx context.Context, accessToken string, after, before time.Time, page, perPage int) ([]Activity, error) {
	q := url.Values{}
	if !after.IsZero() {
		q.Set("after", strconv.FormatInt(after.Unix(), 10))
	}
	if !before.IsZero() {
		q.Set("before", strconv.FormatInt(before.Unix(), 10))
	}
	q.Set("page", strconv.Itoa(max(page, 1)))
	q.Set("per_page", strconv.Itoa(min(max(perPage, 1), 200)))
	var raw []apiActivity
	if err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /athlete/activities", path: "/api/v3/athlete/activities", query: q, token: accessToken}, &raw); err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(raw))
	for _, a := range raw {
		out = append(out, a.activity())
	}
	return out, nil
}

func (c *Client) GetActivity(ctx context.Context, accessToken string, id int64) (*Activity, error) {
	var raw apiActivity
	err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /activities/{id}", path: "/api/v3/activities/" + strconv.FormatInt(id, 10),
		query: url.Values{"include_all_efforts": {"false"}}, token: accessToken}, &raw)
	if err != nil {
		return nil, err
	}
	a := raw.activity()
	now := c.now().UTC()
	a.FetchedAt = &now
	return &a, nil
}

func (c *Client) GetStreams(ctx context.Context, accessToken string, id int64) (*Streams, error) {
	var raw map[string]struct {
		Data json.RawMessage `json:"data"`
	}
	err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /activities/{id}/streams", path: "/api/v3/activities/" + strconv.FormatInt(id, 10) + "/streams",
		query: url.Values{"keys": {streamKeys}, "key_by_type": {"true"}}, token: accessToken}, &raw)
	if err != nil {
		return nil, err
	}
	s := &Streams{}
	floats := map[string]*[]float64{
		"time": &s.TimeS, "distance": &s.DistanceM, "heartrate": &s.HeartRate, "cadence": &s.CadenceRaw, "watts": &s.Watts,
		"altitude": &s.AltitudeM, "velocity_smooth": &s.VelocityMS, "grade_smooth": &s.GradePct, "temp": &s.TemperatureC,
	}
	for key, st := range raw {
		var err error
		switch key {
		case "moving":
			err = json.Unmarshal(st.Data, &s.Moving)
		case "latlng":
			err = json.Unmarshal(st.Data, &s.LatLng)
		default:
			if dst, ok := floats[key]; ok {
				err = json.Unmarshal(st.Data, dst)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("strava GET /activities/{id}/streams: decode %s: %w", key, err)
		}
	}
	return s, nil
}

func (c *Client) GetGear(ctx context.Context, accessToken, gearID string) (*Gear, error) {
	var raw apiGear
	if err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /gear/{id}", path: "/api/v3/gear/" + url.PathEscape(gearID), token: accessToken}, &raw); err != nil {
		return nil, err
	}
	g := raw.gear("")
	return &g, nil
}

func (c *Client) ListAthleteGear(ctx context.Context, accessToken string) ([]Gear, error) {
	var raw struct {
		Shoes []apiGear `json:"shoes"`
		Bikes []apiGear `json:"bikes"`
	}
	if err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /athlete", path: "/api/v3/athlete", token: accessToken}, &raw); err != nil {
		return nil, err
	}
	out := make([]Gear, 0, len(raw.Shoes)+len(raw.Bikes))
	for _, g := range raw.Shoes {
		out = append(out, g.gear("shoe"))
	}
	for _, g := range raw.Bikes {
		out = append(out, g.gear("bike"))
	}
	return out, nil
}

func (c *Client) GetAthlete(ctx context.Context, accessToken string) (*Athlete, error) {
	var raw struct {
		ID            int64  `json:"id"`
		Firstname     string `json:"firstname"`
		Lastname      string `json:"lastname"`
		ResourceState int    `json:"resource_state"`
	}
	if err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /athlete", path: "/api/v3/athlete", token: accessToken}, &raw); err != nil {
		return nil, err
	}
	if raw.ID == 0 {
		return nil, errors.New("strava GET /athlete: missing athlete id")
	}
	return &Athlete{ID: raw.ID, Name: strings.TrimSpace(raw.Firstname + " " + raw.Lastname), Detailed: raw.ResourceState >= 3}, nil
}

type apiTotals struct {
	Count         int     `json:"count"`
	Distance      float64 `json:"distance"`
	MovingTime    float64 `json:"moving_time"`
	ElevationGain float64 `json:"elevation_gain"`
}

func (t apiTotals) totals() Totals {
	return Totals{Count: t.Count, DistanceM: t.Distance, MovingS: t.MovingTime, ElevGainM: t.ElevationGain}
}

func (c *Client) GetAthleteStats(ctx context.Context, accessToken string, athleteID int64) (*AthleteStats, error) {
	var raw struct {
		RecentRun  apiTotals `json:"recent_run_totals"`
		YTDRun     apiTotals `json:"ytd_run_totals"`
		AllRun     apiTotals `json:"all_run_totals"`
		RecentRide apiTotals `json:"recent_ride_totals"`
		YTDRide    apiTotals `json:"ytd_ride_totals"`
	}
	err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /athletes/{id}/stats",
		path: "/api/v3/athletes/" + strconv.FormatInt(athleteID, 10) + "/stats", token: accessToken}, &raw)
	if err != nil {
		return nil, err
	}
	return &AthleteStats{RecentRun: raw.RecentRun.totals(), YTDRun: raw.YTDRun.totals(), AllRun: raw.AllRun.totals(),
		RecentRide: raw.RecentRide.totals(), YTDRide: raw.YTDRide.totals()}, nil
}

func (c *Client) appCredentials() url.Values {
	return url.Values{"client_id": {c.cfg.ClientID}, "client_secret": {c.cfg.ClientSecret}}
}

func (c *Client) EnsureSubscription(ctx context.Context, callbackURL, verifyToken string) (int64, error) {
	var subs []struct {
		ID          int64  `json:"id"`
		CallbackURL string `json:"callback_url"`
	}
	if err := c.do(ctx, request{method: http.MethodGet, endpoint: "GET /push_subscriptions", path: "/api/v3/push_subscriptions", query: c.appCredentials()}, &subs); err != nil {
		return 0, err
	}
	for _, s := range subs {
		if s.CallbackURL == callbackURL {
			return s.ID, nil
		}
		err := c.do(ctx, request{method: http.MethodDelete, endpoint: "DELETE /push_subscriptions/{id}",
			path: "/api/v3/push_subscriptions/" + strconv.FormatInt(s.ID, 10), query: c.appCredentials()}, nil)
		if err != nil {
			return 0, err
		}
	}
	form := c.appCredentials()
	form.Set("callback_url", callbackURL)
	form.Set("verify_token", verifyToken)
	var created struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, request{method: http.MethodPost, endpoint: "POST /push_subscriptions", path: "/api/v3/push_subscriptions", form: form}, &created); err != nil {
		return 0, err
	}
	return created.ID, nil
}
