// Package domainapi exposes the domain services over HTTP.
package domainapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"stride/internal/athlete"
	"stride/internal/auth"
	"stride/internal/domain"
	"stride/internal/httpx"
	"stride/internal/provider/strava"
)

type Deps struct {
	Services     *domain.Services
	RequireUser  func(http.Handler) http.Handler
	RequireAdmin func(http.Handler) http.Handler
	Log          *slog.Logger
}

type api struct {
	s   *domain.Services
	log *slog.Logger
}

// Register mounts every domain route on mux (Go 1.22 patterns, e.g. "GET /api/activities/{id}").
func Register(mux *http.ServeMux, d Deps) {
	a := &api{s: d.Services, log: d.Log}
	if a.log == nil {
		a.log = slog.Default()
	}
	user := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, d.RequireUser(h)) }
	admin := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, d.RequireUser(d.RequireAdmin(h))) }

	user("GET /api/integrations", a.integrations)
	user("PUT /api/integrations/garmin", a.putGarmin)
	user("POST /api/integrations/garmin/test", a.testGarmin)
	user("DELETE /api/integrations/garmin", a.deleteGarmin)
	user("GET /api/integrations/strava/connect", a.stravaConnect)
	user("GET /api/integrations/strava/callback", a.stravaCallback)
	user("DELETE /api/integrations/strava", a.deleteStrava)
	mux.HandleFunc("GET /api/webhooks/strava", a.webhookVerify)
	mux.HandleFunc("POST /api/webhooks/strava", a.webhookEvent)

	user("POST /api/sync", a.sync)
	user("GET /api/sync/status", a.syncStatus)

	user("GET /api/activities", a.listActivities)
	user("GET /api/activities/{id}", a.activity)
	user("GET /api/activities/{id}/report", a.activityReport)
	user("PUT /api/activities/{id}/notes", a.putNotes)
	user("GET /api/days/{date}", a.day)
	user("GET /api/days/{date}/report", a.dayReport)
	user("PUT /api/days/{date}/checkin", a.putCheckin)

	user("GET /api/profile", a.getProfile)
	user("PUT /api/profile", a.putProfile)
	user("GET /api/profile/presets", a.presets)
	user("GET /api/goals", a.listGoals)
	user("POST /api/goals", a.createGoal)
	user("PATCH /api/goals/{id}", a.patchGoal)
	user("DELETE /api/goals/{id}", a.deleteGoal)
	user("GET /api/plan", a.listPlan)
	user("POST /api/plan", a.createPlanned)
	user("GET /api/plan/week", a.weekPlan)
	user("PATCH /api/plan/{id}", a.patchPlanned)
	user("DELETE /api/plan/{id}", a.deletePlanned)
	user("GET /api/shoes", a.listShoes)
	user("POST /api/shoes", a.createShoe)
	user("POST /api/shoes/import-strava", a.importShoes)
	user("PATCH /api/shoes/{id}", a.patchShoe)
	user("DELETE /api/shoes/{id}", a.deleteShoe)

	user("POST /api/exports/multi", a.exportMulti)
	user("GET /api/backup", a.backup)
	user("POST /api/backup/restore", a.restore)

	registerV3(user, a)

	admin("GET /api/admin/connections", a.adminConnections)
	admin("POST /api/admin/users/{id}/resync", a.adminResync)
	admin("GET /api/admin/users/{id}/backup", a.adminBackup)
	admin("POST /api/admin/users/{id}/restore", a.adminRestore)
}

// ---------- helpers ----------

func uid(r *http.Request) (int64, bool) {
	u, ok := auth.UserFrom(r.Context())
	return u.ID, ok && u.ID > 0
}

// withUser runs h with the authenticated user id (never taken from the request).
func (a *api) withUser(w http.ResponseWriter, r *http.Request, h func(id int64)) {
	id, ok := uid(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "Authentification requise.")
		return
	}
	h(id)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		httpx.Fail(w, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "Corps de requête trop volumineux.")
		return nil, false
	}
	return b, true
}

