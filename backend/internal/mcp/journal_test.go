// journal_test.go covers the call-journal building blocks (issue #135): the JSON-RPC body
// summarization, the argument sanitization, the answer error parsing (JSON and SSE) and the
// ring / 24 h error counter.
package mcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseReqBodyToolsCall(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"plan_create","arguments":{"date":"2026-11-24","sport":"run"}}}`)
	info := parseReqBody(body)
	if info.method != "tools/call" || info.tool != "plan_create" || string(info.id) != "7" {
		t.Fatalf("info: %+v", info)
	}
	if !strings.Contains(info.args, `"date":"2026-11-24"`) {
		t.Fatalf("args: %s", info.args)
	}
}

func TestParseReqBodyOtherMethodsAndGibberish(t *testing.T) {
	info := parseReqBody([]byte(`{"jsonrpc":"2.0","id":"a","method":"initialize","params":{}}`))
	if info.method != "initialize" || info.tool != "" || string(info.id) != `"a"` {
		t.Fatalf("initialize: %+v", info)
	}
	if info := parseReqBody([]byte(`not json`)); info.method != "" {
		t.Fatalf("gibberish: %+v", info)
	}
	if info := parseReqBody(nil); info.method != "" {
		t.Fatalf("empty: %+v", info)
	}
}

func TestParseReqBodyBatch(t *testing.T) {
	body := []byte(`[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"profile","arguments":{}}}]`)
	info := parseReqBody(body)
	if info.method != "batch(2)" || info.tool != "profile" || string(info.id) != "2" {
		t.Fatalf("info: %+v", info)
	}
}

func TestSanitizeArgsTruncatesLongValues(t *testing.T) {
	long := strings.Repeat("x", 500)
	summary := sanitizeArgs(mustRaw(t, map[string]any{"a": long, "n": 3}))
	if !strings.Contains(summary, "…") || strings.Contains(summary, long) {
		t.Fatalf("long value not truncated: %s", summary)
	}
	if len(summary) > argsCap+3 {
		t.Fatalf("summary too long: %d bytes", len(summary))
	}
	if got := sanitizeArgs(nil); got != "" {
		t.Fatalf("nil args: %q", got)
	}
}

func TestParseRespBody(t *testing.T) {
	if code, msg := parseRespBody([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad param"}}`)); code != -32602 || msg != "bad param" {
		t.Fatalf("json error: %d %q", code, msg)
	}
	if code, _ := parseRespBody([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)); code != 0 {
		t.Fatalf("json result: %d", code)
	}

	// SSE: keep-alive comment first, then an error envelope.
	sseErr := ": \n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-32000,\"message\":\"boom\"}}\n\n"
	if code, msg := parseRespBody([]byte(sseErr)); code != -32000 || msg != "boom" {
		t.Fatalf("sse error: %d %q", code, msg)
	}

	// SSE result embedding `"error":null`: must not be mistaken for a JSON-RPC error.
	sseOK := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"garmin\":{\"error\":null}}}\n\n"
	if code, _ := parseRespBody([]byte(sseOK)); code != 0 {
		t.Fatalf("sse result: %d", code)
	}

	// JSON batch answer: the error may be in the middle.
	batch := `[{"jsonrpc":"2.0","id":1,"result":{}},{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"nope"}}]`
	if code, _ := parseRespBody([]byte(batch)); code != -32601 {
		t.Fatalf("batch: %d", code)
	}
}

func TestJournalRingOrderAndCap(t *testing.T) {
	j := &journal{}
	for i := 0; i < journalSize+50; i++ {
		j.Add(Call{At: time.Now(), Method: "tools/call", Status: 200, UserID: int64(i)}, false)
	}
	l := j.List()
	if len(l) != journalSize {
		t.Fatalf("kept: %d (want %d)", len(l), journalSize)
	}
	// Most recent first: the newest is the last one written.
	if l[0].UserID != journalSize+49 || l[journalSize-1].UserID != 50 {
		t.Fatalf("order: first=%+v last=%+v", l[0], l[journalSize-1])
	}
}

func TestJournalErrors24h(t *testing.T) {
	j := &journal{}
	now := time.Now()
	j.Add(Call{At: now}, true)
	j.Add(Call{At: now}, true)
	j.Add(Call{At: now}, false)
	if got := j.errors24h(); got != 2 {
		t.Fatalf("errors24h: %d (want 2)", got)
	}
}
