package garminbridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- fixtures ----------

// fakeGarmin is the SSO + Connect API, with hooks to shape each answer. No real account is involved:
// the shapes below are the ones the community clients parse, pinned here so a change is visible.
type fakeGarmin struct {
	mu sync.Mutex

	csrf         string
	signinBody   string // answer of POST /sso/signin
	signinCode   int
	mfaBody      string
	mfaCode      int
	exchange     string
	exchangeTkn  string
	profile      string
	workouts     string
	createBody   string
	createCode   int
	putCode      int
	schedule     int
	unauthorized bool // first API call answers 401 once

	gotPassword        string
	signinCalls        int
	exchanges          int
	calls              []string
	signinForm         url.Values
	signinReferer      string
	mfaForm            url.Values
	mfaReferer         string
	mfaQuery           string
	codeRequest        int
	codeRequestQ       string
	codeRequestBody    []byte
	codeRequestReferer string
	lastTicket         string
	lastService        string
	lastClient         string
	lastRefresh        string
}

func newFake(t *testing.T) (*fakeGarmin, *httptest.Server) {
	t.Helper()
	f := &fakeGarmin{
		csrf:       "csrf-123",
		signinBody: `<!DOCTYPE html><script>var response_url = "https:\/\/sso.garmin.com\/sso\/redirect?ticket=ST-1a2b3c4d5e6f"</script>`,
		signinCode: 200,
		mfaBody:    `<!DOCTYPE html><script>var response_url = "https:\/\/sso.garmin.com\/sso\/redirect?ticket=ST-9f8e7d6c5b4a"</script>`,
		mfaCode:    200,
		exchange:   `{"access_token":"bearer-1","refresh_token":"refresh-1","expires_in":31536000}`,
		profile:    `{"displayName":"Antoine","userName":"aguyon","profileId":42}`,
		workouts:   `[]`,
		createBody: `{"workoutId":4242,"workoutName":"x"}`,
		createCode: 200,
		putCode:    200,
		schedule:   200,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sso/signin", func(w http.ResponseWriter, r *http.Request) {
		f.record("GET /sso/signin")
		fmt.Fprint(w, f.signinPage())
	})
	mux.HandleFunc("POST /sso/signin", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.signinCalls++
		f.gotPassword = r.PostFormValue("password")
		f.signinForm = r.PostForm
		f.signinReferer = r.Header.Get("Referer")
		f.mu.Unlock()
		f.record("POST /sso/signin")
		w.WriteHeader(f.signinCode)
		fmt.Fprint(w, f.signinBody)
	})
	mux.HandleFunc("GET /sso/embed", func(w http.ResponseWriter, r *http.Request) {
		f.record("GET /sso/embed")
		fmt.Fprint(w, `<html><title>GARMIN</title></html>`)
	})
	mux.HandleFunc("POST /sso/verifyMFA/mfaCode", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.codeRequest++
		f.codeRequestQ = r.URL.RawQuery
		f.codeRequestBody, _ = io.ReadAll(r.Body)
		f.codeRequestReferer = r.Header.Get("Referer")
		f.mu.Unlock()
		f.record("POST mfaCode")
		fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("POST /sso/verifyMFA/loginEnterMfaCode", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.mfaForm = r.PostForm
		f.mfaReferer = r.Header.Get("Referer")
		f.mfaQuery = r.URL.RawQuery
		f.mu.Unlock()
		f.record("POST mfa")
		w.WriteHeader(f.mfaCode)
		fmt.Fprint(w, f.mfaBody)
	})
	mux.HandleFunc("GET /sso/redirect", func(w http.ResponseWriter, r *http.Request) {
		f.record("GET redirect")
		// The ticket only appears on the hop that follows: that is where Garmin puts it.
		http.Redirect(w, r, "/modern?ticket=ST-0a9b8c7d6e5f", http.StatusFound)
	})
	mux.HandleFunc("GET /modern", func(w http.ResponseWriter, r *http.Request) {
		f.record("GET modern")
		fmt.Fprint(w, `<html>connect</html>`)
	})
	// The native DI OAuth2 exchange: Basic auth on the client id, the ticket in the body.
	mux.HandleFunc("POST /di-oauth2-service/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		f.record("POST di token")
		if ua := r.Header.Get("User-Agent"); ua != "com.garmin.android.apps.connectmobile" {
			t.Errorf("token UA %q", ua)
		}
		_ = r.ParseForm()
		clientID := r.PostFormValue("client_id")
		auth := r.Header.Get("Authorization")
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(clientID+":"))
		if auth != want {
			t.Errorf("token Basic auth %q, want the client id only", auth)
		}
		switch r.PostFormValue("grant_type") {
		case "refresh_token":
			f.mu.Lock()
			f.lastRefresh = r.PostFormValue("refresh_token")
			f.lastClient = clientID
			f.exchanges++
			rotated := f.exchangeTkn
			f.mu.Unlock()
			if rotated == "" {
				rotated = "bearer-1"
			}
			fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh-1","expires_in":31536000}`, rotated)
			return
		case diGrant:
			f.mu.Lock()
			f.lastTicket = r.PostFormValue("service_ticket")
			f.lastService = r.PostFormValue("service_url")
			f.exchanges++
			f.mu.Unlock()
			// Only the current generation of ids is accepted, like Garmin: the others answer 400.
			if clientID != diClientIDs[0] {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"invalid_client"}`)
				return
			}
			f.mu.Lock()
			f.lastClient = clientID
			f.mu.Unlock()
			fmt.Fprint(w, f.exchange)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	mux.HandleFunc("GET /userprofile-service/socialProfile", func(w http.ResponseWriter, r *http.Request) {
		f.record("GET profile")
		f.mu.Lock()
		una := f.unauthorized
		f.unauthorized = false
		f.mu.Unlock()
		if una {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, f.profile)
	})
	mux.HandleFunc("GET /workout-service/workouts", func(w http.ResponseWriter, r *http.Request) {
		f.record("GET workouts")
		f.mu.Lock()
		una := f.unauthorized
		f.unauthorized = false
		f.mu.Unlock()
		if una {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, f.workouts)
	})
	mux.HandleFunc("POST /workout-service/workout", func(w http.ResponseWriter, r *http.Request) {
		f.record("POST workout")
		w.WriteHeader(f.createCode)
		fmt.Fprint(w, f.createBody)
	})
	mux.HandleFunc("PUT /workout-service/workout/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.record("PUT workout " + r.PathValue("id"))
		w.WriteHeader(f.putCode)
	})
	mux.HandleFunc("POST /workout-service/schedule/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.record("POST schedule " + r.PathValue("id"))
		w.WriteHeader(f.schedule)
		fmt.Fprint(w, `{}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

// signinPage is the form the fake serves. It ships the reCaptcha wiring on every request, like the
// real page: a detector that reads « captcha » here rejects every single login.
func (f *fakeGarmin) signinPage() string {
	return `<html><script>var reCaptchaURL = "\/sso\/reCaptcha?service=https%3A%2Fsso.garmin.com";</script>` +
		`<form><input type="hidden" name="_csrf" value="` + f.csrf + `"></form></html>`
}

func (f *fakeGarmin) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeGarmin) has(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == s {
			return true
		}
	}
	return false
}

