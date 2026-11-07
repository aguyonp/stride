package admin

import (
	"context"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"stride/internal/apimeter"
	"stride/internal/buildinfo"
	"stride/internal/database"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/httpx"
	"stride/internal/queue"
	"stride/internal/store"
)

// Control center: overview, cross-user jobs and share links, session revocation, on-demand sync and
// database maintenance. Every route is admin-only (RequireAdmin) and audited by the httpapi layer;
// responses never carry secrets, share URLs or tokens (only their 8-character prefix).

const lastAdminMsg = "Impossible : ce compte est le dernier administrateur actif. Promouvez d'abord un autre compte."

// RuntimeConfig is the non-secret part of the server configuration (allowlist: keys, tokens,
// passwords, Valkey URL and listen addresses are never copied here).
type RuntimeConfig struct {
	PublicURL          string `json:"publicUrl"`
	RPID               string `json:"rpId"`
	SecureCookies      bool   `json:"secureCookies"`
	SessionTTLHours    int    `json:"sessionTtlHours"`
	Timezone           string `json:"timezone"`
	LogLevel           string `json:"logLevel"`
	LogFormat          string `json:"logFormat"`
	CacheBackend       string `json:"cacheBackend"`
	MetricsEnabled     bool   `json:"metricsEnabled"`
	StravaEnabled      bool   `json:"stravaEnabled"`
	WeatherProvider    string `json:"weatherProvider"`
	AllowPrivateEgress bool   `json:"allowPrivateEgress"`
	SyncHour           int    `json:"syncHour"`
}

// RuntimeFromEnv copies the allowlisted, non-secret settings of cfg.
func RuntimeFromEnv(cfg env.Config, cacheBackend string) RuntimeConfig {
	return RuntimeConfig{
		PublicURL: cfg.PublicURL, RPID: cfg.RPID, SecureCookies: cfg.CookieSecure, SessionTTLHours: int(cfg.SessionTTL.Hours()),
		Timezone: cfg.Timezone, LogLevel: cfg.LogLevel, LogFormat: cfg.LogFormat, CacheBackend: cacheBackend,
		MetricsEnabled: cfg.MetricsAddr != "", StravaEnabled: cfg.StravaEnabled(), WeatherProvider: cfg.WeatherProvider,
		AllowPrivateEgress: cfg.AllowPrivateEgress, SyncHour: cfg.SyncHour,
	}
}

type DatabaseStatus struct {
	OK        bool  `json:"ok"`
	SizeBytes int64 `json:"sizeBytes"`
	WALBytes  int64 `json:"walBytes"`
	FreeBytes int64 `json:"freeBytes"`
}

type CacheStatus struct {
	Backend   string   `json:"backend"`
	OK        bool     `json:"ok"`
	LatencyMs *float64 `json:"latencyMs"`
}

type JobsStatus struct {
	Groups    []queue.GroupStats `json:"groups"`
	Active    int                `json:"active"`
	Failed24h int                `json:"failed24h"`
}

type StreamStatus struct {
	Clients int `json:"clients"`
	Users   int `json:"users"`
}

type Overview struct {
	GeneratedAt      time.Time                `json:"generatedAt"`
	Build            buildinfo.Info           `json:"build"`
	StartedAt        time.Time                `json:"startedAt"`
	UptimeS          int64                    `json:"uptimeS"`
	RegistrationMode string                   `json:"registrationMode"`
	Database         DatabaseStatus           `json:"database"`
	Cache            CacheStatus              `json:"cache"`
	Jobs             JobsStatus               `json:"jobs"`
	Streams          StreamStatus             `json:"streams"`
	Errors24h        int                      `json:"errors24h"`
	LogsDropped      int64                    `json:"logsDropped"`
	Providers        []apimeter.ProviderStats `json:"providers"`
	Counts           domain.AdminCounts       `json:"counts"`
	Config           RuntimeConfig            `json:"config"`
}

// AdminJob is a job of any user; the result payload is dropped (it may hold a user's AI analysis).
type AdminJob struct {
	queue.JobStatus
	UserName    string `json:"userName"`
	Retryable   bool   `json:"retryable"`
	Cancellable bool   `json:"cancellable"`
}

