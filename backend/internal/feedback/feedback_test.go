package feedback

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"stride/internal/auth"
	"stride/internal/cache"
	"stride/internal/env"
	"stride/internal/store"
)

type env_ struct {
	t  *testing.T
	st *store.Store
	h  http.Handler
}

func setup(t *testing.T) *env_ {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "fb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := auth.New(auth.Options{Env: env.Config{RPID: "localhost", RPOrigins: []string{"http://localhost"}, SessionTTL: time.Hour},
		Store: st, Cache: cache.NewMemory(), Log: log})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(Deps{DB: st.DB, Auth: svc, Log: log}).Register(mux)
	return &env_{t: t, st: st, h: mux}
}

func (e *env_) user(name string) (int64, string) {
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
	tok := "token-" + name
	h := sha256.Sum256([]byte(tok))
	if err := e.st.CreateSession(ctx, store.Session{TokenHash: h[:], UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}); err != nil {
		e.t.Fatal(err)
	}
	return u.ID, tok
}

func (e *env_) do(tok, method, path, body string) (int, []byte) {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: tok})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

type listResp struct {
	Items  []Item `json:"items"`
	Counts Counts `json:"counts"`
}

func (e *env_) list(tok, query string) listResp {
	e.t.Helper()
	code, body := e.do(tok, "GET", "/api/admin/feedback"+query, "")
	var l listResp
	if code != http.StatusOK || json.Unmarshal(body, &l) != nil {
		e.t.Fatalf("list %s: %d %s", query, code, body)
	}
	return l
}

func TestCreateSanitizesAndKeepsEmailOnlyWithConsent(t *testing.T) {
	e := setup(t)
	_, adminTok := e.user("admin")
	aliceID, aliceTok := e.user("alice")
	body := `{"category":"bug","message":"  Le graphique\u0000 plante\r\nquand je zoome.​ ","contactOk":true,"contactEmail":"alice@example.org",
		"context":{"appVersion":"4.1.0","route":"/share/st4_abcdefghijklmnop?invite=SECRET#x","viewport":"390×844","userAgent":"Mozilla/5.0\nTest"}}`
	if code, b := e.do(aliceTok, "POST", "/api/feedback", body); code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, b)
	}
	if code, b := e.do(aliceTok, "POST", "/api/feedback", `{"category":"idea","message":"Un mode sombre plus contrasté","contactOk":false,"contactEmail":"alice@example.org","context":{}}`); code != http.StatusCreated {
		t.Fatalf("create without consent: %d %s", code, b)
	}
	var stored int
	if err := e.st.DB.QueryRow(`SELECT COUNT(*) FROM feedback WHERE contact_email IS NOT NULL`).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("an e-mail must only be stored with consent: %d %v", stored, err)
	}
	l := e.list(adminTok, "")
	if len(l.Items) != 2 || l.Counts.New != 2 || l.Counts.Total != 2 {
		t.Fatalf("list: %+v", l)
	}
	bug := l.Items[1]
	if bug.Category != "bug" || bug.UserID != aliceID || bug.UserName != "alice" || bug.Message != "Le graphique plante\nquand je zoome." ||
		bug.ContactEmail == nil || *bug.ContactEmail != "alice@example.org" {
		t.Fatalf("bug: %+v", bug)
	}
	if bug.Context.Route != "/share/st4_abcd…" || bug.Context.UserAgent != "Mozilla/5.0 Test" || bug.Context.Viewport != "390×844" {
		t.Fatalf("context must drop queries, fragments and share tokens: %+v", bug.Context)
	}
	if l.Items[0].ContactEmail != nil || l.Items[0].ContactOK {
		t.Fatalf("no consent, no e-mail: %+v", l.Items[0])
	}
}

