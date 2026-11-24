// journal.go records every call of the public MCP route in an in-memory journal — the
// "Journal des appels MCP" of the admin panel (issue #135): a ring of the last calls, with
// per-call fields (JSON-RPC method and tool, sanitized arguments, HTTP status and RPC error,
// duration, concurrency, client cancellation), and a rolling 24 h "errors / timeouts" counter.
// It is in-memory by design: the MCP path must not take extra SQLite load, and the journal
// resets on restart.
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/httpx"
)

const (
	// journalSize is the number of calls kept in memory (spec: 200–500).
	journalSize = 300
	// slowCallMs: calls slower than this are logged at warn level (spec: 2 s).
	slowCallMs = 2000
	// respCap bounds the copy of the answer kept for after-the-fact analysis.
	respCap = 256 << 10
	// argsCap bounds the sanitized arguments summary kept in the journal.
	argsCap = 1024
	// errCap bounds the RPC error message kept in the journal.
	errCap = 500
)

// Call is one recorded call of the public route POST /mcp/{token}.
type Call struct {
	At             time.Time `json:"at"`
	TokenPrefix    string    `json:"tokenPrefix"`
	UserID         int64     `json:"userId"` // 0 when the token is unknown (404)
	UserName       string    `json:"userName"`
	Method         string    `json:"method"` // JSON-RPC method; "" when the body was not read
	Tool           string    `json:"tool"`   // tools/call: the tool name
	ArgsSummary    string    `json:"argsSummary"`
	Status         int       `json:"status"`
	RPCErrorCode   int       `json:"rpcErrorCode"` // 0 when no RPC error
	RPCError       string    `json:"rpcError"`
	DurationMs     int64     `json:"durationMs"`
	InFlight       int       `json:"inFlight"` // concurrent calls when the call started
	ClientCanceled bool      `json:"clientCanceled"`
}

// journal is the in-memory call journal: a ring of the last journalSize calls plus a rolling
// 24 h failed-call counter (the "Erreurs / Timeouts · 24 h" KPI).
type journal struct {
	mu     sync.Mutex
	ring   [journalSize]Call
	next   int // index of the next write (of the oldest call when the ring is full)
	filled bool
	err24h usageRing
}

// Add records one call; failed feeds the 24 h error counter.
func (j *journal) Add(c Call, failed bool) {
	if c.At.IsZero() {
		c.At = time.Now()
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ring[j.next] = c
	j.next = (j.next + 1) % journalSize
	if j.next == 0 {
		j.filled = true
	}
	if failed {
		j.err24h.add(c.At)
	}
}

// List returns the recorded calls, most recent first.
func (j *journal) List() []Call {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := j.next
	if j.filled {
		n = journalSize
	}
	out := make([]Call, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, j.ring[(j.next-1-i+journalSize)%journalSize])
	}
	return out
}

// errors24h counts the failed calls of the last 24 h.
func (j *journal) errors24h() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.err24h.total()
}

// JournalView is the admin view of the journal: every kept call (most recent first) and the
// current concurrency.
type JournalView struct {
	InFlight int    `json:"inFlight"`
	Entries  []Call `json:"entries"`
}

// Journal builds the admin view.
func (s *Server) Journal() JournalView {
	return JournalView{InFlight: int(s.inFlight.Load()), Entries: s.jrn.List()}
}

// ---------- request / answer parsing ----------

// reqInfo is what the instrumentation learns from the JSON-RPC body before handing it to the
// SDK handler: the method, the call id (to match the answer envelope), the tool and a
// sanitized summary of its arguments.
type reqInfo struct {
	method string
	tool   string
	id     json.RawMessage
	args   string
}

// parseReqBody summarizes a JSON-RPC body (single message or batch) without altering it.
func parseReqBody(body []byte) reqInfo {
	var info reqInfo
	b := bytes.TrimSpace(body)
	if len(b) == 0 {
		return info
	}
	if b[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(b, &arr); err != nil {
			return info
		}
		info.method = fmt.Sprintf("batch(%d)", len(arr))
		for _, m := range arr {
			if d, ok := decodeReq(m); ok && d.method == "tools/call" {
				info.tool = d.tool
				info.args = d.args
				if len(info.id) == 0 {
					info.id = d.id
				}
				break
			}
		}
		return info
	}
	if d, ok := decodeReq(b); ok {
		info.method = d.method
		info.tool = d.tool
		info.id = d.id
		info.args = d.args
	}
	return info
}