func (h *Handler) registerControl(mux *http.ServeMux) {
	a := h.d.Auth.RequireAdmin
	mux.Handle("GET /api/admin/overview", a(http.HandlerFunc(h.overview)))
	mux.Handle("DELETE /api/admin/users/{id}/sessions", a(http.HandlerFunc(h.revokeSessions)))
	mux.Handle("POST /api/admin/users/{id}/sync", a(http.HandlerFunc(h.syncUser)))
	mux.Handle("GET /api/admin/jobs", a(http.HandlerFunc(h.listJobs)))
	mux.Handle("POST /api/admin/jobs/{id}/retry", a(http.HandlerFunc(h.retryJob)))
	mux.Handle("POST /api/admin/jobs/{id}/cancel", a(http.HandlerFunc(h.cancelJob)))
	mux.Handle("GET /api/admin/shares", a(http.HandlerFunc(h.listShares)))
	mux.Handle("DELETE /api/admin/shares/{id}", a(http.HandlerFunc(h.revokeShare)))
	mux.Handle("POST /api/admin/maintenance/purge", a(http.HandlerFunc(h.purge)))
	mux.Handle("POST /api/admin/maintenance/optimize", a(http.HandlerFunc(h.optimize)))
	mux.Handle("POST /api/admin/maintenance/check", a(http.HandlerFunc(h.check)))
}

func (h *Handler) unavailable(w http.ResponseWriter) {
	httpx.Fail(w, http.StatusServiceUnavailable, httpx.CodeInternal, "Fonction indisponible sur ce serveur.")
}

// ---------- overview ----------

func (h *Handler) dbStatus(ctx context.Context) DatabaseStatus {
	var pages, size, free int64
	db := h.d.Store.DB
	err := errors.Join(
		db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages),
		db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&size),
		db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&free),
	)
	st := DatabaseStatus{OK: err == nil, SizeBytes: pages * size, FreeBytes: free * size}
	if h.d.DBPath != "" {
		if fi, err := os.Stat(h.d.DBPath + "-wal"); err == nil {
			st.WALBytes = fi.Size()
		}
	}
	return st
}

func (h *Handler) cacheStatus(ctx context.Context) CacheStatus {
	st := CacheStatus{Backend: h.d.Runtime.CacheBackend}
	if st.Backend == "" {
		st.Backend = "memory"
	}
	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	t0 := time.Now()
	// The ping error text is never returned: it may name internal hosts.
	if err := h.d.Cache.Ping(pctx); err == nil {
		ms := float64(time.Since(t0).Microseconds()) / 1000
		st.OK, st.LatencyMs = true, &ms
	}
	return st
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := h.d.Now()
	ov := Overview{GeneratedAt: now.UTC(), Build: buildinfo.Get(), StartedAt: h.d.StartedAt.UTC(), UptimeS: int64(now.Sub(h.d.StartedAt).Seconds()),
		Database: h.dbStatus(ctx), Cache: h.cacheStatus(ctx), Providers: h.d.Meter.Snapshot().Providers, Config: h.d.Runtime}
	if ov.Providers == nil {
		ov.Providers = []apimeter.ProviderStats{}
	}
	mode, err := h.d.Store.RegistrationMode(ctx)
	if err != nil {
		h.internal(w, r, "Lecture des réglages", err)
		return
	}
	ov.RegistrationMode = mode
	ov.Jobs.Groups = []queue.GroupStats{}
	if h.d.Domain != nil {
		jobs := h.d.Domain.Jobs()
		ov.Jobs.Groups = jobs.Stats()
		for _, g := range ov.Jobs.Groups {
			ov.Jobs.Active += g.Running + g.Queued + g.Delayed
		}
		if ov.Jobs.Failed24h, err = jobs.CountFailedSince(ctx, now.Add(-24*time.Hour)); err != nil {
			h.internal(w, r, "Comptage des tâches", err)
			return
		}
		if ov.Counts, err = h.d.Domain.AdminCounts(ctx); err != nil {
			h.internal(w, r, "Comptages de l'instance", err)
			return
		}
	}
	ov.Streams.Clients, ov.Streams.Users = h.d.SSE.Stats()
	if h.d.Logs != nil {
		if ov.Errors24h, err = h.d.Logs.CountErrorsSince(ctx, now.Add(-24*time.Hour)); err != nil {
			h.internal(w, r, "Comptage des erreurs", err)
			return
		}
		ov.LogsDropped = h.d.Logs.Dropped()
	}
	httpx.JSON(w, http.StatusOK, ov)
}

// ---------- users ----------