func (f *fakeGarmin) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// memStore is the TokenStore the tests inject.
type memStore struct {
	mu   sync.Mutex
	data map[int64]Tokens
	err  error
}

func newMemStore() *memStore { return &memStore{data: map[int64]Tokens{}} }

func (m *memStore) Load(_ context.Context, uid int64) (*Tokens, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	t, ok := m.data[uid]
	if !ok {
		return nil, nil
	}
	return &t, nil
}

func (m *memStore) Save(_ context.Context, uid int64, t Tokens) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[uid] = t
	return nil
}

func (m *memStore) Delete(_ context.Context, uid int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, uid)
	return nil
}

// service wires the gateway to the fake, with a frozen clock.
func service(t *testing.T, f *fakeGarmin, srv *httptest.Server, st TokenStore) *Service {
	t.Helper()
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	return New(Options{
		HTTP: srv.Client(), Store: st, Now: func() time.Time { return now },
		SSOBase: srv.URL, APIBase: srv.URL, DIBase: srv.URL,
	})
}

// ---------- OAuth1 ----------

// The signature vector of RFC 5849 §3.4.1.1: it proves the encoder and the base string.
func TestLoginSuccess(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)

	res, err := svc.Login(context.Background(), 1, "antoine@example.com", "s3cret!")
	if err != nil {
		t.Fatalf("login: %v (calls: %v)", err, f.calls)
	}
	if res.State != StateConnected || res.DisplayName != "Antoine" {
		t.Fatalf("result: %+v", res)
	}
	if f.gotPassword != "s3cret!" {
		t.Fatalf("password did not reach the SSO form")
	}
	tok, _ := st.Load(context.Background(), 1)
	if tok == nil || tok.AccessToken != "bearer-1" || tok.RefreshToken != diClientIDs[0]+"|refresh-1" || tok.DisplayName != "Antoine" {
		t.Fatalf("tokens: %+v", tok)
	}
	// The verification really happens: profile, then a workout listing.
	// The ticket exchange repeats the service the ticket was issued for, and the client id answers
	// for it: Garmin binds the three together.
	// It must be the very service the ticket was issued for: the embed page of the SSO.
	if !strings.HasSuffix(f.lastService, "/sso/embed") {
		t.Fatalf("service_url %q, want the login service", f.lastService)
	}
	if f.lastTicket == "" {
		t.Fatal("the ticket must reach the token endpoint")
	}
	if f.lastClient != diClientIDs[0] {
		t.Fatalf("client id %q, want the current generation", f.lastClient)
	}
	if !f.has("GET profile") || !f.has("GET workouts") {
		t.Fatalf("verification calls: %v", f.calls)
	}
	// Nothing of the password survives in what we persist.
	raw, _ := json.Marshal(*tok)
	if strings.Contains(string(raw), "s3cret") {
		t.Fatalf("password in the stored tokens: %s", raw)
	}
}

