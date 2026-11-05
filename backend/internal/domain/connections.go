package domain

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"stride/internal/apimeter"
	"stride/internal/garmin"
	"stride/internal/provider/strava"
	"stride/internal/view"
)

const (
	pGarmin = "garmin"
	pStrava = "strava"
)

type conn struct {
	UserID         int64
	Provider       string
	Status         string
	ExternalID     sql.NullString
	DisplayName    sql.NullString
	KeyHint        sql.NullString
	SecretEnc      []byte
	TokenEnc       []byte
	TokenExpiresAt sql.NullInt64
	Scope          sql.NullString
	HistoryFrom    sql.NullString
	LastSyncAt     sql.NullInt64
	LastLiveAt     sql.NullInt64
	LastNew        int
	LastError      sql.NullString
	LastErrorAt    sql.NullInt64
}

const connCols = `user_id, provider, status, external_id, display_name, key_hint, secret_enc, token_enc, token_expires_at, scope,
	history_from, last_sync_at, last_live_at, last_new_activities, last_error, last_error_at`

func scanConn(sc interface{ Scan(...any) error }) (*conn, error) {
	c := &conn{}
	err := sc.Scan(&c.UserID, &c.Provider, &c.Status, &c.ExternalID, &c.DisplayName, &c.KeyHint, &c.SecretEnc, &c.TokenEnc,
		&c.TokenExpiresAt, &c.Scope, &c.HistoryFrom, &c.LastSyncAt, &c.LastLiveAt, &c.LastNew, &c.LastError, &c.LastErrorAt)
	return c, err
}

// getConn returns nil (no error) when the user has no connection for provider.
func (s *Services) getConn(ctx context.Context, uid int64, provider string) (*conn, error) {
	c, err := scanConn(s.db().QueryRowContext(ctx, `SELECT `+connCols+` FROM provider_connections WHERE user_id = ? AND provider = ?`, uid, provider))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

func (c *conn) connected() bool {
	return c != nil && c.Status != "disconnected" && len(c.SecretEnc) > 0
}

func (s *Services) saveConn(ctx context.Context, c *conn) error {
	now := s.now().Unix()
	_, err := s.db().ExecContext(ctx, `INSERT INTO provider_connections (`+connCols+`, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, provider) DO UPDATE SET status = excluded.status, external_id = excluded.external_id,
		display_name = excluded.display_name, key_hint = excluded.key_hint, secret_enc = excluded.secret_enc,
		token_enc = excluded.token_enc, token_expires_at = excluded.token_expires_at, scope = excluded.scope,
		history_from = excluded.history_from, last_sync_at = excluded.last_sync_at, last_live_at = excluded.last_live_at,
		last_new_activities = excluded.last_new_activities, last_error = excluded.last_error, last_error_at = excluded.last_error_at,
		updated_at = excluded.updated_at`,
		c.UserID, c.Provider, c.Status, c.ExternalID, c.DisplayName, c.KeyHint, c.SecretEnc, c.TokenEnc, c.TokenExpiresAt, c.Scope,
		c.HistoryFrom, c.LastSyncAt, c.LastLiveAt, c.LastNew, c.LastError, c.LastErrorAt, now, now)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return &ConflictError{Msg: "Ce compte est déjà associé à un autre utilisateur Stride."}
	}
	return err
}

func (s *Services) setConnError(ctx context.Context, uid int64, provider string, err error) {
	msg := errText(err)
	status := "error"
	_, _ = s.db().ExecContext(ctx, `UPDATE provider_connections SET status = ?, last_error = ?, last_error_at = ?, updated_at = ? WHERE user_id = ? AND provider = ? AND status <> 'disconnected'`,
		status, msg, s.now().Unix(), s.now().Unix(), uid, provider)
}

func aad(provider string, uid int64, field string) string {
	return fmt.Sprintf("%s:%d:%s", provider, uid, field)
}

func (s *Services) seal(provider string, uid int64, field, value string) ([]byte, error) {
	return s.deps.Sealer.SealString(value, aad(provider, uid, field))
}

