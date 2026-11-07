package domainapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"stride/internal/domain"
	"stride/internal/httpx"
)

func registerV31(user func(string, http.HandlerFunc), a *api) {
	user("GET /api/records", a.records)
	user("PUT /api/records/{key}", a.putRecord)
	user("DELETE /api/records/{key}", a.deleteRecord)

	user("POST /api/activities", a.createActivity)
	user("PATCH /api/activities/{id}", a.patchActivity)
	user("DELETE /api/activities/{id}", a.deleteActivity)

	user("GET /api/calendar/events", a.listEvents)
	user("POST /api/calendar/events", a.createEvent)
	user("PATCH /api/calendar/events/{id}", a.patchEvent)
	user("DELETE /api/calendar/events/{id}", a.deleteEvent)

	user("POST /api/plan/{id}/shift", a.shiftPlanned)
	user("POST /api/plan/adapt", a.adaptPlan)
	user("POST /api/plan/replace", a.replacePlan)
	user("GET /api/plan/phases", a.phases)

	user("GET /api/integrations/strava/stats", a.stravaStats)
}

// ---------- records ----------

func (a *api) records(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		v, err := a.s.Records(ctx, uid)
		a.reply(w, r, v, err)
	})
}

func (a *api) putRecord(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body domain.RecordInput
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({timeS?, distanceM?, date?} attendu).")
			return
		}
		v, err := a.s.PutRecord(r.Context(), uid, r.PathValue("key"), body)
		a.reply(w, r, v, err)
	})
}

func (a *api) deleteRecord(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		a.noContent(w, r, a.s.DeleteRecord(r.Context(), uid, r.PathValue("key")))
	})
}

// ---------- manual activities ----------

func (a *api) createActivity(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if body, ok := readBody(w, r, 16<<10); ok {
			v, err := a.s.CreateManualActivity(r.Context(), uid, body)
			a.created(w, r, v, err)
		}
	})
}

func (a *api) patchActivity(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		if body, ok := readBody(w, r, 16<<10); ok {
			v, err := a.s.PatchManualActivity(r.Context(), uid, id, body)
			a.reply(w, r, v, err)
		}
	})
}

func (a *api) deleteActivity(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		a.noContent(w, r, a.s.DeleteManualActivity(r.Context(), uid, id))
	})
}

// ---------- calendar events ----------

func (a *api) listEvents(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.Events(r.Context(), uid, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		a.reply(w, r, v, err)
	})
}

func (a *api) createEvent(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if body, ok := readBody(w, r, 16<<10); ok {
			v, err := a.s.CreateEvent(r.Context(), uid, body)
			a.created(w, r, v, err)
		}
	})
}

func (a *api) patchEvent(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		if body, ok := readBody(w, r, 16<<10); ok {
			v, err := a.s.PatchEvent(r.Context(), uid, id, body)
			a.reply(w, r, v, err)
		}
	})
}

func (a *api) deleteEvent(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		a.noContent(w, r, a.s.DeleteEvent(r.Context(), uid, id))
	})
}

// ---------- plan adaptation ----------

func (a *api) shiftPlanned(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		var body domain.ShiftInput
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({days} ou {toDate} attendu).")
			return
		}
		v, err := a.s.ShiftPlanned(r.Context(), uid, id, body)
		a.reply(w, r, v, err)
	})
}

func (a *api) adaptPlan(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body domain.AdaptInput
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({from, mode, eventId?} attendu).")
			return
		}
		v, err := a.s.AdaptPlan(r.Context(), uid, body, isAsync(r))
		a.replyJob(w, r, v, err)
	})
}

func (a *api) replacePlan(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			RemoveIDs []int64           `json:"removeIds"`
			Sessions  []json.RawMessage `json:"sessions"`
		}
		if err := httpx.Decode(w, r, &body, 1<<20); err != nil {
			badRequest(w, "Corps JSON invalide ({removeIds, sessions} attendu).")
			return
		}
		v, err := a.s.ReplacePlan(r.Context(), uid, body.RemoveIDs, body.Sessions)
		a.reply(w, r, v, err)
	})
}

func (a *api) phases(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.Phases(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

func (a *api) stravaStats(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		v, err := a.s.StravaStats(ctx, uid)
		a.reply(w, r, v, err)
	})
}