func TestLoginMFAThenCode(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	f.signinBody = `<html><title>GARMIN Authentication Application</title><input type="hidden" name="_csrf" value="mfa-token-2"><div id="mfa-code-entry"></div></html>`

	_, err := svc.Login(context.Background(), 1, "a@b.c", "pw")
	if !errors.Is(err, ErrMFARequired) {
		t.Fatalf("first step: %v", err)
	}
	// The panel reads this route: right after the password step it must say « mfa », otherwise the
	// code form never renders.
	if st := svc.Status(context.Background(), 1); st.State != StateMFA {
		t.Fatalf("status after MFA request: %+v", st)
	}
	res, err := svc.LoginMFA(context.Background(), 1, "123456")
	if err != nil {
		t.Fatalf("mfa: %v", err)
	}
	if res.State != StateConnected {
		t.Fatalf("result: %+v", res)
	}
	if !f.has("POST mfa") {
		t.Fatalf("calls: %v", f.calls)
	}
	// The pending session is consumed: a second code cannot replay it.
	if _, err := svc.LoginMFA(context.Background(), 1, "123456"); !errors.Is(err, ErrExpired) {
		t.Fatalf("replayed code: %v", err)
	}
}

// After the password step, the state is « mfa » — not « disconnected ». Reporting disconnected
// redisplayed the password form and made the code step unreachable (field report, 202 ×2 in the log).
func TestStatusReportsPendingMFA(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	f.signinBody = `<html><title>GARMIN Authentication Application</title><input type="hidden" name="_csrf" value="mfa-token-2"><div id="mfa-code-entry"></div></html>`

	if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("login: %v", err)
	}
	if got := svc.Status(context.Background(), 1); got.State != StateMFA {
		t.Fatalf("a pending code must be visible: %+v", got)
	}
	// The panel polls this route: it must keep answering mfa until the code arrives.
	if got := svc.Status(context.Background(), 1); got.State != StateMFA {
		t.Fatalf("second read: %+v", got)
	}
	if res, err := svc.LoginMFA(context.Background(), 1, "123456"); err != nil || res.State != StateConnected {
		t.Fatalf("code: %+v %v", res, err)
	}
	if got := svc.Status(context.Background(), 1); got.State != StateConnected {
		t.Fatalf("after the code: %+v", got)
	}
}

