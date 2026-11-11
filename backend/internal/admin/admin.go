// Package admin exposes the platform administration API (users, invitations, settings, meter, cache).
package admin

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"stride/internal/apimeter"
	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/backup"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/httpx"
	"stride/internal/sse"
	"stride/internal/store"
)

type Deps struct {
	Store     *store.Store
	Cache     cache.Cache
	Meter     apimeter.Meter
	Auth      *auth.Service
	PublicURL string
	Log       *slog.Logger
	Now       func() time.Time

	// Control center (optional: a missing piece is reported as unavailable).
	Domain    *domain.Services
	SSE       *sse.Hub
	Logs      *applog.Log
	DBPath    string
	Runtime   RuntimeConfig
	StartedAt time.Time
	// Maintenance purges expired sessions, invitations, API events, logs, jobs and share links.
	Maintenance func(context.Context) (map[string]int64, error)
	// Backups serves « Backups & S3 » (optional).
	Backups *backup.Service
}

type Handler struct {
	d Deps
}

func New(d Deps) *Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.StartedAt.IsZero() {
		d.StartedAt = d.Now()
	}
	return &Handler{d: d}
}

type User struct {
	ID          int64      `json:"id"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	Passkeys    int        `json:"passkeys"`
	domain.AdminUserStats
}

type Invitation struct {
	ID         int64      `json:"id"`
	Label      string     `json:"label"`
	CodePrefix string     `json:"codePrefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	MaxUses    int        `json:"maxUses"`
	Uses       int        `json:"uses"`
	Revoked    bool       `json:"revoked"`
}

type Settings struct {
	RegistrationMode string `json:"registrationMode"`
}

func (h *Handler) Register(mux *http.ServeMux) {
	a := h.d.Auth.RequireAdmin
	mux.Handle("GET /api/admin/users", a(http.HandlerFunc(h.listUsers)))
	mux.Handle("PATCH /api/admin/users/{id}", a(http.HandlerFunc(h.patchUser)))
	mux.Handle("DELETE /api/admin/users/{id}", a(http.HandlerFunc(h.deleteUser)))
	mux.Handle("GET /api/admin/invitations", a(http.HandlerFunc(h.listInvitations)))
	mux.Handle("POST /api/admin/invitations", a(http.HandlerFunc(h.createInvitation)))
	mux.Handle("DELETE /api/admin/invitations/{id}", a(http.HandlerFunc(h.revokeInvitation)))
	mux.Handle("GET /api/admin/settings", a(http.HandlerFunc(h.getSettings)))
	mux.Handle("PUT /api/admin/settings", a(http.HandlerFunc(h.putSettings)))
	mux.Handle("GET /api/admin/meter", a(http.HandlerFunc(h.meter)))
	mux.Handle("POST /api/admin/cache/flush", a(http.HandlerFunc(h.flushCache)))
	h.registerControl(mux)
	h.registerBackups(mux)
}

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	h.d.Log.Error("❌ "+what, "path", r.URL.Path, "err", err.Error())
	httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Identifiant invalide.")
		return 0, false
	}
	return id, true
}

func admin(r *http.Request) auth.User {
	u, _ := auth.UserFrom(r.Context())
	return u
}

func userView(u store.User, stats map[int64]domain.AdminUserStats) User {
	v := User{ID: u.ID, DisplayName: u.DisplayName, Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt, Passkeys: u.Passkeys,
		AdminUserStats: stats[u.ID]}
	if v.Integrations == nil {
		v.Integrations = []string{}
	}
	return v
}

// usersStats returns the footprint of every user, or nil without domain services.
func (h *Handler) usersStats(ctx context.Context) (map[int64]domain.AdminUserStats, error) {
	if h.d.Domain == nil {
		return nil, nil
	}
	return h.d.Domain.AdminUsersStats(ctx)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.d.Store.ListUsers(r.Context())
	if err != nil {
		h.internal(w, r, "Liste des utilisateurs", err)
		return
	}
	stats, err := h.usersStats(r.Context())
	if err != nil {
		h.internal(w, r, "Statistiques des utilisateurs", err)
		return
	}
	out := make([]User, 0, len(users))
	for _, u := range users {
		out = append(out, userView(u, stats))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) patchUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if id == admin(r).ID {
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Vous ne pouvez pas modifier votre propre compte depuis l'administration.")
		return
	}
	var body struct {
		Status *string `json:"status"`
		Role   *string `json:"role"`
	}
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	if body.Status != nil && *body.Status != store.StatusActive && *body.Status != store.StatusDisabled {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Statut invalide (active ou disabled).")
		return
	}
	if body.Role != nil && *body.Role != store.RoleUser && *body.Role != store.RoleAdmin {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Rôle invalide (user ou admin).")
		return
	}
	switch err := h.d.Store.UpdateUserAccess(r.Context(), id, body.Role, body.Status); {
	case errors.Is(err, store.ErrUnknownUser):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Utilisateur introuvable.")
		return
	case errors.Is(err, store.ErrLastAdmin):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, lastAdminMsg)
		return
	case err != nil:
		h.internal(w, r, "Mise à jour d'un utilisateur", err)
		return
	}
	h.d.Log.Info("🛡️ Utilisateur modifié par un administrateur", "adminId", admin(r).ID, "userId", id,
		"role", deref(body.Role), "status", deref(body.Status))
	users, err := h.d.Store.ListUsers(r.Context())
	if err != nil {
		h.internal(w, r, "Liste des utilisateurs", err)
		return
	}
	stats, err := h.usersStats(r.Context())
	if err != nil {
		h.internal(w, r, "Statistiques des utilisateurs", err)
		return
	}
	for _, u := range users {
		if u.ID == id {
			httpx.JSON(w, http.StatusOK, userView(u, stats))
			return
		}
	}
	httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Utilisateur introuvable.")
}

