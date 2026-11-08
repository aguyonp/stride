package domainapi

import (
	"net/http"
	"strconv"

	"stride/internal/domain"
	"stride/internal/httpx"
)

func (a *api) healthStatus(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.HealthStatus(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

// healthConditions serves the static pathology catalog (zones in canonical order, §14.7).
func (a *api) healthConditions(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		a.reply(w, r, a.s.HealthConditions(r.Context(), uid), nil)
	})
}

func (a *api) putHealthStatus(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var in domain.HealthStatusInput
		if err := httpx.Decode(w, r, &in, 8<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({mode, zones} attendu).")
			return
		}
		v, err := a.s.PutHealthStatus(r.Context(), uid, in)
		a.reply(w, r, v, err)
	})
}

func (a *api) syncHistory(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var in domain.HistorySyncInput
		if err := httpx.Decode(w, r, &in, 1<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({from} attendu).")
			return
		}
		v, err := a.s.SyncHistory(r.Context(), uid, in)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusAccepted, v)
	})
}

func (a *api) search(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				badRequest(w, "limit doit être un entier positif.")
				return
			}
			limit = n
		}
		v, err := a.s.Search(r.Context(), uid, r.URL.Query().Get("q"), limit)
		a.reply(w, r, v, err)
	})
}
