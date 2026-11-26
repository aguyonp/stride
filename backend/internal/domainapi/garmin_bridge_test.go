package domainapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stride/internal/domain"
	"stride/internal/garminbridge"
	"stride/internal/store"
)

// The experimental Garmin bridge over the real routes (§23, issue #147): kill switch, login flow with
// a fake SSO, bulk push, and the promise that the password never leaves the request.

// bridgeFake is the SSO + web API the gateway talks to.
func bridgeFake(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	// The real form ships the reCaptcha wiring on every request: it must not be read as a block.
	mux.HandleFunc("GET /sso/signin", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><script>var reCaptchaURL = "\/sso\/reCaptcha?service=x";</script>`+
			`<input type="hidden" name="_csrf" value="csrf-1">`)
	})
	mux.HandleFunc("POST /sso/signin", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.PostFormValue("password") {
		case "correct-horse":
			// the success path below
		case "challenge":
			fmt.Fprint(w, `<html><script>var reCaptchaURL = "\/sso\/reCaptcha?service=x";</script><div class="g-recaptcha"></div></html>`)
			return
		case "weird":
			fmt.Fprint(w, `<html><body>maintenance</body></html>`)
			return
		default:
			fmt.Fprint(w, `<html><script>var reCaptchaURL = "x";</script><p class="error">Invalid username or password</p></html>`)
			return
		}
		fmt.Fprint(w, `<html><script>var response_url = "/sso/redirect?ticket=ST-4f3e2d1c0b9a"</script></html>`)
	})
	mux.HandleFunc("GET /oauth-service/oauth/preauthorized", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `oauth_token=o1&oauth_token_secret=s1`)
	})
	mux.HandleFunc("POST /oauth-service/oauth/exchange/user/2.0", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"access_token":"bearer-1","expires_in":31536000}`)
	})
	mux.HandleFunc("GET /userprofile-service/socialProfile", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"displayName":"Antoine"}`)
	})
	mux.HandleFunc("GET /workout-service/workouts", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	})
	mux.HandleFunc("POST /workout-service/workout", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"workoutId":991}`)
	})
	mux.HandleFunc("POST /workout-service/schedule/{id}", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// withBridge installs a gateway on the fake and returns the previous factory.
func withBridge(t *testing.T, fake *httptest.Server) {
	t.Helper()
	prev := bridgeFactory
	bridgeFactory = func(s *domain.Services) *garminbridge.Service {
		return garminbridge.New(garminbridge.Options{
			HTTP: fake.Client(), Store: s.BridgeTokenStore(), SSOBase: fake.URL, APIBase: fake.URL,
		})
	}
	t.Cleanup(func() { bridgeFactory = prev })
}

func setBridgeSetting(t *testing.T, e *env2, on bool) {
	t.Helper()
	v := "false"
	if on {
		v = "true"
	}
	if err := e.st.SetSetting(context.Background(), store.SettingGarminBridgeEnabled, v, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// Without a gateway at all (a build that removed the package), the routes answer a plain 200 with
// `available:false`: nothing to offer, no error to explain.
func TestBridgeRoutesWithoutGateway(t *testing.T) {
	e := setup(t) // no bridgeFactory: the domain has no gateway
	var st garminbridge.Status
	if code, body := e.do(1, "GET", "/api/garmin/bridge", nil, &st); code != http.StatusOK || st.Available || st.State != "disconnected" {
		t.Fatalf("status: %d %+v %s", code, st, body)
	}
	if code, _ := e.do(1, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "x"}, nil); code != http.StatusConflict {
		t.Fatalf("login without a gateway: %d", code)
	}
}

func TestBridgeRoutesStatusAndSetting(t *testing.T) {
	withBridge(t, bridgeFake(t))
	e := setup(t)

	if code, _ := e.do(0, "GET", "/api/garmin/bridge", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
	// Every answer is decoded into a fresh value: `message` is omitted when empty, and reusing the
	// variable would keep the previous state's text (§19.2, on the test side again).
	status := func() garminbridge.Status {
		t.Helper()
		var out garminbridge.Status
		if code, body := e.do(2, "GET", "/api/garmin/bridge", nil, &out); code != 200 {
			t.Fatalf("status: %d %s", code, body)
		}
		return out
	}
	if st := status(); st.State != garminbridge.StateDisconnected || st.Available {
		t.Fatalf("setting off: %+v", st)
	}

	// Admin switch on: the login flow becomes reachable.
	setBridgeSetting(t, e, true)
	if st := status(); st.State != garminbridge.StateDisconnected || !st.Available {
		t.Fatalf("setting on: %+v", st)
	}
	// Wrong password: a plain 400, nothing stored.
	if code, _ := e.do(2, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "wrong"}, nil); code != http.StatusBadRequest {
		t.Fatalf("wrong password: %d", code)
	}
	// Right password: connected, profile read, tokens stored sealed.
	var res map[string]any
	if code, body := e.do(2, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "correct-horse"}, &res); code != 200 {
		t.Fatalf("login: %d %s", code, body)
	}
	if res["state"] != "connected" || res["displayName"] != "Antoine" {
		t.Fatalf("login answer: %+v", res)
	}
	if st := status(); st.State != "connected" || st.DisplayName != "Antoine" {
		t.Fatalf("connected status: %+v", st)
	}

	// The password is nowhere: not in the answer, not in the row (the blob is sealed).
	var blob []byte
	if err := e.st.R.QueryRow(`SELECT token_enc FROM provider_connections WHERE user_id = 2 AND provider = 'garmin_web'`).Scan(&blob); err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if strings.Contains(string(blob), "correct-horse") || strings.Contains(string(blob), "bearer-1") {
		t.Fatal("the token blob is readable")
	}

	// Disconnect: the row is gone, the state is back to disconnected.
	if code, _ := e.do(2, "DELETE", "/api/garmin/bridge", nil, nil); code != http.StatusNoContent {
		t.Fatalf("disconnect: %d", code)
	}
	if st := status(); st.State != "disconnected" {
		t.Fatalf("after disconnect: %+v", st)
	}
}

func TestBridgeLoginConflictWhenDisabled(t *testing.T) {
	withBridge(t, bridgeFake(t))
	e := setup(t)
	setBridgeSetting(t, e, false)
	code, body := e.do(2, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "correct-horse"}, nil)
	if code != http.StatusConflict {
		t.Fatalf("disabled login: %d %s", code, body)
	}
}