func TestValidationAndRateLimit(t *testing.T) {
	e := setup(t)
	_, tok := e.user("alice")
	long := strings.Repeat("é", MaxMessage+1)
	for name, body := range map[string]string{
		"category":      `{"category":"rant","message":"Bonjour à tous"}`,
		"short":         `{"category":"bug","message":"  ok  "}`,
		"long":          `{"category":"bug","message":"` + long + `"}`,
		"consent-email": `{"category":"bug","message":"Bonjour à tous","contactOk":true}`,
		"bad-email":     `{"category":"bug","message":"Bonjour à tous","contactOk":true,"contactEmail":"Alice <a@b.fr>"}`,
		"unknown":       `{"category":"bug","message":"Bonjour à tous","userId":2}`,
		"oversized":     `{"category":"bug","message":"Bonjour à tous","context":{"userAgent":"` + strings.Repeat("a", 20<<10) + `"}}`,
	} {
		if code, b := e.do(tok, "POST", "/api/feedback", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, b)
		}
	}
	if code, _ := e.do("", "POST", "/api/feedback", `{"category":"bug","message":"Bonjour à tous"}`); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
	for i := range RateLimit {
		if code, b := e.do(tok, "POST", "/api/feedback", `{"category":"other","message":"Avis numéro `+strconv.Itoa(i)+`"}`); code != http.StatusCreated {
			t.Fatalf("create %d: %d %s", i, code, b)
		}
	}
	if code, _ := e.do(tok, "POST", "/api/feedback", `{"category":"other","message":"Un avis de trop"}`); code != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", code)
	}
}

func TestAdminTriageIsAdminOnlyAndCascades(t *testing.T) {
	e := setup(t)
	_, adminTok := e.user("admin")
	aliceID, aliceTok := e.user("alice")
	_, bobTok := e.user("bob")
	for _, b := range []string{`{"category":"bug","message":"Bug de alice"}`, `{"category":"idea","message":"Idée de alice"}`} {
		if code, _ := e.do(aliceTok, "POST", "/api/feedback", b); code != http.StatusCreated {
			t.Fatal(code)
		}
	}
	if code, _ := e.do(bobTok, "POST", "/api/feedback", `{"category":"other","message":"Remarque de bob"}`); code != http.StatusCreated {
		t.Fatal(code)
	}
	for _, r := range [][3]string{{"GET", "/api/admin/feedback", ""}, {"PATCH", "/api/admin/feedback/1", `{"status":"done"}`}} {
		if code, _ := e.do("", r[0], r[1], r[2]); code != http.StatusUnauthorized {
			t.Fatalf("%s anonymous: %d", r[1], code)
		}
		if code, _ := e.do(aliceTok, r[0], r[1], r[2]); code != http.StatusForbidden {
			t.Fatalf("%s non-admin: %d", r[1], code)
		}
	}

	ideas := e.list(adminTok, "?category=idea")
	if len(ideas.Items) != 1 || ideas.Items[0].Message != "Idée de alice" || ideas.Counts.Total != 3 {
		t.Fatalf("category filter: %+v", ideas)
	}
	id := strconv.FormatInt(ideas.Items[0].ID, 10)
	code, b := e.do(adminTok, "PATCH", "/api/admin/feedback/"+id, `{"status":"done","adminNote":"Prévu pour la 4.2"}`)
	var it Item
	if code != http.StatusOK || json.Unmarshal(b, &it) != nil || it.Status != "done" || it.AdminNote != "Prévu pour la 4.2" {
		t.Fatalf("patch: %d %s", code, b)
	}
	if l := e.list(adminTok, "?status=new"); len(l.Items) != 2 || l.Counts.New != 2 || l.Counts.Done != 1 {
		t.Fatalf("status filter: %+v", l)
	}
	for body, want := range map[string]int{`{"status":"archived"}`: 400, `{"adminNote":"` + strings.Repeat("x", MaxNote+1) + `"}`: 400} {
		if code, _ := e.do(adminTok, "PATCH", "/api/admin/feedback/"+id, body); code != want {
			t.Fatalf("patch %s: %d", body[:20], code)
		}
	}
	if code, _ := e.do(adminTok, "PATCH", "/api/admin/feedback/999", `{"status":"read"}`); code != http.StatusNotFound {
		t.Fatalf("unknown: %d", code)
	}
	if code, _ := e.do(adminTok, "GET", "/api/admin/feedback?status=bogus", ""); code != http.StatusBadRequest {
		t.Fatalf("bad filter: %d", code)
	}

	// Deleting a user deletes their feedback, nobody else's.
	if err := e.st.DeleteUser(context.Background(), aliceID); err != nil {
		t.Fatal(err)
	}
	if l := e.list(adminTok, ""); len(l.Items) != 1 || l.Items[0].Message != "Remarque de bob" {
		t.Fatalf("cascade: %+v", l.Items)
	}
}