type decodedReq struct {
	method string
	id     json.RawMessage
	tool   string
	args   string
}

func decodeReq(raw json.RawMessage) (decodedReq, bool) {
	var env struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Method == "" {
		return decodedReq{}, false
	}
	d := decodedReq{method: env.Method, id: env.ID}
	if env.Method == "tools/call" {
		var p struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(env.Params, &p); err == nil {
			d.tool = p.Name
			d.args = sanitizeArgs(p.Args)
		}
	}
	return d, true
}

// sanitizeArgs turns a tools/call arguments object into a compact summary: every string is
// capped (a long value must not flood the journal) and the whole summary is capped too.
func sanitizeArgs(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	enc, err := json.Marshal(truncateValues(v))
	if err != nil {
		return ""
	}
	s := string(enc)
	if len(s) > argsCap {
		s = s[:argsCap] + "…"
	}
	return s
}

func truncateValues(v any) any {
	switch t := v.(type) {
	case string:
		if len(t) > 120 {
			return t[:120] + "…"
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = truncateValues(e)
		}
		return t
	case map[string]any:
		for k, e := range t {
			t[k] = truncateValues(e)
		}
		return t
	}
	return v
}

// parseRespBody scans the (buffered) answer — application/json or SSE `data:` lines — and
// returns the first JSON-RPC error envelope, if any.
func parseRespBody(body []byte) (code int, msg string) {
	b := bytes.TrimSpace(body)
	if len(b) == 0 {
		return 0, ""
	}
	var envs []json.RawMessage
	switch {
	case b[0] == '[':
		_ = json.Unmarshal(b, &envs)
	case bytes.HasPrefix(b, []byte("event:")) || bytes.HasPrefix(b, []byte("data:")) || b[0] == ':':
		for _, line := range strings.Split(string(b), "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "data:"); ok {
				envs = append(envs, json.RawMessage(strings.TrimSpace(rest)))
			}
		}
	default:
		envs = []json.RawMessage{b}
	}
	for _, e := range envs {
		var env struct {
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(e, &env); err == nil && env.Error != nil {
			msg = env.Error.Message
			if len(msg) > errCap {
				msg = msg[:errCap] + "…"
			}
			return env.Error.Code, msg
		}
	}
	return 0, ""
}

// ---------- instrumentation ----------

// recWriter captures the answer status and a bounded copy of the body so the middleware can
// read the RPC error without re-serializing the SDK's writes.
type recWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
	buf    bytes.Buffer
	cap    int
	over   bool
}

// Unwrap lets http.ResponseController (Flush on the panic path) reach the real writer.
func (r *recWriter) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *recWriter) WriteHeader(code int) {
	if !r.wrote {
		r.wrote = true
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recWriter) Write(p []byte) (int, error) {
	if !r.wrote {
		r.wrote = true
		if r.status == 0 {
			r.status = http.StatusOK
		}
	}
	if !r.over {
		if r.buf.Len()+len(p) > r.cap {
			r.buf.Truncate(r.cap)
			r.over = true
		} else {
			r.buf.Write(p)
		}
	}
	return r.ResponseWriter.Write(p)
}

// serveCall instruments one MCP call (journal entry + structured log, issue #135) and serves
// it through the SDK handler. A panic becomes a clean JSON-RPC error — never a hung
// connection — and is recorded like any other failure.
func (s *Server) serveCall(w http.ResponseWriter, r *http.Request, token string, user auth.User, start time.Time) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		s.noteEarly(token, &user, http.StatusBadRequest, start)
		return
	}
	if len(body) > maxBodyBytes {
		http.Error(w, fmt.Sprintf("request body exceeds %d bytes", maxBodyBytes), http.StatusRequestEntityTooLarge)
		s.noteEarly(token, &user, http.StatusRequestEntityTooLarge, start)
		return
	}
	info := parseReqBody(body)

	s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	inFlight := int(s.inFlight.Load())

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	rec := &recWriter{ResponseWriter: w, cap: respCap}
	panicMsg := ""
	func() {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				panicMsg = fmt.Sprintf("%v", p)
				if len(panicMsg) > errCap {
					panicMsg = panicMsg[:errCap] + "…"
				}
				// A clean JSON-RPC error for the in-flight call instead of a hung connection.
				if len(info.id) > 0 {
					if rec.wrote {
						// The SSE stream committed: answer the call id with an error event.
						writeSSEError(rec, info.id, -32603, "internal error")
					} else {
						writeJSONError(w, http.StatusInternalServerError, info.id, -32603, "internal error")
					}
				}
			}
		}()
		s.srv.ServeHTTP(rec, req)
	}()

	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	rpcCode, rpcMsg := parseRespBody(rec.buf.Bytes())
	call := Call{
		At: time.Now(), TokenPrefix: tokenPrefix(token), UserID: user.ID, UserName: user.DisplayName,
		Method: info.method, Tool: info.tool, ArgsSummary: info.args,
		Status: status, RPCErrorCode: rpcCode, RPCError: rpcMsg,
		DurationMs: time.Since(start).Milliseconds(), InFlight: inFlight,
		ClientCanceled: r.Context().Err() != nil,
	}
	s.jrn.Add(call, panicMsg != "" || status >= http.StatusBadRequest || rpcCode != 0 || call.ClientCanceled)
	s.logMCP(call, panicMsg)
}

