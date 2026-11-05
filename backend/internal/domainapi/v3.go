package domainapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"stride/internal/domain"
	"stride/internal/httpx"
)

func registerV3(user func(string, http.HandlerFunc), a *api) {
	user("POST /api/shoes/{id}/merge", a.mergeShoe)
	user("GET /api/activities/{id}/track", a.track)
	user("GET /api/trends/efficiency", a.efficiencyTrend)

	user("GET /api/llm", a.getLLM)
	user("PUT /api/llm", a.putLLM)
	user("DELETE /api/llm", a.deleteLLM)
	user("POST /api/llm/test", a.testLLM)
	user("POST /api/llm/analyze", a.analyze)
	user("GET /api/llm/analyses", a.analyses)

	user("POST /api/plan/generate", a.planGenerate)
	user("POST /api/plan/import", a.planImport)
	user("POST /api/plan/bulk", a.planBulk)
	user("POST /api/plan/{id}/push-garmin", a.pushGarmin)
	user("GET /api/plan/{id}/workout.fit", a.workoutFIT)
	user("GET /api/calendar", a.calendar)

	user("GET /api/notifications", a.listNotifications)
	user("POST /api/notifications", a.createNotification)
	user("PATCH /api/notifications/{id}", a.patchNotification)
	user("DELETE /api/notifications/{id}", a.deleteNotification)
	user("POST /api/notifications/{id}/test", a.testNotification)

	user("POST /api/imports", a.importFile)
}

// v3Fail handles the V3-specific errors; false when err must go through fail.
func (a *api) v3Fail(w http.ResponseWriter, err error) bool {
	var pu *domain.PushUnsupportedError
	switch {
	case errors.Is(err, domain.ErrLLMNotConfigured):
		httpx.Fail(w, http.StatusPreconditionFailed, httpx.CodeNotConnected, "IA non configurée : renseignez votre fournisseur dans Réglages › IA.")
	case errors.As(err, &pu):
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": pu.Error(), "code": httpx.CodeConflict, "session": pu.Session})
	default:
		return false
	}
	return true
}

func (a *api) reply3(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil && a.v3Fail(w, err) {
		return
	}
	a.reply(w, r, v, err)
}

func (a *api) withID(w http.ResponseWriter, r *http.Request, h func(uid, id int64)) {
	a.withUser(w, r, func(uid int64) {
		id, ok := pathID(r)
		if !ok {
			notFound(w)
			return
		}
		h(uid, id)
	})
}

// ---------- shoes, track, trends ----------

func (a *api) mergeShoe(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		var body struct {
			IntoID int64 `json:"intoId"`
		}
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil || body.IntoID <= 0 {
			badRequest(w, "Corps JSON invalide ({\"intoId\": …} attendu).")
			return
		}
		v, err := a.s.MergeShoe(r.Context(), uid, id, body.IntoID)
		a.reply(w, r, v, err)
	})
}

func (a *api) track(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		v, err := a.s.Track(r.Context(), uid, id)
		if err == nil {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		a.reply(w, r, v, err)
	})
}

func (a *api) efficiencyTrend(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		weeks := 0
		if q := r.URL.Query().Get("weeks"); q != "" {
			n, err := strconv.Atoi(q)
			if err != nil {
				badRequest(w, "weeks doit être un entier.")
				return
			}
			weeks = n
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		v, err := a.s.EfficiencyTrend(ctx, uid, weeks)
		a.reply(w, r, v, err)
	})
}

// ---------- LLM ----------

func (a *api) getLLM(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.LLMConfig(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

func (a *api) putLLM(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body domain.LLMConfigInput
		if err := httpx.Decode(w, r, &body, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({provider, model, baseUrl?, apiKey?} attendu).")
			return
		}
		v, err := a.s.PutLLMConfig(r.Context(), uid, body)
		a.reply(w, r, v, err)
	})
}

func (a *api) deleteLLM(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) { a.noContent(w, r, a.s.DeleteLLMConfig(r.Context(), uid)) })
}

func (a *api) testLLM(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.TestLLM(r.Context(), uid)
		a.reply3(w, r, v, err)
	})
}

