package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"stride/internal/apimeter"
	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/secret"
	"stride/internal/store"
)

// Fake configuration values: the overview must never echo any of them.
var secretValues = []string{"master-key-0123456789abcdef012345", "strava-client-secret-xyz", "garmin-admin-key-abc",
	"redis://:valkey-pass@cache:6379", "strava-webhook-token-123", "strava-refresh-token-456"}

type controlEnv struct {
	*testEnv
	dom    *domain.Services
	purged int
}

func setupControl(t *testing.T) *controlEnv {
	t.Helper()
	e := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mk := make([]byte, 32)
	_, _ = rand.Read(mk)
	sealer, err := secret.New(mk, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := env.Config{Timezone: "Europe/Paris", PublicURL: "https://stride.example", RPID: "stride.example", SessionTTL: 720 * time.Hour,
		MasterKey: []byte(secretValues[0]), StravaClientID: "42", StravaClientSecret: secretValues[1], AdminGarminKey: secretValues[2],
		ValkeyURL: secretValues[3], StravaWebhookToken: secretValues[4], AdminStravaRefreshToken: secretValues[5], MetricsAddr: "127.0.0.1:9090"}
	dom, err := domain.New(domain.Deps{Env: cfg, Store: e.st, Cache: e.cache, Sealer: sealer, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		dom.Close(ctx)
	})
	c := &controlEnv{testEnv: e, dom: dom}
	svc, err := auth.New(auth.Options{Env: env.Config{RPID: "localhost", RPOrigins: []string{"http://localhost:8080"}, SessionTTL: time.Hour},
		Store: e.st, Cache: e.cache, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	logs := applog.New(e.st.DB, log)
	t.Cleanup(logs.Close)
	mux := http.NewServeMux()
	New(Deps{Store: e.st, Cache: e.cache, Meter: apimeter.NewLocal(apimeter.Options{Log: log}), Auth: svc, PublicURL: cfg.PublicURL, Log: log,
		Domain: dom, Logs: logs, Runtime: RuntimeFromEnv(cfg, "valkey"),
		Maintenance: func(context.Context) (map[string]int64, error) {
			c.purged++
			return map[string]int64{"sessions": 3}, nil
		}}).Register(mux)
	e.h = mux
	return c
}

func decode[T any](t *testing.T, code int, body []byte, want int) T {
	t.Helper()
	var v T
	if code != want {
		t.Fatalf("status %d (want %d): %s", code, want, body)
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return v
}

// expect runs a request, checks its status and decodes the JSON answer.
func expect[T any](c *controlEnv, want int, token, method, path, body string) T {
	c.t.Helper()
	code, raw := c.do(token, method, path, body)
	var v T
	if code != want {
		c.t.Fatalf("%s %s: status %d (want %d): %s", method, path, code, want, raw)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		c.t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}

// session opens another session for an existing user.
func (c *controlEnv) session(uid int64, token string) string {
	c.t.Helper()
	now := time.Now()
	h := sha256.Sum256([]byte(token))
	if err := c.st.CreateSession(context.Background(), store.Session{TokenHash: h[:], UserID: uid, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}); err != nil {
		c.t.Fatal(err)
	}
	return token
}

var controlRoutes = [][2]string{
	{"GET", "/api/admin/overview"},
	{"GET", "/api/admin/jobs"},
	{"POST", "/api/admin/jobs/job_x/retry"},
	{"POST", "/api/admin/jobs/job_x/cancel"},
	{"GET", "/api/admin/shares"},
	{"DELETE", "/api/admin/shares/1"},
	{"DELETE", "/api/admin/users/1/sessions"},
	{"POST", "/api/admin/users/1/sync"},
	{"POST", "/api/admin/maintenance/purge"},
	{"POST", "/api/admin/maintenance/optimize"},
	{"POST", "/api/admin/maintenance/check"},
}

func TestControlCenterAuthorization(t *testing.T) {
	c := setupControl(t)
	_, _ = c.user("admin")
	_, userTok := c.user("user")
	for _, r := range controlRoutes {
		if code, _ := c.do("", r[0], r[1], ""); code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d", r[0], r[1], code)
		}
		if code, body := c.do(userTok, r[0], r[1], ""); code != http.StatusForbidden {
			t.Errorf("%s %s non-admin: %d %s", r[0], r[1], code, body)
		}
	}
	if c.purged != 0 {
		t.Fatal("maintenance ran for a non-admin")
	}
}

func TestSelfAndLastAdminGuards(t *testing.T) {
	c := setupControl(t)
	adminID, adminTok := c.user("admin")
	otherID, _ := c.user("other")
	self := strconv.FormatInt(adminID, 10)
	for _, r := range [][3]string{
		{"PATCH", "/api/admin/users/" + self, `{"role":"user"}`},
		{"PATCH", "/api/admin/users/" + self, `{"status":"disabled"}`},
		{"DELETE", "/api/admin/users/" + self, ""},
		{"DELETE", "/api/admin/users/" + self + "/sessions", ""},
	} {
		if code, _ := c.do(adminTok, r[0], r[1], r[2]); code != http.StatusConflict {
			t.Fatalf("%s %s on self: %d", r[0], r[1], code)
		}
	}
	// Promote then demote another admin: allowed while the caller stays admin.
	other := strconv.FormatInt(otherID, 10)
	if u := expect[User](c, http.StatusOK, adminTok, "PATCH", "/api/admin/users/"+other, `{"role":"admin"}`); u.Role != "admin" {
		t.Fatalf("promote: %+v", u)
	}
	if code, _ := c.do(adminTok, "PATCH", "/api/admin/users/"+other, `{"role":"user"}`); code != http.StatusOK {
		t.Fatalf("demote: %d", code)
	}
	// The store refuses to remove the last active admin whatever the caller.
	if err := c.st.EnsureNotLastAdmin(context.Background(), adminID); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("last admin: %v", err)
	}
}

func TestDisableAndRevokeSessions(t *testing.T) {
	c := setupControl(t)
	_, adminTok := c.user("admin")
	userID, userTok := c.user("user")
	uid := strconv.FormatInt(userID, 10)

	got := expect[map[string]int64](c, http.StatusOK, adminTok, "DELETE", "/api/admin/users/"+uid+"/sessions", "")
	if got["revoked"] != 1 {
		t.Fatalf("revoke: %+v", got)
	}
	if code, _ := c.do(userTok, "GET", "/api/admin/users", ""); code != http.StatusUnauthorized {
		t.Fatalf("revoked session still valid: %d", code)
	}
	if code, _ := c.do(adminTok, "DELETE", "/api/admin/users/9999/sessions", ""); code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", code)
	}

	// Disabling revokes the new session too and refuses further sessions (login is refused with 403 in auth).
	userTok = c.session(userID, "token-user-2")
	if code, _ := c.do(adminTok, "PATCH", "/api/admin/users/"+uid, `{"status":"disabled"}`); code != http.StatusOK {
		t.Fatalf("disable: %d", code)
	}
	var n int
	if err := c.st.DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, userID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sessions left after disable: %d %v", n, err)
	}
	if code, _ := c.do(userTok, "GET", "/api/admin/users", ""); code != http.StatusUnauthorized {
		t.Fatalf("disabled user session: %d", code)
	}
	c.session(userID, "token-user-3")
	if code, _ := c.do("token-user-3", "GET", "/api/admin/users", ""); code != http.StatusUnauthorized {
		t.Fatalf("a disabled account must not authenticate: %d", code)
	}
	if code, body := c.do(adminTok, "POST", "/api/admin/users/"+uid+"/sync", ""); code != http.StatusConflict {
		t.Fatalf("sync disabled user: %d %s", code, body)
	}
}

func TestUsersStatsAreIsolated(t *testing.T) {
	c := setupControl(t)
	_, adminTok := c.user("admin")
	aliceID, _ := c.user("alice")
	bobID, _ := c.user("bob")
	for i := range 3 {
		if _, err := c.st.DB.Exec(`INSERT INTO activities (user_id, start_utc, start_local, local_date, type, sport, name, created_at, updated_at) VALUES (?, 1, '2026-11-01T10:00:00', '2026-11-01', 'running', 'run', ?, 1, 1)`,
			aliceID, "Sortie "+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	users := expect[[]User](c, http.StatusOK, adminTok, "GET", "/api/admin/users", "")
	byID := map[int64]User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	if byID[aliceID].Activities != 3 || byID[bobID].Activities != 0 || byID[aliceID].Sessions != 1 || byID[aliceID].LastSeenAt == nil || byID[bobID].Integrations == nil {
		t.Fatalf("stats: alice=%+v bob=%+v", byID[aliceID], byID[bobID])
	}
	if code, body := c.do(adminTok, "POST", "/api/admin/users/"+strconv.FormatInt(bobID, 10)+"/sync", ""); code != http.StatusPreconditionFailed {
		t.Fatalf("sync without provider: %d %s", code, body)
	}
	if code, _ := c.do(adminTok, "POST", "/api/admin/users/9999/sync", ""); code != http.StatusNotFound {
		t.Fatalf("sync unknown: %d", code)
	}
}

func TestJobsAcrossUsers(t *testing.T) {
	c := setupControl(t)
	_, adminTok := c.user("admin")
	aliceID, _ := c.user("alice")
	bobID, _ := c.user("bob")
	now := time.Now().Unix()
	for _, j := range []struct {
		id, kind, state string
		uid             int64
		payload         any
	}{
		{"job_hist", "sync.history", "failed", aliceID, `{"from":"2024-01-01"}`},
		{"job_llm", "llm.analyze", "failed", bobID, nil},
		{"job_done", "sync.provider", "completed", bobID, `{"morning":false}`},
	} {
		if _, err := c.st.DB.Exec(`INSERT INTO async_jobs (id, user_id, kind, queue, state, label, attempts, max_attempts, error, result_json, created_at, updated_at, payload_json)
			VALUES (?, ?, ?, 'low', ?, 'Tâche', 1, 1, 'Échec public.', '{"markdown":"analyse privée"}', ?, ?, ?)`, j.id, j.uid, j.kind, j.state, now, now, j.payload); err != nil {
			t.Fatal(err)
		}
	}
	code, body := c.do(adminTok, "GET", "/api/admin/jobs?state=all", "")
	if strings.Contains(string(body), "analyse privée") {
		t.Fatal("job results must not reach the administration")
	}
	list := decode[struct {
		Items  []AdminJob `json:"items"`
		Groups []struct {
			Group string `json:"group"`
		} `json:"groups"`
	}](t, code, body, http.StatusOK)
	if len(list.Items) != 3 || len(list.Groups) != 3 {
		t.Fatalf("jobs: %s", body)
	}
	for _, j := range list.Items {
		if j.UserName == "" || j.Retryable != (j.ID == "job_hist") || j.Cancellable {
			t.Fatalf("job view: %+v", j)
		}
	}
	failed := expect[struct {
		Items []AdminJob `json:"items"`
	}](c, http.StatusOK, adminTok, "GET", "/api/admin/jobs?state=failed", "")
	if len(failed.Items) != 2 {
		t.Fatalf("failed filter: %+v", failed.Items)
	}
	if code, _ := c.do(adminTok, "GET", "/api/admin/jobs?state=bogus", ""); code != http.StatusBadRequest {
		t.Fatalf("bad filter: %d", code)
	}

	retried := expect[AdminJob](c, http.StatusAccepted, adminTok, "POST", "/api/admin/jobs/job_hist/retry", "")
	if retried.UserID != aliceID || retried.ID == "job_hist" || retried.Kind != "sync.history" || retried.UserName != "alice" {
		t.Fatalf("retry must replay for the job owner: %+v", retried)
	}
	if code, _ := c.do(adminTok, "POST", "/api/admin/jobs/job_llm/retry", ""); code != http.StatusConflict {
		t.Fatalf("llm retry: %d", code)
	}
	if code, _ := c.do(adminTok, "POST", "/api/admin/jobs/job_done/cancel", ""); code != http.StatusConflict {
		t.Fatalf("cancel finished: %d", code)
	}
	if code, _ := c.do(adminTok, "POST", "/api/admin/jobs/job_nope/cancel", ""); code != http.StatusNotFound {
		t.Fatalf("cancel unknown: %d", code)
	}
	if code, _ := c.do(adminTok, "POST", "/api/admin/jobs/bad%20id/cancel", ""); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}
}

func TestShareLinksAcrossUsers(t *testing.T) {
	c := setupControl(t)
	ctx := context.Background()
	_, adminTok := c.user("admin")
	aliceID, aliceTok := c.user("alice")
	bobID, _ := c.user("bob")
	link, err := c.dom.CreateShareLink(ctx, aliceID, map[string]json.RawMessage{"label": json.RawMessage(`"Kiné"`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.dom.CreateShareLink(ctx, bobID, map[string]json.RawMessage{"label": json.RawMessage(`"Coach"`)}); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimPrefix(link.URL, "/share/")
	code, body := c.do(adminTok, "GET", "/api/admin/shares", "")
	if strings.Contains(string(body), token) || strings.Contains(string(body), `"url"`) {
		t.Fatalf("share tokens and URLs must never be listed: %s", body)
	}
	links := decode[[]domain.AdminShareLink](t, code, body, http.StatusOK)
	if len(links) != 2 || links[0].TokenPrefix == "" || links[0].State != "active" {
		t.Fatalf("links: %s", body)
	}
	if code, _ := c.do(aliceTok, "DELETE", "/api/admin/shares/"+strconv.FormatInt(link.ID, 10), ""); code != http.StatusForbidden {
		t.Fatalf("non-admin revoke: %d", code)
	}
	if code, _ := c.do(adminTok, "DELETE", "/api/admin/shares/"+strconv.FormatInt(link.ID, 10), ""); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if _, err := c.dom.GetPublicShare(ctx, token); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoked link still public: %v", err)
	}
	active := expect[[]domain.AdminShareLink](c, http.StatusOK, adminTok, "GET", "/api/admin/shares", "")
	if len(active) != 1 || active[0].UserID != bobID {
		t.Fatalf("only bob's link stays active: %+v", active)
	}
	all := expect[[]domain.AdminShareLink](c, http.StatusOK, adminTok, "GET", "/api/admin/shares?state=all", "")
	if len(all) != 2 {
		t.Fatalf("all: %+v", all)
	}
	if code, _ := c.do(adminTok, "DELETE", "/api/admin/shares/9999", ""); code != http.StatusNotFound {
		t.Fatalf("unknown link: %d", code)
	}
}

func TestOverviewAndMaintenance(t *testing.T) {
	c := setupControl(t)
	_, adminTok := c.user("admin")
	_, _ = c.user("alice")
	code, body := c.do(adminTok, "GET", "/api/admin/overview", "")
	for _, s := range secretValues {
		if strings.Contains(string(body), s) {
			t.Fatalf("overview leaks a secret: %s", body)
		}
	}
	ov := decode[Overview](t, code, body, http.StatusOK)
	if ov.Counts.Users != 2 || ov.Counts.Admins != 1 || len(ov.Jobs.Groups) != 3 || !ov.Cache.OK || ov.Cache.Backend != "valkey" ||
		!ov.Database.OK || ov.Database.SizeBytes == 0 || !ov.Config.MetricsEnabled || !ov.Config.StravaEnabled || ov.Config.SessionTTLHours != 720 ||
		ov.RegistrationMode == "" || ov.Build.Version == "" {
		t.Fatalf("overview: %s", body)
	}

	purge := expect[struct {
		Removed map[string]int64 `json:"removed"`
	}](c, http.StatusOK, adminTok, "POST", "/api/admin/maintenance/purge", "")
	if purge.Removed["sessions"] != 3 || c.purged != 1 {
		t.Fatalf("purge: %+v", purge)
	}
	check := expect[struct {
		OK       bool     `json:"ok"`
		Problems []string `json:"problems"`
	}](c, http.StatusOK, adminTok, "POST", "/api/admin/maintenance/check", "")
	if !check.OK || len(check.Problems) != 0 {
		t.Fatalf("check: %+v", check)
	}
	opt := expect[struct {
		Vacuum bool           `json:"vacuum"`
		After  DatabaseStatus `json:"after"`
	}](c, http.StatusOK, adminTok, "POST", "/api/admin/maintenance/optimize", `{"vacuum":true}`)
	if !opt.Vacuum || !opt.After.OK {
		t.Fatalf("optimize: %+v", opt)
	}
	if code, _ := c.do(adminTok, "POST", "/api/admin/maintenance/optimize", `{"vacuum":"yes"}`); code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", code)
	}
}
