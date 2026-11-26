package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"stride/internal/apimeter"
	"stride/internal/auth"
	"stride/internal/cache"
	"stride/internal/env"
	"stride/internal/store"
)

type testEnv struct {
	t       *testing.T
	st      *store.Store
	cache   *cache.Memory
	h       http.Handler
	deleted []int64
}

func setup(t *testing.T, opts ...func(*Deps)) *testEnv {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &testEnv{t: t, st: st, cache: cache.NewMemory()}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := auth.New(auth.Options{
		Env:   env.Config{RPID: "localhost", RPOrigins: []string{"http://localhost:8080"}, SessionTTL: time.Hour},
		Store: st, Cache: e.cache, Log: log,
		OnUserDeleted: func(_ context.Context, id int64) error { e.deleted = append(e.deleted, id); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	d := Deps{Store: st, Cache: e.cache, Meter: apimeter.NewLocal(apimeter.Options{Log: log}), Auth: svc, PublicURL: "https://stride.example/", Log: log}
	for _, o := range opts {
		o(&d)
	}
	New(d).Register(mux)
	e.h = mux
	return e
}

// user registers a user directly in the store and returns a session token.
func (e *testEnv) user(name string) (int64, string) {
	e.t.Helper()
	ctx := context.Background()
	now := time.Now()
	if n, _ := e.st.CountUsers(ctx); n > 0 {
		_ = e.st.SetSetting(ctx, store.SettingRegistrationMode, store.RegistrationOpen, now)
	}
	u, err := e.st.RegisterUser(ctx, store.NewUser{WebAuthnID: []byte("h-" + name), DisplayName: name,
		Credential: store.NewCredential{ID: []byte("c-" + name), Name: "k", Data: []byte(`{}`)}}, now)
	if err != nil {
		e.t.Fatal(err)
	}
	token := "token-" + name
	h := sha256.Sum256([]byte(token))
	if err := e.st.CreateSession(ctx, store.Session{TokenHash: h[:], UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}); err != nil {
		e.t.Fatal(err)
	}
	return u.ID, token
}

// count runs a COUNT(*) query against the read pool (the notification-centre assertions).
func (e *testEnv) count(query string) int64 {
	e.t.Helper()
	var n int64
	if err := e.st.R.QueryRow(query).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *testEnv) do(token, method, path, body string) (int, []byte) {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func TestAccessControl(t *testing.T) {
	e := setup(t)
	_, adminTok := e.user("admin")
	_, userTok := e.user("user")
	if code, _ := e.do("", "GET", "/api/admin/users", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
	if code, body := e.do(userTok, "GET", "/api/admin/users", ""); code != http.StatusForbidden || !bytes.Contains(body, []byte("FORBIDDEN")) {
		t.Fatalf("non-admin: %d %s", code, body)
	}
	code, body := e.do(adminTok, "GET", "/api/admin/users", "")
	var users []User
	_ = json.Unmarshal(body, &users)
	if code != 200 || len(users) != 2 || users[0].Role != "admin" || users[0].Passkeys != 1 {
		t.Fatalf("list: %d %s", code, body)
	}
}

func TestUserManagement(t *testing.T) {
	e := setup(t)
	adminID, adminTok := e.user("admin")
	userID, userTok := e.user("user")
	uid := strconv.FormatInt(userID, 10)

	if code, _ := e.do(adminTok, "PATCH", "/api/admin/users/"+strconv.FormatInt(adminID, 10), `{"status":"disabled"}`); code != http.StatusConflict {
		t.Fatalf("self patch: %d", code)
	}
	if code, _ := e.do(adminTok, "PATCH", "/api/admin/users/"+uid, `{"role":"root"}`); code != http.StatusBadRequest {
		t.Fatalf("bad role: %d", code)
	}
	code, body := e.do(adminTok, "PATCH", "/api/admin/users/"+uid, `{"status":"disabled","role":"admin"}`)
	var u User
	_ = json.Unmarshal(body, &u)
	if code != 200 || u.Status != "disabled" || u.Role != "admin" {
		t.Fatalf("patch: %d %s", code, body)
	}
	if code, _ := e.do(userTok, "GET", "/api/admin/users", ""); code != http.StatusUnauthorized {
		t.Fatalf("disabled user session must be revoked: %d", code)
	}
	if code, _ := e.do(adminTok, "PATCH", "/api/admin/users/9999", `{"status":"active"}`); code != http.StatusNotFound {
		t.Fatalf("unknown: %d", code)
	}
	if code, _ := e.do(adminTok, "DELETE", "/api/admin/users/"+strconv.FormatInt(adminID, 10), ""); code != http.StatusConflict {
		t.Fatalf("self delete: %d", code)
	}
	if code, _ := e.do(adminTok, "DELETE", "/api/admin/users/"+uid, ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if len(e.deleted) != 1 || e.deleted[0] != userID {
		t.Fatalf("purge hook: %+v", e.deleted)
	}
}

func TestInvitations(t *testing.T) {
	e := setup(t)
	_, adminTok := e.user("admin")
	code, body := e.do(adminTok, "POST", "/api/admin/invitations", `{"label":"Club"}`)
	var created struct {
		Invitation Invitation `json:"invitation"`
		Code       string     `json:"code"`
		Link       string     `json:"link"`
	}
	_ = json.Unmarshal(body, &created)
	if code != http.StatusCreated || !regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){5}$`).MatchString(created.Code) {
		t.Fatalf("create: %d %s", code, body)
	}
	if created.Link != "https://stride.example/register?invite="+created.Code || created.Invitation.CodePrefix != created.Code[:4] ||
		created.Invitation.MaxUses != 1 || created.Invitation.ExpiresAt == nil || created.Invitation.ExpiresAt.Sub(created.Invitation.CreatedAt) != 168*time.Hour {
		t.Fatalf("invitation: %+v", created)
	}
	ctx := context.Background()
	if err := e.st.CheckInvitation(ctx, auth.InviteHash(strings.ToLower(strings.ReplaceAll(created.Code, "-", ""))), time.Now()); err != nil {
		t.Fatalf("code must validate: %v", err)
	}
	code, body = e.do(adminTok, "POST", "/api/admin/invitations", `{"maxUses":5,"expiresInHours":0}`)
	if code != http.StatusCreated || !bytes.Contains(body, []byte(`"expiresAt":null`)) {
		t.Fatalf("no expiry: %d %s", code, body)
	}
	if code, _ := e.do(adminTok, "POST", "/api/admin/invitations", `{"maxUses":0}`); code != http.StatusBadRequest {
		t.Fatalf("bad maxUses: %d", code)
	}
	code, body = e.do(adminTok, "GET", "/api/admin/invitations", "")
	if code != 200 || bytes.Contains(body, []byte(created.Code)) || !bytes.Contains(body, []byte(`"codePrefix"`)) {
		t.Fatalf("list must never expose codes: %d %s", code, body)
	}
	id := strconv.FormatInt(created.Invitation.ID, 10)
	if code, _ := e.do(adminTok, "DELETE", "/api/admin/invitations/"+id, ""); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if err := e.st.CheckInvitation(ctx, auth.InviteHash(created.Code), time.Now()); err == nil {
		t.Fatal("revoked invitation still valid")
	}
}

func TestSettingsMeterAndCacheFlush(t *testing.T) {
	e := setup(t)
	_, adminTok := e.user("admin")
	// The notification-centre retention ships with the defaults (issue #142).
	if code, body := e.do(adminTok, "GET", "/api/admin/settings", ""); code != 200 ||
		string(bytes.TrimSpace(body)) != `{"registrationMode":"invite","inboxRetentionDays":10,"inboxRetentionMax":50,"updateCheckEnabled":true,"garminBridgeEnabled":false}` {
		t.Fatalf("settings: %d %s", code, body)
	}
	if code, _ := e.do(adminTok, "PUT", "/api/admin/settings", `{"registrationMode":"public"}`); code != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", code)
	}
	if code, _ := e.do(adminTok, "PUT", "/api/admin/settings", `{"registrationMode":"open"}`); code != 200 {
		t.Fatalf("put: %d", code)
	}
	if m, _ := e.st.RegistrationMode(context.Background()); m != "open" {
		t.Fatalf("mode %q", m)
	}

	// Retention is clamped, not rejected, and a field left out keeps its current value.
	code, body := e.do(adminTok, "PUT", "/api/admin/settings", `{"registrationMode":"open","inboxRetentionDays":0,"inboxRetentionMax":5000}`)
	if code != 200 || !strings.Contains(string(body), `"inboxRetentionDays":10`) || !strings.Contains(string(body), `"inboxRetentionMax":200`) {
		t.Fatalf("retention clamp: %d %s", code, body)
	}
	if days, max := e.st.InboxRetention(context.Background()); days != 10 || max != 200 {
		t.Fatalf("stored retention: %d/%d", days, max)
	}
	if code, body := e.do(adminTok, "PUT", "/api/admin/settings", `{"inboxRetentionDays":3,"inboxRetentionMax":2}`); code != 200 ||
		!strings.Contains(string(body), `"inboxRetentionDays":3`) || !strings.Contains(string(body), `"registrationMode":"open"`) {
		t.Fatalf("retention save: %d %s", code, body)
	}
	// The experimental bridge switch is off by default and persists false/true like the others.
	if code, body := e.do(adminTok, "PUT", "/api/admin/settings", `{"garminBridgeEnabled":true}`); code != 200 ||
		!strings.Contains(string(body), `"garminBridgeEnabled":true`) {
		t.Fatalf("bridge switch: %d %s", code, body)
	}
	if !e.st.GarminBridgeEnabled(context.Background()) {
		t.Fatal("bridge switch not stored")
	}
	if code, body := e.do(adminTok, "PUT", "/api/admin/settings", `{"inboxRetentionDays":4}`); code != 200 ||
		!strings.Contains(string(body), `"garminBridgeEnabled":true`) {
		t.Fatalf("bridge switch reset by another field: %d %s", code, body)
	}

	// A setting left out keeps its value: the retention panel never resets the registration mode.
	if m, _ := e.st.RegistrationMode(context.Background()); m != "open" {
		t.Fatalf("retention-only PUT changed the mode: %q", m)
	}
	if code, _ := e.do(adminTok, "PUT", "/api/admin/settings", `{"registrationMode":"nope"}`); code != http.StatusBadRequest {
		t.Fatalf("invalid mode accepted: %d", code)
	}
	code, body = e.do(adminTok, "GET", "/api/admin/meter", "")
	var snap apimeter.Snapshot
	_ = json.Unmarshal(body, &snap)
	if code != 200 || len(snap.Providers) != 3 || snap.RecentErrors == nil {
		t.Fatalf("meter: %d %s", code, body)
	}
	ctx := context.Background()
	for _, k := range []string{"stride:cache:a", "stride:cache:b", "stride:webauthn:x"} {
		_ = e.cache.Set(ctx, k, []byte("1"), time.Minute)
	}
	if code, body := e.do(adminTok, "POST", "/api/admin/cache/flush", ""); code != 200 || string(bytes.TrimSpace(body)) != `{"removed":2}` {
		t.Fatalf("flush: %d %s", code, body)
	}
	if _, ok, _ := e.cache.Get(ctx, "stride:webauthn:x"); !ok {
		t.Fatal("flush must keep non-cache keys")
	}
}
