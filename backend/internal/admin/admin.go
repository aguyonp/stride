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
	"stride/internal/buildinfo"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/httpx"
	"stride/internal/sse"
	"stride/internal/store"
	"stride/internal/updates"
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
	// Updates detects a newer release on GitHub (optional: nil disables GET /api/updates).
	Updates *updates.Service
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
	// Notification centre retention (issue #142): days of history and entries kept per user.
	InboxRetentionDays int `json:"inboxRetentionDays"`
	InboxRetentionMax  int `json:"inboxRetentionMax"`
	// UpdateCheckEnabled is the GitHub release check (issue #143). A pointer on input so an absent
	// field keeps the current value, like the retention pair.
	UpdateCheckEnabled *bool `json:"updateCheckEnabled,omitempty"`
	// GarminBridgeEnabled is the admin half of the experimental bridge kill switch (§23, #147); the
	// env variable STRIDE_GARMIN_BRIDGE is the other half. Two switches, both must be true.
	GarminBridgeEnabled *bool `json:"garminBridgeEnabled,omitempty"`
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
	mux.Handle("GET /api/updates", a(http.HandlerFunc(h.updates)))
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
	days, max := h.d.Store.InboxRetention(r.Context())
	enabled := h.d.Store.UpdateCheckEnabled(r.Context())
	bridge := h.d.Store.GarminBridgeEnabled(r.Context())
	httpx.JSON(w, http.StatusOK, Settings{RegistrationMode: mode, InboxRetentionDays: days, InboxRetentionMax: max,
		UpdateCheckEnabled: &enabled, GarminBridgeEnabled: &bridge})
}

// updates answers the release check for an administrator (issue #143). The check is cached on the
// server: one GitHub request per TTL whatever the number of admins or devices, and a failure is
// never an error here — the banner simply does not appear.
func (h *Handler) updates(w http.ResponseWriter, r *http.Request) {
	current := buildinfo.Version
	if h.d.Updates == nil || !h.d.Store.UpdateCheckEnabled(r.Context()) {
		httpx.JSON(w, http.StatusOK, updates.Status{Current: current, Disabled: h.d.Updates != nil})
		return
	}
	st := h.d.Updates.Check(r.Context(), current)
	if !st.Available {
		httpx.JSON(w, http.StatusOK, st)
		return
	}
	// One entry per admin and per version: the notice must exist for every admin, not only for the
	// one whose browser happened to load the page first.
	if h.d.Domain != nil && st.Latest != "" {
		ids, err := h.d.Store.AdminUserIDs(r.Context())
		if err != nil {
			h.d.Log.Warn("⚠️ Liste des administrateurs indisponible", "err", err.Error())
		}
		for _, uid := range ids {
			h.d.Domain.WriteUpdateInbox(r.Context(), uid, st.Latest, st.PublishedAt, st.URL)
		}
		// The caller's own entry drives the banner: it disappears once read.
		if u, ok := auth.UserFrom(r.Context()); ok {
			if e, err := h.d.Domain.UpdateInboxEntry(r.Context(), u.ID, st.Latest); err == nil && e != nil {
				st.Notification, st.Read = e.ID, e.ReadAt != nil
			}
		}
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var body Settings
	if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
		return
	}
	// A field left out keeps its current value, so the panel that owns one setting cannot reset
	// the others; a wrong value is still rejected.
	curMode, err := h.d.Store.RegistrationMode(r.Context())
	if err != nil {
		h.internal(w, r, "Lecture des réglages", err)
		return
	}
	mode := body.RegistrationMode
	if mode == "" {
		mode = curMode
	}
	if mode != store.RegistrationOpen && mode != store.RegistrationInvite {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Mode d'inscription invalide (open ou invite).")
		return
	}
	if err := h.d.Store.SetSetting(r.Context(), store.SettingRegistrationMode, mode, h.d.Now()); err != nil {
		h.internal(w, r, "Mise à jour des réglages", err)
		return
	}
	// Retention is clamped, not rejected; the purge applies the new bounds at once (issue #142).
	curDays, curMax := h.d.Store.InboxRetention(r.Context())
	days, max := store.ClampInboxRetention(keepIfZero(body.InboxRetentionDays, curDays), keepIfZero(body.InboxRetentionMax, curMax))
	for _, kv := range []struct {
		key   string
		value int
	}{{store.SettingInboxRetentionDays, days}, {store.SettingInboxRetentionMax, max}} {
		if err := h.d.Store.SetSetting(r.Context(), kv.key, strconv.Itoa(kv.value), h.d.Now()); err != nil {
			h.internal(w, r, "Mise à jour de la rétention", err)
			return
		}
	}
	// Release check (issue #143): a pointer tells « absent » from « false ».
	enabled := h.d.Store.UpdateCheckEnabled(r.Context())
	if body.UpdateCheckEnabled != nil {
		enabled = *body.UpdateCheckEnabled
		if err := h.d.Store.SetSetting(r.Context(), store.SettingUpdateCheckEnabled, strconv.FormatBool(enabled), h.d.Now()); err != nil {
			h.internal(w, r, "Mise à jour des réglages", err)
			return
		}
	}
	// Experimental Garmin bridge (§23): same « absent keeps the value » rule.
	bridgeOn := h.d.Store.GarminBridgeEnabled(r.Context())
	if body.GarminBridgeEnabled != nil {
		bridgeOn = *body.GarminBridgeEnabled
		if err := h.d.Store.SetSetting(r.Context(), store.SettingGarminBridgeEnabled, strconv.FormatBool(bridgeOn), h.d.Now()); err != nil {
			h.internal(w, r, "Mise à jour des réglages", err)
			return
		}
	}
	if h.d.Domain != nil {
		if n, err := h.d.Domain.PurgeInbox(r.Context()); err != nil {
			h.d.Log.Warn("⚠️ Purge du centre de notifications impossible", "err", err.Error())
		} else if n > 0 {
			h.d.Log.Info("🧹 Centre de notifications purgé", "adminId", admin(r).ID, "removed", n)
		}
	}
	h.d.Log.Info("⚙️ Réglages modifiés", "adminId", admin(r).ID, "mode", mode, "inboxDays", days, "inboxMax", max, "updateCheck", enabled)
	httpx.JSON(w, http.StatusOK, Settings{RegistrationMode: mode, InboxRetentionDays: days, InboxRetentionMax: max,
		UpdateCheckEnabled: &enabled, GarminBridgeEnabled: &bridgeOn})
}

func (h *Handler) meter(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, h.d.Meter.Snapshot())
}

// keepIfZero keeps the current value of a settings field the client omitted.
func keepIfZero(v, cur int) int {
	if v == 0 {
		return cur
	}
	return v
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
