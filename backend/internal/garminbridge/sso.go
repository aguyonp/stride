package garminbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	// mfaVarRE reads the inline variables the widget page uses to drive its MFA step.
	mfaVarRE = regexp.MustCompile(`var\s+(customerGuid|mfaMethod|locale|clientId|codeSentTo)\s*=\s*"([^"]*)"\s*;`)
	// mfaMarkers say "Garmin wants a code", across the widget and the mobile variants.
	mfaMarkers = []string{"verifyMFA", "MFA_REQUIRED", "mfa-code", "enterMfaCode", "mfaVerification", "mfaMethod"}
	// blockedMarkers are the anti-bot *answers*: status 200 with a rate-limit message. They are
	// never matched on the sign-in form, which legitimately ships a `reCaptchaURL` field for the
	// case where a challenge is asked later — matching it there rejects every single login.
	blockedMarkers = []string{"unusual traffic", "too many requests"}
	// challengeMarkers identify a real CAPTCHA wall: the page offers a challenge and, being a
	// challenge, carries neither a ticket nor the MFA step.
	challengeMarkers = []string{"/sso/recaptcha", "g-recaptcha", "recaptcha/api.js"}
	// refusalMarkers are Garmin's own words for a wrong password. They win over the CAPTCHA wiring:
	// a refusal page ships that wiring too, and « identifiants refusés » is then the true cause.
	refusalMarkers = []string{
		"invalid username or password",
		"username or password you entered is incorrect",
		"mot de passe ou nom d'utilisateur incorrect",
	}
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
	// The widget parameters also ride in the query, as the official apps send them. Only the widget
	// parameters: the e-mail and the password stay in the body, never in a URL that a proxy logs.
	res, body, err := s.post(ctx, sess, s.opt.SSOBase+"/sso/signin?"+signinParams(s.opt).Encode(), embedURL(s.opt), form)
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
	form.Set("embed", "true")
	form.Set("fromPage", "setupEnterMfaCode")
	for k, vs := range p.params {
		if form.Get(k) == "" {
			form.Set(k, vs[0])
		}
	}
	res, body, err := s.post(ctx, p.sess, s.opt.SSOBase+"/sso/verifyMFA/loginEnterMfaCode?"+p.params.Encode(), p.referer, form)
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
	// The embed page first: it sets the session cookies the form is bound to, as the widget does.
	if _, _, err := s.get(ctx, sess, embedURL(s.opt)+"?"+embedParams(s.opt).Encode(), ""); err != nil {
		return nil, stepErr(stepForm, err)
	}
	res, body, err := s.get(ctx, sess, s.opt.SSOBase+"/sso/signin?"+signinParams(s.opt).Encode(), embedURL(s.opt))
	if err != nil {
		return nil, stepErr(stepForm, err)
	}
	// Only the status is judged here: the form always contains reCaptcha wiring, so no body
	// marker may be read as a block.
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 427 {
		return nil, stepErr(stepForm, ErrBlocked)
	}
	m := csrfRE.FindSubmatch(body)
	if m == nil {
		if isChallenge(body) {
			return nil, stepErr(stepForm, ErrChallenge)
		}
		return nil, stepErr(stepForm, ErrFlow)
	}
	form := signinParams(s.opt)
	form.Set("_csrf", string(m[1]))
	// The widget embeds its form: embed=true is what makes Garmin answer with the widget page, the
	// one whose MFA step can be completed.
	form.Set("embed", "true")
	return form, nil
}

