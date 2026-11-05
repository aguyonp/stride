// Package httpapi is reduced to its middlewares in the domain branch; the platform workstream owns
// the real server (routes are mounted by domainapi.Register).
package httpapi

import (
	"net/http"

	"stride/internal/httpx"
)

func writeError(w http.ResponseWriter, status int, code, msg string) {
	httpx.Fail(w, status, code, msg)
}
