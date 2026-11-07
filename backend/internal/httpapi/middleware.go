package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"stride/internal/httpx"
	"stride/internal/logx"
)

const (
	csrfHeader   = "X-Stride-CSRF"
	maxBodyBytes = 16 << 20
	// maxImportBodyBytes fits a 25 MB rescue import plus the multipart envelope.
	maxImportBodyBytes = 27 << 20
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

var sensitiveParams = []string{"code", "state", "token", "invite", "challenge", "secret", "key"}

// redactedQuery keeps parameter names but masks values that may carry secrets (OAuth code, invite, hub.*).
func redactedQuery(raw string) string {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "…"
	}
	parts := make([]string, 0, len(q))
	for k, vs := range q {
		v := strings.Join(vs, ",")
		lk := strings.ToLower(k)
		for _, s := range sensitiveParams {
			if strings.Contains(lk, s) {
				v = "•••"
				break
			}
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "&")
}

// shareTokenPrefix is the part of a share token kept in logs (the prefix shown to its owner).
const shareTokenPrefix = 8

// redactPath keeps only the short prefix of share tokens (/share/{token}, /api/public/share/{token}/…);
// the portal sub-path after the token is kept.
func redactPath(p string) string {
	for _, prefix := range []string{"/api/public/share/", "/share/"} {
		if rest, ok := strings.CutPrefix(p, prefix); ok {
			tok, tail, _ := strings.Cut(rest, "/")
			if len(tok) > shareTokenPrefix {
				if tail != "" {
					tail = "/" + tail
				}
				return prefix + tok[:shareTokenPrefix] + "…" + tail
			}
		}
	}
	return p
}

// shareActor is the audit actor of a portal write: share:<token prefix>.
func shareActor(p string) string {
	rest, _ := strings.CutPrefix(p, "/api/public/share/")
	tok, _, _ := strings.Cut(rest, "/")
	return "share:" + tok[:min(len(tok), shareTokenPrefix)]
}

// withLogging logs "🌐 GET /api/activities 200 · 42ms" (health checks and static files at debug level).
func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		path := redactPath(r.URL.Path)
		isAPI := strings.HasPrefix(path, "/api/")
		if isAPI && r.URL.RawQuery != "" {
			path += "?" + redactedQuery(r.URL.RawQuery)
		}
		msg := fmt.Sprintf("🌐 %s %s %d · %s", r.Method, path, sw.status, logx.Ms(time.Since(start)))
		lvl := slog.LevelInfo
		switch {
		case sw.status >= 500:
			lvl = slog.LevelError
		case sw.status == http.StatusUnauthorized:
			lvl = slog.LevelInfo
		case sw.status >= 400:
			lvl = slog.LevelWarn
		case !isAPI:
			lvl = slog.LevelDebug
		}
		log.Log(r.Context(), lvl, msg)
	})
}

func withRecover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Error(fmt.Sprintf("❌ Panique sur %s %s : %v", r.Method, redactPath(r.URL.Path), v), "stack", string(debug.Stack()))
				httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// csp allows only same-origin resources; inline styles stay allowed for React style props and font loaders.
const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self' data:; connect-src 'self'; manifest-src 'self'; worker-src 'self'; object-src 'none'; " +
	"base-uri 'self'; frame-ancestors 'none'"

func withSecurityHeaders(https bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), "+
			"publickey-credentials-create=(self), publickey-credentials-get=(self)")
		if https {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// withCSRF requires X-Stride-CSRF: 1 on state-changing /api/ calls, except the public Strava webhook.
func withCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !isSafeMethod(r.Method) &&
			!(r.Method == http.MethodPost && r.URL.Path == "/api/webhooks/strava") &&
			r.Header.Get(csrfHeader) != "1" {
			httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "Requête refusée : en-tête X-Stride-CSRF manquant.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			limit := int64(maxBodyBytes)
			if r.Method == http.MethodPost && r.URL.Path == "/api/imports" {
				limit = maxImportBodyBytes
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}