func (s *Services) open(provider string, uid int64, field string, sealed []byte) (string, error) {
	if len(sealed) == 0 {
		return "", nil
	}
	return s.deps.Sealer.OpenString(sealed, aad(provider, uid, field))
}

// keyHint shows the last 4 characters of keys of at least 12 characters.
func keyHint(key string) string {
	r := []rune(key)
	if len(r) >= 12 {
		return "••••" + string(r[len(r)-4:])
	}
	return "••••"
}

// ---------- Garmin ----------

func (s *Services) dropClient(uid int64) {
	s.mu.Lock()
	delete(s.clients, uid)
	s.mu.Unlock()
}

func (s *Services) newGarminClient(uid int64, key, tok string, exp time.Time, onTok func(string, time.Time)) *garmin.Client {
	return garmin.New(garmin.Options{BaseURL: s.deps.Env.GarminBaseURL, UserID: uid, APIKey: key, Meter: s.deps.Meter,
		Log: s.log, Now: s.Now, Token: tok, TokenExpiresAt: exp, OnToken: onTok})
}

// garminClient returns the per-user client (key and JWT decrypted from the database).
func (s *Services) garminClient(ctx context.Context, uid int64) (*garmin.Client, error) {
	s.mu.Lock()
	c := s.clients[uid]
	s.mu.Unlock()
	if c != nil {
		return c, nil
	}
	cn, err := s.getConn(ctx, uid, pGarmin)
	if err != nil {
		return nil, err
	}
	if !cn.connected() {
		return nil, ErrNotConnected
	}
	key, err := s.open(pGarmin, uid, "api_key", cn.SecretEnc)
	if err != nil {
		return nil, fmt.Errorf("garmin key: %w", err)
	}
	tok, _ := s.open(pGarmin, uid, "jwt", cn.TokenEnc)
	var exp time.Time
	if cn.TokenExpiresAt.Valid && tok != "" {
		exp = time.Unix(cn.TokenExpiresAt.Int64, 0)
	} else {
		tok = ""
	}
	c = s.newGarminClient(uid, key, tok, exp, func(t string, e time.Time) { s.storeGarminToken(uid, t, e) })
	s.mu.Lock()
	if existing := s.clients[uid]; existing != nil {
		c = existing
	} else {
		s.clients[uid] = c
	}
	s.mu.Unlock()
	return c, nil
}

