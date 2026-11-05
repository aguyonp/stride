package domain

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"stride/internal/apimeter"
	"stride/internal/provider/strava"
)

const (
	settingStravaSeeded = "seed_strava_admin"
	// seededTokenTTL is assumed for an env access token whose expiry is unknown; a rejected token is refreshed at once.
	seededTokenTTL = 6 * time.Hour
)

// scopeList splits a Strava scope string ("read,activity:read_all").
func scopeList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

func parseAthleteID(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func hasScope(scope, want string) bool {
	for _, s := range scopeList(scope) {
		if s == want || (s == want+"_all") {
			return true
		}
	}
	return false
}

// missingScopes lists the scopes Stride asks for (strava.DefaultScope) that the token lacks.
func missingScopes(scope string) []string {
	out := []string{}
	for _, want := range scopeList(strava.DefaultScope) {
		found := false
		for _, s := range scopeList(scope) {
			found = found || s == want
		}
		if !found {
			out = append(out, want)
		}
	}
	return out
}

// withStrava runs fn with a valid access token; a token rejected by Strava (seeded tokens have no
// known expiry) is refreshed once and fn retried.
func (s *Services) withStrava(ctx context.Context, uid int64, fn func(tok string) error) error {
	tok, err := s.stravaToken(ctx, uid)
	if err != nil {
		return err
	}
	if err = fn(tok); !strava.TokenRejected(err) {
		return err
	}
	s.log.Info("🔗 Jeton Strava refusé, renouvellement", "user", uid)
	if _, e := s.db().ExecContext(ctx, `UPDATE provider_connections SET token_expires_at = 0 WHERE user_id = ? AND provider = 'strava'`, uid); e != nil {
		return err
	}
	if tok, err = s.stravaToken(ctx, uid); err != nil {
		return err
	}
	return fn(tok)
}

// probeStravaScopes infers the scopes of a token without introspection: /athlete answers with
// the detailed representation only with profile:read_all; listing activities needs activity:read.
func (s *Services) probeStravaScopes(ctx context.Context, tok string, ath *strava.Athlete) (string, error) {
	scopes := []string{"read"}
	if ath.Detailed {
		scopes = append(scopes, "profile:read_all")
	}
	_, err := s.deps.Strava.ListActivities(ctx, tok, time.Time{}, time.Time{}, 1, 1)
	switch {
	case err == nil:
		scopes = append(scopes, "activity:read")
	case !strava.MissingScope(err):
		return "", err
	}
	return strings.Join(scopes, ","), nil
}

// seedStrava connects Strava for uid from the env tokens (first admin only, once).
func (s *Services) seedStrava(ctx context.Context, uid int64) error {
	at, rt := strings.TrimSpace(s.deps.Env.AdminStravaAccessToken), strings.TrimSpace(s.deps.Env.AdminStravaRefreshToken)
	if at == "" && rt == "" {
		return nil
	}
	if err := s.stravaAvailable(); err != nil {
		return err
	}
	if cn, err := s.getConn(ctx, uid, pStrava); err != nil || cn.connected() {
		return err
	}
	ctx = apimeter.WithUser(ctx, uid)
	tok := strava.Tokens{AccessToken: at, RefreshToken: rt}
	var ath *strava.Athlete
	var err error
	if at != "" {
		ath, err = s.deps.Strava.GetAthlete(ctx, at)
	}
	if (at == "" || strava.TokenRejected(err)) && rt != "" {
		if tok, err = s.deps.Strava.Refresh(ctx, rt); err == nil {
			if tok.RefreshToken == "" {
				tok.RefreshToken = rt
			}
			ath, err = s.deps.Strava.GetAthlete(ctx, tok.AccessToken)
		}
	}
	if err != nil {
		return providerErr(pStrava, err)
	}
	if tok.ExpiresAt.IsZero() {
		tok.ExpiresAt = s.now().Add(seededTokenTTL)
	}
	scope, err := s.probeStravaScopes(ctx, tok.AccessToken, ath)
	if err != nil {
		return providerErr(pStrava, err)
	}
	tok.AthleteID, tok.AthleteName = ath.ID, ath.Name
	if err := s.saveStravaTokens(ctx, uid, tok, scope); err != nil {
		return err
	}
	_ = s.deps.Store.SetSetting(ctx, settingStravaSeeded, "1", s.now())
	s.log.Info("🔗 Strava connecté automatiquement pour le premier administrateur", "user", uid, "scopes", scope)
	if hasScope(scope, "activity:read") {
		s.startSync(uid)
	}
	return nil
}

// SeedFirstAdmin seeds, in the background, the Strava connection of the first admin at startup when
// the env provides tokens, the admin has none and no seed was done yet (a later disconnection is kept).
func (s *Services) SeedFirstAdmin(ctx context.Context) {
	if s.deps.Env.AdminStravaAccessToken == "" && s.deps.Env.AdminStravaRefreshToken == "" {
		return
	}
	var done string
	err := s.db().QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, settingStravaSeeded).Scan(&done)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return
	}
	var uid int64
	if err := s.db().QueryRowContext(ctx, `SELECT id FROM users WHERE role = 'admin' AND status = 'active' ORDER BY id LIMIT 1`).Scan(&uid); err != nil {
		return
	}
	s.goBackground("strava-seed", time.Minute, func(ctx context.Context) {
		if err := s.seedStrava(ctx, uid); err != nil {
			s.log.Warn("🔗 Jetons Strava de l'environnement refusés ou Strava injoignable", "user", uid, "err", errText(err))
		}
	})
}

// StravaStats serves GET /api/integrations/strava/stats (works with the read scope).
func (s *Services) StravaStats(ctx context.Context, uid int64) (*strava.AthleteStats, error) {
	cn, err := s.getConn(ctx, uid, pStrava)
	if err != nil {
		return nil, err
	}
	if !cn.connected() {
		return nil, ErrNotConnected
	}
	ctx = apimeter.WithUser(ctx, uid)
	var out *strava.AthleteStats
	err = s.withStrava(ctx, uid, func(tok string) error {
		id, err := parseAthleteID(cn.ExternalID.String)
		if err != nil {
			ath, err := s.deps.Strava.GetAthlete(ctx, tok)
			if err != nil {
				return err
			}
			id = ath.ID
		}
		out, err = s.deps.Strava.GetAthleteStats(ctx, tok, id)
		return err
	})
	return out, providerErr(pStrava, err)
}