// noteEarly records a call refused (429, 404, 413…) before the SDK handler, so no call of the
// public route escapes the journal.
func (s *Server) noteEarly(token string, user *auth.User, status int, start time.Time) {
	c := Call{
		At: time.Now(), TokenPrefix: tokenPrefix(token), Status: status,
		DurationMs: time.Since(start).Milliseconds(), InFlight: int(s.inFlight.Load()),
	}
	if user != nil {
		c.UserID = user.ID
		c.UserName = user.DisplayName
	}
	s.jrn.Add(c, status >= http.StatusBadRequest)
	s.logMCP(c, "")
}

// logMCP writes the structured log line of one call (issue #135): info for a normal call,
// warn for a slow one (> 2 s), a 429 or a client cancellation, error for everything else
// (HTTP ≥ 400, RPC error, panic). The line lands in Administration > Journaux like any other.
func (s *Server) logMCP(c Call, panicMsg string) {
	if s.logs == nil {
		return
	}
	level := applog.LevelInfo
	switch {
	case panicMsg != "" || c.Status >= http.StatusInternalServerError || c.RPCErrorCode != 0:
		level = applog.LevelError
	case c.Status == http.StatusTooManyRequests || c.ClientCanceled || c.DurationMs > slowCallMs:
		level = applog.LevelWarn
	case c.Status >= http.StatusBadRequest:
		level = applog.LevelError
	}
	msg := "mcp:" + c.TokenPrefix
	if c.Method != "" {
		if c.Tool != "" {
			msg += " · " + c.Method + " " + c.Tool
		} else {
			msg += " · " + c.Method
		}
	}
	switch {
	case panicMsg != "":
		msg += " · panique : " + panicMsg
	case c.Status == http.StatusTooManyRequests:
		msg += " · 429 (quota)"
	case c.Status == http.StatusNotFound:
		msg += " · 404 (jeton inconnu)"
	case c.RPCErrorCode != 0:
		msg += fmt.Sprintf(" · rpc %d %s", c.RPCErrorCode, c.RPCError)
	case c.ClientCanceled:
		msg += " · annulé par le client"
	}
	e := applog.Entry{Category: applog.CategoryAction, Level: level, Method: "POST", Target: "/mcp/" + c.TokenPrefix + "…",
		Status: c.Status, DurationMs: c.DurationMs, Action: "mcp.request", Message: msg}
	if c.UserID != 0 {
		e.UserID = &c.UserID
		e.UserName = &c.UserName
	}
	s.logs.Add(e)
}

// ---------- panic answers ----------

func rpcErrorEnvelope(id json.RawMessage, code int, msg string) map[string]any {
	idv := json.RawMessage("null")
	if len(id) > 0 {
		idv = id
	}
	return map[string]any{"jsonrpc": "2.0", "id": idv, "error": map[string]any{"code": code, "message": msg}}
}

// writeJSONError answers with a plain JSON-RPC error (headers not committed yet).
func writeJSONError(w http.ResponseWriter, status int, id json.RawMessage, code int, msg string) {
	b, _ := json.Marshal(rpcErrorEnvelope(id, code, msg))
	httpx.JSON(w, status, json.RawMessage(b))
}

// writeSSEError writes the error envelope as an SSE event on a stream that already committed,
// so the client's in-flight call completes with an error instead of hanging.
func writeSSEError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	b, _ := json.Marshal(rpcErrorEnvelope(id, code, msg))
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
	_ = http.NewResponseController(w).Flush()
}