func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	var br *domain.BadRequestError
	var ce *domain.ConflictError
	var pe *domain.ProviderError
	switch {
	case errors.Is(err, context.Canceled) && r.Context().Err() != nil:
		return
	case errors.As(err, &br):
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, br.Msg)
	case errors.Is(err, domain.ErrNotFound) || errors.Is(err, athlete.ErrNotFound):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Ressource introuvable.")
	case errors.As(err, &ce):
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, ce.Msg)
	case errors.Is(err, domain.ErrNotConnected):
		httpx.Fail(w, http.StatusPreconditionFailed, httpx.CodeNotConnected, "Fournisseur non connecté : configurez-le dans Réglages › Intégrations.")
	case errors.Is(err, domain.ErrStravaDisabled):
		httpx.Fail(w, http.StatusPreconditionFailed, httpx.CodeNotConnected, "Strava n'est pas configuré sur ce serveur.")
	case errors.As(err, &pe):
		switch {
		case pe.Unauthorized:
			httpx.Fail(w, http.StatusBadGateway, httpx.CodeProviderAuth, pe.Message())
		case pe.RateLimited:
			if !pe.CooldownUntil.IsZero() {
				w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(pe.CooldownUntil).Seconds())+1))
			}
			httpx.Fail(w, http.StatusServiceUnavailable, httpx.CodeRateLimited, pe.Message())
		case pe.Timeout:
			httpx.Fail(w, http.StatusGatewayTimeout, httpx.CodeProviderDown, pe.Message())
		default:
			httpx.Fail(w, http.StatusBadGateway, httpx.CodeProviderDown, pe.Message())
		}
	default:
		a.log.Error(fmt.Sprintf("❌ %s %s", r.Method, r.Pattern), "err", err.Error())
		httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
	}
}

func (a *api) reply(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (a *api) created(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

func (a *api) noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func notFound(w http.ResponseWriter) {
	httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Ressource introuvable.")
}

func badRequest(w http.ResponseWriter, msg string) {
	httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, msg)
}

// ---------- integrations ----------

func (a *api) integrations(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		v, err := a.s.Integrations(r.Context(), id)
		a.reply(w, r, v, err)
	})
}

func (a *api) putGarmin(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		var body struct {
			APIKey string `json:"apiKey"`
		}
		if err := httpx.Decode(w, r, &body, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({\"apiKey\": \"…\"} attendu).")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		v, err := a.s.ConnectGarmin(ctx, id, body.APIKey)
		var pe *domain.ProviderError
		if errors.As(err, &pe) && pe.Unauthorized {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeProviderAuth, "Clé API refusée par Garmin.")
			return
		}
		a.reply(w, r, v, err)
	})
}

func (a *api) testGarmin(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		v, err := a.s.TestGarmin(ctx, id)
		a.reply(w, r, v, err)
	})
}

func (a *api) deleteGarmin(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		v, err := a.s.DisconnectGarmin(r.Context(), id)
		a.reply(w, r, v, err)
	})
}

func withQuery(path string, kv ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + q.Encode()
}

func (a *api) stravaConnect(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		ret := domain.SafeReturnPath(r.URL.Query().Get("return"))
		u, err := a.s.StravaConnectURL(r.Context(), id, ret)
		if err != nil {
			http.Redirect(w, r, withQuery(ret, "strava", "error", "reason", "disabled"), http.StatusFound)
			return
		}
		http.Redirect(w, r, u, http.StatusFound)
	})
}

func (a *api) stravaCallback(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		q := r.URL.Query()
		ret, reason := a.s.StravaCallback(r.Context(), id, q.Get("state"), q.Get("code"), q.Get("scope"), q.Get("error"))
		if reason != "" {
			http.Redirect(w, r, withQuery(ret, "strava", "error", "reason", reason), http.StatusFound)
			return
		}
		http.Redirect(w, r, withQuery(ret, "strava", "connected"), http.StatusFound)
	})
}

func (a *api) deleteStrava(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		v, err := a.s.DisconnectStrava(r.Context(), id)
		a.reply(w, r, v, err)
	})
}

func (a *api) webhookVerify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ch, ok := a.s.VerifyStravaWebhook(q.Get("hub.mode"), q.Get("hub.verify_token"), q.Get("hub.challenge"))
	if !ok {
		httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "Jeton de vérification invalide.")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"hub.challenge": ch})
}

func (a *api) webhookEvent(w http.ResponseWriter, r *http.Request) {
	var ev strava.WebhookEvent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&ev); err != nil {
		badRequest(w, "Événement illisible.")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	if ev.OwnerID != 0 {
		a.s.HandleStravaEvent(ev)
	}
}

