package auth

import (
	"context"
	"encoding/base32"
	"errors"
	"net/http"
	"strings"
	"time"

	"stride/internal/httpx"
	"stride/internal/ratelimit"
	"stride/internal/store"
)

const (
	recoveryCodeSetSize = 10 // one-time codes per generated set
	recoveryCodeBytes   = 10 // 80 bits of entropy per code
	recoveryLoginLimit  = 10 // attempts per client IP per window (brute force)
	recoveryLoginWindow = time.Minute
)

// newRecoveryCode returns a code: 10 random bytes in base32 (RFC 4648), 16 uppercase
// characters, displayed as four groups of four. The second value is the normalized form
// (no separators) whose SHA-256 is what gets stored.
func newRecoveryCode() (display, normalized string) {
	enc := strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes(recoveryCodeBytes)))
	return enc[0:4] + "-" + enc[4:8] + "-" + enc[8:12] + "-" + enc[12:16], enc
}

// normalizeRecoveryCode uppercases and drops separators: the base32 alphabet makes case
// and dashes irrelevant to the user.
func normalizeRecoveryCode(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	return strings.NewReplacer("-", "", " ", "").Replace(v)
}

// GenerateRecoveryCodes atomically invalidates the user's remaining codes and returns a
// fresh set in plaintext — the only moment the plaintext ever exists outside the screen.
func (s *Service) GenerateRecoveryCodes(ctx context.Context, userID int64) ([]string, error) {
	codes := make([]string, 0, recoveryCodeSetSize)
	hashes := make([][]byte, 0, recoveryCodeSetSize)
	for i := 0; i < recoveryCodeSetSize; i++ {
		display, normalized := newRecoveryCode()
		codes = append(codes, display)
		hashes = append(hashes, tokenHash(normalized))
	}
	if err := s.store.ReplaceRecoveryCodes(ctx, userID, hashes, s.now()); err != nil {
		return nil, err
	}
	return codes, nil
}

// recoveryStatus answers GET /api/me/recovery: how many unused codes remain.
func (s *Service) recoveryStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUnusedRecoveryCodes(r.Context(), sessionUser(r).ID)
	if err != nil {
		s.internal(w, r, "Lecture des codes de récupération", err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Remaining int `json:"remaining"`
	}{n})
}

// regenerateRecovery answers POST /api/me/recovery: a new set of codes, old ones void.
func (s *Service) regenerateRecovery(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	codes, err := s.GenerateRecoveryCodes(r.Context(), u.ID)
	if err != nil {
		s.internal(w, r, "Génération des codes de récupération", err)
		return
	}
	s.log.Info("🔐 Codes de récupération régénérés", "userId", u.ID)
	httpx.JSON(w, http.StatusOK, struct {
		Codes []string `json:"codes"`
	}{codes})
}

// loginRecovery answers the public POST /api/auth/login/recovery: one unused code signs the
// user in (session issued like for a passkey) and is consumed; a disabled account is 403.
func (s *Service) loginRecovery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.recoveryLim.Allow(ratelimit.ClientIP(r), s.now()) {
		httpx.Fail(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "Trop de tentatives : réessaie dans une minute.")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	code := normalizeRecoveryCode(body.Code)
	if len(code) != 16 {
		httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "Code de récupération invalide.")
		return
	}
	userID, err := s.store.ConsumeRecoveryCode(ctx, tokenHash(code), s.now())
	if err != nil {
		if errors.Is(err, store.ErrRecoveryCodeInvalid) {
			s.log.Warn("🔐 Code de récupération refusé (inconnu ou déjà utilisé)")
			httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "Code de récupération invalide ou déjà utilisé.")
			return
		}
		s.internal(w, r, "Vérification du code de récupération", err)
		return
	}
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		s.internal(w, r, "Lecture du compte", err)
		return
	}
	if u.Status != store.StatusActive {
		s.log.Warn("🔐 Connexion par code refusée : compte désactivé", "userId", u.ID)
		httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "Ce compte est désactivé.")
		return
	}
	if err := s.issueSession(ctx, w, r, u.ID); err != nil {
		s.internal(w, r, "Création de session", err)
		return
	}
	s.log.Info("🔑 Connexion par code de récupération", "userId", u.ID)
	httpx.JSON(w, http.StatusOK, MeFrom(u))
}