// finishLogin reads the answer of the final password step (or of the MFA step).
func (s *Service) finishLogin(ctx context.Context, userID int64, sess *session, res *http.Response, body []byte, csrf string) (LoginResult, error) {
	if err := classify(res.StatusCode, body); err != nil {
		if errors.Is(err, ErrMFARequired) {
			// The MFA page is a page of its own: its hidden token is not the sign-in one, and it is
			// the one Garmin checks when the code comes back. Reusing the sign-in token is what made
			// every code look refused.
			mfaCSRF := csrf
			if m := csrfRE.FindSubmatch(body); m != nil {
				mfaCSRF = string(m[1])
			}
			s.keepPending(userID, &pendingLogin{
				sess: sess, csrf: mfaCSRF, params: signinParams(s.opt), referer: res.Request.URL.String(),
			})
			// The widget does not send the e-mail itself: its page calls /sso/verifyMFA/mfaCode when
			// the user asks for a code, and the mail only leaves then. Without this call the flow
			// waits forever for a code nobody received.
			s.requestMFACode(ctx, userID, sess, body, res.Request.URL.String())
			return LoginResult{State: StateMFA}, stepErr(stepMFA, err)
		}
		return LoginResult{State: stateOfError(err)}, stepErr(stepLogin, err)
	}
	if !isMFA(body) && !ticketRE.Match(body) && !responseURLRE.Match(body) && !ticketRE.MatchString(res.Request.URL.String()) {
		s.trace(userID, stepLogin, res, body)
		if !isRefusal(body) && isChallenge(body) {
			return LoginResult{State: StateBlocked}, stepErr(stepLogin, ErrChallenge)
		}
		if !isRefusal(body) && !hasSigninForm(body) {
			// No ticket, no code, no refusal and no form either: this is not a wrong password,
			// it is a page the flow does not know.
			return LoginResult{State: StateDisconnected}, stepErr(stepLogin, ErrFlow)
		}
		// The form came back: the credentials were refused.
		return LoginResult{State: StateDisconnected}, stepErr(stepLogin, ErrCredentials)
	}
	ticket, err := s.serviceTicket(ctx, sess, res, body)
	if err != nil {
		s.keepPending(userID, &pendingLogin{sess: sess, csrf: csrf, params: signinParams(s.opt)})
		return LoginResult{State: stateOfError(err)}, err
	}
	s.dropPending(userID)
	tok, err := s.exchange(ctx, ticket, "connexion")
	if err != nil {
		return LoginResult{State: stateOfError(err)}, stepErr(stepTokens, err)
	}
	name, err := s.verify(ctx, tok)
	if err != nil {
		return LoginResult{State: stateOfError(err)}, stepErr(stepProfile, err)
	}
	tok.DisplayName = name
	if err := s.opt.Store.Save(ctx, userID, *tok); err != nil {
		return LoginResult{State: StateDisconnected}, err
	}
	s.log().Info("🔗 Pont Garmin connecté", "user", userID)
	return LoginResult{State: StateConnected, DisplayName: name}, nil
}

// serviceTicket follows the response URL and reads the ticket from where it lands.
func (s *Service) serviceTicket(ctx context.Context, sess *session, res *http.Response, body []byte) (string, error) {
	if m := ticketRE.FindSubmatch(body); m != nil {
		return string(m[1]), nil
	}
	// A redirect straight to the service carries the ticket in the final URL, not in a body.
	if m := ticketRE.FindStringSubmatch(res.Request.URL.String()); m != nil {
		return m[1], nil
	}
	m := responseURLRE.FindSubmatch(body)
	if m == nil {
		return "", stepErr(stepTicket, ErrFlow)
	}
	target := strings.ReplaceAll(string(m[1]), `\/`, `/`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	s.headers(req)
	hop, err := sess.cli.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = hop.Body.Close() }()
	if hop.StatusCode == http.StatusTooManyRequests || hop.StatusCode == 427 {
		return "", stepErr(stepTicket, ErrBlocked)
	}
	rest, _ := readBody(hop)
	if m := ticketRE.FindStringSubmatch(hop.Request.URL.String()); m != nil {
		return m[1], nil
	}
	if m := ticketRE.FindSubmatch(rest); m != nil {
		return string(m[1]), nil
	}
	return "", stepErr(stepTicket, ErrFlow)
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

// classify maps an SSO answer to the bridge errors. It never guesses a CAPTCHA here: every page of
// the Garmin app ships the reCaptcha wiring, including the success page, so a challenge is only
// concluded later, on an answer that carries neither a ticket nor a code step (see finishLogin).
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
		// A broken SSO is neither a wrong password nor a block: it is a flow failure, and saying so
		// is what tells the next person where to look.
		return ErrFlow
	}
	return nil
}

