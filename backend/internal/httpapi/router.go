// Package httpapi composes the HTTP server: platform routes, domain routes, health checks and the embedded SPA.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/klauspost/compress/gzhttp"

	"stride/internal/admin"
	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/buildinfo"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/domainapi"
	"stride/internal/feedback"
	"stride/internal/httpx"
	"stride/internal/mcp"
	"stride/internal/observability"
	"stride/internal/sse"
	"stride/internal/store"
)

type Deps struct {
	Store *store.Store
	Cache cache.Cache
	Auth  *auth.Service
	Admin *admin.Handler
	// Feedback is optional (POST /api/feedback, admin listing and triage).
	Feedback *feedback.Handler
	Domain   *domain.Services
	// MCP is the per-user MCP server (POST /mcp/{token} + /api/mcp* management).
	MCP     *mcp.Server
	Log     *slog.Logger
	Logs    *applog.Log
	HTTPS   bool
	Static  http.Handler
	SSE     *sse.Hub
	Metrics *observability.Metrics
}

func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok", "version": buildinfo.Version})
	})
	mux.HandleFunc("GET /health/ready", d.ready)

	// Unauthenticated platform endpoints
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, buildinfo.Get())
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
	if d.Feedback != nil {
		d.Feedback.Register(mux)
	}
	if d.MCP != nil {
		// The public /mcp/ subtree is mounted in the gzip bypass below (single mount
		// point); only the session-protected management API goes on the mux.
		mcp.RegisterAPI(mux, mcp.Deps{Store: d.Store, RequireUser: d.Auth.RequireUser, RequireAdmin: d.Auth.RequireAdmin})
	}
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

	// Streaming handlers bypass the gzip layer (it would buffer the first events): the SSE
	// stream and the MCP subtree (Streamable HTTP, token-authenticated, no CORS).
	type passthrough struct {
		match   func(*http.Request) bool
		handler http.Handler
	}
	var pass []passthrough
	if d.SSE != nil {
		pass = append(pass, passthrough{
			match:   func(r *http.Request) bool { return r.Method == http.MethodGet && r.URL.Path == "/api/events/stream" },
			handler: d.Auth.RequireUser(http.HandlerFunc(d.SSE.ServeHTTP)),
		})
	}
	if d.MCP != nil {
		pass = append(pass, passthrough{
			match:   func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/mcp/") },
			handler: d.MCP.PublicHandler(),
		})
	}

	var h http.Handler = d.Metrics.HTTPMiddleware(mux)
	h = gzhttp.GzipHandler(h)

	if len(pass) > 0 {
		gzHandler := h
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range pass {
				if p.match(r) {
					p.handler.ServeHTTP(w, r)
					return
				}
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
