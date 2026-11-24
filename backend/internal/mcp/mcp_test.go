// mcp_test.go tests the per-user MCP server end to end: the /api/mcp* management endpoints,
// the public route POST /mcp/{token} (stateless Streamable HTTP, official SDK client), the
// token lifecycle, the per-user isolation, the rate limits and the cross-origin protection.
package mcp_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/mcp"
	"stride/internal/secret"
	"stride/internal/store"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var paris, _ = time.LoadLocation("Europe/Paris")

// initMsg is a minimal valid initialize request.
const initMsg = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18",` +
	`"capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`

type harn struct {
	t    *testing.T
	srv  *httptest.Server
	st   *store.Store
	dom  *domain.Services
	logs *applog.Log
}

// newHarness builds a real SQLite store, domain services (no providers) and the MCP routes,
// following the domainapi test pattern (X-Test-User stands in for the session).
func newHarness(t *testing.T) *harn {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for i, name := range []string{"admin", "alice", "bob"} {
		role := "user"
		if i == 0 {
			role = "admin"
		}
		if _, err := st.DB.Exec(`INSERT INTO users (id, display_name, role, created_at) VALUES (?, ?, ?, 1)`, i+1, name, role); err != nil {
			t.Fatal(err)
		}
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	sealer, _ := secret.New(key, nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	logs := applog.New(st.DB, log)
	t.Cleanup(func() { logs.Close() })
	dom, err := domain.New(domain.Deps{
		Env:   env.Config{Timezone: "Europe/Paris", PublicURL: "http://localhost:8080"},
		Store: st, Cache: cache.NewMemory(), Sealer: sealer, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dom.Wait)

	requireUser := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseInt(r.Header.Get("X-Test-User"), 10, 64)
			if id == 0 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			role := auth.RoleUser
			if id == 1 {
				role = auth.RoleAdmin // user 1 is the harness admin row
			}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: id, DisplayName: "u", Role: role})))
		})
	}
	// Like the production middleware: RequireUser first, then the role check.
	requireAdmin := func(next http.Handler) http.Handler {
		return requireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := auth.UserFrom(r.Context())
			if !u.IsAdmin() {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
	api := mcp.New(st, dom, log, logs)
	mux := http.NewServeMux()
	mcp.RegisterAPI(mux, mcp.Deps{Store: st, RequireUser: requireUser, RequireAdmin: requireAdmin, Srv: api})
	mux.Handle("/mcp/", api.PublicHandler())
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &harn{t: t, srv: ts, st: st, dom: dom, logs: logs}
}

// do sends a request to the harness; user 0 means unauthenticated.
func (h *harn) do(user int, method, path string, body any, out any) (int, string, http.Header) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rd)
	if user > 0 {
		req.Header.Set("X-Test-User", strconv.Itoa(user))
	}
	if strings.HasPrefix(path, "/mcp/") && method == "POST" {
		// Streamable HTTP: the server answers 415 without the MCP accept header.
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("%s %s: decode: %v (%s)", method, path, err, raw)
		}
	}
	return resp.StatusCode, string(raw), resp.Header
}

