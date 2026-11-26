package domainapi

import (
	"net/http"
	"strconv"

	"stride/internal/httpx"
)

// registerV46 mounts the notification centre (§21, issue #142). The prefix `inbox` is deliberately
// distinct from `/api/notifications`, which manages the external Discord / Telegram / webhook
// targets: the two features never share a route, a query key or a name.
func registerV46(mux *http.ServeMux, user func(string, http.HandlerFunc), a *api) {
	user("GET /api/inbox", a.inbox)
	user("POST /api/inbox/read", a.markInboxRead)
	user("POST /api/inbox/read-all", a.markInboxAllRead)
}

// inbox lists the newest entries with the unread count. Query: limit (1..200), before (id cursor).
func (a *api) inbox(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		page, err := a.s.Inbox(r.Context(), uid, limit, before)
		a.reply(w, r, page, err)
	})
}

// markInboxRead marks {ids: [...]} as read and answers the new unread count.
func (a *api) markInboxRead(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		var body struct {
			IDs []int64 `json:"ids"`
		}
		if err := httpx.Decode(w, r, &body, 8<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide.")
			return
		}
		unread, err := a.s.MarkInboxRead(r.Context(), uid, body.IDs)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]int{"unread": unread})
	})
}

// markInboxAllRead empties the badge and answers 0.
func (a *api) markInboxAllRead(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		unread, err := a.s.MarkAllInboxRead(r.Context(), uid)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]int{"unread": unread})
	})
}
