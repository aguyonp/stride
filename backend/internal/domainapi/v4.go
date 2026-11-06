package domainapi

import (
	"errors"
	"net/http"
	"strconv"

	"stride/internal/analysis"
	"stride/internal/domain"
	"stride/internal/httpx"
)

func registerV4(mux *http.ServeMux, user func(string, http.HandlerFunc), a *api) {
	// Cycles engine
	user("GET /api/cycles", a.cycles)
	user("POST /api/cycles/plans", a.createPlan)
	user("PATCH /api/cycles/plans/{id}", a.patchPlan)
	user("DELETE /api/cycles/plans/{id}", a.deletePlan)
	user("POST /api/cycles/phases", a.createPhase)
	user("PATCH /api/cycles/phases/{id}", a.patchPhase)
	user("DELETE /api/cycles/phases/{id}", a.deletePhase)
	user("PUT /api/cycles/weeks", a.putWeek)

	// Body stress map & correlations
	user("GET /api/bodymap", a.bodyMap)
	user("POST /api/bodymap/entries", a.createBodyEntry)
	user("PATCH /api/bodymap/entries/{id}", a.patchBodyEntry)
	user("DELETE /api/bodymap/entries/{id}", a.deleteBodyEntry)

	// Secure read-only share links
	user("GET /api/share", a.listShareLinks)
	user("POST /api/share", a.createShareLink)
	user("DELETE /api/share/{id}", a.deleteShareLink)
	mux.HandleFunc("GET /api/public/share/{token}", a.publicShare)

	// Async jobs
	user("GET /api/jobs", a.listJobs)
	user("GET /api/jobs/{id}", a.getJob)
}

// ---------- Cycles ----------

func (a *api) cycles(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var planID *int64
		if raw := r.URL.Query().Get("planId"); raw != "" {
			if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
				planID = &id
			}
		}
		v, err := a.s.Cycles(r.Context(), uid, planID)
		a.reply(w, r, v, err)
	})
}

func (a *api) createPlan(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var in domain.TrainingPlanInput
		if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.CreatePlan(r.Context(), uid, in)
		a.created(w, r, v, err)
	})
}

func (a *api) patchPlan(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		var in domain.TrainingPlanInput
		if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.PatchPlan(r.Context(), uid, id, in)
		a.reply(w, r, v, err)
	})
}

func (a *api) deletePlan(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		a.noContent(w, r, a.s.DeletePlan(r.Context(), uid, id))
	})
}

func (a *api) createPhase(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var in domain.TrainingPhaseInput
		if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.CreatePhase(r.Context(), uid, in)
		a.created(w, r, v, err)
	})
}

func (a *api) patchPhase(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		var in domain.TrainingPhaseInput
		if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.PatchPhase(r.Context(), uid, id, in)
		a.reply(w, r, v, err)
	})
}

func (a *api) deletePhase(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		a.noContent(w, r, a.s.DeletePhase(r.Context(), uid, id))
	})
}

func (a *api) putWeek(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var in domain.TrainingWeekInput
		if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.PutWeek(r.Context(), uid, in)
		a.reply(w, r, v, err)
	})
}

// ---------- Body Map ----------

func (a *api) bodyMap(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		days := 90
		if raw := r.URL.Query().Get("days"); raw != "" {
			if d, err := strconv.Atoi(raw); err == nil && d > 0 {
				days = d
			}
		}
		zone := r.URL.Query().Get("zone")
		v, err := a.s.BodyMap(r.Context(), uid, days, zone)
		a.reply(w, r, v, err)
	})
}

func (a *api) createBodyEntry(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var in analysis.BodyStressInput
		if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.CreateBodyStressEntry(r.Context(), uid, in)
		a.created(w, r, v, err)
	})
}

func (a *api) patchBodyEntry(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		var in analysis.BodyStressInput
		if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.PatchBodyStressEntry(r.Context(), uid, id, in)
		a.reply(w, r, v, err)
	})
}

func (a *api) deleteBodyEntry(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		a.noContent(w, r, a.s.DeleteBodyStressEntry(r.Context(), uid, id))
	})
}

// ---------- Share Links ----------

func (a *api) listShareLinks(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.ListShareLinks(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

func (a *api) createShareLink(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var in domain.ShareLinkInput
		if err := httpx.Decode(w, r, &in, 4<<10); err != nil {
			badRequest(w, "Corps JSON invalide.")
			return
		}
		v, err := a.s.CreateShareLink(r.Context(), uid, in)
		a.created(w, r, v, err)
	})
}

func (a *api) deleteShareLink(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		a.noContent(w, r, a.s.DeleteShareLink(r.Context(), uid, id))
	})
}

func (a *api) publicShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	v, err := a.s.GetPublicShare(r.Context(), token)
	if err != nil {
		if errors.Is(err, domain.ErrGone) {
			httpx.Fail(w, http.StatusGone, "GONE", "Ce lien de partage a expiré.")
			return
		}
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Lien de partage introuvable ou révoqué.")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.JSON(w, http.StatusOK, v)
}

// ---------- Jobs ----------

func (a *api) listJobs(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		limit := 20
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if l, err := strconv.Atoi(raw); err == nil && l > 0 {
				limit = l
			}
		}
		v, err := a.s.ListJobs(r.Context(), uid, limit)
		a.reply(w, r, v, err)
	})
}

func (a *api) getJob(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		id := r.PathValue("id")
		v, err := a.s.GetJob(r.Context(), uid, id)
		a.reply(w, r, v, err)
	})
}