type mcpStatus struct {
	Enabled     bool       `json:"enabled"`
	HasToken    bool       `json:"hasToken"`
	TokenPrefix string     `json:"tokenPrefix"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
}

// genToken generates (or regenerates) the token of user user via the API and returns it.
func (h *harn) genToken(user int) string {
	h.t.Helper()
	code, body, _ := h.do(user, "POST", "/api/mcp/token", nil, nil)
	if code != http.StatusOK {
		h.t.Fatalf("generate: %d %s", code, body)
	}
	var out struct {
		Token  string `json:"token"`
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		h.t.Fatal(err)
	}
	if out.Prefix != out.Token[:len(mcp.TokenPrefix)+4] {
		h.t.Fatalf("prefix %q != %q", out.Prefix, out.Token[:len(mcp.TokenPrefix)+4])
	}
	return out.Token
}

// mcpPost posts a raw JSON-RPC message to the public route and returns the status code.
func (h *harn) mcpPost(token, msg string) int {
	h.t.Helper()
	code, _, _ := h.do(0, "POST", "/mcp/"+token, msg, nil)
	return code
}

// connect opens a real SDK client session on the public route.
func (h *harn) connect(t *testing.T, token string, lang string) *mcpsdk.ClientSession {
	t.Helper()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "t", Version: "0"}, nil)
	tr := &mcpsdk.StreamableClientTransport{Endpoint: h.srv.URL + "/mcp/" + token, DisableStandaloneSSE: true}
	if lang != "" {
		tr.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if lang != "" {
				req.Header.Set("Accept-Language", lang)
			}
			return http.DefaultTransport.RoundTrip(req)
		})}
	}
	cs, err := client.Connect(context.Background(), tr, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return cs
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mustMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// ---------- management API ----------

func TestMCPTokenLifecycle(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// No session: 401.
	if code, _, _ := h.do(0, "GET", "/api/mcp", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", code)
	}

	var st mcpStatus
	code, _, _ := h.do(2, "GET", "/api/mcp", nil, &st)
	if code != http.StatusOK || st.Enabled || st.HasToken {
		t.Fatalf("initial: %d %+v", code, st)
	}

	// Enable keeps no token: the switch alone does not mint credentials.
	if code, _, _ := h.do(2, "PUT", "/api/mcp", map[string]any{"enabled": true}, nil); code != http.StatusNoContent {
		t.Fatalf("enable: %d", code)
	}
	code, _, _ = h.do(2, "GET", "/api/mcp", nil, &st)
	if code != http.StatusOK || !st.Enabled || st.HasToken {
		t.Fatalf("enabled: %d %+v", code, st)
	}

	// A malformed body is rejected.
	if code, _, _ := h.do(2, "PUT", "/api/mcp", "not-json", nil); code != http.StatusBadRequest {
		t.Fatalf("malformed: %d", code)
	}

	// Generate: the plaintext is returned exactly once, prefixed and 256 bits long.
	token := h.genToken(2)
	if !strings.HasPrefix(token, mcp.TokenPrefix) || len(token) != len(mcp.TokenPrefix)+43 {
		t.Fatalf("token shape: %q", token)
	}
	code, _, _ = h.do(2, "GET", "/api/mcp", nil, &st)
	if !st.HasToken || st.TokenPrefix != token[:len(mcp.TokenPrefix)+4] {
		t.Fatalf("status after generate: %+v", st)
	}

	// The token is live: a valid initialize gets 200.
	if code := h.mcpPost(token, initMsg); code != http.StatusOK {
		t.Fatalf("live token: %d", code)
	}
	// And the last use is recorded (throttled, but set on first use).
	code, _, _ = h.do(2, "GET", "/api/mcp", nil, &st)
	if st.LastUsedAt == nil {
		t.Fatalf("last use not recorded: %+v", st)
	}

	// Regenerate: the old token is revoked, the new one is live.
	old := token
	token = h.genToken(2)
	if token == old {
		t.Fatal("regenerate returned the same token")
	}
	if code := h.mcpPost(old, initMsg); code != http.StatusNotFound {
		t.Fatalf("old token: %d", code)
	}
	if code := h.mcpPost(token, initMsg); code != http.StatusOK {
		t.Fatalf("new token: %d", code)
	}

	// Revoke: the token stops working, the switch is left as is.
	if code, _, _ := h.do(2, "DELETE", "/api/mcp/token", nil, nil); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	code, _, _ = h.do(2, "GET", "/api/mcp", nil, &st)
	if !st.Enabled || st.HasToken {
		t.Fatalf("after revoke: %+v", st)
	}
	if code := h.mcpPost(token, initMsg); code != http.StatusNotFound {
		t.Fatalf("revoked token: %d", code)
	}

	// Disabling and re-enabling keeps the master switch state consistent.
	if code, _, _ := h.do(2, "PUT", "/api/mcp", map[string]any{"enabled": false}, nil); code != http.StatusNoContent {
		t.Fatalf("disable: %d", code)
	}
	code, _, _ = h.do(2, "GET", "/api/mcp", nil, &st)
	if st.Enabled {
		t.Fatalf("disabled: %+v", st)
	}
	_ = ctx
}

// ---------- public route: shape of the failure answers ----------

func TestMCPFailureShapes(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2)

	// Unknown, revoked and empty tokens all get a uniform 404 (no token exists to be revealed).
	unknown, _, _ := h.do(0, "POST", "/mcp/sm4_unknownTokenValue1234567890123456789", initMsg, nil)
	if unknown != http.StatusNotFound {
		t.Fatalf("unknown: %d", unknown)
	}
	if code := h.mcpPost("", initMsg); code != http.StatusNotFound {
		t.Fatalf("empty: %d", code)
	}

	// A valid token on the wrong method gets a 405 with Allow: POST (stateless).
	if code, _, hdr := h.do(0, "GET", "/mcp/"+token, nil, nil); code != http.StatusMethodNotAllowed || hdr.Get("Allow") != "POST" {
		t.Fatalf("GET: %d %q", code, hdr.Get("Allow"))
	}

	// The disabled master switch stops a live token with the same 404.
	if code, _, _ := h.do(2, "PUT", "/api/mcp", map[string]any{"enabled": false}, nil); code != http.StatusNoContent {
		t.Fatalf("disable: %d", code)
	}
	disabled, _, _ := h.do(0, "POST", "/mcp/"+token, initMsg, nil)
	if disabled != http.StatusNotFound {
		t.Fatalf("disabled: %d", disabled)
	}
	// Uniformity: unknown and disabled are indistinguishable.
	code, bodyA, _ := h.do(0, "POST", "/mcp/sm4_unknownTokenValue1234567890123456789", initMsg, nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown2: %d", code)
	}
	if _, bodyB, _ := h.do(0, "POST", "/mcp/"+token, initMsg, nil); bodyA != bodyB {
		t.Fatalf("non-uniform 404: %q vs %q", bodyA, bodyB)
	}

	// Re-enabling reuses the same token.
	if code, _, _ := h.do(2, "PUT", "/api/mcp", map[string]any{"enabled": true}, nil); code != http.StatusNoContent {
		t.Fatalf("enable: %d", code)
	}
	if code := h.mcpPost(token, initMsg); code != http.StatusOK {
		t.Fatalf("re-enabled: %d", code)
	}
}

// Cross-site requests are rejected before the token is even resolved: the answer is a 403
// (Go 1.26 CrossOriginProtection), whatever the token.
func TestMCPCrossOriginRejected(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2)
	post := func(hdr map[string]string) int {
		h.t.Helper()
		req, _ := http.NewRequest("POST", h.srv.URL+"/mcp/"+token, strings.NewReader(initMsg))
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			h.t.Fatal(err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	if code := post(map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}); code != http.StatusForbidden {
		t.Fatalf("sec-fetch-site: %d", code)
	}
	if code := post(map[string]string{"Origin": "https://evil.example"}); code != http.StatusForbidden {
		t.Fatalf("origin: %d", code)
	}
	// Same origin and non-browser clients (no Origin at all) pass.
	if code := post(map[string]string{"Origin": h.srv.URL}); code != http.StatusOK {
		t.Fatalf("same-origin: %d", code)
	}
	if code := post(nil); code != http.StatusOK {
		t.Fatalf("no origin: %d", code)
	}
}

// ---------- protocol, with the official SDK client ----------

func TestMCPProtocol(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2)
	cs := h.connect(t, token, "")
	defer cs.Close()

	if got := cs.InitializeResult().ServerInfo.Name; got != "stride" {
		t.Fatalf("server name: %q", got)
	}

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"planning": true, "day": true, "activities": true, "activity": true, "health": true,
		"cycles": true, "profile": true, "plan_create": true, "plan_update": true, "plan_delete": true,
		"event_create": true, "event_update": true, "event_delete": true,
	}
	if len(tools.Tools) != len(want) {
		t.Fatalf("tool count: %d", len(tools.Tools))
	}
	for _, tl := range tools.Tools {
		if !want[tl.Name] {
			t.Fatalf("unexpected tool %q", tl.Name)
		}
		delete(want, tl.Name)
	}

	ctx := context.Background()
	today := time.Now().In(paris).Format("2006-01-02")
	call := func(name string, args any) map[string]any {
		h.t.Helper()
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			h.t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			h.t.Fatalf("%s: IsError: %v", name, res.Content)
		}
		return mustMap(t, res.StructuredContent)
	}

	// health works with no provider connected (empty zones, default mode).
	if m := call("health", struct{}{}); m["mode"] != "performance" {
		t.Fatalf("health: %v", m)
	}

	// plan_create returns the created session; planning then lists it.
	created := call("plan_create", map[string]any{"date": today, "sport": "run", "title": "MCP test"})
	id, _ := created["id"].(float64)
	if id <= 0 {
		t.Fatalf("plan_create: %v", created)
	}
	if m := call("planning", struct{}{}); m["totals"].(map[string]any)["sessionsPlanned"] != float64(1) {
		t.Fatalf("planning: %v", m)
	}

	// plan_update applies only the provided fields.
	updated := call("plan_update", map[string]any{"id": int64(id), "instructions": "zone 2"})
	if updated["instructions"] != "zone 2" || updated["title"] != "MCP test" {
		t.Fatalf("plan_update: %v", updated)
	}
	if m := call("day", map[string]any{"date": today}); m["date"] != today {
		t.Fatalf("day: %v", m)
	}
	if m := call("activities", struct{}{}); m["items"] == nil {
		t.Fatalf("activities: %v", m)
	}
	if m := call("cycles", struct{}{}); m["plans"] == nil && m["cycles"] == nil {
		t.Fatalf("cycles: %v", m)
	}
	if m := call("profile", struct{}{}); m["language"] == nil && m["timezone"] == nil {
		t.Fatalf("profile: %v", m)
	}

	// A tool error (unknown activity) is reported in-band, not as a protocol failure.
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "activity", Arguments: map[string]any{"id": int64(999999999)}})
	if err == nil && !res.IsError {
		t.Fatalf("activity: expected an error, got %+v", res.StructuredContent)
	}

	// events: create, partial update, delete.
	ev := call("event_create", map[string]any{"kind": "fatigue", "startDate": today})
	eid, _ := ev["id"].(float64)
	if ev["severity"] != float64(2) {
		t.Fatalf("event_create default severity: %v", ev)
	}
	evu := call("event_update", map[string]any{"id": int64(eid), "severity": 3})
	if evu["severity"] != float64(3) || evu["kind"] != "fatigue" {
		t.Fatalf("event_update: %v", evu)
	}
	if m := call("event_delete", map[string]any{"id": int64(eid)}); m["deleted"] != true {
		t.Fatalf("event_delete: %v", m)
	}

	// Missing required arguments are rejected (schema validation, or the domain bad-request
	// surfaced as an in-band tool error).
	if res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "plan_create", Arguments: map[string]any{}}); err == nil && !res.IsError {
		t.Fatalf("plan_create without date: %+v", res.StructuredContent)
	}

	if m := call("plan_delete", map[string]any{"id": int64(id)}); m["deleted"] != true {
		t.Fatalf("plan_delete: %v", m)
	}
}

// TestMCPDayDefaultDate covers issue #131: the optional date of the dated tools (day, planning)
// must default to "today" in the token owner's timezone — never an empty string propagated to the
// Garmin API and SQLite — and a malformed date must surface as an explicit, localized tool error.
func TestMCPDayDefaultDate(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2) // user 2 has no profile row: default timezone Europe/Paris

	// Pin the clock to an instant just after midnight in Paris (CET, UTC+1) but still the
	// previous day in UTC: 2026-11-22 23:30 UTC == 2026-11-23 00:30 Paris.
	fixed := time.Date(2026, 11, 22, 23, 30, 0, 0, time.UTC)
	parisToday := fixed.In(paris).Format("2006-01-02")
	utcToday := fixed.In(time.UTC).Format("2006-01-02")
	if parisToday != "2026-11-23" || utcToday != "2026-11-22" || parisToday == utcToday {
		t.Fatalf("test setup: wanted a Paris/UTC date boundary, got Paris=%s UTC=%s", parisToday, utcToday)
	}
	h.dom.Now = func() time.Time { return fixed }

	ctx := context.Background()
	// callRaw reports the result without failing on a tool error, so the in-band IsError
	// cases can be inspected.
	callRaw := func(cs *mcpsdk.ClientSession, name string, args any) (map[string]any, bool, string) {
		h.t.Helper()
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			h.t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			var b strings.Builder
			for _, c := range res.Content {
				if tc, ok := c.(*mcpsdk.TextContent); ok {
					b.WriteString(tc.Text)
				}
			}
			return nil, true, b.String()
		}
		return mustMap(h.t, res.StructuredContent), false, ""
	}
	ok := func(cs *mcpsdk.ClientSession, name string, args any) map[string]any {
		h.t.Helper()
		m, isErr, msg := callRaw(cs, name, args)
		if isErr {
			h.t.Fatalf("%s: unexpected IsError: %s", name, msg)
		}
		return m
	}

	csEN := h.connect(t, token, "")
	defer csEN.Close()
	csFR := h.connect(t, token, "fr")
	defer csFR.Close()

	// 1) No argument: the date defaults to today in the owner's (Paris) timezone, not UTC.
	m := ok(csEN, "day", struct{}{})
	if m["date"] != parisToday {
		t.Fatalf("day {}: date %v, want %s (Paris today; UTC would be %s)", m["date"], parisToday, utcToday)
	}
	// The crossLoad window is well-formed for the resolved day: J-1 00:00:00 .. J 23:59:59.
	cl, _ := m["crossLoad"].(map[string]any)
	if cl == nil {
		t.Fatalf("day {}: no crossLoad: %v", m)
	}
	if cl["windowStart"] != "2026-11-22T00:00:00" || cl["windowEnd"] != "2026-11-23T23:59:59" {
		t.Fatalf("day {}: crossLoad window %v", cl)
	}

	// 2) Empty and blank dates resolve to today the same way.
	for _, d := range []string{"", "   "} {
		if m := ok(csEN, "day", map[string]any{"date": d}); m["date"] != parisToday {
			t.Fatalf("day {date:%q}: %v, want %s", d, m["date"], parisToday)
		}
	}

	// 3) An explicit valid date passes through unchanged.
	if m := ok(csEN, "day", map[string]any{"date": "2026-01-15"}); m["date"] != "2026-01-15" {
		t.Fatalf("day explicit: %v", m["date"])
	}

	// 4) Malformed dates are rejected in-band (IsError, LLM-readable and localized) before
	//    reaching DayView, Garmin or SQLite.
	for _, d := range []string{"22-11-2026", "invalid"} {
		if _, isErr, msg := callRaw(csEN, "day", map[string]any{"date": d}); !isErr || !strings.Contains(msg, "YYYY-MM-DD") {
			t.Fatalf("day {date:%q}: isErr=%v msg=%q (want a YYYY-MM-DD hint)", d, isErr, msg)
		}
		if _, isErr, msg := callRaw(csFR, "day", map[string]any{"date": d}); !isErr || !strings.Contains(msg, "AAAA-MM-JJ") {
			t.Fatalf("day(fr) {date:%q}: isErr=%v msg=%q (want an AAAA-MM-JJ hint)", d, isErr, msg)
		}
	}

	// 5) planning is aligned on the same helper: a malformed date errors, an empty date works.
	if _, isErr, msg := callRaw(csEN, "planning", map[string]any{"date": "22-11-2026"}); !isErr || !strings.Contains(msg, "YYYY-MM-DD") {
		t.Fatalf("planning malformed: isErr=%v msg=%q", isErr, msg)
	}
	if m := ok(csEN, "planning", struct{}{}); m["totals"] == nil {
		t.Fatalf("planning {}: %v", m)
	}
}

// The language of the tools follows the effective language: profile ?? Accept-Language ?? en.
func TestMCPProtocolLanguage(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2)

	// No profile row: Accept-Language wins.
	csFR := h.connect(t, token, "fr")
	toolsFR, err := csFR.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	csEN := h.connect(t, token, "en")
	toolsEN, err := csEN.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	desc := func(tools *mcpsdk.ListToolsResult, name string) string {
		for _, tl := range tools.Tools {
			if tl.Name == name {
				return tl.Description
			}
		}
		return ""
	}
	if desc(toolsFR, "planning") == desc(toolsEN, "planning") {
		t.Fatalf("identical descriptions: %q", desc(toolsFR, "planning"))
	}
	if !strings.HasPrefix(desc(toolsFR, "planning"), "Le planning de la semaine") {
		t.Fatalf("fr: %q", desc(toolsFR, "planning"))
	}
	if !strings.HasPrefix(desc(toolsEN, "planning"), "The week plan") {
		t.Fatalf("en: %q", desc(toolsEN, "planning"))
	}
	csFR.Close()
	csEN.Close()

	// The profile language beats Accept-Language.
	if _, err := h.st.DB.Exec(`INSERT INTO athlete_profiles (user_id, timezone, updated_at, language) VALUES (2, 'Europe/Paris', 1, 'en')`); err != nil {
		t.Fatal(err)
	}
	cs := h.connect(t, token, "fr")
	defer cs.Close()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(desc(tools, "planning"), "The week plan") {
		t.Fatalf("profile language ignored: %q", desc(tools, "planning"))
	}
}

// A token only ever acts on its owner's data: bob cannot read or write alice's plan.
func TestMCPPerUserIsolation(t *testing.T) {
	h := newHarness(t)
	today := time.Now().In(paris).Format("2006-01-02")
	// alice (2) plans a session.
	alice := h.genToken(2)
	csA := h.connect(t, alice, "")
	defer csA.Close()
	ctx := context.Background()
	created, err := csA.CallTool(ctx, &mcpsdk.CallToolParams{Name: "plan_create", Arguments: map[string]any{"date": today, "sport": "run"}})
	if err != nil || created.IsError {
		t.Fatalf("alice plan_create: %v %v", err, created)
	}
	id, _ := mustMap(t, created.StructuredContent)["id"].(float64)

	// bob (3) sees an empty week and cannot touch alice's session.
	bob := h.genToken(3)
	csB := h.connect(t, bob, "")
	defer csB.Close()
	r, err := csB.CallTool(ctx, &mcpsdk.CallToolParams{Name: "planning", Arguments: struct{}{}})
	if err != nil {
		t.Fatal(err)
	}
	m := mustMap(t, r.StructuredContent)
	if f, _ := m["totals"].(map[string]any)["sessionsPlanned"].(float64); f != 0 {
		t.Fatalf("bob sees alice's plan: %v", m)
	}
	res, err := csB.CallTool(ctx, &mcpsdk.CallToolParams{Name: "plan_delete", Arguments: map[string]any{"id": int64(id)}})
	if err == nil && !res.IsError {
		t.Fatalf("bob deleted alice's session: %v", res.StructuredContent)
	}
	// alice's session is still there.
	rA, err := csA.CallTool(ctx, &mcpsdk.CallToolParams{Name: "plan_delete", Arguments: map[string]any{"id": int64(id)}})
	if err != nil || rA.IsError {
		t.Fatalf("alice delete: %v %v", err, rA)
	}
}

// ---------- rate limits ----------

// The per-token limit (100 requests / 10 min) stops a flooding client; the per-IP limit
// (200 / 10 min) bounds unauthenticated guessing.
func TestMCPRateLimits(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2)
	for i := 1; i <= 100; i++ {
		if code := h.mcpPost(token, initMsg); code != http.StatusOK {
			t.Fatalf("request %d: %d", i, code)
		}
	}
	if code := h.mcpPost(token, initMsg); code != http.StatusTooManyRequests {
		t.Fatalf("101st: %d", code)
	}
}

// ---------- admin usage & quota ----------

type mcpQuotaOut struct {
	TokenLimit  int `json:"tokenLimit"`
	TokenWindow int `json:"tokenWindow"`
	IPLimit     int `json:"ipLimit"`
	IPWindow    int `json:"ipWindow"`
}

type mcpUsageOut struct {
	Quota  mcpQuotaOut `json:"quota"`
	Tokens []struct {
		Prefix      string     `json:"prefix"`
		UserID      int64      `json:"userId"`
		DisplayName string     `json:"displayName"`
		Enabled     bool       `json:"enabled"`
		LastUsedAt  *time.Time `json:"lastUsedAt"`
		Used        int        `json:"used"`
		Denied24h   int        `json:"denied24h"`
		Requests24h int        `json:"requests24h"`
	} `json:"tokens"`
	Requests24h int       `json:"requests24h"`
	Denied24h   int       `json:"denied24h"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// The admin sees the rolling usage per token and tunes the public-route quota: the change
// applies live (limiters restart), is persisted (read back by a fresh service) and clamped.
func TestAdminMCPUsageAndQuota(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2)

	// Non-admins never see the admin routes.
	if code, _, _ := h.do(2, "GET", "/api/admin/mcp/usage", nil, nil); code != http.StatusForbidden {
		t.Fatalf("non-admin usage: %d", code)
	}
	if code, _, _ := h.do(2, "PUT", "/api/admin/mcp/quota", map[string]any{"tokenLimit": 5}, nil); code != http.StatusForbidden {
		t.Fatalf("non-admin quota: %d", code)
	}

	// No traffic yet: the token is listed with zero counters and the default quota.
	var u mcpUsageOut
	code, _, _ := h.do(1, "GET", "/api/admin/mcp/usage", nil, &u)
	if code != http.StatusOK {
		t.Fatalf("usage: %d", code)
	}
	if want := (mcpQuotaOut{100, 10, 200, 10}); u.Quota != want {
		t.Fatalf("quota: %+v, want %+v", u.Quota, want)
	}
	if len(u.Tokens) != 1 || u.Tokens[0].UserID != 2 || u.Tokens[0].Prefix != token[:len(mcp.TokenPrefix)+4] ||
		u.Tokens[0].DisplayName != "alice" || !u.Tokens[0].Enabled || u.Tokens[0].Requests24h != 0 {
		t.Fatalf("tokens: %+v", u.Tokens)
	}

	// Three public calls are served and counted (window + 24 h).
	for i := 0; i < 3; i++ {
		if c := h.mcpPost(token, initMsg); c != http.StatusOK {
			t.Fatalf("call %d: %d", i, c)
		}
	}
	u = mcpUsageOut{}
	h.do(1, "GET", "/api/admin/mcp/usage", nil, &u)
	if u.Tokens[0].Requests24h != 3 || u.Tokens[0].Used != 3 || u.Requests24h != 3 {
		t.Fatalf("counters: %+v", u)
	}
	if u.Tokens[0].LastUsedAt == nil {
		t.Fatal("lastUsedAt must be set after a call")
	}

	// The admin lowers the per-token quota: 5 allowed in the fresh window, the 6th is a 429
	// counted for the token, and the persisted row is returned clamped.
	var q mcpQuotaOut
	code, _, _ = h.do(1, "PUT", "/api/admin/mcp/quota", map[string]any{"tokenLimit": 5, "tokenWindow": 1, "ipLimit": 200, "ipWindow": 10}, &q)
	if code != http.StatusOK || q != (mcpQuotaOut{5, 1, 200, 10}) {
		t.Fatalf("put quota: %d %+v", code, q)
	}
	for i := 0; i < 5; i++ {
		if c := h.mcpPost(token, initMsg); c != http.StatusOK {
			t.Fatalf("call %d: %d", i, c)
		}
	}
	if c := h.mcpPost(token, initMsg); c != http.StatusTooManyRequests {
		t.Fatalf("6th: %d", c)
	}
	u = mcpUsageOut{}
	h.do(1, "GET", "/api/admin/mcp/usage", nil, &u)
	if u.Tokens[0].Requests24h != 8 || u.Tokens[0].Used != 5 || u.Tokens[0].Denied24h < 1 || u.Denied24h < 1 {
		t.Fatalf("after 429: %+v", u)
	}

	// Out-of-bounds input is clamped, not rejected.
	q = mcpQuotaOut{}
	code, _, _ = h.do(1, "PUT", "/api/admin/mcp/quota", map[string]any{"tokenLimit": 0, "tokenWindow": 0, "ipLimit": 999999, "ipWindow": 999999}, &q)
	if code != http.StatusOK || q != (mcpQuotaOut{1, 1, 100000, 1440}) {
		t.Fatalf("clamped: %d %+v", code, q)
	}

	// The change is persisted: a fresh service reads the stored row.
	stored, err := mcp.NewService(h.st, nil).GetQuota(context.Background())
	if err != nil || stored.TokenLimit != 1 || stored.TokenWindow != 1 || stored.IPLimit != 100000 || stored.IPWindow != 1440 {
		t.Fatalf("persisted quota: %+v (%v)", stored, err)
	}
}
