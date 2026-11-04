// Package garmin est le client de la Garmin Personal API : échange clé → JWT,
// retry/backoff, cache mémoire + disque et méthodes typées par endpoint.
package garmin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"stride/internal/logx"
)

// BaseURL est l'URL de la Garmin Personal API.
const BaseURL = "https://personal.connectapi.garmin.com"

const (
	tokenPath      = "/personalapitoken-service/personal-api/personal-api-key/token"
	userAgent      = "stride/1.0"
	requestTimeout = 30 * time.Second
	renewBefore    = time.Hour
	maxRetries     = 2 // nouvelles tentatives max sur 429/5xx/erreur réseau
	maxBodyBytes   = 64 << 20
)

// ErrNoAPIKey : aucune clé configurée (ni UI ni env).
var ErrNoAPIKey = errors.New("aucune clé API Garmin configurée")

// ErrNoData : réponse vide (204 / corps vide) — pas une erreur, juste pas de donnée.
var ErrNoData = errors.New("aucune donnée Garmin pour cette requête")

// APIError décrit une réponse HTTP non-2xx (Status=0 : erreur réseau / timeout).
type APIError struct {
	Op     string // "token" | "data" | "fit"
	Path   string // chemin sans query
	Status int
	Err    error
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("Garmin %s %s : %v", e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("Garmin %s %s : HTTP %d", e.Op, e.Path, e.Status)
}

func (e *APIError) Unwrap() error { return e.Err }

// IsUnauthorized : clé/jeton refusé par Garmin.
func IsUnauthorized(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && (ae.Status == http.StatusUnauthorized || ae.Status == http.StatusForbidden)
}

// IsNotFound : ressource inexistante.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// IsTimeout : délai dépassé côté Garmin.
func IsTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne interface{ Timeout() bool }
	return errors.As(err, &ne) && ne.Timeout()
}

// KeyFunc retourne la clé API courante ("" si aucune).
type KeyFunc func() string

// Options de construction du client.
type Options struct {
	BaseURL    string
	DataDir    string
	Key        KeyFunc
	Logger     *slog.Logger
	Location   func() *time.Location
	HTTPClient *http.Client
	Now        func() time.Time
}

// Client Garmin Personal API (thread-safe).
type Client struct {
	base    string
	dataDir string
	key     KeyFunc
	log     *slog.Logger
	loc     func() *time.Location
	hc      *http.Client
	now     func() time.Time

	tokMu      sync.Mutex
	tok        *tokenState
	diskLoaded bool

	mem *memCache
	sf  singleflight.Group
}

type tokenState struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	KeySHA256 string    `json:"keySha256"`
}

