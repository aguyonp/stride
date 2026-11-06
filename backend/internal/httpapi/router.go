// Package httpapi composes the HTTP server: platform routes, domain routes, health checks and the embedded SPA.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/klauspost/compress/gzhttp"

	"stride/internal/admin"
	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/buildinfo"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/domainapi"
	"stride/internal/httpx"
	"stride/internal/observability"
	"stride/internal/openapi"
	"stride/internal/sse"
	"stride/internal/store"
)

type Deps struct {
	Store   *store.Store
	Cache   cache.Cache
	Auth    *auth.Service
	Admin   *admin.Handler
	Domain  *domain.Services
	Log     *slog.Logger
	Logs    *applog.Log
	HTTPS   bool
	Static  http.Handler
	SSE     *sse.Hub
	Metrics *observability.Metrics
}

func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok", "version": buildinfo.Version})
	})
	mux.HandleFunc("GET /health/ready", d.ready)

	// Unauthenticated platform endpoints
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, buildinfo.Get())
	})
	if d.Metrics != nil {
		mux.Handle("GET /metrics", d.Metrics.Handler())
	}
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(openapi.SpecJSON())
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(openapi.SpecYAML())
	})

	d.Auth.Register(mux)
	mux.Handle("GET /api/version", d.Auth.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, buildinfo.Get())
	})))
	if d.Logs != nil {
		mux.Handle("POST /api/actions", d.Auth.RequireUser(postAction(d.Logs)))
		mux.Handle("GET /api/admin/logs", d.Auth.RequireAdmin(adminLogs(d.Logs)))
	}
	d.Admin.Register(mux)
	domainapi.Register(mux, domainapi.Deps{
		Services:     d.Domain,
		RequireUser:  d.Auth.RequireUser,
		RequireAdmin: d.Auth.RequireAdmin,
		Log:          d.Log,
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Route d'API inconnue.")
	})
	if d.Static != nil {
		mux.Handle("/", d.Static)
	}

	var sseHandler http.Handler
	if d.SSE != nil {
		sseHandler = d.Auth.RequireUser(http.HandlerFunc(d.SSE.ServeHTTP))
	}

	var h http.Handler = mux
	h = gzhttp.GzipHandler(h)

	if sseHandler != nil {
		gzHandler := h
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/events/stream" {
				sseHandler.ServeHTTP(w, r)
				return
			}
			gzHandler.ServeHTTP(w, r)
		})
	}

	h = withCSRF(h)
	h = withBodyLimit(h)
	h = withSecurityHeaders(d.HTTPS, h)
	h = withAudit(d.Logs, h)
	h = withLogging(d.Log, h)
	h = withRecover(d.Log, h)
	return h
}

func (d Deps) ready(w http.ResponseWriter, r *http.Request) {
	check := func(ping func(context.Context) error) string {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			msg := err.Error()
			if len(msg) > 200 {
				msg = msg[:200]
			}
			return "error: " + msg
		}
		return "ok"
	}
	checks := map[string]string{"db": check(d.Store.Ping), "cache": check(d.Cache.Ping)}
	if checks["db"] != "ok" || checks["cache"] != "ok" {
		d.Log.Warn("🩺 Service non prêt", "db", checks["db"], "cache", checks["cache"])
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unavailable", "checks": checks})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "ready", "checks": checks})
}
