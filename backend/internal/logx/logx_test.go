package logx

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrettyFormatAndRedaction(t *testing.T) {
	var buf bytes.Buffer
	no := false
	l := New(Options{Level: "debug", Format: "pretty", Out: &buf, Color: &no})
	l.Info("🌐 GET /api/activities 200 · 42ms")
	l.Warn("essai", "token", "ne-doit-pas-apparaitre", "Authorization", "Bearer xyz", "inviteCode", "INV123", "code", "oauth-code", "clé", "cache/sleep", "statusCode", 404)
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines: %q", out)
	}
	if !strings.HasSuffix(lines[0], " INFO  🌐 GET /api/activities 200 · 42ms") || len(lines[0]) < 20 || lines[0][4] != '-' {
		t.Errorf("format: %q", lines[0])
	}
	for _, secret := range []string{"ne-doit-pas-apparaitre", "xyz", "INV123", "oauth-code"} {
		if strings.Contains(out, secret) {
			t.Errorf("secret %q not masked: %q", secret, out)
		}
	}
	if !strings.Contains(out, "clé=cache/sleep") || !strings.Contains(out, "statusCode=404") || strings.Contains(out, "\x1b[") {
		t.Errorf("attributes / colours: %q", out)
	}
}

func TestJSONOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	j := New(Options{Level: "info", Format: "json", Out: &buf})
	j.Debug("debug-invisible")
	j.Info("ok", "apiKey", "zzz", "cookie", "stride_session=abc")
	j.Warn("two", "n", 2)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("json lines: %q", buf.String())
	}
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid json %q: %v", line, err)
		}
	}
	if s := buf.String(); strings.Contains(s, "zzz") || strings.Contains(s, "abc") || !strings.Contains(s, `"msg":"ok"`) {
		t.Errorf("json: %q", s)
	}
}