func TestPushTodayRoute(t *testing.T) {
	withBridge(t, bridgeFake(t))
	e := setup(t)
	setBridgeSetting(t, e, true)

	// Two sessions today: one with steps, one without.
	today := time.Now().In(time.FixedZone("CET", 3600)).Format("2006-01-02")
	if _, err := e.svc.CreatePlanned(context.Background(), 2, []byte(`{"date":"`+today+`","sport":"run","title":"Footing","steps":[{"type":"warmup","durationS":600}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreatePlanned(context.Background(), 2, []byte(`{"date":"`+today+`","sport":"run","title":"Sans structure"}`)); err != nil {
		t.Fatal(err)
	}

	// Not connected yet: both are reported, the push fails cleanly (no 500).
	var out struct {
		Date    string                   `json:"date"`
		Results []domain.PushTodayResult `json:"results"`
	}
	if code, body := e.do(2, "POST", "/api/plan/push-today", m{"date": today}, &out); code != 200 {
		t.Fatalf("push-today: %d %s", code, body)
	}
	if out.Date != today || len(out.Results) != 2 {
		t.Fatalf("answer: %+v", out)
	}
	statuses := map[string]int{}
	for _, r := range out.Results {
		statuses[r.Status]++
	}
	if statuses["skipped"] != 1 || statuses["failed"] != 1 {
		t.Fatalf("statuses: %+v", statuses)
	}

	// Connected: the session with steps goes through the bridge.
	if code, body := e.do(2, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "correct-horse"}, nil); code != 200 {
		t.Fatalf("login: %d %s", code, body)
	}
	if code, body := e.do(2, "POST", "/api/plan/push-today", m{"date": today}, &out); code != 200 {
		t.Fatalf("push-today connected: %d %s", code, body)
	}
	pushed := 0
	for _, r := range out.Results {
		if r.Status == "pushed" {
			pushed++
			if r.WorkoutID != 991 {
				t.Fatalf("workout id: %+v", r)
			}
		}
	}
	if pushed != 1 {
		t.Fatalf("pushed: %+v", out.Results)
	}

	// A second run reports « déjà envoyée » and creates nothing new.
	var again struct {
		Results []domain.PushTodayResult `json:"results"`
	}
	if code, body := e.do(2, "POST", "/api/plan/push-today", m{"date": today}, &again); code != 200 {
		t.Fatalf("push-today twice: %d %s", code, body)
	}
	for _, r := range again.Results {
		if r.Status == "pushed" {
			t.Fatalf("a second push created a workout: %+v", again.Results)
		}
	}
}

// A CAPTCHA wall is not a rate limit: different cause, different sentence, same .FIT fallback.
func TestBridgeLoginChallengeIsNotARateLimit(t *testing.T) {
	withBridge(t, bridgeFake(t))
	e := setup(t)
	setBridgeSetting(t, e, true)

	code, body := e.do(2, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "challenge"}, nil)
	if code != http.StatusTooManyRequests {
		t.Fatalf("challenge: %d %s", code, body)
	}
	if !strings.Contains(body, "anti-robot") {
		t.Fatalf("the message must name the challenge: %s", body)
	}
	if strings.Contains(body, "429/427") {
		t.Fatalf("a challenge is not a rate limit: %s", body)
	}
	// The history keeps it: it is not a typo the user can fix in place.
	var inbox struct {
		Items []struct {
			Code string `json:"code"`
		} `json:"items"`
	}
	if c, b := e.do(2, "GET", "/api/inbox", nil, &inbox); c != http.StatusOK {
		t.Fatalf("inbox: %d %s", c, b)
	}
	found := false
	for _, it := range inbox.Items {
		if it.Code == "GARMIN_BRIDGE_CHALLENGE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no challenge notice in the centre: %+v", inbox.Items)
	}
}

// An unexpected page is a 502 that names the step: « identifiants », not a vague provider error.
func TestBridgeLoginFlowErrorNamesTheStep(t *testing.T) {
	withBridge(t, bridgeFake(t))
	e := setup(t)
	setBridgeSetting(t, e, true)

	code, body := e.do(2, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "weird"}, nil)
	if code != http.StatusBadGateway {
		t.Fatalf("flow error: %d %s", code, body)
	}
	if !strings.Contains(body, "identifiants") {
		t.Fatalf("the step must be named: %s", body)
	}
}

func TestBridgeMFAOverHTTP(t *testing.T) {
	fake := bridgeFake(t)
	// A sign-in that asks for a code.
	mfaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sso/signin":
			fmt.Fprint(w, `<html><input type="hidden" name="_csrf" value="c"></html>`)
		case r.Method == http.MethodPost && r.URL.Path == "/sso/signin":
			fmt.Fprint(w, `<html><div id="mfaVerification"></div></html>`)
		case r.Method == http.MethodPost && r.URL.Path == "/sso/verifyMFA/loginEnterMfaCode":
			fmt.Fprint(w, `<html><script>var response_url = "/sso/redirect?ticket=ST-9f8e7d6c5b4a"</script></html>`)
		default:
			// Everything else is the API fake.
			proxy(w, r, fake.URL)
		}
	}))
	t.Cleanup(mfaSrv.Close)
	withBridge(t, mfaSrv)

	e := setup(t)
	setBridgeSetting(t, e, true)
	var res map[string]any
	if code, body := e.do(2, "POST", "/api/garmin/bridge/login", m{"email": "a@b.c", "password": "pw"}, &res); code != http.StatusAccepted || res["state"] != "mfa" {
		t.Fatalf("mfa step: %d %+v %s", code, res, body)
	}
	if code, body := e.do(2, "POST", "/api/garmin/bridge/mfa", m{"code": "123456"}, &res); code != 200 || res["state"] != "connected" {
		t.Fatalf("mfa code: %d %+v %s", code, res, body)
	}
}

// proxy forwards to the API fake (the MFA test needs two hosts behind one URL).
func proxy(w http.ResponseWriter, r *http.Request, base string) {
	req, _ := http.NewRequestWithContext(r.Context(), r.Method, base+r.URL.Path+"?"+r.URL.RawQuery, r.Body)
	req.Header = r.Header.Clone()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = res.Body.Close() }()
	w.WriteHeader(res.StatusCode)
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 512)
	for {
		n, err := res.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil || n == 0 {
			break
		}
	}
	_, _ = w.Write(buf)
}
