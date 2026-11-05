// Package garmin is a per-user client for the Garmin Connect Personal API.
// Every outbound call goes through apimeter; responses are returned raw so the
// domain layer decides what to persist (SQLite) or cache (TTL).
package garmin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"stride/internal/apimeter"
	"stride/internal/logx"
)

const DefaultBaseURL = "https://personal.connectapi.garmin.com"

const (
	tokenPath      = "/personalapitoken-service/personal-api/personal-api-key/token"
	userAgent      = "stride/2.0"
	requestTimeout = 45 * time.Second
	renewBefore    = time.Hour
	maxRetries     = 2
	maxBodyBytes   = 64 << 20
	fitAccept      = "application/zip, application/x-zip-compressed, application/octet-stream"
)

var (
	ErrNoAPIKey = errors.New("garmin: no personal API key")
	ErrNoData   = errors.New("garmin: no data")
)

// APIError is a non-2xx answer (Status 0 = network error or timeout).
type APIError struct {
	Op       string
	Endpoint string
	Status   int
	Err      error
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("garmin %s %s: %v", e.Op, e.Endpoint, e.Err)
	}
	return fmt.Sprintf("garmin %s %s: HTTP %d", e.Op, e.Endpoint, e.Status)
}

func (e *APIError) Unwrap() error { return e.Err }

func status(err error) int {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

func IsUnauthorized(err error) bool {
	s := status(err)
	return s == http.StatusUnauthorized || s == http.StatusForbidden
}

func IsNotFound(err error) bool { return status(err) == http.StatusNotFound }

func IsRateLimited(err error) bool {
	return status(err) == http.StatusTooManyRequests || errors.Is(err, apimeter.ErrCooldown)
}

func IsTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne interface{ Timeout() bool }
	return errors.As(err, &ne) && ne.Timeout()
}

type Options struct {
	BaseURL    string
	UserID     int64
	APIKey     string
	Meter      apimeter.Meter
	HTTPClient *http.Client
	Log        *slog.Logger
	Now        func() time.Time
	// Token / TokenExpiresAt seed the JWT (decrypted from the database).
	Token          string
	TokenExpiresAt time.Time
	// OnToken is called after every successful exchange so the caller can persist the JWT.
	OnToken func(token string, expiresAt time.Time)
	// Sleep is overridable for tests.
	Sleep func(ctx context.Context, d time.Duration) error
}

type Client struct {
	base    string
	userID  int64
	key     string
	meter   apimeter.Meter
	hc      *http.Client
	log     *slog.Logger
	now     func() time.Time
	onToken func(string, time.Time)
	sleep   func(context.Context, time.Duration) error

	mu  sync.Mutex
	tok string
	exp time.Time
}

func New(o Options) *Client {
	c := &Client{
		base:    strings.TrimRight(o.BaseURL, "/"),
		userID:  o.UserID,
		key:     strings.TrimSpace(o.APIKey),
		meter:   o.Meter,
		hc:      o.HTTPClient,
		log:     o.Log,
		now:     o.Now,
		onToken: o.OnToken,
		sleep:   o.Sleep,
		tok:     o.Token,
		exp:     o.TokenExpiresAt,
	}
	if c.base == "" {
		c.base = DefaultBaseURL
	}
	if c.meter == nil {
		c.meter = apimeter.Nop()
	}
	if c.hc == nil {
		c.hc = &http.Client{Timeout: requestTimeout}
	}
	if c.log == nil {
		c.log = slog.Default()
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.sleep == nil {
		c.sleep = sleepCtx
	}
	return c
}

// TokenExpiresAt returns the expiry of the cached JWT, if still valid.
func (c *Client) TokenExpiresAt() *time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tok == "" || !c.now().Before(c.exp) {
		return nil
	}
	t := c.exp
	return &t
}

// Token returns a JWT valid for at least one more hour, exchanging the key when needed.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tok != "" && c.exp.Sub(c.now()) > renewBefore {
		return c.tok, nil
	}
	return c.renewLocked(ctx)
}

// Refresh forces a new token exchange (used to validate a key).
func (c *Client) Refresh(ctx context.Context) (time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.renewLocked(ctx); err != nil {
		return time.Time{}, err
	}
	return c.exp, nil
}

func (c *Client) forceRefresh(ctx context.Context, stale string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tok != "" && c.tok != stale && c.exp.Sub(c.now()) > renewBefore {
		return c.tok, nil
	}
	return c.renewLocked(ctx)
}

func (c *Client) renewLocked(ctx context.Context) (string, error) {
	if c.key == "" {
		return "", ErrNoAPIKey
	}
	tok, exp, err := c.exchange(ctx)
	if err != nil {
		return "", err
	}
	c.tok, c.exp = tok, exp
	if c.onToken != nil {
		c.onToken(tok, exp)
	}
	c.log.Info("🎫 Jeton Garmin renouvelé", "user", c.userID, "expire", exp.UTC().Format(time.RFC3339))
	return tok, nil
}

