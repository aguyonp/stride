// Package domainapi exposes the domain services over HTTP.
package domainapi

import (
	"log/slog"
	"net/http"

	"stride/internal/domain"
)

type Deps struct {
	Services     *domain.Services
	RequireUser  func(http.Handler) http.Handler
	RequireAdmin func(http.Handler) http.Handler
	Log          *slog.Logger
}

// Register mounts every domain route on mux (Go 1.22 patterns, e.g. "GET /api/activities/{id}").
func Register(mux *http.ServeMux, d Deps) {}
