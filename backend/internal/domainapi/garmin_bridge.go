package domainapi

import (
	"net/http"

	"stride/internal/garminbridge"
	"stride/internal/httpx"
)

// Experimental Garmin Connect bridge (§23, issue #147).
//
// Two switches govern these routes: STRIDE_GARMIN_BRIDGE (env, master) and the admin setting
// `garminBridgeEnabled`. Without the env variable the gateway is not even built and the routes are
// not mounted — a decommission is « delete the package, its routes and its tokens ». With the env on
// but the setting off, the answer is 409: the feature exists, it is switched off.

// registerGarminBridge mounts the bridge routes. Called only when the gateway exists.
func registerGarminBridge(mux *http.ServeMux, user func(string, http.HandlerFunc), a *api) {
	user("GET /api/garmin/bridge", a.bridgeStatus)
	user("POST /api/garmin/bridge/login", a.bridgeLogin)
	user("POST /api/garmin/bridge/mfa", a.bridgeMFA)
	user("DELETE /api/garmin/bridge", a.bridgeDisconnect)
	user("POST /api/plan/push-today", a.pushToday)
}

// bridgeStatus answers the state of the bridge for the caller (no network call).
func (a *api) bridgeStatus(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		st := a.s.BridgeStatus(r.Context(), uid)
		st.Locked = a.s.BridgeLocked(uid)
		httpx.JSON(w, http.StatusOK, st)
	})
}

// bridgeLogin signs in with the account credentials. The body is read once and dropped with it; the
// password is never logged, never stored, never echoed.
func (a *api) bridgeLogin(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := httpx.Decode(w, r, &body, 8<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
			return
		}
		defer func() { body.Password = "" }()
		res, err := a.s.BridgeLogin(r.Context(), uid, body.Email, body.Password)
		a.bridgeReply(w, r, res, err)
	})
}

// bridgeMFA completes a sign-in challenged with a code.
func (a *api) bridgeMFA(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			Code string `json:"code"`
		}
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
			return
		}
		res, err := a.s.BridgeLoginMFA(r.Context(), uid, body.Code)
		a.bridgeReply(w, r, res, err)
	})
}

func (a *api) bridgeDisconnect(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		if err := a.s.BridgeDisconnect(r.Context(), uid); err != nil {
			a.bridgeReply(w, r, garminbridge.LoginResult{}, err)
			return
		}
		httpx.JSON(w, http.StatusNoContent, nil)
	})
}

// bridgeReply maps the gateway errors to HTTP. MFA is a 202 (accepted, more input needed), so a form
// can follow the flow without treating it as an error.
func (a *api) bridgeReply(w http.ResponseWriter, r *http.Request, res garminbridge.LoginResult, err error) {
	if err == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"state": res.State, "displayName": res.DisplayName})
		return
	}
	switch err {
	case garminbridge.ErrMFARequired:
		httpx.JSON(w, http.StatusAccepted, map[string]any{"state": res.State})
	case garminbridge.ErrDisabled:
		httpx.Fail(w, http.StatusConflict, httpx.CodeConflict, "Le pont Garmin est désactivé par l'administrateur.")
	case garminbridge.ErrBlocked:
		httpx.Fail(w, http.StatusTooManyRequests, httpx.CodeRateLimited,
			"Garmin bloque cette connexion (429/427). Réessayez plus tard, ou utilisez le fichier .FIT.")
	case garminbridge.ErrExpired:
		httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized,
			"Session Garmin expirée : reconnectez le pont, ou utilisez le fichier .FIT.")
	case garminbridge.ErrCredentials:
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Identifiants ou code refusés par Garmin.")
	default:
		httpx.Fail(w, http.StatusBadGateway, httpx.CodeProviderDown, "Garmin n'a pas répondu correctement : "+err.Error())
	}
}

// pushToday sends the sessions of a day that are worth sending, serially, under the user's lock.
func (a *api) pushToday(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			Date string `json:"date"`
		}
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
			return
		}
		if body.Date == "" {
			// Default: today, in the athlete's timezone.
			body.Date = a.s.Today(uid)
		}
		res, err := a.s.PushToday(r.Context(), uid, body.Date)
		if err != nil {
			a.reply(w, r, nil, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"date": body.Date, "results": res})
	})
}