// requestMFACode asks Garmin to deliver the code, exactly as the widget's « Request a new code »
// link does. Best effort: a failure here does not fail the sign-in, it just leaves the user waiting
// for a mail Garmin may not have sent.
func (s *Service) requestMFACode(ctx context.Context, userID int64, sess *session, page []byte, pageURL string) {
	vars := map[string]string{}
	for _, m := range mfaVarRE.FindAllSubmatch(page, -1) {
		vars[string(m[1])] = string(m[2])
	}
	method := strings.ToLower(vars["mfaMethod"])
	if method != "email" && method != "sms" {
		return // an authenticator app needs no delivery
	}
	if vars["codeSentTo"] != "" {
		return // Garmin already sent it during the credential POST
	}
	payload, err := json.Marshal(map[string]string{
		"customerGuid": vars["customerGuid"],
		"mfaMethod":    vars["mfaMethod"],
		"locale":       vars["locale"],
	})
	if err != nil {
		return
	}
	target := s.opt.SSOBase + "/sso/verifyMFA/mfaCode?clientId=" + url.QueryEscape(vars["clientId"])
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", pageURL)
	s.headers(req)
	res, err := sess.cli.Do(req)
	if err != nil {
		s.log().Warn("🔗 Pont Garmin : demande de code impossible", "user", userID, "err", err.Error())
		return
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = readBody(res)
	if res.StatusCode >= 400 {
		s.log().Warn("🔗 Pont Garmin : demande de code refusée", "user", userID, "status", res.StatusCode)
		return
	}
	s.log().Info("🔗 Pont Garmin : code demandé à Garmin", "user", userID, "methode", method)
}

// trace writes what Garmin actually answered when a sign-in stops, in booleans and a status: enough
// to tell a refused code from an unknown page, and nothing that could carry an e-mail, a password or
// a token. It is the line a field report needs.
func (s *Service) trace(userID int64, step string, res *http.Response, body []byte) {
	s.log().Warn("🔗 Pont Garmin : réponse inexploitée",
		"user", userID, "etape", step, "status", res.StatusCode, "octets", len(body),
		"page", res.Request.URL.Path,
		"ticket", ticketRE.Match(body), "code_requis", isMFA(body), "formulaire", hasSigninForm(body),
		"captcha", isChallenge(body), "refus", isRefusal(body))
}

// hasSigninForm reports the page re-rendering the sign-in form, which is how Garmin answers a
// refused password (the error text itself is localized and not worth matching).
func hasSigninForm(body []byte) bool {
	low := strings.ToLower(string(body))
	return strings.Contains(low, `name="_csrf"`) && strings.Contains(low, `name="password"`)
}

// isRefusal reports Garmin's explicit « wrong password » wording.
func isRefusal(body []byte) bool {
	low := strings.ToLower(string(body))
	for _, m := range refusalMarkers {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

// isChallenge reports a real CAPTCHA wall (a challenge page, hence with no ticket).
func isChallenge(body []byte) bool {
	low := strings.ToLower(string(body))
	for _, m := range challengeMarkers {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
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
	if errors.Is(err, ErrChallenge) {
		return StateBlocked // a wall: no retry, the .FIT fallback stands
	}
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

// embedURL is the embed page that hosts the widget form: the sign-in page and the MFA page are both
// rendered there, and it is the URL every parameter of the flow points at.
func embedURL(o Options) string { return o.SSOBase + "/sso/embed" }

// signinParams are the fixed SSO parameters of the widget flow. Garmin now serves the *embedded*
// widget (service = …/sso/embed, embed=true); the older portal shape answered our credential POST
// with a page whose MFA step could not be completed.
func signinParams(o Options) url.Values {
	v := url.Values{}
	v.Set("id", "gauth-widget")
	v.Set("embedWidget", "true")
	v.Set("gauthHost", embedURL(o))
	v.Set("service", embedURL(o))
	v.Set("source", embedURL(o))
	v.Set("redirectAfterAccountLoginUrl", embedURL(o))
	v.Set("redirectAfterAccountCreationUrl", embedURL(o))
	return v
}

// embedParams are the parameters of the embed page itself (no service: it *is* the service).
func embedParams(o Options) url.Values {
	v := url.Values{}
	v.Set("id", "gauth-widget")
	v.Set("embedWidget", "true")
	v.Set("gauthHost", o.SSOBase+"/sso")
	return v
}

func (s *Service) headers(req *http.Request) {
	req.Header.Set("User-Agent", s.opt.UserAgent)
	// The official clients send this on every SSO call; without it the MFA hop looks like a
	// different client than the one that opened the session.
	req.Header.Set("NK", "NT")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func (s *Service) post(ctx context.Context, sess *session, target, referer string, form url.Values) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", s.opt.SSOBase)
	// A browser posts a form with the page that rendered it as its referer; Garmin checks it on the
	// MFA hop, where the form lives on the sign-in answer rather than on /sso/signin.
	if referer != "" {
		req.Header.Set("Referer", referer)
	} else {
		req.Header.Set("Referer", target)
	}
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

// get performs a GET with the session's cookies.
func (s *Service) get(ctx context.Context, sess *session, target, referer string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, err
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
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
