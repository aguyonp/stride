// handlers.go exposes the per-user MCP management endpoints (/api/mcp*). They are session
// protected (and CSRF-checked by the /api/ middleware); the token itself never appears in the
// session API except the one-time plaintext answer of the generate endpoint.
package mcp

import (
	"net/http"

	"stride/internal/auth"
	"stride/internal/httpx"
	"stride/internal/store"
)

// Deps carries what the /api/mcp* routes need.
type Deps struct {
	Store        *store.Store
	RequireUser  func(http.Handler) http.Handler
	RequireAdmin func(http.Handler) http.Handler
}

// RegisterAPI mounts the MCP management endpoints on mux.
func RegisterAPI(mux *http.ServeMux, d Deps) {
	t := NewService(d.Store, nil)
	user := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, d.RequireUser(h)) }

	user("GET /api/mcp", func(w http.ResponseWriter, r *http.Request) {
		uid, ok := userID(w, r)
		if !ok {
			return
		}
		st, err := t.Status(r.Context(), uid)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
			return
		}
		httpx.JSON(w, http.StatusOK, st)
	})

	user("PUT /api/mcp", func(w http.ResponseWriter, r *http.Request) {
		uid, ok := userID(w, r)
		if !ok {
			return
		}
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if err := httpx.Decode(w, r, &in, 4<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
			return
		}
		if err := t.SetEnabled(r.Context(), uid, in.Enabled); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Generate creates or replaces the token; the plaintext is returned exactly once.
	user("POST /api/mcp/token", func(w http.ResponseWriter, r *http.Request) {
		uid, ok := userID(w, r)
		if !ok {
			return
		}
		token, err := t.Generate(r.Context(), uid)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"token": token, "prefix": tokenPrefix(token)})
	})

	user("DELETE /api/mcp/token", func(w http.ResponseWriter, r *http.Request) {
		uid, ok := userID(w, r)
		if !ok {
			return
		}
		if err := t.Revoke(r.Context(), uid); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func userID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "Authentification requise.")
		return 0, false
	}
	return u.ID, true
}

func tokenPrefix(token string) string {
	if len(token) > len(TokenPrefix)+4 {
		return token[:len(TokenPrefix)+4]
	}
	return token
}