// otherUser parses {id} and refuses the caller's own account (409).
func (h *Handler) otherUser(w http.ResponseWriter, r *http.Request, msg string) (int64, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return 0, false
	}
	if id == admin(r).ID {
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, msg)
		return 0, false
	}
	if _, err := h.d.Store.GetUser(r.Context(), id); errors.Is(err, store.ErrUnknownUser) {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Utilisateur introuvable.")
		return 0, false
	} else if err != nil {
		h.internal(w, r, "Lecture d'un utilisateur", err)
		return 0, false
	}
	return id, true
}

func (h *Handler) revokeSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := h.otherUser(w, r, "Pour fermer vos propres sessions, utilisez la déconnexion.")
	if !ok {
		return
	}
	n, err := h.d.Store.DeleteUserSessions(r.Context(), id)
	if err != nil {
		h.internal(w, r, "Révocation des sessions", err)
		return
	}
	h.d.Log.Info("🔒 Sessions révoquées par un administrateur", "adminId", admin(r).ID, "userId", id, "sessions", n)
	httpx.JSON(w, http.StatusOK, map[string]int64{"revoked": n})
}

func (h *Handler) syncUser(w http.ResponseWriter, r *http.Request) {
	if h.d.Domain == nil {
		h.unavailable(w)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	job, err := h.d.Domain.AdminSyncNow(r.Context(), id)
	if err != nil {
		h.domainFail(w, r, "Synchronisation d'un utilisateur", err)
		return
	}
	h.d.Log.Info("🔄 Synchronisation lancée par un administrateur", "adminId", admin(r).ID, "userId", id, "job", job.ID)
	httpx.JSON(w, http.StatusAccepted, h.adminJob(*job, nil))
}

func (h *Handler) domainFail(w http.ResponseWriter, r *http.Request, what string, err error) {
	var ce *domain.ConflictError
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, queue.ErrNotFound):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Ressource introuvable.")
	case errors.Is(err, domain.ErrNotConnected):
		httpx.Fail(w, http.StatusPreconditionFailed, httpx.CodeNotConnected, "Aucun fournisseur connecté pour ce compte.")
	case errors.As(err, &ce):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, ce.Msg)
	case errors.Is(err, queue.ErrNotReplayable):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Seules les synchronisations en échec peuvent être relancées d'ici.")
	case errors.Is(err, queue.ErrNotCancellable):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Cette tâche est déjà terminée.")
	default:
		h.internal(w, r, what, err)
	}
}

// ---------- jobs ----------

var jobIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (h *Handler) names(ctx context.Context) map[int64]string {
	out := map[int64]string{}
	if users, err := h.d.Store.ListUsers(ctx); err == nil {
		for _, u := range users {
			out[u.ID] = u.DisplayName
		}
	}
	return out
}

func (h *Handler) adminJob(j queue.JobStatus, names map[int64]string) AdminJob {
	j.Result = nil
	name := names[j.UserID]
	if names == nil {
		if u, err := h.d.Store.GetUser(context.Background(), j.UserID); err == nil {
			name = u.DisplayName
		}
	}
	return AdminJob{JobStatus: j, UserName: name,
		Retryable:   j.State == queue.StateFailed && queue.Replayable(j.Kind),
		Cancellable: j.State == queue.StateQueued || j.State == queue.StateRunning || j.State == queue.StateRetrying}
}

func (h *Handler) listJobs(w http.ResponseWriter, r *http.Request) {
	if h.d.Domain == nil {
		h.unavailable(w)
		return
	}
	state := r.URL.Query().Get("state")
	switch state {
	case "", "all":
		state = ""
	case "active", queue.StateFailed:
	default:
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Filtre inconnu (active, failed ou all).")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs := h.d.Domain.Jobs()
	items, err := jobs.ListAll(r.Context(), queue.AdminFilter{State: state, Limit: limit})
	if err != nil {
		h.internal(w, r, "Liste des tâches", err)
		return
	}
	names := h.names(r.Context())
	out := make([]AdminJob, 0, len(items))
	for _, j := range items {
		out = append(out, h.adminJob(j, names))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "groups": jobs.Stats()})
}

func (h *Handler) jobAction(w http.ResponseWriter, r *http.Request, what string, fn func(context.Context, string) (*queue.JobStatus, error), status int) {
	if h.d.Domain == nil {
		h.unavailable(w)
		return
	}
	id := r.PathValue("id")
	if !jobIDRe.MatchString(id) {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Identifiant de tâche invalide.")
		return
	}
	job, err := fn(r.Context(), id)
	if err != nil {
		h.domainFail(w, r, what, err)
		return
	}
	h.d.Log.Info("⚙️ "+what+" par un administrateur", "adminId", admin(r).ID, "job", id, "userId", job.UserID)
	httpx.JSON(w, status, h.adminJob(*job, nil))
}

