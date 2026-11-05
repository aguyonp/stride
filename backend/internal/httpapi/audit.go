package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/httpx"
)

// actions maps routed patterns to the business events recorded as user actions.
var actions = map[string]string{
	"POST /api/auth/register/finish":     "auth.register",
	"POST /api/auth/login/finish":        "auth.login",
	"POST /api/auth/logout":              "auth.logout",
	"POST /api/me/passkeys/finish":       "passkey.add",
	"PATCH /api/me/passkeys/{id}":        "passkey.rename",
	"DELETE /api/me/passkeys/{id}":       "passkey.delete",
	"PATCH /api/me":                      "account.update",
	"DELETE /api/me":                     "account.delete",
	"POST /api/sync":                     "sync.start",
	"GET /api/activities/{id}/report":    "prompt.generate",
	"GET /api/days/{date}/report":        "prompt.generate_day",
	"POST /api/exports/multi":            "prompt.generate_multi",
	"POST /api/llm/analyze":              "llm.analyze",
	"POST /api/plan/generate":            "plan.generate",
	"POST /api/plan/import":              "plan.import",
	"POST /api/plan":                     "plan.create",
	"POST /api/plan/bulk":                "plan.create_bulk",
	"PATCH /api/plan/{id}":               "plan.update",
	"DELETE /api/plan/{id}":              "plan.delete",
	"POST /api/plan/{id}/push-garmin":    "plan.push_garmin",
	"GET /api/plan/{id}/workout.fit":     "plan.download_fit",
	"PUT /api/activities/{id}/notes":     "feeling.save",
	"PUT /api/days/{date}/checkin":       "checkin.save",
	"PUT /api/profile":                   "settings.profile",
	"PUT /api/llm":                       "settings.llm",
	"DELETE /api/llm":                    "settings.llm_delete",
	"PUT /api/integrations/garmin":       "settings.garmin_connect",
	"DELETE /api/integrations/garmin":    "settings.garmin_disconnect",
	"DELETE /api/integrations/strava":    "settings.strava_disconnect",
	"POST /api/notifications":            "settings.notification_create",
	"PATCH /api/notifications/{id}":      "settings.notification_update",
	"DELETE /api/notifications/{id}":     "settings.notification_delete",
	"POST /api/goals":                    "goal.create",
	"PATCH /api/goals/{id}":              "goal.update",
	"DELETE /api/goals/{id}":             "goal.delete",
	"POST /api/shoes":                    "shoe.create",
	"PATCH /api/shoes/{id}":              "shoe.update",
	"DELETE /api/shoes/{id}":             "shoe.delete",
	"POST /api/shoes/{id}/merge":         "shoe.merge",
	"POST /api/shoes/import-strava":      "shoe.import_strava",
	"POST /api/imports":                  "activity.import",
	"GET /api/backup":                    "backup.export",
	"POST /api/backup/restore":           "backup.restore",
	"POST /api/admin/invitations":        "admin.invitation_create",
	"DELETE /api/admin/invitations/{id}": "admin.invitation_revoke",
	"PATCH /api/admin/users/{id}":        "admin.user_update",
	"DELETE /api/admin/users/{id}":       "admin.user_delete",
	"PUT /api/admin/settings":            "admin.settings",
	"POST /api/admin/cache/flush":        "admin.cache_flush",
	"POST /api/admin/users/{id}/resync":  "admin.user_resync",
	"GET /api/admin/users/{id}/backup":   "admin.user_backup",
	"POST /api/admin/users/{id}/restore": "admin.user_restore",
}

// clientActions are events only the browser sees (POST /api/actions).
var clientActions = map[string]bool{"prompt.copy": true, "prompt.download": true, "prompt.share": true}

// withAudit records API requests and mapped user actions; static files and health checks are skipped.
func withAudit(l *applog.Log, next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, meta := applog.WithMeta(r.Context())
		r = r.WithContext(ctx)
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		level := applog.LevelInfo
		if status >= 400 {
			level = applog.LevelError
		}
		uid := meta.UserID()
		l.Add(applog.Entry{Category: applog.CategoryRequest, Level: level, UserID: uid, Method: r.Method, Target: r.URL.Path,
			Status: status, DurationMs: time.Since(start).Milliseconds()})
		if action, ok := actions[r.Pattern]; ok && uid != nil && status != http.StatusUnauthorized {
			l.Add(applog.Entry{Category: applog.CategoryAction, Level: level, UserID: uid, Method: r.Method, Target: r.URL.Path,
				Status: status, Action: action})
		}
	})
}

func postAction(l *applog.Log) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string `json:"action"`
			Detail string `json:"detail"`
		}
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil || !clientActions[body.Action] {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Action inconnue.")
			return
		}
		u, _ := auth.UserFrom(r.Context())
		l.Add(applog.Entry{Category: applog.CategoryAction, UserID: &u.ID, Method: r.Method, Target: r.URL.Path, Status: http.StatusNoContent,
			Action: body.Action, Message: body.Detail})
		w.WriteHeader(http.StatusNoContent)
	}
}

func adminLogs(l *applog.Log) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f := applog.Filter{}
		switch q.Get("category") {
		case "", "all":
		case "request", "requests":
			f.Category = applog.CategoryRequest
		case "user_action", "actions":
			f.Category = applog.CategoryAction
		default:
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Catégorie inconnue (all, request, user_action).")
			return
		}
		switch q.Get("level") {
		case "", "all":
		case "error":
			f.Level = applog.LevelError
		default:
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Sévérité inconnue (all, error).")
			return
		}
		f.UserID, _ = strconv.ParseInt(q.Get("userId"), 10, 64)
		f.Before, _ = strconv.ParseInt(q.Get("before"), 10, 64)
		f.Limit, _ = strconv.Atoi(q.Get("limit"))
		page, err := l.Query(r.Context(), f)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Lecture des journaux impossible.")
			return
		}
		httpx.JSON(w, http.StatusOK, page)
	}
}
