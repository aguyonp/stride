package garminbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	gotPassword string
	signinCalls int
	exchanges   int
	calls       []string
}

func newFake(t *testing.T) (*fakeGarmin, *httptest.Server) {
	t.Helper()
	f := &fakeGarmin{
		csrf:       "csrf-123",
		signinBody: `<!DOCTYPE html><script>var response_url = "https:\/\/sso.garmin.com\/sso\/redirect?ticket=ST-1a2b3c4d5e6f"</script>`,
		signinCode: 200,
		mfaBody:    `<!DOCTYPE html><script>var response_url = "https:\/\/sso.garmin.com\/sso\/redirect?ticket=ST-9f8e7d6c5b4a"</script>`,
		mfaCode:    200,
		exchange:   `{"oauth_token":"oauth1-token","oauth_token_secret":"oauth1-secret","oauth2_token":{"access_token":"bearer-1","expires_in":31536000}}`,
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
		fmt.Fprintf(w, `<html><form><input type="hidden" name="_csrf" value="%s"></form></html>`, f.csrf)
	})
	mux.HandleFunc("POST /sso/signin", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.signinCalls++
		f.gotPassword = r.PostFormValue("password")
		f.mu.Unlock()
		f.record("POST /sso/signin")
		w.WriteHeader(f.signinCode)
		fmt.Fprint(w, f.signinBody)
	})
	mux.HandleFunc("POST /sso/verifyMFA/loginEnterMfaCode", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
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
	mux.HandleFunc("POST /oauth-service/oauth/exchange/user/2.0", func(w http.ResponseWriter, r *http.Request) {
		f.record("POST exchange")
		// The exchange must be OAuth1-signed with the consumer key.
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "OAuth ") || !strings.Contains(auth, "oauth_consumer_key") || !strings.Contains(auth, "oauth_signature") {
			t.Errorf("unsigned exchange: %q", auth)
		}
		f.mu.Lock()
		f.exchanges++
		n := f.exchanges
		rotated := f.exchangeTkn
		f.mu.Unlock()
		if n > 1 && rotated != "" {
			// The refresh answers a different token, like Garmin does.
			fmt.Fprintf(w, `{"oauth_token":"oauth1-token","oauth_token_secret":"oauth1-secret","oauth2_token":{"access_token":%q,"expires_in":31536000}}`, rotated)
			return
		}
		fmt.Fprint(w, f.exchange)
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
		SSOBase: srv.URL, APIBase: srv.URL,
	})
}

// ---------- OAuth1 ----------

// The signature vector of RFC 5849 §3.4.1.1: it proves the encoder and the base string.
func TestOAuth1RFCVector(t *testing.T) {
	params := url.Values{}
	params.Set("b5", "=%3D")
	params.Add("a3", "a")
	params.Add("a3", "2 q")
	params.Set("c@", "")
	params.Set("c2", "")
	params.Set("a2", "r b")
	oauth := url.Values{}
	oauth.Set("oauth_consumer_key", "9djdj82h48djs9d2")
	oauth.Set("oauth_token", "kkk9d7dh3k39sjv7")
	oauth.Set("oauth_signature_method", "HMAC-SHA1")
	oauth.Set("oauth_timestamp", "137131201")
	oauth.Set("oauth_nonce", "7d8f3e4a")
	got := signHMACSHA1("POST", "http://example.com/request", params, oauth, "j49sk3j29djd", "dh893hdasih9")
	if got != "r6/TJjbCOr97/+UU0NsvSne7s5g=" {
		t.Fatalf("signature: %q", got)
	}
}

func TestPercentEncode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"abcXYZ019-._~", "abcXYZ019-._~"},
		{"a b", "a%20b"},
		{"100%", "100%25"},
		{"é", "%C3%A9"},
		{"+", "%2B"},
	} {
		if got := percentEncode(tc.in); got != tc.want {
			t.Fatalf("percentEncode(%q) = %q", tc.in, got)
		}
	}
}

// ---------- login ----------

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
	if tok == nil || tok.AccessToken != "bearer-1" || tok.RefreshToken != "oauth1-secret" || tok.DisplayName != "Antoine" {
		t.Fatalf("tokens: %+v", tok)
	}
	// The verification really happens: profile, then a workout listing.
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
	f.signinBody = `<html><title>GARMIN Authentication Application</title><div id="mfa-code-entry"></div></html>`

	_, err := svc.Login(context.Background(), 1, "a@b.c", "pw")
	if !errors.Is(err, ErrMFARequired) {
		t.Fatalf("first step: %v", err)
	}
	if st := svc.Status(context.Background(), 1); st.State != StateDisconnected {
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
		{"captcha page", 200, `<html><div class="g-recaptcha">captcha</div></html>`, StateBlocked},
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
	if !f.has("POST exchange") {
		t.Fatalf("no refresh attempted: %v", f.calls)
	}
	tok, _ := st.Load(ctx, 1)
	if tok.AccessToken == "bearer-1" {
		t.Fatalf("rotated token not persisted: %+v", tok)
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
