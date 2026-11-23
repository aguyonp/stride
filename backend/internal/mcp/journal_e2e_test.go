// journal_e2e_test.go exercises the call journal end to end (issue #135): every public call
// lands in the ring, the admin endpoint exposes it with the token redacted, the structured
// log keeps one mcp.request line per call, and the 24 h error counter feeds the usage view.
package mcp_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"stride/internal/applog"
	"stride/internal/mcp"
)

type journalEntry struct {
	At             time.Time `json:"at"`
	TokenPrefix    string    `json:"tokenPrefix"`
	UserID         int64     `json:"userId"`
	UserName       string    `json:"userName"`
	Method         string    `json:"method"`
	Tool           string    `json:"tool"`
	ArgsSummary    string    `json:"argsSummary"`
	Status         int       `json:"status"`
	RPCErrorCode   int       `json:"rpcErrorCode"`
	RPCError       string    `json:"rpcError"`
	DurationMs     int64     `json:"durationMs"`
	InFlight       int       `json:"inFlight"`
	ClientCanceled bool      `json:"clientCanceled"`
}

func TestJournalRecordsPublicCalls(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2) // alice
	prefix := token[:len(mcp.TokenPrefix)+4]

	code, raw, _ := h.do(0, "POST", "/mcp/"+token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"profile","arguments":{}}}`, nil)
	if code != http.StatusOK {
		t.Fatalf("profile call: %d %s", code, raw)
	}
	code, raw, _ = h.do(0, "POST", "/mcp/sm4_unknown0000000000", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"profile","arguments":{}}}`, nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown token: %d %s", code, raw)
	}
	h.do(0, "GET", "/mcp/"+token, nil, nil) // wrong method: not journaled

	var v struct {
		InFlight int            `json:"inFlight"`
		Entries  []journalEntry `json:"entries"`
	}
	code, raw, _ = h.do(1, "GET", "/api/admin/mcp/journal", nil, &v)
	if code != http.StatusOK {
		t.Fatalf("journal: %d %s", code, raw)
	}
	if len(v.Entries) != 2 {
		t.Fatalf("entries: %+v", v.Entries)
	}
	refused, called := v.Entries[0], v.Entries[1] // most recent first: the 404 came last
	if called.Status != 200 || called.Method != "tools/call" || called.Tool != "profile" ||
		called.TokenPrefix != prefix || called.UserID != 2 || called.InFlight != 1 {
		t.Fatalf("called: %+v", called)
	}
	if refused.Status != 404 || refused.Method != "" || refused.UserID != 0 {
		t.Fatalf("refused: %+v", refused)
	}
	if v.InFlight != 0 {
		t.Fatalf("inFlight now: %d", v.InFlight)
	}
	if strings.Contains(raw, token) {
		t.Fatalf("full token in the journal: %s", raw)
	}
}

func TestJournalLogsOneLinePerCall(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(3) // bob
	prefix := token[:len(mcp.TokenPrefix)+4]

	code, raw, _ := h.do(0, "POST", "/mcp/"+token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"profile","arguments":{}}}`, nil)
	if code != http.StatusOK {
		t.Fatalf("profile call: %d %s", code, raw)
	}
	h.logs.Close() // flush the pending entries
	page, err := h.logs.Query(context.Background(), applog.Filter{Category: applog.CategoryAction})
	if err != nil {
		t.Fatal(err)
	}
	var req *applog.Entry
	for i := range page.Items {
		if page.Items[i].Action == "mcp.request" {
			req = &page.Items[i]
		}
	}
	if req == nil {
		t.Fatalf("no mcp.request entry: %+v", page.Items)
	}
	if req.Level != applog.LevelInfo || req.Status != 200 || req.UserID == nil || *req.UserID != 3 {
		t.Fatalf("entry: %+v", req)
	}
	if !strings.HasPrefix(req.Message, "mcp:"+prefix) || !strings.Contains(req.Message, "tools/call profile") ||
		req.Target != "/mcp/"+prefix+"…" {
		t.Fatalf("redaction: %+v", req)
	}
	if strings.Contains(req.Message, token) || strings.Contains(req.Target, token) {
		t.Fatal("full token in the log")
	}
}

func TestUsageExposesErrors24h(t *testing.T) {
	h := newHarness(t)
	// A failed call (unknown token) feeds the 24 h counter.
	h.do(0, "POST", "/mcp/sm4_unknown0000000000", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, nil)
	var u struct {
		Requests24h int `json:"requests24h"`
		Denied24h   int `json:"denied24h"`
		Errors24h   int `json:"errors24h"`
	}
	code, raw, _ := h.do(1, "GET", "/api/admin/mcp/usage", nil, &u)
	if code != http.StatusOK {
		t.Fatalf("usage: %d %s", code, raw)
	}
	if u.Errors24h < 1 {
		t.Fatalf("errors24h: %+v", u)
	}
}
