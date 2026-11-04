package logx

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrettyFormatAndRedaction(t *testing.T) {
	var buf bytes.Buffer
	no := false
	l := New(Options{Level: "debug", Format: "pretty", Out: &buf, Color: &no})
	l.Info("🌐 GET /api/activities 200 · 42ms")
	l.Warn("essai", "token", "ne-doit-pas-apparaitre", "Authorization", "Bearer xyz", "clé", "cache/sleep")
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("lignes : %q", out)
	}
	if !strings.HasSuffix(lines[0], " INFO  🌐 GET /api/activities 200 · 42ms") || len(lines[0]) < 20 || lines[0][4] != '-' {
		t.Errorf("format : %q", lines[0])
	}
	if strings.Contains(out, "ne-doit-pas-apparaitre") || strings.Contains(out, "xyz") {
		t.Errorf("secret non masqué : %q", out)
	}
	if !strings.Contains(out, "clé=cache/sleep") || strings.Contains(out, "\x1b[") {
		t.Errorf("attributs / couleurs : %q", out)
	}
	buf.Reset()
	j := New(Options{Level: "info", Format: "json", Out: &buf})
	j.Debug("debug-invisible")
	j.Info("ok", "apiKey", "zzz")
	if strings.Contains(buf.String(), "debug-invisible") || strings.Contains(buf.String(), "zzz") || !strings.Contains(buf.String(), `"msg":"ok"`) {
		t.Errorf("json : %q", buf.String())
	}
}
