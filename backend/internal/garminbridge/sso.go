package garminbridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The sign-in flow, as the Garmin Connect apps perform it: an SSO form on sso.garmin.com, a service
// ticket, then an OAuth1 → OAuth2 exchange on connectapi. No password is ever kept: it lives in the
// closure of one POST.
//
// Every shape below (CSRF field, ticket, MFA page, ticket exchange) is parsed defensively and the
// tests pin them against fixtures. A real account is the only way to confirm the live HTML, which is
// why the bridge ships off by default and keeps the .FIT fallback.

var (
	// ticketRE reads the service ticket from the sign-in answer or its redirect URL.
	ticketRE = regexp.MustCompile(`ticket=(ST-[A-Za-z0-9_.\-]{4,})`)
	// csrfRE reads the hidden field of the form.
	csrfRE = regexp.MustCompile(`name="_csrf"\s+value="([^"]+)"`)
	// responseURLRE reads the script variable a successful sign-in embeds.
	responseURLRE = regexp.MustCompile(`response_url\s*=\s*"([^"]+)"`)
	// mfaMarkers say "Garmin wants a code", across the widget and the mobile variants.
	mfaMarkers = []string{"verifyMFA", "MFA_REQUIRED", "mfa-code", "enterMfaCode", "mfaVerification"}
	// blockedMarkers are the anti-bot pages that answer 200 with a CAPTCHA.
	blockedMarkers = []string{"captcha", "unusual traffic", "too many requests"}
)

// LoginResult is what Login/LoginMFA return: the state reached and, when connected, the profile.
type LoginResult struct {
	State       string
	DisplayName string
}

// Login signs in with the account credentials. On success the tokens are stored; when Garmin asks for
// a code, ErrMFARequired is returned and the SSO session is kept in memory for MFATTL.
func (s *Service) Login(ctx context.Context, userID int64, email, password string) (LoginResult, error) {
	if strings.TrimSpace(email) == "" || password == "" {
		return LoginResult{State: StateDisconnected}, ErrCredentials
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return LoginResult{State: StateDisconnected}, err
	}
	cli := *s.client
	cli.Jar = jar
	sess := &session{cli: &cli, jar: jar}

	form, err := s.signinForm(ctx, sess)
	if err != nil {
		return LoginResult{State: StateDisconnected}, err
	}
	// The password only travels in this request body, and is dropped with it.
	form.Set("username", strings.TrimSpace(email))
	form.Set("password", password)
	res, body, err := s.post(ctx, sess, s.opt.SSOBase+"/sso/signin", form)
	if err != nil {
		return LoginResult{State: StateDisconnected}, err
	}
	return s.finishLogin(ctx, userID, sess, res, body, form.Get("_csrf"))
}

// LoginMFA completes a sign-in that Garmin challenged with a code.
func (s *Service) LoginMFA(ctx context.Context, userID int64, code string) (LoginResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return LoginResult{State: StateMFA}, ErrCredentials
	}
	p, ok := s.takePending(userID)
	if !ok {
		// The pending session expired (or the server restarted): start over.
		return LoginResult{State: StateDisconnected}, ErrExpired
	}
	form := url.Values{}
	form.Set("mfa-code", code)
	form.Set("_csrf", p.csrf)
	form.Set("embed", "false")
	form.Set("fromPage", "setupEnterMfaCode")
	for k, vs := range p.params {
		if form.Get(k) == "" {
			form.Set(k, vs[0])
		}
	}
	res, body, err := s.post(ctx, p.sess, s.opt.SSOBase+"/sso/verifyMFA/loginEnterMfaCode", form)
	if err != nil {
		// Keep the session: a wrong code is worth another try.
		s.keepPending(userID, p)
		return LoginResult{State: StateMFA}, err
	}
	return s.finishLogin(ctx, userID, p.sess, res, body, p.csrf)
}

// session is one sign-in: its own cookie jar, so two users never share a cookie.
type session struct {
	cli *http.Client
	jar http.CookieJar
}