func (a *api) analyze(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body domain.AnalyzeInput
		if err := httpx.Decode(w, r, &body, 64<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({kind, activityId?, date?, activityIds?, weeks?, include?, question?} attendu).")
			return
		}
		v, err := a.s.Analyze(r.Context(), uid, body)
		a.reply3(w, r, v, err)
	})
}

func (a *api) analyses(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		q := r.URL.Query()
		v, err := a.s.Analyses(r.Context(), uid, q.Get("kind"), q.Get("ref"))
		a.reply(w, r, v, err)
	})
}

// ---------- plan ----------

func (a *api) planGenerate(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body domain.PlanGenerateInput
		if err := httpx.Decode(w, r, &body, 16<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({mode, horizon, startDate, note?} attendu).")
			return
		}
		v, err := a.s.GeneratePlan(r.Context(), uid, body)
		a.reply3(w, r, v, err)
	})
}

func (a *api) planImport(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			Text string `json:"text"`
		}
		if err := httpx.Decode(w, r, &body, 512<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({\"text\": \"…\"} attendu, 512 Kio max).")
			return
		}
		v, err := a.s.ImportPlan(body.Text)
		a.reply(w, r, v, err)
	})
}

func (a *api) planBulk(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			Sessions []json.RawMessage `json:"sessions"`
		}
		if err := httpx.Decode(w, r, &body, 512<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({\"sessions\": […]} attendu).")
			return
		}
		v, err := a.s.BulkCreatePlanned(r.Context(), uid, body.Sessions)
		a.created(w, r, v, err)
	})
}

func (a *api) pushGarmin(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		v, err := a.s.PushGarmin(ctx, uid, id)
		a.reply3(w, r, v, err)
	})
}

func (a *api) workoutFIT(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		b, name, err := a.s.WorkoutFIT(r.Context(), uid, id)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.ant.fit")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(b)
	})
}

func (a *api) calendar(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		q := r.URL.Query()
		v, err := a.s.Calendar(r.Context(), uid, q.Get("from"), q.Get("to"))
		a.reply(w, r, v, err)
	})
}

// ---------- notifications ----------

func (a *api) listNotifications(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.Notifications(r.Context(), uid)
		a.reply(w, r, v, err)
	})
}

func decodeNotify(w http.ResponseWriter, r *http.Request) (domain.NotificationInput, bool) {
	var in domain.NotificationInput
	if err := httpx.Decode(w, r, &in, 16<<10); err != nil {
		badRequest(w, "Corps JSON invalide ({kind, label, enabled, url? | botToken? + chatId?} attendu).")
		return in, false
	}
	return in, true
}

func (a *api) createNotification(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if in, ok := decodeNotify(w, r); ok {
			v, err := a.s.CreateNotification(r.Context(), uid, in)
			a.created(w, r, v, err)
		}
	})
}

func (a *api) patchNotification(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		if in, ok := decodeNotify(w, r); ok {
			v, err := a.s.PatchNotification(r.Context(), uid, id, in)
			a.reply(w, r, v, err)
		}
	})
}

func (a *api) deleteNotification(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) { a.noContent(w, r, a.s.DeleteNotification(r.Context(), uid, id)) })
}

func (a *api) testNotification(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		v, err := a.s.TestNotification(r.Context(), uid, id)
		a.reply(w, r, v, err)
	})
}

// ---------- rescue import ----------

func (a *api) importFile(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		r.Body = http.MaxBytesReader(w, r.Body, domain.MaxImportBytes+1<<20)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			httpx.Fail(w, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "Fichier trop volumineux ou formulaire invalide (25 Mo max, champ « file »).")
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		f, hdr, err := r.FormFile("file")
		if err != nil {
			badRequest(w, "Champ « file » manquant.")
			return
		}
		defer f.Close()
		if hdr.Size > domain.MaxImportBytes {
			httpx.Fail(w, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "Fichier trop volumineux (25 Mo max).")
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, domain.MaxImportBytes+1))
		if err != nil || len(data) > domain.MaxImportBytes {
			httpx.Fail(w, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "Fichier trop volumineux (25 Mo max).")
			return
		}
		v, err := a.s.ImportFile(r.Context(), uid, hdr.Filename, data)
		if err == nil && v.Merged {
			a.reply(w, r, v, nil)
			return
		}
		a.created(w, r, v, err)
	})
}
