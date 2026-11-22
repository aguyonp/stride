// server.go embeds the per-user MCP server: the public route POST /mcp/{token}
// (Streamable HTTP, stateless, official Go SDK) and the fr/en tool instances.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/domain"
	"stride/internal/frfmt"
	"stride/internal/httpx"
	"stride/internal/ratelimit"
	"stride/internal/store"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxBodyBytes caps the public MCP body (16 MiB).
const maxBodyBytes = 16 << 20

// Server serves the public MCP route. The fr/en tool instances are shared:
// their handlers resolve the caller from the request context at call time.
type Server struct {
	svc  *domain.Services
	tok  *Service
	frs  *mcpsdk.Server
	ens  *mcpsdk.Server
	hdl  *mcpsdk.StreamableHTTPHandler
	cors *http.CrossOriginProtection
	met  *meter
}

// New builds the per-user MCP server. The public-route quota is loaded from mcp_quota
// (defaults when unreadable) so an admin change survives a restart.
func New(st *store.Store, svc *domain.Services, log *slog.Logger) *Server {
	tok := NewService(st, log)
	q := DefaultQuota
	if stored, err := tok.GetQuota(context.Background()); err == nil {
		q = stored
	} else {
		log.Warn("⚠️ MCP : quota stockée illisible, valeurs par défaut", "err", err.Error())
	}
	s := &Server{svc: svc, tok: tok, met: newMeter(q)}
	s.frs = newTools(svc, frfmt.FR).build()
	s.ens = newTools(svc, frfmt.EN).build()
	s.hdl = mcpsdk.NewStreamableHTTPHandler(s.serverFor, &mcpsdk.StreamableHTTPOptions{
		Stateless:           true,
		MaxRequestBodyBytes: maxBodyBytes,
		// A disconnected client stops the in-flight tool call.
		PropagateRequestCancellation: true,
		// The SDK's DNS-rebinding (localhost) protection stays enabled.
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	s.cors = http.NewCrossOriginProtection()
	return s
}

// PublicHandler serves the whole /mcp/ subtree: POST /mcp/{token} is the MCP endpoint; any
// other path or method gets a uniform answer, so the subtree never falls through to the SPA
// and never reveals whether a token is valid.
func (s *Server) PublicHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/mcp/")
		if token == "" || strings.Contains(token, "/") {
			httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Ressource introuvable.")
			return
		}
		s.serve(w, r, token)
	})
}

// serve applies the rate limits, resolves the token (uniform 404 when unknown, revoked or
// disabled) and serves MCP with the owner injected in the request context.
func (s *Server) serve(w http.ResponseWriter, r *http.Request, token string) {
	now := time.Now()
	if !s.met.limI.Allow(ratelimit.ClientIP(r), now) {
		s.met.noteDenied(now)
		failRateLimited(w)
		return
	}
	if r.Method != http.MethodPost {
		MethodNotAllowed(w, r)
		return
	}
	hash := sha256.Sum256([]byte(token))
	key := hex.EncodeToString(hash[:])
	if !s.met.limT.Allow(key, now) {
		// Attribute the 429 to the token when it exists; unknown probes are counted globally.
		if _, _, err := s.tok.Resolve(r.Context(), token); err == nil {
			s.met.noteTokenDenied(key, now)
		} else {
			s.met.noteDenied(now)
		}
		failRateLimited(w)
		return
	}
	_, user, err := s.tok.Resolve(r.Context(), token)
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Ressource introuvable.")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
		return
	}
	applog.SetUser(r.Context(), user.ID)
	s.tok.Touch(r.Context(), user.ID)
	s.met.noteRequest(key, now)
	s.cors.Handler(s.hdl).ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
}

func failRateLimited(w http.ResponseWriter) {
	httpx.Fail(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "Trop de requêtes, réessayez dans quelques minutes.")
}

// MethodNotAllowed answers the methods a stateless MCP server does not serve (GET, DELETE, …).
func MethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "POST")
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// serverFor picks the tool instance matching the caller's language
// (§15.2: profile.language ?? Accept-Language ?? en).
func (s *Server) serverFor(r *http.Request) *mcpsdk.Server {
	var profile *string
	if u, ok := auth.UserFrom(r.Context()); ok {
		if p, err := s.svc.Profile(r.Context(), u.ID); err == nil {
			profile = p.Language
		}
	}
	if frfmt.Resolve(profile, frfmt.FromAcceptLanguage(r.Header.Get("Accept-Language"))) == frfmt.FR {
		return s.frs
	}
	return s.ens
}