func (c *Client) exchange(ctx context.Context) (string, time.Time, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, backoff(attempt)); err != nil {
				return "", time.Time{}, err
			}
		}
		st, body, err := c.send(ctx, "token", tokenPath, http.MethodPost, tokenPath, func(r *http.Request) {
			r.Header.Set("Accept", "*/*")
			r.Header.Set("Authorization", "ApiKey "+c.key)
		})
		switch {
		case err != nil && retryable(err, st):
			lastErr = err
			continue
		case err != nil:
			return "", time.Time{}, err
		}
		tok := strings.Trim(strings.TrimSpace(string(body)), `"`)
		if tok == "" || strings.Count(tok, ".") != 2 {
			return "", time.Time{}, &APIError{Op: "token", Endpoint: tokenPath, Status: st, Err: errors.New("unexpected response (JWT expected)")}
		}
		exp, ok := jwtExpiry(tok)
		if !ok {
			exp = c.now().Add(24 * time.Hour)
		}
		return tok, exp, nil
	}
	return "", time.Time{}, lastErr
}

func retryable(err error, st int) bool {
	if errors.Is(err, apimeter.ErrCooldown) || errors.Is(err, context.Canceled) {
		return false
	}
	return st == 0 || st >= 500
}

func jwtExpiry(tok string) (time.Time, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == "" {
		return time.Time{}, false
	}
	f, err := claims.Exp.Float64()
	if err != nil || f <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(f), 0), true
}

// send performs one metered HTTP exchange. A non-2xx status is returned as *APIError
// together with the status; the body is only returned on success.
func (c *Client) send(ctx context.Context, op, endpoint, method, pathQ string, hdr func(*http.Request)) (int, []byte, error) {
	uid := c.userID
	if uid == 0 {
		uid = apimeter.UserFrom(ctx)
	}
	release, err := c.meter.Acquire(ctx, apimeter.Garmin, uid)
	if err != nil {
		return 0, nil, err
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, method, c.base+pathQ, nil)
	if err != nil {
		return 0, nil, err
	}
	hdr(req)
	req.Header.Set("User-Agent", userAgent)
	call := apimeter.Call{Provider: apimeter.Garmin, UserID: uid, Endpoint: endpoint}
	start := time.Now()
	resp, err := c.hc.Do(req)
	if err != nil {
		call.Duration, call.Err = time.Since(start), errors.New(sanitizeErr(err))
		c.meter.Observe(call)
		c.log.Warn(fmt.Sprintf("📡 %s %s erreur réseau · %s", method, endpoint, logx.Ms(call.Duration)), "user", uid, "err", sanitizeErr(err))
		return 0, nil, &APIError{Op: op, Endpoint: endpoint, Err: errors.New(sanitizeErr(err))}
	}
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	resp.Body.Close()
	call.Duration, call.Status = time.Since(start), resp.StatusCode
	if resp.StatusCode == http.StatusTooManyRequests {
		call.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	if resp.StatusCode >= 400 {
		call.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	c.meter.Observe(call)
	lvl := slog.LevelInfo
	if resp.StatusCode >= 400 {
		lvl = slog.LevelWarn
	}
	c.log.Log(ctx, lvl, fmt.Sprintf("📡 %s %s %d · %s", method, endpoint, resp.StatusCode, logx.Ms(call.Duration)), "user", uid, "octets", len(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, nil, &APIError{Op: op, Endpoint: endpoint, Status: resp.StatusCode}
	}
	if rerr != nil {
		return resp.StatusCode, nil, &APIError{Op: op, Endpoint: endpoint, Status: resp.StatusCode, Err: rerr}
	}
	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil, nil
	}
	return resp.StatusCode, body, nil
}

// get is an authenticated GET: one forced token refresh on 401, two retries on 5xx/network errors,
// no retry on 429 (the meter starts a cooldown). Empty bodies yield ErrNoData.
func (c *Client) get(ctx context.Context, op, endpoint, pathQ, accept string) ([]byte, error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}
	retried401, retries := false, 0
	for {
		st, body, err := c.send(ctx, op, endpoint, http.MethodGet, pathQ, func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+tok)
			r.Header.Set("Accept", accept)
		})
		switch {
		case err == nil:
			if isEmptyBody(body) {
				return nil, ErrNoData
			}
			return body, nil
		case st == http.StatusUnauthorized && !retried401:
			retried401 = true
			if tok, err = c.forceRefresh(ctx, tok); err != nil {
				return nil, err
			}
		case retryable(err, st) && retries < maxRetries && ctx.Err() == nil:
			retries++
			if err := c.sleep(ctx, backoff(retries)); err != nil {
				return nil, err
			}
		default:
			return nil, err
		}
	}
}

func isEmptyBody(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return s == "" || s == "null" || s == `""`
}

func backoff(attempt int) time.Duration {
	return time.Duration(attempt*attempt) * 500 * time.Millisecond
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// sanitizeErr drops the URL (and its query) from transport errors.
func sanitizeErr(err error) string {
	s := err.Error()
	if i := strings.Index(s, "\": "); i >= 0 && strings.Contains(s[:i], "http") {
		return strings.TrimSpace(s[i+3:])
	}
	return s
}