func (s *Services) storeGarminToken(uid int64, tok string, exp time.Time) {
	sealed, err := s.seal(pGarmin, uid, "jwt", tok)
	if err != nil {
		s.log.Error("❌ Chiffrement du jeton Garmin impossible", "user", uid, "err", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.db().ExecContext(ctx, `UPDATE provider_connections SET token_enc = ?, token_expires_at = ?, updated_at = ? WHERE user_id = ? AND provider = 'garmin'`,
		sealed, exp.Unix(), s.now().Unix(), uid); err != nil {
		s.log.Warn("⚠️ Jeton Garmin non persisté", "user", uid, "err", err.Error())
	}
}

// ConnectGarmin validates a personal API key by token exchange and stores it encrypted.
func (s *Services) ConnectGarmin(ctx context.Context, uid int64, apiKey string) (*view.Integrations, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" || len(key) > 4096 {
		return nil, badRequest("Clé API Garmin requise.")
	}
	ctx = apimeter.WithUser(ctx, uid)
	var tok string
	var exp time.Time
	c := s.newGarminClient(uid, key, "", time.Time{}, func(t string, e time.Time) { tok, exp = t, e })
	if _, err := c.Refresh(ctx); err != nil {
		s.log.Warn("🔑 Clé Garmin refusée ou Garmin injoignable", "user", uid, "err", errText(providerErr(pGarmin, err)))
		return nil, providerErr(pGarmin, err)
	}
	sk, err := s.seal(pGarmin, uid, "api_key", key)
	if err != nil {
		return nil, err
	}
	st, err := s.seal(pGarmin, uid, "jwt", tok)
	if err != nil {
		return nil, err
	}
	cn, err := s.getConn(ctx, uid, pGarmin)
	if err != nil {
		return nil, err
	}
	if cn == nil {
		cn = &conn{UserID: uid, Provider: pGarmin}
	}
	cn.Status, cn.SecretEnc, cn.TokenEnc = "ok", sk, st
	cn.KeyHint = sql.NullString{String: keyHint(key), Valid: true}
	cn.TokenExpiresAt = sql.NullInt64{Int64: exp.Unix(), Valid: true}
	cn.LastError, cn.LastErrorAt = sql.NullString{}, sql.NullInt64{}
	if err := s.saveConn(ctx, cn); err != nil {
		return nil, err
	}
	s.dropClient(uid)
	s.log.Info("🔑 Garmin connecté", "user", uid, "jetonExpire", exp.UTC().Format(time.RFC3339))
	s.startSync(uid)
	return s.Integrations(ctx, uid)
}

// TestGarmin re-validates the stored key.
func (s *Services) TestGarmin(ctx context.Context, uid int64) (*view.Integrations, error) {
	c, err := s.garminClient(ctx, uid)
	if err != nil {
		return nil, err
	}
	if _, err := c.Refresh(apimeter.WithUser(ctx, uid)); err != nil {
		perr := providerErr(pGarmin, err)
		s.setConnError(ctx, uid, pGarmin, perr)
		return nil, perr
	}
	_, _ = s.db().ExecContext(ctx, `UPDATE provider_connections SET status = 'ok', last_error = NULL, last_error_at = NULL WHERE user_id = ? AND provider = 'garmin'`, uid)
	return s.Integrations(ctx, uid)
}

func (s *Services) DisconnectGarmin(ctx context.Context, uid int64) (*view.Integrations, error) {
	if _, err := s.db().ExecContext(ctx, `DELETE FROM provider_connections WHERE user_id = ? AND provider = 'garmin'`, uid); err != nil {
		return nil, err
	}
	s.dropClient(uid)
	_ = s.PurgeUserCache(ctx, uid)
	s.log.Info("🔑 Garmin déconnecté", "user", uid)
	return s.Integrations(ctx, uid)
}

func (s *Services) Integrations(ctx context.Context, uid int64) (*view.Integrations, error) {
	out := &view.Integrations{}
	g, err := s.getConn(ctx, uid, pGarmin)
	if err != nil {
		return nil, err
	}
	out.Garmin.Status = "disconnected"
	if g != nil {
		out.Garmin.Connected = g.connected()
		out.Garmin.Status = g.Status
		if g.KeyHint.Valid {
			out.Garmin.KeyHint = &g.KeyHint.String
		}
		if g.TokenExpiresAt.Valid && g.TokenExpiresAt.Int64 > s.now().Unix() {
			out.Garmin.TokenExpiresAt = rfc3339(g.TokenExpiresAt.Int64)
		}
		out.Garmin.LastSyncAt = rfc3339(g.LastSyncAt.Int64)
		if g.LastError.Valid {
			out.Garmin.LastError = &g.LastError.String
		}
	}
	st, err := s.getConn(ctx, uid, pStrava)
	if err != nil {
		return nil, err
	}
	out.Strava.Available = s.deps.Strava != nil && s.deps.Env.StravaEnabled()
	out.Strava.Status = "disconnected"
	out.Strava.Scopes, out.Strava.MissingScopes = []string{}, []string{}
	if st.connected() && st.Scope.Valid {
		out.Strava.Scopes, out.Strava.MissingScopes = scopeList(st.Scope.String), missingScopes(st.Scope.String)
	}
	if st != nil {
		out.Strava.Connected = st.connected()
		out.Strava.Status = st.Status
		if st.DisplayName.Valid {
			out.Strava.AthleteName = &st.DisplayName.String
		}
		out.Strava.LastSyncAt = rfc3339(st.LastSyncAt.Int64)
		if st.LastError.Valid {
			out.Strava.LastError = &st.LastError.String
		}
	}
	out.Weather.Enabled = s.deps.Weather != nil
	out.Weather.Provider = s.deps.Env.WeatherProvider
	if !out.Weather.Enabled {
		out.Weather.Provider = "off"
	}
	return out, nil
}

// ---------- Strava ----------

const stravaStateTTL = 10 * time.Minute

type oauthState struct {
	UserID int64  `json:"u"`
	Return string `json:"r"`
}

func stateKey(state string) string { return "stride:cache:oauth:strava:" + state }

func (s *Services) stravaAvailable() error {
	if s.deps.Strava == nil || !s.deps.Env.StravaEnabled() {
		return ErrStravaDisabled
	}
	return nil
}

func (s *Services) stravaRedirectURI() string {
	return s.deps.Env.PublicURL + "/api/integrations/strava/callback"
}

// SafeReturnPath keeps only same-site relative paths.
func SafeReturnPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") {
		return "/"
	}
	return p
}

