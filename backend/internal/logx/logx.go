// Package logx fournit un handler slog lisible (emoji + couleurs si TTY) ou JSON.
//
// Format pretty :
//
//	2026-11-03 18:42:01 INFO  🌐 GET /api/activities 200 · 42ms
//
// Les attributs éventuels sont ajoutés en fin de ligne sous la forme key=value.
// Toute clé d'attribut sensible (authorization, token, apiKey, password…) est masquée.
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Options de construction du logger.
type Options struct {
	Level  string // debug|info|warn|error
	Format string // pretty|json
	Out    io.Writer
	Color  *bool // nil = auto (TTY)
}

// New construit un *slog.Logger selon les options.
func New(o Options) *slog.Logger {
	out := o.Out
	if out == nil {
		out = os.Stdout
	}
	lvl := ParseLevel(o.Level)
	if strings.EqualFold(o.Format, "json") {
		return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
			Level:       lvl,
			ReplaceAttr: redactAttr,
		}))
	}
	color := false
	if o.Color != nil {
		color = *o.Color
	} else {
		color = isTTY(out)
	}
	return slog.New(&prettyHandler{mu: &sync.Mutex{}, out: out, level: lvl, color: color})
}

// ParseLevel convertit une chaîne en niveau slog (défaut info).
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

var sensitiveKeys = []string{"authorization", "token", "jwt", "apikey", "api_key", "password", "secret", "bearer"}

// IsSensitiveKey indique si une clé d'attribut doit être masquée.
func IsSensitiveKey(k string) bool {
	lk := strings.ToLower(k)
	for _, s := range sensitiveKeys {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}

func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, "[masqué]")
	}
	return a
}

const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
	ansiGray   = "\x1b[90m"
)

type prettyHandler struct {
	mu     *sync.Mutex
	out    io.Writer
	level  slog.Level
	color  bool
	attrs  []slog.Attr
	groups []string
}

func (h *prettyHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *prettyHandler) WithAttrs(as []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), as...)
	return &nh
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	nh.groups = append(append([]string{}, h.groups...), name)
	return &nh
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	h.paint(&b, ansiGray, ts.Format("2006-01-02 15:04:05"))
	b.WriteByte(' ')
	lvl, col := levelLabel(r.Level)
	h.paint(&b, col, fmt.Sprintf("%-5s", lvl))
	b.WriteString(" ")
	b.WriteString(r.Message)

	prefix := ""
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}
	var extra strings.Builder
	write := func(a slog.Attr) bool {
		appendAttr(&extra, prefix, a)
		return true
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(write)
	if extra.Len() > 0 {
		b.WriteByte(' ')
		h.paint(&b, ansiDim, strings.TrimSpace(extra.String()))
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, b.String())
	return err
}

func (h *prettyHandler) paint(b *strings.Builder, color, s string) {
	if h.color && color != "" {
		b.WriteString(color)
		b.WriteString(s)
		b.WriteString(ansiReset)
		return
	}
	b.WriteString(s)
}

func appendAttr(b *strings.Builder, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p = prefix + a.Key + "."
		}
		for _, ga := range a.Value.Group() {
			appendAttr(b, p, ga)
		}
		return
	}
	key := prefix + a.Key
	val := a.Value.String()
	if IsSensitiveKey(a.Key) {
		val = "[masqué]"
	}
	if strings.ContainsAny(val, " \t\"") {
		val = fmt.Sprintf("%q", val)
	}
	b.WriteByte(' ')
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(val)
}

func levelLabel(l slog.Level) (string, string) {
	switch {
	case l >= slog.LevelError:
		return "ERROR", ansiRed
	case l >= slog.LevelWarn:
		return "WARN", ansiYellow
	case l >= slog.LevelInfo:
		return "INFO", ansiGreen
	default:
		return "DEBUG", ansiCyan
	}
}

// Ms formate une durée en millisecondes lisibles ("42ms", "1.3s").
func Ms(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