func (h *Handler) retryJob(w http.ResponseWriter, r *http.Request) {
	var fn func(context.Context, string) (*queue.JobStatus, error)
	if h.d.Domain != nil {
		fn = h.d.Domain.AdminRetryJob
	}
	h.jobAction(w, r, "Tâche relancée", fn, http.StatusAccepted)
}

func (h *Handler) cancelJob(w http.ResponseWriter, r *http.Request) {
	var fn func(context.Context, string) (*queue.JobStatus, error)
	if h.d.Domain != nil {
		fn = h.d.Domain.Jobs().Cancel
	}
	h.jobAction(w, r, "Tâche annulée", fn, http.StatusOK)
}

// ---------- share links ----------

func (h *Handler) listShares(w http.ResponseWriter, r *http.Request) {
	if h.d.Domain == nil {
		h.unavailable(w)
		return
	}
	all := false
	switch r.URL.Query().Get("state") {
	case "", "active":
	case "all":
		all = true
	default:
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Filtre inconnu (active ou all).")
		return
	}
	links, err := h.d.Domain.AdminShareLinks(r.Context(), all)
	if err != nil {
		h.internal(w, r, "Liste des liens de partage", err)
		return
	}
	httpx.JSON(w, http.StatusOK, links)
}

func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	if h.d.Domain == nil {
		h.unavailable(w)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	owner, err := h.d.Domain.AdminRevokeShareLink(r.Context(), id)
	if err != nil {
		h.domainFail(w, r, "Révocation d'un lien de partage", err)
		return
	}
	h.d.Log.Info("🔗 Lien de partage révoqué par un administrateur", "adminId", admin(r).ID, "lien", id, "userId", owner)
	w.WriteHeader(http.StatusNoContent)
}

// ---------- maintenance ----------

func (h *Handler) purge(w http.ResponseWriter, r *http.Request) {
	if h.d.Maintenance == nil {
		h.unavailable(w)
		return
	}
	removed, err := h.d.Maintenance(r.Context())
	if err != nil {
		h.internal(w, r, "Purge des données expirées", err)
		return
	}
	h.d.Log.Info("🧹 Purge lancée par un administrateur", "adminId", admin(r).ID)
	httpx.JSON(w, http.StatusOK, map[string]any{"removed": removed})
}

func (h *Handler) optimize(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Vacuum bool `json:"vacuum"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(w, r, &body, 1<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
			return
		}
	}
	ctx := r.Context()
	before := h.dbStatus(ctx)
	t0 := time.Now()
	stmts := []string{`PRAGMA wal_checkpoint(TRUNCATE)`, `PRAGMA optimize`}
	if body.Vacuum {
		stmts = append(stmts, `VACUUM`, `PRAGMA wal_checkpoint(TRUNCATE)`)
	}
	for _, q := range stmts {
		if _, err := h.d.Store.DB.ExecContext(ctx, q); err != nil {
			h.internal(w, r, "Optimisation de la base", err)
			return
		}
	}
	after := h.dbStatus(ctx)
	h.d.Log.Info("🗜️ Base optimisée par un administrateur", "adminId", admin(r).ID, "vacuum", body.Vacuum,
		"avant", before.SizeBytes+before.WALBytes, "après", after.SizeBytes+after.WALBytes)
	httpx.JSON(w, http.StatusOK, map[string]any{"vacuum": body.Vacuum, "before": before, "after": after, "durationMs": time.Since(t0).Milliseconds()})
}

const maxProblems = 50

func (h *Handler) check(w http.ResponseWriter, r *http.Request) {
	t0 := time.Now()
	problems, err := database.CheckIntegrity(r.Context(), h.d.Store.DB)
	if err != nil {
		h.internal(w, r, "Contrôle d'intégrité", err)
		return
	}
	total := len(problems)
	if total > maxProblems {
		problems = problems[:maxProblems]
	}
	if problems == nil {
		problems = []string{}
	}
	h.d.Log.Info("🩺 Contrôle d'intégrité lancé par un administrateur", "adminId", admin(r).ID, "problèmes", total)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": total == 0, "problems": problems, "total": total, "durationMs": time.Since(t0).Milliseconds()})
}