// A code nobody types does not pin the panel in « mfa » forever.
func TestPendingMFAExpires(t *testing.T) {
	f, srv := newFake(t)
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc := New(Options{
		HTTP: srv.Client(), Store: newMemStore(), Now: func() time.Time { return now },
		SSOBase: srv.URL, APIBase: srv.URL, DIBase: srv.URL,
	})
	f.signinBody = `<html><input type="hidden" name="_csrf" value="mfa-token-2"><script>var customerGuid = "guid-1"; var mfaMethod = "email"; var locale = "fr-FR"; var clientId = "client-1"; var codeSentTo = "";</script><div id="mfa-code-entry"></div></html>`
	if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("login: %v", err)
	}
	if got := svc.Status(context.Background(), 1); got.State != StateMFA {
		t.Fatalf("pending: %+v", got)
	}
	now = now.Add(10 * time.Minute) // beyond the 5-minute window
	if got := svc.Status(context.Background(), 1); got.State != StateDisconnected {
		t.Fatalf("stale pending: %+v", got)
	}
}

// The widget flow, as Garmin serves it now: the credential POST is embedded, the code POST carries
// the token of the page that asked for the code (not the sign-in one), and its referer is that page.
// Reusing the sign-in token is what made every real code look refused.
func TestMFAUsesThePageTokenAndReferer(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	f.signinBody = `<html><input type="hidden" name="_csrf" value="mfa-token-2"><script>var customerGuid = "guid-1"; var mfaMethod = "email"; var locale = "fr-FR"; var clientId = "client-1"; var codeSentTo = "";</script><div id="mfa-code-entry"></div></html>`

	if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("login: %v", err)
	}
	// The credential POST asks for the embedded widget, not the portal page.
	if got := f.signinForm.Get("embed"); got != "true" {
		t.Fatalf("credential embed=%q, want true", got)
	}
	if want := "/sso/embed"; !strings.HasSuffix(f.signinReferer, want) {
		t.Fatalf("credential referer %q, want the embed page", f.signinReferer)
	}
	if res, err := svc.LoginMFA(context.Background(), 1, "123456"); err != nil || res.State != StateConnected {
		t.Fatalf("code: %+v %v", res, err)
	}
	if got := f.mfaForm.Get("_csrf"); got != "mfa-token-2" {
		t.Fatalf("code token %q, want the MFA page token", got)
	}
	if got := f.mfaForm.Get("embed"); got != "true" {
		t.Fatalf("code embed=%q, want true", got)
	}
	if got := f.mfaForm.Get("mfa-code"); got != "123456" {
		t.Fatalf("code %q", got)
	}
	// The referer is the page that rendered the form: the sign-in answer.
	if !strings.Contains(f.mfaReferer, "/sso/signin") {
		t.Fatalf("code referer %q", f.mfaReferer)
	}
	// And the widget parameters ride in the query, never the code.
	if !strings.Contains(f.mfaQuery, "gauthHost=") || strings.Contains(f.mfaQuery, "mfa-code") {
		t.Fatalf("code query %q", f.mfaQuery)
	}
}

// Garmin's widget does not deliver the code by itself: its page calls /sso/verifyMFA/mfaCode when the
// user asks for one. Without that call the user waits for a mail nobody sent.
func TestMFACodeIsRequestedWhenNothingWasSent(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	f.signinBody = `<html><input type="hidden" name="_csrf" value="mfa-token-2"><script>var customerGuid = "guid-1"; var mfaMethod = "email"; var locale = "fr-FR"; var clientId = "client-1"; var codeSentTo = "";</script></html>`

	if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("login: %v", err)
	}
	if f.codeRequest != 1 {
		t.Fatalf("code requests: %d, want 1", f.codeRequest)
	}
	if !strings.Contains(f.codeRequestQ, "clientId=client-1") {
		t.Fatalf("query %q", f.codeRequestQ)
	}
	var payload map[string]string
	if err := json.Unmarshal(f.codeRequestBody, &payload); err != nil {
		t.Fatalf("payload %s: %v", f.codeRequestBody, err)
	}
	if payload["customerGuid"] != "guid-1" || payload["mfaMethod"] != "email" || payload["locale"] != "fr-FR" {
		t.Fatalf("payload %+v", payload)
	}
	if !strings.Contains(f.codeRequestReferer, "/sso/signin") {
		t.Fatalf("referer %q", f.codeRequestReferer)
	}
}

