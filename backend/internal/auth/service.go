package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"stride/internal/apimeter"
	"stride/internal/cache"
	"stride/internal/env"
	"stride/internal/httpx"
	"stride/internal/store"
)

const (
	SessionCookie  = "stride_session"
	ceremonyCookie = "stride_webauthn"
	ceremonyTTL    = 5 * time.Minute
	touchEvery     = time.Minute
)

type Options struct {
	Env   env.Config
	Store *store.Store
	Cache cache.Cache
	Log   *slog.Logger
	// OnUserCreated runs after a successful registration (domain bootstrap); errors are logged only.
	OnUserCreated func(ctx context.Context, userID int64, firstAdmin bool) error
	// OnUserDeleted runs after a user deletion (domain cache purge); errors are logged only.
	OnUserDeleted func(ctx context.Context, userID int64) error
	Now           func() time.Time
}

type Service struct {
	store     *store.Store
	cache     cache.Cache
	wa        *webauthn.WebAuthn
	log       *slog.Logger
	ttl       time.Duration
	secure    bool
	rpID      string
	now       func() time.Time
	onCreated func(context.Context, int64, bool) error
	onDeleted func(context.Context, int64) error
}

func New(o Options) (*Service, error) {
	timeouts := webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  o.Env.RPID,
		RPDisplayName:         "Stride",
		RPOrigins:             o.Env.RPOrigins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		},
		Timeouts: webauthn.TimeoutsConfig{Login: timeouts, Registration: timeouts},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn: %w", err)
	}
	s := &Service{
		store:     o.Store,
		cache:     o.Cache,
		wa:        wa,
		log:       o.Log,
		ttl:       o.Env.SessionTTL,
		secure:    o.Env.CookieSecure,
		rpID:      o.Env.RPID,
		now:       o.Now,
		onCreated: o.OnUserCreated,
		onDeleted: o.OnUserDeleted,
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.ttl <= 0 {
		s.ttl = 720 * time.Hour
	}
	return s, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func randomToken(n int) string { return base64.RawURLEncoding.EncodeToString(randomBytes(n)) }

func tokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func (s *Service) setCookie(w http.ResponseWriter, name, value, path string, maxAge time.Duration, sameSite http.SameSite) {
	c := &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: s.secure, SameSite: sameSite}
	if maxAge > 0 {
		c.MaxAge = int(maxAge.Seconds())
	} else {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func (s *Service) issueSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID int64) error {
	token := randomToken(32)
	now := s.now()
	ua := r.UserAgent()
	if len(ua) > 256 {
		ua = ua[:256]
	}
	err := s.store.CreateSession(ctx, store.Session{TokenHash: tokenHash(token), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(s.ttl), LastSeenAt: now, UserAgent: ua})
	if err != nil {
		return err
	}
	s.setCookie(w, SessionCookie, token, "/", s.ttl, http.SameSiteLaxMode)
	return nil
}

func (s *Service) clearSession(w http.ResponseWriter) {
	s.setCookie(w, SessionCookie, "", "/", 0, http.SameSiteLaxMode)
}

// lookup resolves the session cookie and applies the sliding expiry.
func (s *Service) lookup(w http.ResponseWriter, r *http.Request) (*store.User, error) {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" || len(c.Value) > 128 {
		return nil, store.ErrUnknownSession
	}
	now := s.now()
	hash := tokenHash(c.Value)
	sess, u, err := s.store.GetSession(r.Context(), hash, now)
	if err != nil {
		return nil, err
	}
	if u.Status != store.StatusActive {
		return nil, store.ErrUnknownSession
	}
	if now.Sub(sess.LastSeenAt) >= touchEvery {
		if err := s.store.TouchSession(r.Context(), hash, now, now.Add(s.ttl)); err != nil {
			s.log.Warn("⚠️ Mise à jour de session impossible", "err", err.Error())
		} else {
			s.setCookie(w, SessionCookie, c.Value, "/", s.ttl, http.SameSiteLaxMode)
		}
	}
	return u, nil
}

type storeUserKey struct{}

func withSessionUser(r *http.Request, u *store.User) *http.Request {
	ctx := WithUser(r.Context(), User{ID: u.ID, DisplayName: u.DisplayName, Role: u.Role})
	ctx = apimeter.WithUser(ctx, u.ID)
	ctx = context.WithValue(ctx, storeUserKey{}, u)
	return r.WithContext(ctx)
}

func sessionUser(r *http.Request) *store.User {
	u, _ := r.Context().Value(storeUserKey{}).(*store.User)
	return u
}

// RequireUser rejects requests without a valid session (401) and injects auth.User and the meter user.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.lookup(w, r)
		if err != nil {
			if !errors.Is(err, store.ErrUnknownSession) {
				s.log.Error("❌ Lecture de session impossible", "err", err.Error())
				httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
				return
			}
			httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "Connexion requise.")
			return
		}
		next.ServeHTTP(w, withSessionUser(r, u))
	})
}

// RequireAdmin is RequireUser plus a 403 for non-admins.
func (s *Service) RequireAdmin(next http.Handler) http.Handler {
	return s.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := UserFrom(r.Context()); !ok || !u.IsAdmin() {
			httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "Accès réservé aux administrateurs.")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// DeleteUser removes a user (sessions, passkeys and domain rows cascade) then purges its cached data.
func (s *Service) DeleteUser(ctx context.Context, userID int64) error {
	if err := s.store.DeleteUser(ctx, userID); err != nil {
		return err
	}
	if s.onDeleted != nil {
		if err := s.onDeleted(context.WithoutCancel(ctx), userID); err != nil {
			s.log.Warn("⚠️ Purge du cache utilisateur incomplète", "userId", userID, "err", err.Error())
		}
	}
	return nil
}

// NormalizeName trims and collapses whitespace; returns "" when empty or longer than max runes.
func NormalizeName(v string, max int) string {
	v = strings.Join(strings.Fields(v), " ")
	if v == "" || utf8.RuneCountInString(v) > max {
		return ""
	}
	return v
}

// InviteHash normalises an invitation code (case-insensitive, separators ignored) and hashes it.
func InviteHash(code string) []byte {
	c := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	h := sha256.Sum256([]byte(c))
	return h[:]
}
