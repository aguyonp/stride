package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"patella/internal/logx"
)

// statusWriter capture le statut et la taille de la réponse.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
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
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// withLogging : 🌐 GET /api/activities 200 · 42ms (health et assets statiques en debug).
func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		path := r.URL.Path
		if r.URL.RawQuery != "" && strings.HasPrefix(path, "/api/") {
			path += "?" + r.URL.RawQuery
		}
		msg := fmt.Sprintf("🌐 %s %s %d · %s", r.Method, path, sw.status, logx.Ms(time.Since(start)))
		lvl := slog.LevelInfo
		switch {
		case sw.status >= 500:
			lvl = slog.LevelError
		case sw.status >= 400:
			lvl = slog.LevelWarn
		case r.URL.Path == "/api/health" || !strings.HasPrefix(r.URL.Path, "/api/"):
			lvl = slog.LevelDebug
		}
		log.Log(r.Context(), lvl, msg)
	})
}

// withRecover convertit une panique en 500 JSON.
func withRecover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Error(fmt.Sprintf("❌ panique sur %s %s : %v", r.Method, r.URL.Path, v), "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "INTERNAL", "Erreur interne du serveur.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// withBasicAuth protège tout sauf /api/health (comparaison en temps constant).
func withBasicAuth(user, pass string, next http.Handler) http.Handler {
	uh := sha256.Sum256([]byte(user))
	ph := sha256.Sum256([]byte(pass))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		gu := sha256.Sum256([]byte(u))
		gp := sha256.Sum256([]byte(p))
		uOK := subtle.ConstantTimeCompare(gu[:], uh[:])
		pOK := subtle.ConstantTimeCompare(gp[:], ph[:])
		if !ok || uOK&pOK != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Patella", charset="UTF-8"`)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentification requise.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withSecurityHeaders ajoute quelques en-têtes de sécurité de base.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