// signinForm loads the form and its CSRF token.
func (s *Service) signinForm(ctx context.Context, sess *session) (url.Values, error) {
	params := signinParams(s.opt)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.opt.SSOBase+"/sso/signin?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	s.headers(req)
	res, err := sess.cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := readBody(res)
	if err != nil {
		return nil, err
	}
	if err := classify(res.StatusCode, body); err != nil {
		return nil, err
	}
	m := csrfRE.FindSubmatch(body)
	if m == nil {
		return nil, ErrCredentials
	}
	form := signinParams(s.opt)
	form.Set("_csrf", string(m[1]))
	form.Set("embed", "false")
	return form, nil
}

// finishLogin reads the answer of the final password step (or of the MFA step).
func (s *Service) finishLogin(ctx context.Context, userID int64, sess *session, res *http.Response, body []byte, csrf string) (LoginResult, error) {
	if err := classify(res.StatusCode, body); err != nil {
		if err == ErrMFARequired {
			s.keepPending(userID, &pendingLogin{sess: sess, csrf: csrf, params: signinParams(s.opt)})
		}
		return LoginResult{State: stateOfError(err)}, err
	}
	if !isMFA(body) && !ticketRE.Match(body) && !responseURLRE.Match(body) {
		// Neither a code challenge nor a ticket: the credentials were refused.
		return LoginResult{State: StateDisconnected}, ErrCredentials
	}
	ticket, err := s.serviceTicket(ctx, sess, body)
	if err != nil {
		s.keepPending(userID, &pendingLogin{sess: sess, csrf: csrf, params: signinParams(s.opt)})
		return LoginResult{State: stateOfError(err)}, err
	}
	s.dropPending(userID)
	tok, err := s.exchange(ctx, ticket, "connexion")
	if err != nil {
		return LoginResult{State: StateDisconnected}, err
	}
	name, err := s.verify(ctx, tok)
	if err != nil {
		return LoginResult{State: StateDisconnected}, err
	}
	tok.DisplayName = name
	if err := s.opt.Store.Save(ctx, userID, *tok); err != nil {
		return LoginResult{State: StateDisconnected}, err
	}
	s.log().Info("🔗 Pont Garmin connecté", "user", userID)
	return LoginResult{State: StateConnected, DisplayName: name}, nil
}