func deref(p *string) string {
	if p == nil {
		return "-"
	}
	return *p
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if id == admin(r).ID {
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Vous ne pouvez pas supprimer votre propre compte depuis l'administration.")
		return
	}
	err := h.d.Store.EnsureNotLastAdmin(r.Context(), id)
	if errors.Is(err, store.ErrLastAdmin) {
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, lastAdminMsg)
		return
	}
	if err == nil {
		err = h.d.Auth.DeleteUser(r.Context(), id)
	}
	switch {
	case errors.Is(err, store.ErrUnknownUser):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Utilisateur introuvable.")
		return
	case err != nil:
		h.internal(w, r, "Suppression d'un utilisateur", err)
		return
	}
	h.d.Log.Info("🗑️ Utilisateur supprimé par un administrateur", "adminId", admin(r).ID, "userId", id)
	w.WriteHeader(http.StatusNoContent)
}

func invitationView(i store.Invitation) Invitation {
	return Invitation{ID: i.ID, Label: i.Label, CodePrefix: i.CodePrefix, CreatedAt: i.CreatedAt, ExpiresAt: i.ExpiresAt, MaxUses: i.MaxUses, Uses: i.Uses, Revoked: i.Revoked}
}

func (h *Handler) listInvitations(w http.ResponseWriter, r *http.Request) {
	invs, err := h.d.Store.ListInvitations(r.Context())
	if err != nil {
		h.internal(w, r, "Liste des invitations", err)
		return
	}
	out := make([]Invitation, 0, len(invs))
	for _, i := range invs {
		out = append(out, invitationView(i))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// newInviteCode returns 24 base32 characters grouped by 4 (120 bits).
func newInviteCode() string {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	raw := base32.StdEncoding.EncodeToString(b)
	parts := make([]string, 0, 6)
	for i := 0; i < len(raw); i += 4 {
		parts = append(parts, raw[i:i+4])
	}
	return strings.Join(parts, "-")
}

func (h *Handler) createInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label          string `json:"label"`
		MaxUses        *int   `json:"maxUses"`
		ExpiresInHours *int   `json:"expiresInHours"`
	}
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	label := strings.Join(strings.Fields(body.Label), " ")
	if len([]rune(label)) > 80 {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Libellé trop long (80 caractères maximum).")
		return
	}
	maxUses, hours := 1, 168
	if body.MaxUses != nil {
		maxUses = *body.MaxUses
	}
	if body.ExpiresInHours != nil {
		hours = *body.ExpiresInHours
	}
	if maxUses < 1 || maxUses > 1000 {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Nombre d'utilisations invalide (1 à 1000).")
		return
	}
	if hours < 0 || hours > 24*365 {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Durée de validité invalide (0 à 8760 heures, 0 = sans expiration).")
		return
	}
	now := h.d.Now().UTC().Truncate(time.Second)
	var expires *time.Time
	if hours > 0 {
		e := now.Add(time.Duration(hours) * time.Hour)
		expires = &e
	}
	code := newInviteCode()
	creator := admin(r).ID
	inv, err := h.d.Store.CreateInvitation(r.Context(), store.Invitation{
		Label: label, CodePrefix: code[:4], MaxUses: maxUses, ExpiresAt: expires, CreatedBy: &creator, CreatedAt: now,
	}, auth.InviteHash(code))
	if err != nil {
		h.internal(w, r, "Création d'invitation", err)
		return
	}
	h.d.Log.Info("✉️ Invitation créée", "adminId", creator, "invitationId", inv.ID, "maxUses", maxUses)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"invitation": invitationView(*inv),
		"code":       code,
		"link":       strings.TrimRight(h.d.PublicURL, "/") + "/register?invite=" + url.QueryEscape(code),
	})
}

func (h *Handler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	switch err := h.d.Store.RevokeInvitation(r.Context(), id); {
	case errors.Is(err, store.ErrUnknownInvitation):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Invitation introuvable.")
		return
	case err != nil:
		h.internal(w, r, "Révocation d'invitation", err)
		return
	}
	h.d.Log.Info("✉️ Invitation révoquée", "adminId", admin(r).ID, "invitationId", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	mode, err := h.d.Store.RegistrationMode(r.Context())
	if err != nil {
		h.internal(w, r, "Lecture des réglages", err)
		return
	}
	httpx.JSON(w, http.StatusOK, Settings{RegistrationMode: mode})
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var body Settings
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	if body.RegistrationMode != store.RegistrationOpen && body.RegistrationMode != store.RegistrationInvite {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Mode d'inscription invalide (open ou invite).")
		return
	}
	if err := h.d.Store.SetSetting(r.Context(), store.SettingRegistrationMode, body.RegistrationMode, h.d.Now()); err != nil {
		h.internal(w, r, "Mise à jour des réglages", err)
		return
	}
	h.d.Log.Info("⚙️ Mode d'inscription modifié", "adminId", admin(r).ID, "mode", body.RegistrationMode)
	httpx.JSON(w, http.StatusOK, body)
}

func (h *Handler) meter(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, h.d.Meter.Snapshot())
}

func (h *Handler) flushCache(w http.ResponseWriter, r *http.Request) {
	n, err := h.d.Cache.DelPrefix(r.Context(), cache.Prefix+"cache:")
	if err != nil {
		h.internal(w, r, "Vidage du cache", err)
		return
	}
	h.d.Log.Info("🧹 Cache vidé", "adminId", admin(r).ID, "removed", n)
	httpx.JSON(w, http.StatusOK, map[string]int{"removed": n})
}