// StravaConnectURL creates an OAuth state bound to the user and returns the authorize URL.
func (s *Services) StravaConnectURL(ctx context.Context, uid int64, returnPath string) (string, error) {
	if err := s.stravaAvailable(); err != nil {
		return "", err
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)
	val, _ := json.Marshal(oauthState{UserID: uid, Return: SafeReturnPath(returnPath)})
	if err := s.deps.Cache.Set(ctx, stateKey(state), val, stravaStateTTL); err != nil {
		return "", err
	}
	return s.deps.Strava.AuthorizeURL(state, s.stravaRedirectURI()), nil
}

// StravaCallback completes the OAuth flow. It always returns the return path (defaults to "/")
// and a short reason code on failure.
func (s *Services) StravaCallback(ctx context.Context, uid int64, state, code, scope, oauthErr string) (string, string) {
	ret := "/"
	if state == "" {
		return ret, "state"
	}
	raw, ok := s.cacheGet(ctx, stateKey(state))
	_ = s.deps.Cache.Del(ctx, stateKey(state))
	var st oauthState
	if !ok || json.Unmarshal(raw, &st) != nil || st.UserID != uid {
		return ret, "state"
	}
	ret = st.Return
	if err := s.stravaAvailable(); err != nil {
		return ret, "disabled"
	}
	if oauthErr != "" || code == "" {
		return ret, "denied"
	}
	if !strings.Contains(scope, "activity:read") {
		return ret, "scope"
	}
	ctx = apimeter.WithUser(ctx, uid)
	tok, err := s.deps.Strava.Exchange(ctx, code)
	if err != nil {
		s.log.Warn("🔗 Échange OAuth Strava refusé", "user", uid, "err", errText(providerErr(pStrava, err)))
		return ret, "exchange"
	}
	if tok.Scope != "" {
		scope = tok.Scope
	}
	if err := s.saveStravaTokens(ctx, uid, tok, scope); err != nil {
		var ce *ConflictError
		if errors.As(err, &ce) {
			return ret, "already_linked"
		}
		s.log.Error("❌ Enregistrement de la connexion Strava impossible", "user", uid, "err", err.Error())
		return ret, "internal"
	}
	s.log.Info("🔗 Strava connecté", "user", uid)
	s.ensureStravaSubscription()
	s.startSync(uid)
	return ret, ""
}

func (s *Services) saveStravaTokens(ctx context.Context, uid int64, tok strava.Tokens, scope string) error {
	at, err := s.seal(pStrava, uid, "access_token", tok.AccessToken)
	if err != nil {
		return err
	}
	rt, err := s.seal(pStrava, uid, "refresh_token", tok.RefreshToken)
	if err != nil {
		return err
	}
	cn, err := s.getConn(ctx, uid, pStrava)
	if err != nil {
		return err
	}
	if cn == nil {
		cn = &conn{UserID: uid, Provider: pStrava}
	}
	cn.Status, cn.SecretEnc, cn.TokenEnc = "ok", rt, at
	cn.TokenExpiresAt = sql.NullInt64{Int64: tok.ExpiresAt.Unix(), Valid: true}
	if tok.AthleteID != 0 {
		cn.ExternalID = sql.NullString{String: strconv.FormatInt(tok.AthleteID, 10), Valid: true}
	}
	if tok.AthleteName != "" {
		cn.DisplayName = sql.NullString{String: tok.AthleteName, Valid: true}
	}
	if scope != "" {
		cn.Scope = sql.NullString{String: scope, Valid: true}
	}
	cn.LastError, cn.LastErrorAt = sql.NullString{}, sql.NullInt64{}
	return s.saveConn(ctx, cn)
}

