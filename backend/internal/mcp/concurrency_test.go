package mcp_test

// concurrency_test.go is the regression test of issue #135: several tools/call requests in
// flight at once (stateless Streamable HTTP) must all complete with a result — no hung
// connection, no serialization of the calls. It cannot trigger the original
// DEADLINE_EXCEEDED locally (a call finishes in milliseconds, far under any client timeout):
// the keep-alive fix (StreamKeepAlive, go-sdk #1155) is what prevents the first-byte timeout
// for slow calls, and the journal (journal_test.go) is what traces them in production.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// sseHasResult decodes the JSON-RPC envelopes in the SSE answer and reports
// whether call id got a result without an rpc error.
func sseHasResult(raw string, id int) bool {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var env struct {
			ID     json.RawMessage `json:"id"`
			Result any             `json:"result"`
			Error  any             `json:"error"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &env); err != nil {
			continue
		}
		var got int
		_ = json.Unmarshal(env.ID, &got)
		if got != id {
			continue
		}
		return env.Error == nil
	}
	return false
}

func TestConcurrentPlanCreatesComplete(t *testing.T) {
	h := newHarness(t)
	token := h.genToken(2) // alice

	const n = 8
	type result struct {
		i      int
		dur    time.Duration
		status int
		ok     bool
		raw    string
	}
	res := make([]result, n)
	var wg sync.WaitGroup
	startAll := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"plan_create","arguments":{"date":"2026-11-24","sport":"run","title":"burst %d"}}}`, i, i)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, h.srv.URL+"/mcp/"+token, strings.NewReader(msg))
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Content-Type", "application/json")
			start := time.Now()
			resp, err := http.DefaultClient.Do(req)
			dur := time.Since(start)
			if err != nil {
				res[i] = result{i: i, dur: dur, status: -1, raw: "ERR:" + err.Error()}
				return
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			raw := string(b)
			ok := sseHasResult(raw, i)
			res[i] = result{i: i, dur: dur, status: resp.StatusCode, ok: ok, raw: raw}
		}(i)
	}
	wg.Wait()
	total := time.Since(startAll)
	for _, r := range res {
		t.Logf("call %-2d status=%-3d ok=%-5v dur=%-9s %s", r.i, r.status, r.ok, r.dur, firstLine(r.raw, 200))
	}
	t.Logf("total wall: %s", total)
	for _, r := range res {
		if !r.ok {
			t.Errorf("call %d failed: %s", r.i, r.raw)
		}
	}
}

func firstLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 && i < max {
		return s[:i]
	}
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
