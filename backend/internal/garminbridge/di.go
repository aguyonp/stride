package garminbridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The native DI OAuth2 exchange the Garmin Connect apps perform today. The OAuth1 dance this gateway
// used to implement (consumer-signed preauthorized → user-signed exchange/user/2.0) is refused now:
// the token endpoint wants the service ticket plus the client id it was issued for.
//
//	POST https://diauth.garmin.com/di-oauth2-service/oauth/token
//	Authorization: Basic base64("<client_id>:")
//	client_id, service_ticket, grant_type, service_url
//
// The answer is a Bearer JWT, refreshed with grant_type=refresh_token and the client id that issued
// it — which is why both are stored together.

const (
	// diGrant is the grant type of a service-ticket exchange.
	diGrant = "https://connectapi.garmin.com/di-oauth2-service/oauth/grant/service_ticket"
	// diAccept is what the native clients ask for: JSON first, HTML tolerated.
	diAccept = "application/json,text/html;q=0.9,*/*;q=0.8"
	// diRefreshAccept is narrower, like the refresh call of the native clients.
	diRefreshAccept = "application/json"
	// diRefreshPrefix marks a RefreshToken as a DI pair rather than a service ticket.
	diRefreshPrefix = "di:"
)

// diClientIDs are the ids the native apps present, newest first: a ticket issued today is bound to
// the current generation, an older one to its predecessor.
var diClientIDs = []string{
	"GARMIN_CONNECT_MOBILE_ANDROID_DI_2025Q2",
	"GARMIN_CONNECT_MOBILE_ANDROID_DI_2024Q4",
	"GARMIN_CONNECT_MOBILE_ANDROID_DI",
	"GARMIN_CONNECT_MOBILE_IOS_DI",
}

// diToken posts to the token endpoint on behalf of one client id.
func (s *Service) diToken(ctx context.Context, clientID, accept string, form url.Values) (map[string]any, int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.opt.DIBase+"/di-oauth2-service/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(clientID+":")))
	req.Header.Set("Accept", accept)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", oauthUserAgent)
	res, err := s.client.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := readBody(res)
	if err != nil {
		return nil, res.StatusCode, nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, res.StatusCode, body, nil
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, res.StatusCode, body, nil
	}
	return out, res.StatusCode, body, nil
}

// loginDI turns a service ticket into tokens, trying the client ids the apps use. The service_url is
// the one the ticket was issued for: Garmin binds them, and a mismatch is a 401.
func (s *Service) loginDI(ctx context.Context, ticket string) (*Tokens, error) {
	var lastStatus int
	var lastBody []byte
	for _, clientID := range diClientIDs {
		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("service_ticket", ticket)
		form.Set("grant_type", diGrant)
		form.Set("service_url", s.opt.Service)
		out, status, body, err := s.diToken(ctx, clientID, diAccept, form)
		if err != nil {
			return nil, err
		}
		if out == nil {
			lastStatus, lastBody = status, body
			if status == http.StatusTooManyRequests || status == 427 {
				return nil, fmt.Errorf("%w (jetons HTTP %d)", ErrBlocked, status)
			}
			continue
		}
		tok, err := tokensFromDI(s.opt.Now(), out, clientID)
		if err != nil {
			return nil, err
		}
		return tok, nil
	}
	s.log().Warn("🔗 Pont Garmin : jeton DI refusé", "status", lastStatus, "reponse", excerpt(lastBody))
	return nil, fmt.Errorf("%w (jetons HTTP %d)", ErrCredentials, lastStatus)
}

// refreshDI renews the Bearer token with the client id that issued it.
func (s *Service) refreshDI(ctx context.Context, pair string) (*Tokens, error) {
	clientID, refresh, ok := strings.Cut(pair, "|")
	if !ok || clientID == "" || refresh == "" {
		return nil, ErrExpired
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("refresh_token", refresh)
	out, status, body, err := s.diToken(ctx, clientID, diRefreshAccept, form)
	if err != nil {
		return nil, err
	}
	if out == nil {
		s.log().Warn("🔗 Pont Garmin : rafraîchissement refusé", "status", status, "reponse", excerpt(body))
		if status == http.StatusTooManyRequests || status == 427 {
			return nil, ErrBlocked
		}
		return nil, ErrExpired
	}
	return tokensFromDI(s.opt.Now(), out, clientID)
}

// tokensFromDI reads the answer, keeping the client id with the refresh token: a refresh with
// another id is refused.
func tokensFromDI(now time.Time, out map[string]any, clientID string) (*Tokens, error) {
	access, _ := out["access_token"].(string)
	if access == "" {
		return nil, ErrCredentials
	}
	refresh, _ := out["refresh_token"].(string)
	exp := int64(0)
	switch v := out["expires_in"].(type) {
	case float64:
		exp = int64(v)
	case string:
		_, _ = fmt.Sscan(v, &exp)
	}
	if exp <= 0 {
		exp = int64(defaultTokenLifetime / time.Second)
	}
	return &Tokens{
		AccessToken:  access,
		RefreshToken: clientID + "|" + refresh,
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Duration(exp) * time.Second),
	}, nil
}