// serviceTicket follows the response URL and reads the ticket from where it lands.
func (s *Service) serviceTicket(ctx context.Context, sess *session, body []byte) (string, error) {
	if m := ticketRE.FindSubmatch(body); m != nil {
		return string(m[1]), nil
	}
	m := responseURLRE.FindSubmatch(body)
	if m == nil {
		return "", ErrCredentials
	}
	target := strings.ReplaceAll(string(m[1]), `\/`, `/`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	s.headers(req)
	res, err := sess.cli.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 427 {
		return "", ErrBlocked
	}
	// The ticket is in the final URL, or in the body of the last hop.
	if m := ticketRE.FindStringSubmatch(res.Request.URL.String()); m != nil {
		return m[1], nil
	}
	rest, _ := readBody(res)
	if m := ticketRE.FindSubmatch(rest); m != nil {
		return string(m[1]), nil
	}
	return "", ErrCredentials
}

// exchange turns a service ticket into tokens (or replays the stored OAuth1 token to refresh).
func (s *Service) exchange(ctx context.Context, ticket, op string) (*Tokens, error) {
	base := s.opt.APIBase + "/oauth-service/oauth/exchange/user/2.0"
	form := url.Values{}
	form.Set("service_ticket", ticket)
	secret := ""
	if strings.HasPrefix(ticket, "oauth:") {
		// Refresh: the stored OAuth1 token signs its own exchange.
		parts := strings.SplitN(strings.TrimPrefix(ticket, "oauth:"), ":", 2)
		if len(parts) == 2 {
			form = url.Values{}
			form.Set("oauth_token", parts[0])
			secret = parts[1]
		}
	}
	hdr := oauth1Header(http.MethodPost, base, form, s.opt.ConsumerKey, s.opt.ConsumerSecret, secret, s.opt.Now())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", hdr)
	req.Header.Set("DI-Backend", "connectapi.garmin.com")
	s.headers(req)
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := readBody(res)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 427 {
		return nil, ErrBlocked
	}
	if res.StatusCode != http.StatusOK {
		s.log().Warn("🔗 Pont Garmin : échange du ticket refusé", "op", op, "status", res.StatusCode)
		return nil, ErrCredentials
	}
	// Two shapes are documented by the community clients; accept both.
	var payload struct {
		AccessToken string `json:"access_token"`
		OAuthToken  string `json:"oauth_token"`
		OAuthSecret string `json:"oauth_token_secret"`
		ExpiresIn   int64  `json:"expires_in"`
		OAuth1Token struct {
			Token  string `json:"oauth_token"`
			Secret string `json:"oauth_token_secret"`
		} `json:"oauth1_token"`
		OAuth2Token struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		} `json:"oauth2_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, ErrCredentials
	}
	at, exp := payload.AccessToken, payload.ExpiresIn
	rt := payload.OAuthSecret
	if payload.OAuth2Token.AccessToken != "" {
		at, exp = payload.OAuth2Token.AccessToken, payload.OAuth2Token.ExpiresIn
	}
	if payload.OAuth1Token.Secret != "" {
		rt = payload.OAuth1Token.Secret
	}
	if at == "" {
		return nil, ErrCredentials
	}
	if exp <= 0 {
		// Garmin's OAuth2 token lives about a year; keep a conservative default.
		exp = int64(defaultTokenLifetime / time.Second)
	}
	now := s.opt.Now()
	return &Tokens{AccessToken: at, RefreshToken: rt, CreatedAt: now, ExpiresAt: now.Add(time.Duration(exp) * time.Second)}, nil
}

const defaultTokenLifetime = 300 * 24 * time.Hour

// verify checks the new tokens against the profile endpoint, then a light workout listing: a token
// that cannot read the account is useless to us.
func (s *Service) verify(ctx context.Context, t *Tokens) (string, error) {
	body, status, err := s.do(ctx, t, http.MethodGet, "/userprofile-service/socialProfile", nil)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "", ErrCredentials
	}
	var p profile
	name := ""
	if status == http.StatusOK {
		_ = json.Unmarshal(body, &p)
		name = p.name()
	}
	if _, status, err := s.do(ctx, t, http.MethodGet, "/workout-service/workouts?limit=1", nil); err != nil {
		return "", err
	} else if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "", ErrCredentials
	}
	return name, nil
}

// classify maps an SSO answer to the bridge errors.
func classify(status int, body []byte) error {
	if status == http.StatusTooManyRequests || status == 427 {
		return ErrBlocked
	}
	low := strings.ToLower(string(body))
	for _, m := range blockedMarkers {
		if strings.Contains(low, m) {
			return ErrBlocked
		}
	}
	if isMFA(body) {
		return ErrMFARequired
	}
	if status >= 500 {
		return ErrCredentials // a broken SSO is not worth a retry loop either
	}
	return nil
}

func isMFA(body []byte) bool {
	low := strings.ToLower(string(body))
	for _, m := range mfaMarkers {
		if strings.Contains(low, strings.ToLower(m)) {
			return true
		}
	}
	return false
}

func stateOfError(err error) string {
	switch {
	case err == ErrMFARequired:
		return StateMFA
	case err == ErrBlocked:
		return StateBlocked
	case err == ErrExpired:
		return StateExpired
	default:
		return StateDisconnected
	}
}

// signinParams are the fixed SSO parameters of the mobile flow.
func signinParams(o Options) url.Values {
	v := url.Values{}
	v.Set("id", "gauth-widget")
	v.Set("embedWidget", "true")
	v.Set("gauthHost", o.SSOBase+"/sso")
	v.Set("service", o.SSOBase+"/sso")
	v.Set("source", o.SSOBase+"/sso")
	v.Set("redirectAfterAccountLoginUrl", o.SSOBase+"/sso")
	v.Set("redirectAfterAccountCreationUrl", o.SSOBase+"/sso")
	return v
}

func (s *Service) headers(req *http.Request) {
	req.Header.Set("User-Agent", s.opt.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func (s *Service) post(ctx context.Context, sess *session, target string, form url.Values) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", s.opt.SSOBase)
	req.Header.Set("Referer", s.opt.SSOBase+"/sso/signin")
	s.headers(req)
	res, err := sess.cli.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := readBody(res)
	if err != nil {
		return nil, nil, err
	}
	return res, body, nil
}

func readBody(res *http.Response) ([]byte, error) {
	// The SSO answers are a few dozen kilobytes; a bound stops a hostile or broken proxy.
	return io.ReadAll(io.LimitReader(res.Body, 512<<10))
}