// …and stays silent when Garmin already sent it, or when an authenticator app is used.
func TestMFACodeIsNotRequestedTwice(t *testing.T) {
	for _, tc := range []struct{ name, vars string }{
		{"already sent", `var mfaMethod = "email"; var codeSentTo = "f***@gmail.com"; var clientId = "c";`},
		{"authenticator", `var mfaMethod = "totp"; var clientId = "c";`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, srv := newFake(t)
			svc := service(t, f, srv, newMemStore())
			f.signinBody = `<html><input type="hidden" name="_csrf" value="mfa-token-2"><script>` + tc.vars + `</script></html>`
			if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); !errors.Is(err, ErrMFARequired) {
				t.Fatalf("login: %v", err)
			}
			if f.codeRequest != 0 {
				t.Fatalf("code requested %d times, want 0", f.codeRequest)
			}
		})
	}
}

func TestLoginRefused(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	f.signinBody = `<html><p class="error">Invalid username or password</p></html>`
	if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("refused: %v", err)
	}
	// Empty credentials never leave the server.
	if _, err := svc.Login(context.Background(), 1, "", ""); !errors.Is(err, ErrCredentials) {
		t.Fatalf("empty: %v", err)
	}
	if f.signinCalls != 1 {
		t.Fatalf("empty credentials reached the SSO: %d calls", f.signinCalls)
	}
}

func TestLoginBlocked(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       int
		body       string
		wantStatus string
	}{
		{"429", http.StatusTooManyRequests, `{}`, StateBlocked},
		{"427", 427, `{}`, StateBlocked},
		{"unusual traffic", 200, `<html>We have detected unusual traffic from your network</html>`, StateBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, srv := newFake(t)
			svc := service(t, f, srv, newMemStore())
			f.signinCode, f.signinBody = tc.code, tc.body
			res, err := svc.Login(context.Background(), 1, "a@b.c", "pw")
			if !errors.Is(err, ErrBlocked) {
				t.Fatalf("err: %v", err)
			}
			if res.State != tc.wantStatus {
				t.Fatalf("state: %+v", res)
			}
		})
	}
}

// The sign-in form ships the reCaptcha wiring on every page; reading it as a block made every
// login answer 429 in ~450 ms without ever sending the password (issue #147, field report).
func TestLoginFormWiringIsNotABlock(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())

	if !strings.Contains(f.signinPage(), "reCaptchaURL") {
		t.Fatalf("the fixture dropped the wiring this test exists for")
	}
	res, err := svc.Login(context.Background(), 1, "antoine@example.com", "s3cret!")
	if err != nil {
		t.Fatalf("login must proceed past the form: %v", err)
	}
	if res.State != StateConnected || f.gotPassword != "s3cret!" {
		t.Fatalf("state=%q password_reached=%v", res.State, f.gotPassword != "")
	}
}

// A CAPTCHA wall is not a rate limit: it is told apart, and the step is named, so the message can
// say « Garmin wants a human » instead of « 429 ».
func TestLoginChallenge(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	f.signinCode = 200
	f.signinBody = `<html><script>var reCaptchaURL = "\/sso\/reCaptcha?service=x";</script><div class="g-recaptcha"></div></html>`

	res, err := svc.Login(context.Background(), 1, "a@b.c", "pw")
	if !errors.Is(err, ErrChallenge) {
		t.Fatalf("err: %v", err)
	}
	if errors.Is(err, ErrBlocked) {
		t.Fatalf("a challenge must not be reported as a rate limit")
	}
	if res.State != StateBlocked {
		t.Fatalf("state: %+v", res)
	}
	if step := StepOf(err); step != "identifiants" {
		t.Fatalf("step: %q", step)
	}
	if !strings.Contains(err.Error(), "identifiants") {
		t.Fatalf("the message must name the step: %v", err)
	}
}

// An explicit refusal wins over the reCaptcha wiring that the refusal page also carries.
func TestLoginRefusedWinsOverTheChallengeWiring(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	f.signinBody = `<html><script>var reCaptchaURL = "x";</script><p>Invalid username or password</p></html>`
	if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("err: %v", err)
	}
}

