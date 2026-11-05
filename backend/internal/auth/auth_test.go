package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"stride/internal/cache"
	"stride/internal/env"
	"stride/internal/store"
)

type created struct {
	id    int64
	first bool
}

type fixture struct {
	t       *testing.T
	srv     *httptest.Server
	store   *store.Store
	svc     *Service
	authn   softAuthn
	mu      sync.Mutex
	created []created
	deleted []int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	f := &fixture{t: t, store: st, authn: softAuthn{rpID: "localhost", origin: "http://localhost:8080"}}
	svc, err := New(Options{
		Env:   env.Config{RPID: "localhost", RPOrigins: []string{"http://localhost:8080"}, SessionTTL: time.Hour},
		Store: st,
		Cache: cache.NewMemory(),
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnUserCreated: func(_ context.Context, id int64, first bool) error {
			f.mu.Lock()
			f.created = append(f.created, created{id, first})
			f.mu.Unlock()
			return nil
		},
		OnUserDeleted: func(_ context.Context, id int64) error {
			f.mu.Lock()
			f.deleted = append(f.deleted, id)
			f.mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	mux := http.NewServeMux()
	svc.Register(mux)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

type browser struct {
	f *fixture
	c *http.Client
}

func (f *fixture) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{f: f, c: &http.Client{Jar: jar}}
}

func (b *browser) do(method, path string, body []byte) (int, []byte) {
	b.f.t.Helper()
	req, _ := http.NewRequest(method, b.f.srv.URL+path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/140.0 Safari/537.36")
	res, err := b.c.Do(req)
	if err != nil {
		b.f.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func (b *browser) register(name, invite string) (int, []byte, *softCred) {
	b.f.t.Helper()
	req, _ := json.Marshal(map[string]string{"displayName": name, "inviteCode": invite})
	code, opts := b.do("POST", "/api/auth/register/begin", req)
	if code != http.StatusOK {
		return code, opts, nil
	}
	resp, cred := b.f.authn.create(b.f.t, opts)
	code, out := b.do("POST", "/api/auth/register/finish", resp)
	return code, out, cred
}

func (b *browser) login(c *softCred) (int, []byte) {
	b.f.t.Helper()
	code, opts := b.do("POST", "/api/auth/login/begin", []byte(`{}`))
	if code != http.StatusOK {
		b.f.t.Fatalf("login begin %d %s", code, opts)
	}
	return b.do("POST", "/api/auth/login/finish", b.f.authn.get(b.f.t, opts, c))
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return v
}

func TestPasskeyRegistrationLoginAndSessions(t *testing.T) {
	f := newFixture(t)
	alice := f.browser()

	code, body := alice.do("GET", "/api/auth/state", nil)
	st := decode[State](t, body)
	if code != 200 || st.Authenticated || !st.FirstUser || st.RegistrationMode != "invite" || st.RPID != "localhost" {
		t.Fatalf("initial state %d %s", code, body)
	}
	if code, _, _ := alice.register("   ", ""); code != http.StatusBadRequest {
		t.Fatalf("empty name: %d", code)
	}

	code, body, aliceKey := alice.register("Alice  Test", "")
	me := decode[Me](t, body)
	if code != 200 || me.Role != "admin" || me.DisplayName != "Alice Test" || me.OnboardingCompleted {
		t.Fatalf("register: %d %s", code, body)
	}
	if len(f.created) != 1 || !f.created[0].first || f.created[0].id != me.ID {
		t.Fatalf("OnUserCreated: %+v", f.created)
	}
	if code, body := alice.do("GET", "/api/me", nil); code != 200 || decode[Me](t, body).ID != me.ID {
		t.Fatalf("me: %d %s", code, body)
	}
	if code, _ := alice.do("POST", "/api/auth/logout", nil); code != http.StatusNoContent {
		t.Fatalf("logout: %d", code)
	}
	if code, _ := alice.do("GET", "/api/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", code)
	}
	if code, body := alice.login(aliceKey); code != 200 || decode[Me](t, body).ID != me.ID {
		t.Fatalf("login: %d %s", code, body)
	}
	code, body = alice.do("GET", "/api/me/passkeys", nil)
	keys := decode[[]Passkey](t, body)
	if code != 200 || len(keys) != 1 || keys[0].LastUsedAt == nil || !keys[0].BackedUp || keys[0].Name != "Chrome · macOS" {
		t.Fatalf("passkeys: %d %s", code, body)
	}

	// replaying a consumed ceremony fails
	_, opts := alice.do("POST", "/api/auth/login/begin", []byte(`{}`))
	resp := f.authn.get(t, opts, aliceKey)
	if code, _ := alice.do("POST", "/api/auth/login/finish", resp); code != 200 {
		t.Fatalf("login 2: %d", code)
	}
	if code, _ := alice.do("POST", "/api/auth/login/finish", resp); code != http.StatusBadRequest {
		t.Fatalf("replay: %d", code)
	}

	// a challenge from another ceremony is rejected
	_, opts1 := alice.do("POST", "/api/auth/login/begin", []byte(`{}`))
	other := f.browser()
	_, _ = other.do("POST", "/api/auth/login/begin", []byte(`{}`))
	if code, _ := alice.do("POST", "/api/auth/login/finish", f.authn.get(t, bytes.Replace(opts1, []byte(`"challenge":"`), []byte(`"challenge":"AAAA`), 1), aliceKey)); code != http.StatusUnauthorized {
		t.Fatalf("tampered challenge: %d", code)
	}

	code, body = alice.do("PATCH", "/api/me", []byte(`{"displayName":"Alice","onboardingCompleted":true}`))
	if me := decode[Me](t, body); code != 200 || me.DisplayName != "Alice" || !me.OnboardingCompleted {
		t.Fatalf("patch me: %d %s", code, body)
	}
}

func TestInvitationsDisabledUsersAndPasskeyManagement(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	alice, bob := f.browser(), f.browser()
	_, body, _ := alice.register("Alice", "")
	aliceID := decode[Me](t, body).ID

	if code, body, _ := bob.register("Bob", ""); code != http.StatusForbidden || !strings.Contains(string(body), "INVITE_REQUIRED") {
		t.Fatalf("no invite: %d %s", code, body)
	}
	if code, _, _ := bob.register("Bob", "WRONG"); code != http.StatusForbidden {
		t.Fatalf("bad invite: %d", code)
	}
	if _, err := f.store.CreateInvitation(ctx, store.Invitation{CodePrefix: "ABCD", MaxUses: 1, CreatedAt: time.Now()}, InviteHash("ABCD-EFGH")); err != nil {
		t.Fatal(err)
	}
	code, body, bobKey := bob.register("Bob", "abcdefgh")
	bobMe := decode[Me](t, body)
	if code != 200 || bobMe.Role != "user" || f.created[1].first {
		t.Fatalf("invited register: %d %s %+v", code, body, f.created)
	}

	// passkeys: add, rename, delete, refuse last
	code, opts := alice.do("POST", "/api/me/passkeys/begin", []byte(`{"name":"Téléphone"}`))
	if code != 200 || !strings.Contains(string(opts), "excludeCredentials") {
		t.Fatalf("add begin: %d %s", code, opts)
	}
	resp, _ := f.authn.create(t, opts)
	code, body = alice.do("POST", "/api/me/passkeys/finish", resp)
	added := decode[Passkey](t, body)
	if code != 200 || added.Name != "Téléphone" {
		t.Fatalf("add finish: %d %s", code, body)
	}
	if code, body := alice.do("PATCH", "/api/me/passkeys/"+added.ID, []byte(`{"name":"iPhone"}`)); code != 200 || decode[Passkey](t, body).Name != "iPhone" {
		t.Fatalf("rename: %d %s", code, body)
	}
	if code, _ := bob.do("DELETE", "/api/me/passkeys/"+added.ID, nil); code != http.StatusNotFound {
		t.Fatalf("foreign delete: %d", code)
	}
	if code, _ := alice.do("DELETE", "/api/me/passkeys/"+added.ID, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	_, body = alice.do("GET", "/api/me/passkeys", nil)
	last := decode[[]Passkey](t, body)[0]
	if code, _ := alice.do("DELETE", "/api/me/passkeys/"+last.ID, nil); code != http.StatusConflict {
		t.Fatalf("delete last: %d", code)
	}

	// the sole admin cannot delete their account while other users exist
	if code, _ := alice.do("DELETE", "/api/me", nil); code != http.StatusConflict {
		t.Fatalf("sole admin delete: %d", code)
	}

	// disabling revokes sessions and blocks login
	disabled := store.StatusDisabled
	if err := f.store.UpdateUserAccess(ctx, bobMe.ID, nil, &disabled); err != nil {
		t.Fatal(err)
	}
	if code, _ := bob.do("GET", "/api/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("disabled session: %d", code)
	}
	if code, body := bob.login(bobKey); code != http.StatusForbidden {
		t.Fatalf("disabled login: %d %s", code, body)
	}

	if code, _ := bob.do("DELETE", "/api/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous delete: %d", code)
	}
	if err := f.svc.DeleteUser(ctx, bobMe.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := alice.do("DELETE", "/api/me", nil); code != http.StatusNoContent {
		t.Fatalf("delete me: %d", code)
	}
	if len(f.deleted) != 2 || f.deleted[1] != aliceID {
		t.Fatalf("OnUserDeleted: %+v", f.deleted)
	}
	_, body = alice.do("GET", "/api/auth/state", nil)
	if st := decode[State](t, body); st.Authenticated || !st.FirstUser {
		t.Fatalf("state after deletions: %s", body)
	}
}

func TestDefaultPasskeyName(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X) AppleWebKit/605.1.15 Version/19.0 Mobile/15E148 Safari/604.1": "Safari · iPhone",
		"Mozilla/5.0 (Linux; Android 16) AppleWebKit/537.36 Chrome/140.0 Mobile Safari/537.36":                                "Chrome · Android",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/140.0 Safari/537.36 Edg/140.0":                   "Edge · Windows",
		"curl/8": "Clé d'accès du 04/11/2026",
	}
	for ua, want := range cases {
		if got := defaultPasskeyName(ua, now); got != want {
			t.Errorf("%q → %q, want %q", ua, got, want)
		}
	}
}