// ---------- sync & activities ----------

func (a *api) sync(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		v, err := a.s.Sync(r.Context(), id)
		a.reply(w, r, v, err)
	})
}

func (a *api) syncStatus(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		v, err := a.s.SyncStatus(r.Context(), id)
		a.reply(w, r, v, err)
	})
}

var sports = map[string]bool{"run": true, "bike": true, "walk": true, "hike": true, "swim": true, "strength": true, "other": true}

func (a *api) listActivities(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(id int64) {
		q := r.URL.Query()
		limit := 30
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 100 {
				badRequest(w, "limit doit être un entier entre 1 et 100.")
				return
			}
			limit = n
		}
		before, from, to, sport := q.Get("before"), q.Get("from"), q.Get("to"), q.Get("sport")
		if len(before) > 19 || (from != "" && !athlete.ValidDate(from)) || (to != "" && !athlete.ValidDate(to)) {
			badRequest(w, "Paramètres before (AAAA-MM-JJTHH:MM:SS), from et to (AAAA-MM-JJ) invalides.")
			return
		}
		if sport != "" && !sports[sport] {
			badRequest(w, "sport invalide (run, bike, walk, hike, swim, strength, other).")
			return
		}
		v, err := a.s.ListActivities(r.Context(), id, limit, before, from, to, sport)
		a.reply(w, r, v, err)
	})
}

func (a *api) activity(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		aid, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		v, err := a.s.ActivityView(r.Context(), uid, aid)
		a.reply(w, r, v, err)
	})
}

func (a *api) activityReport(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		aid, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		md, err := a.s.ActivityReport(r.Context(), uid, aid, r.URL.Query().Get("include"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		httpx.Text(w, "text/markdown; charset=utf-8", md)
	})
}

func (a *api) putNotes(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		aid, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		body, ok := readBody(w, r, 64<<10)
		if !ok {
			return
		}
		v, err := a.s.PutNotes(r.Context(), uid, aid, body)
		a.reply(w, r, v, err)
	})
}

func (a *api) date(w http.ResponseWriter, r *http.Request, uid int64) (string, bool) {
	d, err := a.s.ResolveDate(r.Context(), uid, r.PathValue("date"))
	if err != nil {
		a.fail(w, r, err)
		return "", false
	}
	return d, true
}

func (a *api) day(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if d, ok := a.date(w, r, uid); ok {
			v, err := a.s.DayView(r.Context(), uid, d)
			a.reply(w, r, v, err)
		}
	})
}

func (a *api) dayReport(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		d, ok := a.date(w, r, uid)
		if !ok {
			return
		}
		md, err := a.s.DayReport(r.Context(), uid, d, r.URL.Query().Get("include"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		httpx.Text(w, "text/markdown; charset=utf-8", md)
	})
}

func (a *api) putCheckin(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		d, ok := a.date(w, r, uid)
		if !ok {
			return
		}
		body, ok := readBody(w, r, 16<<10)
		if !ok {
			return
		}
		v, err := a.s.PutCheckin(r.Context(), uid, d, body)
		a.reply(w, r, v, err)
	})
}

// ---------- athlete data ----------

func (a *api) getProfile(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.Profile(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

func (a *api) putProfile(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		body, ok := readBody(w, r, 64<<10)
		if !ok {
			return
		}
		v, err := a.s.PutProfile(r.Context(), uid, body)
		a.reply(w, r, v, err)
	})
}

func (a *api) presets(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(int64) { httpx.JSON(w, http.StatusOK, a.s.Presets()) })
}

func (a *api) listGoals(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.Goals(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

func (a *api) createGoal(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if body, ok := readBody(w, r, 64<<10); ok {
			v, err := a.s.CreateGoal(r.Context(), uid, body)
			a.created(w, r, v, err)
		}
	})
}

func (a *api) patchGoal(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		id, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		if body, ok := readBody(w, r, 64<<10); ok {
			v, err := a.s.PatchGoal(r.Context(), uid, id, body)
			a.reply(w, r, v, err)
		}
	})
}

func (a *api) deleteGoal(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		id, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		a.noContent(w, r, a.s.DeleteGoal(r.Context(), uid, id))
	})
}