// stravaToken returns a valid access token, refreshing it 5 minutes before expiry.
func (s *Services) stravaToken(ctx context.Context, uid int64) (string, error) {
	if err := s.stravaAvailable(); err != nil {
		return "", ErrNotConnected
	}
	v, err, _ := s.sf.Do(fmt.Sprintf("strava-token:%d", uid), func() (any, error) {
		cn, err := s.getConn(ctx, uid, pStrava)
		if err != nil {
			return "", err
		}
		if !cn.connected() {
			return "", ErrNotConnected
		}
		at, err := s.open(pStrava, uid, "access_token", cn.TokenEnc)
		if err != nil {
			return "", err
		}
		if cn.TokenExpiresAt.Valid && time.Unix(cn.TokenExpiresAt.Int64, 0).Sub(s.now()) > 5*time.Minute && at != "" {
			return at, nil
		}
		rt, err := s.open(pStrava, uid, "refresh_token", cn.SecretEnc)
		if err != nil {
			return "", err
		}
		tok, err := s.deps.Strava.Refresh(apimeter.WithUser(ctx, uid), rt)
		if err != nil {
			perr := providerErr(pStrava, err)
			s.setConnError(ctx, uid, pStrava, perr)
			return "", perr
		}
		if tok.RefreshToken == "" {
			tok.RefreshToken = rt
		}
		if err := s.saveStravaTokens(ctx, uid, tok, cn.Scope.String); err != nil {
			return "", err
		}
		return tok.AccessToken, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (s *Services) DisconnectStrava(ctx context.Context, uid int64) (*view.Integrations, error) {
	if s.stravaAvailable() == nil {
		if cn, _ := s.getConn(ctx, uid, pStrava); cn.connected() {
			if at, err := s.open(pStrava, uid, "access_token", cn.TokenEnc); err == nil && at != "" {
				if err := s.deps.Strava.Deauthorize(apimeter.WithUser(ctx, uid), at); err != nil {
					s.log.Warn("🔗 Révocation Strava en échec (connexion supprimée quand même)", "user", uid, "err", errText(providerErr(pStrava, err)))
				}
			}
		}
	}
	if _, err := s.db().ExecContext(ctx, `DELETE FROM provider_connections WHERE user_id = ? AND provider = 'strava'`, uid); err != nil {
		return nil, err
	}
	_ = s.PurgeUserCache(ctx, uid)
	s.log.Info("🔗 Strava déconnecté", "user", uid)
	return s.Integrations(ctx, uid)
}

func (s *Services) ensureStravaSubscription() {
	if s.stravaAvailable() != nil || s.deps.Env.StravaWebhookToken == "" {
		return
	}
	s.subsOnce.Do(func() {
		s.goBackground("strava-subscription", time.Minute, func(ctx context.Context) {
			if _, err := s.deps.Strava.EnsureSubscription(ctx, s.deps.Env.PublicURL+"/api/webhooks/strava", s.deps.Env.StravaWebhookToken); err != nil {
				s.log.Warn("🔗 Abonnement webhook Strava non créé", "err", errText(providerErr(pStrava, err)))
			}
		})
	})
}

// VerifyStravaWebhook answers the subscription validation (echo hub.challenge).
func (s *Services) VerifyStravaWebhook(mode, token, challenge string) (string, bool) {
	want := s.deps.Env.StravaWebhookToken
	if want == "" || mode != "subscribe" || subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 || challenge == "" {
		return "", false
	}
	return challenge, true
}