// An unexpected page is a flow failure, not a wrong password: the step says where to look.
func TestLoginUnexpectedPageIsAFlowError(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	f.signinBody = `<html><body>maintenance</body></html>`
	_, err := svc.Login(context.Background(), 1, "a@b.c", "pw")
	if !errors.Is(err, ErrFlow) {
		t.Fatalf("err: %v", err)
	}
	if StepOf(err) != "identifiants" {
		t.Fatalf("step: %q", StepOf(err))
	}
}

// The SSO answering 500 is a flow error too: retrying the password would be pointless.
func TestLoginServerErrorIsAFlowError(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	f.signinCode, f.signinBody = http.StatusInternalServerError, `{}`
	_, err := svc.Login(context.Background(), 1, "a@b.c", "pw")
	if !errors.Is(err, ErrFlow) {
		t.Fatalf("err: %v", err)
	}
}

func TestLoginTicketFromResponseURL(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	// The ticket only appears after following the response URL: the second code path.
	// No ticket in the body: only the redirect that follows carries it.
	f.signinBody = `<html><script>var response_url = "` + strings.ReplaceAll(srv.URL, "/", `\/`) + `\/sso\/redirect"</script></html>`
	if _, err := svc.Login(context.Background(), 1, "a@b.c", "pw"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !f.has("GET redirect") {
		t.Fatalf("the response URL was not followed: %v", f.calls)
	}
}

// ---------- status ----------

func TestStatusStates(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	ctx := context.Background()

	if got := svc.Status(ctx, 1); got.State != StateDisconnected {
		t.Fatalf("empty: %+v", got)
	}
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	_ = st.Save(ctx, 1, Tokens{AccessToken: "a", ExpiresAt: now.Add(time.Hour), DisplayName: "Antoine", CreatedAt: now})
	if got := svc.Status(ctx, 1); got.State != StateConnected || got.DisplayName != "Antoine" {
		t.Fatalf("connected: %+v", got)
	}
	_ = st.Save(ctx, 1, Tokens{AccessToken: "a", ExpiresAt: now.Add(-time.Minute)})
	if got := svc.Status(ctx, 1); got.State != StateExpired {
		t.Fatalf("expired: %+v", got)
	}
}

// ---------- push ----------

func TestPushCreatesThenUpdates(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	ctx := context.Background()
	_, _ = svc.Login(ctx, 1, "a@b.c", "pw")
	f.reset()

	payload := json.RawMessage(`{"workoutName":"Footing · Stride 7 · 2026-11-26","sportType":{"sportTypeKey":"running"}}`)
	req := PushRequest{Name: "Footing · Stride 7 · 2026-11-26", Date: "2026-11-26", Payload: payload}

	first, err := svc.Push(ctx, 1, req)
	if err != nil {
		t.Fatalf("first push: %v", err)
	}
	if !first.Created || first.WorkoutID != 4242 || !first.Scheduled {
		t.Fatalf("first result: %+v", first)
	}
	if !f.has("POST workout") || !f.has("POST schedule 4242") {
		t.Fatalf("calls: %v", f.calls)
	}

	// Second push of the same session: the stored id means an update, never a second workout.
	f.reset()
	req.WorkoutID = first.WorkoutID
	second, err := svc.Push(ctx, 1, req)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if second.Created || second.WorkoutID != 4242 {
		t.Fatalf("second result: %+v", second)
	}
	if f.has("POST workout") {
		t.Fatalf("a duplicate workout was created: %v", f.calls)
	}
	if !f.has("PUT workout 4242") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestPushFindsExistingByName(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	ctx := context.Background()
	_, _ = svc.Login(ctx, 1, "a@b.c", "pw")
	f.reset()
	f.workouts = `[{"workoutId":777,"workoutName":"Footing · Stride 7 · 2026-11-26"},{"workoutId":778,"workoutName":"Autre"}]`

	res, err := svc.Push(ctx, 1, PushRequest{Name: "Footing · Stride 7 · 2026-11-26", Date: "2026-11-26", Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if res.WorkoutID != 777 || res.Created {
		t.Fatalf("result: %+v", res)
	}
	if f.has("POST workout") || !f.has("PUT workout 777") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestPushCreatesWhenTheWorkoutWasDeleted(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	ctx := context.Background()
	_, _ = svc.Login(ctx, 1, "a@b.c", "pw")
	f.reset()
	f.putCode = http.StatusNotFound

	res, err := svc.Push(ctx, 1, PushRequest{Name: "n", Date: "2026-11-26", Payload: json.RawMessage(`{}`), WorkoutID: 900})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if !res.Created || res.WorkoutID != 4242 {
		t.Fatalf("result: %+v", res)
	}
}

func TestPushRefreshesOn401(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	ctx := context.Background()
	_, _ = svc.Login(ctx, 1, "a@b.c", "pw")
	f.reset()
	f.unauthorized = true
	f.exchangeTkn = "bearer-2"

	if _, err := svc.Push(ctx, 1, PushRequest{Name: "n", Date: "2026-11-26", Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("push: %v", err)
	}
	if !f.has("POST di token") || f.exchanges < 2 {
		t.Fatalf("no refresh attempted: %v", f.calls)
	}
	tok, _ := st.Load(ctx, 1)
	if tok.AccessToken == "bearer-1" {
		t.Fatalf("rotated token not persisted: %+v", tok)
	}
	// The refresh presents the client id that issued the token, with the stored refresh token.
	if f.lastClient != diClientIDs[0] || f.lastRefresh != "refresh-1" {
		t.Fatalf("refresh %q with %q", f.lastClient, f.lastRefresh)
	}
	if !strings.HasPrefix(tok.RefreshToken, diClientIDs[0]+"|") {
		t.Fatalf("the client id must survive the refresh: %q", tok.RefreshToken)
	}
}

func TestPushExpiredWithoutTokens(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	if _, err := svc.Push(context.Background(), 1, PushRequest{Name: "n", Date: "2026-11-26", Payload: json.RawMessage(`{}`)}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("err: %v", err)
	}
}

func TestPushScheduleFailureIsReported(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	ctx := context.Background()
	_, _ = svc.Login(ctx, 1, "a@b.c", "pw")
	f.schedule = http.StatusInternalServerError

	res, err := svc.Push(ctx, 1, PushRequest{Name: "n", Date: "2026-11-26", Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if res.Scheduled || res.ScheduleErr == "" || res.WorkoutID != 4242 {
		t.Fatalf("result: %+v", res)
	}
}

func TestPushRejectsIncompleteRequest(t *testing.T) {
	f, srv := newFake(t)
	svc := service(t, f, srv, newMemStore())
	if _, err := svc.Push(context.Background(), 1, PushRequest{}); err == nil {
		t.Fatal("empty request accepted")
	}
}

func TestWorkoutName(t *testing.T) {
	got := WorkoutName("Footing 45 min", 12, "2026-11-26")
	if got != "Footing 45 min · Stride 12 · 2026-11-26" {
		t.Fatalf("name: %q", got)
	}
	if a, b := WorkoutName("Footing", 1, "2026-11-26"), WorkoutName("Footing", 2, "2026-11-26"); a == b {
		t.Fatal("two sessions share a name")
	}
	if n := WorkoutName(strings.Repeat("x", 200), 3, "2026-11-26"); len([]rune(n)) > 100 {
		t.Fatalf("name too long: %d", len([]rune(n)))
	}
	// Empty title still gives a stable identity.
	if n := WorkoutName("", 4, "2026-11-26"); !strings.Contains(n, "Stride 4") {
		t.Fatalf("name: %q", n)
	}
}

func TestDisconnectClearsTokens(t *testing.T) {
	f, srv := newFake(t)
	st := newMemStore()
	svc := service(t, f, srv, st)
	ctx := context.Background()
	_, _ = svc.Login(ctx, 1, "a@b.c", "pw")
	if err := svc.Disconnect(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if tok, _ := st.Load(ctx, 1); tok != nil {
		t.Fatalf("tokens left: %+v", tok)
	}
	if st := svc.Status(ctx, 1); st.State != StateDisconnected {
		t.Fatalf("status: %+v", st)
	}
}