// New construit un client.
func New(o Options) *Client {
	c := &Client{
		base:    strings.TrimRight(o.BaseURL, "/"),
		dataDir: o.DataDir,
		key:     o.Key,
		log:     o.Logger,
		loc:     o.Location,
		hc:      o.HTTPClient,
		now:     o.Now,
		mem:     newMemCache(),
	}
	if c.base == "" {
		c.base = BaseURL
	}
	if c.log == nil {
		c.log = slog.Default()
	}
	if c.loc == nil {
		c.loc = func() *time.Location { return time.Local }
	}
	if c.hc == nil {
		c.hc = &http.Client{Timeout: requestTimeout}
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.key == nil {
		c.key = func() string { return "" }
	}
	return c
}

// Configured indique si une clé est disponible.
func (c *Client) Configured() bool { return c.key() != "" }

func keyHash(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func (c *Client) tokenFile() string { return filepath.Join(c.dataDir, "token.json") }

// TokenExpiresAt retourne l'expiration du jeton courant s'il correspond à la clé courante.
func (c *Client) TokenExpiresAt() *time.Time {
	key := c.key()
	if key == "" {
		return nil
	}
	c.tokMu.Lock()
	defer c.tokMu.Unlock()
	c.loadDiskTokenLocked()
	if c.tok == nil || c.tok.KeySHA256 != keyHash(key) || !c.now().Before(c.tok.ExpiresAt) {
		return nil
	}
	t := c.tok.ExpiresAt
	return &t
}

// InvalidateToken oublie le jeton (mémoire + disque) et vide le cache mémoire.
func (c *Client) InvalidateToken() {
	c.tokMu.Lock()
	c.tok = nil
	c.diskLoaded = true
	c.tokMu.Unlock()
	_ = os.Remove(c.tokenFile())
	c.mem.clear()
}

// SetToken installe un jeton déjà obtenu (après validation d'une nouvelle clé).
func (c *Client) SetToken(token string, exp time.Time, key string) {
	c.tokMu.Lock()
	defer c.tokMu.Unlock()
	c.tok = &tokenState{Token: token, ExpiresAt: exp, KeySHA256: keyHash(key)}
	c.diskLoaded = true
	c.saveTokenLocked()
}

func (c *Client) loadDiskTokenLocked() {
	if c.diskLoaded {
		return
	}
	c.diskLoaded = true
	b, err := os.ReadFile(c.tokenFile())
	if err != nil {
		return
	}
	var ts tokenState
	if json.Unmarshal(b, &ts) == nil && ts.Token != "" && ts.KeySHA256 != "" {
		c.tok = &ts
	}
}

func (c *Client) saveTokenLocked() {
	if c.tok == nil || c.dataDir == "" {
		return
	}
	b, err := json.Marshal(c.tok)
	if err != nil {
		return
	}
	if err := os.MkdirAll(c.dataDir, 0o700); err == nil {
		if err := writeFileAtomic(c.tokenFile(), b, 0o600); err != nil {
			c.log.Warn("⚠️ Impossible de persister le jeton Garmin", "err", err.Error())
		}
	}
}

// Token retourne un JWT valide (renouvelé s'il reste moins d'une heure).
func (c *Client) Token(ctx context.Context) (string, error) {
	key := c.key()
	if key == "" {
		return "", ErrNoAPIKey
	}
	h := keyHash(key)
	c.tokMu.Lock()
	defer c.tokMu.Unlock()
	c.loadDiskTokenLocked()
	if c.tok != nil && c.tok.KeySHA256 == h && c.tok.ExpiresAt.Sub(c.now()) > renewBefore {
		return c.tok.Token, nil
	}
	return c.renewLocked(ctx, key)
}

// forceRefresh renouvelle le jeton si `stale` est toujours le jeton courant.
func (c *Client) forceRefresh(ctx context.Context, stale string) (string, error) {
	key := c.key()
	if key == "" {
		return "", ErrNoAPIKey
	}
	c.tokMu.Lock()
	defer c.tokMu.Unlock()
	if c.tok != nil && c.tok.Token != stale && c.tok.KeySHA256 == keyHash(key) {
		return c.tok.Token, nil
	}
	return c.renewLocked(ctx, key)
}

func (c *Client) renewLocked(ctx context.Context, key string) (string, error) {
	tok, exp, err := c.Exchange(ctx, key)
	if err != nil {
		return "", err
	}
	c.tok = &tokenState{Token: tok, ExpiresAt: exp, KeySHA256: keyHash(key)}
	c.saveTokenLocked()
	c.log.Info("🎫 Jeton Garmin renouvelé", "expire", exp.In(c.loc()).Format("2006-01-02 15:04 MST"))
	return tok, nil
}

// Exchange échange une clé API contre un JWT (sans modifier l'état du client).
func (c *Client) Exchange(ctx context.Context, key string) (string, time.Time, error) {
	if strings.TrimSpace(key) == "" {
		return "", time.Time{}, ErrNoAPIKey
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoff(attempt, 0)); err != nil {
				return "", time.Time{}, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+tokenPath, nil)
		if err != nil {
			return "", time.Time{}, err
		}
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Authorization", "ApiKey "+key)
		req.Header.Set("User-Agent", userAgent)
		start := time.Now()
		resp, err := c.hc.Do(req)
		if err != nil {
			c.log.Warn("📡 POST "+tokenPath+" erreur réseau", "durée", logx.Ms(time.Since(start)), "err", sanitizeErr(err))
			lastErr = &APIError{Op: "token", Path: tokenPath, Err: errors.New(sanitizeErr(err))}
			if ctx.Err() != nil {
				return "", time.Time{}, lastErr
			}
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		c.log.Info(fmt.Sprintf("📡 POST %s %d · %s", tokenPath, resp.StatusCode, logx.Ms(time.Since(start))))
		switch {
		case resp.StatusCode == http.StatusOK:
			tok := strings.TrimSpace(string(body))
			tok = strings.Trim(tok, `"`)
			if tok == "" || strings.Count(tok, ".") != 2 {
				return "", time.Time{}, &APIError{Op: "token", Path: tokenPath, Status: resp.StatusCode, Err: errors.New("réponse inattendue (JWT attendu)")}
			}
			exp, ok := jwtExpiry(tok)
			if !ok {
				exp = c.now().Add(24 * time.Hour)
			}
			return tok, exp, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = &APIError{Op: "token", Path: tokenPath, Status: resp.StatusCode}
			continue
		default:
			return "", time.Time{}, &APIError{Op: "token", Path: tokenPath, Status: resp.StatusCode}
		}
	}
	return "", time.Time{}, lastErr
}

// jwtExpiry lit le claim exp du payload (base64url) sans vérifier la signature.
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

// get effectue un GET authentifié avec retry 401 (après refresh forcé) et backoff 429/5xx.
// pathQ inclut la query. Retourne le corps (vide si 204).
func (c *Client) get(ctx context.Context, pathQ, accept, op string) ([]byte, error) {
	path := pathQ
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	tok, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}
	retried401 := false
	retries := 0
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+pathQ, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", accept)
		req.Header.Set("User-Agent", userAgent)
		start := time.Now()
		resp, err := c.hc.Do(req)
		if err != nil {
			dur := time.Since(start)
			c.log.Warn(fmt.Sprintf("📡 GET %s erreur réseau · %s", pathQ, logx.Ms(dur)), "err", sanitizeErr(err))
			if ctx.Err() == nil && retries < maxRetries {
				retries++
				if err := sleepCtx(ctx, backoff(retries, 0)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, &APIError{Op: op, Path: path, Err: errors.New(sanitizeErr(err))}
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		resp.Body.Close()
		dur := time.Since(start)
		lvl := slog.LevelInfo
		if resp.StatusCode >= 400 {
			lvl = slog.LevelWarn
		}
		c.log.Log(ctx, lvl, fmt.Sprintf("📡 GET %s %d · %s", pathQ, resp.StatusCode, logx.Ms(dur)), "octets", len(body))
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if rerr != nil {
				return nil, &APIError{Op: op, Path: path, Status: resp.StatusCode, Err: rerr}
			}
			return body, nil
		case resp.StatusCode == http.StatusUnauthorized && !retried401:
			retried401 = true
			c.log.Info("🎫 401 reçu : renouvellement forcé du jeton")
			tok, err = c.forceRefresh(ctx, tok)
			if err != nil {
				return nil, err
			}
			continue
		case (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && retries < maxRetries:
			retries++
			wait := backoff(retries, parseRetryAfter(resp.Header.Get("Retry-After")))
			c.log.Warn(fmt.Sprintf("⚠️ Garmin %d sur %s : nouvelle tentative dans %s", resp.StatusCode, path, logx.Ms(wait)))
			if err := sleepCtx(ctx, wait); err != nil {
				return nil, err
			}
			continue
		default:
			return nil, &APIError{Op: op, Path: path, Status: resp.StatusCode}
		}
	}
}

func backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > 5*time.Second {
			return 5 * time.Second
		}
		return retryAfter
	}
	d := time.Duration(attempt*attempt) * 400 * time.Millisecond // 400ms, 1.6s
	return d
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && s >= 0 {
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

// sanitizeErr retire l'URL complète des erreurs réseau (aucun secret n'y figure,
// mais on garde des logs courts et sans query).
func sanitizeErr(err error) string {
	s := err.Error()
	if i := strings.Index(s, "\": "); i >= 0 && strings.Contains(s[:i], "http") {
		return strings.TrimSpace(s[i+3:])
	}
	return s
}