func (a *api) listPlan(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		if (from != "" && !athlete.ValidDate(from)) || (to != "" && !athlete.ValidDate(to)) {
			badRequest(w, "Dates from/to attendues au format AAAA-MM-JJ.")
			return
		}
		v, err := a.s.Plan(r.Context(), uid, from, to)
		a.reply(w, r, v, err)
	})
}

func (a *api) createPlanned(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if body, ok := readBody(w, r, 64<<10); ok {
			v, err := a.s.CreatePlanned(r.Context(), uid, body)
			a.created(w, r, v, err)
		}
	})
}

func (a *api) patchPlanned(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		id, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		if body, ok := readBody(w, r, 64<<10); ok {
			v, err := a.s.PatchPlanned(r.Context(), uid, id, body)
			a.reply(w, r, v, err)
		}
	})
}

func (a *api) deletePlanned(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		id, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		a.noContent(w, r, a.s.DeletePlanned(r.Context(), uid, id))
	})
}

func (a *api) weekPlan(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		d := r.URL.Query().Get("date")
		if d != "" && !athlete.ValidDate(d) {
			badRequest(w, "date attendue au format AAAA-MM-JJ.")
			return
		}
		v, err := a.s.WeekPlan(r.Context(), uid, d)
		a.reply(w, r, v, err)
	})
}

func (a *api) listShoes(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.Shoes(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

func (a *api) createShoe(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if body, ok := readBody(w, r, 64<<10); ok {
			v, err := a.s.CreateShoe(r.Context(), uid, body)
			a.created(w, r, v, err)
		}
	})
}

func (a *api) patchShoe(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		id, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		if body, ok := readBody(w, r, 64<<10); ok {
			v, err := a.s.PatchShoe(r.Context(), uid, id, body)
			a.reply(w, r, v, err)
		}
	})
}

func (a *api) deleteShoe(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		id, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		a.noContent(w, r, a.s.DeleteShoe(r.Context(), uid, id))
	})
}

func (a *api) importShoes(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.ImportStravaShoes(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

// ---------- exports & backup ----------

func (a *api) exportMulti(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			ActivityIDs []int64  `json:"activityIds"`
			Weeks       int      `json:"weeks"`
			Include     []string `json:"include"`
		}
		if err := httpx.Decode(w, r, &body, 64<<10); err != nil && !errors.Is(err, io.EOF) {
			badRequest(w, "Corps JSON invalide ({activityIds?, weeks?, include?} attendu).")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		v, err := a.s.MultiExport(ctx, uid, body.ActivityIDs, body.Weeks, body.Include)
		a.reply(w, r, v, err)
	})
}

func (a *api) writeBackup(w http.ResponseWriter, r *http.Request, uid int64) {
	b, err := a.s.Backup(r.Context(), uid)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="stride-backup-%s.json"`, time.Now().Format("2006-01-02")))
	httpx.JSON(w, http.StatusOK, b)
}

func (a *api) restoreFor(w http.ResponseWriter, r *http.Request, uid int64) {
	body, ok := readBody(w, r, 16<<20)
	if !ok {
		return
	}
	counts, err := a.s.Restore(r.Context(), uid, body, r.URL.Query().Get("mode"))
	a.reply(w, r, map[string]any{"imported": counts}, err)
}

func (a *api) backup(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) { a.writeBackup(w, r, uid) })
}

func (a *api) restore(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) { a.restoreFor(w, r, uid) })
}

// ---------- admin ----------

func (a *api) targetUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathID(r)
	if !ok {
		notFound(w)
		return 0, false
	}
	exists, err := a.s.UserExists(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return 0, false
	}
	if !exists {
		notFound(w)
		return 0, false
	}
	return id, true
}

func (a *api) adminConnections(w http.ResponseWriter, r *http.Request) {
	v, err := a.s.AdminConnections(r.Context())
	a.reply(w, r, v, err)
}

func (a *api) adminResync(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.targetUser(w, r); ok {
		if err := a.s.Resync(r.Context(), id); err != nil {
			a.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusAccepted, map[string]bool{"queued": true})
	}
}

func (a *api) adminBackup(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.targetUser(w, r); ok {
		a.writeBackup(w, r, id)
	}
}

func (a *api) adminRestore(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.targetUser(w, r); ok {
		a.restoreFor(w, r, id)
	}
}
