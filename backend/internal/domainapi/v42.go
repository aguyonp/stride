package domainapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"stride/internal/domain"
	"stride/internal/httpx"
)

// 4.2.0: physio/coach portal (§15.1) and Smart Workout Morphing (§15.5).

// portalBodyLimit bounds every portal write body.
const portalBodyLimit = 16 << 10

func registerV42(mux *http.ServeMux, user func(string, http.HandlerFunc), a *api) {
	user("GET /api/share/notes", a.shareNotes)
	user("DELETE /api/share/notes/{id}", a.deleteShareNote)

	mux.HandleFunc("GET /api/public/share/{token}/activities/{id}", a.publicActivity)
	mux.HandleFunc("PUT /api/public/share/{token}/directive", a.portalWrite(a.putDirective))
	mux.HandleFunc("DELETE /api/public/share/{token}/directive", a.portalWrite(a.deleteDirective))
	mux.HandleFunc("PUT /api/public/share/{token}/activities/{id}/remark", a.portalWrite(a.putRemark))
	mux.HandleFunc("DELETE /api/public/share/{token}/activities/{id}/remark", a.portalWrite(a.deleteRemark))
	mux.HandleFunc("POST /api/public/share/{token}/plan", a.portalWrite(a.portalCreatePlanned))
	mux.HandleFunc("PATCH /api/public/share/{token}/plan/{id}", a.portalWrite(a.portalPatchPlanned))
	mux.HandleFunc("DELETE /api/public/share/{token}/plan/{id}", a.portalWrite(a.portalDeletePlanned))
	mux.HandleFunc("POST /api/public/share/{token}/events", a.portalWrite(a.portalCreateEvent))
	mux.HandleFunc("PATCH /api/public/share/{token}/events/{id}", a.portalWrite(a.portalPatchEvent))
	mux.HandleFunc("DELETE /api/public/share/{token}/events/{id}", a.portalWrite(a.portalDeleteEvent))

	user("GET /api/plan/{id}/morph", a.morphPreview)
	user("POST /api/plan/{id}/morph/refine", a.morphRefine)
	user("POST /api/plan/{id}/morph/apply", a.morphApply)
	user("POST /api/plan/{id}/morph/revert", a.morphRevert)
}

// ---------- athlete side ----------

func (a *api) shareNotes(w http.ResponseWriter, r *http.Request) {
	a.withUser(w, r, func(uid int64) {
		v, err := a.s.ShareNotes(r.Context(), uid, r.URL.Query().Get("active") == "1")
		a.reply(w, r, v, err)
	})
}

func (a *api) deleteShareNote(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		a.noContent(w, r, a.s.DeleteShareNote(r.Context(), uid, id))
	})
}

// ---------- portal security ----------

// originOf returns scheme://host[:port] of a URL ("" when unparsable).
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// sameOrigin accepts a portal write only from the Stride origin (STRIDE_PUBLIC_URL): a
// Sec-Fetch-Site other than same-origin, or a foreign Origin, is refused; a request carrying
// neither header (not a browser) is refused too.
func (a *api) sameOrigin(r *http.Request) bool {
	site, origin := r.Header.Get("Sec-Fetch-Site"), r.Header.Get("Origin")
	if site != "" && site != "same-origin" {
		return false
	}
	if origin == "" {
		return site == "same-origin"
	}
	want := originOf(a.s.PublicURL())
	return want != "" && originOf(origin) == want
}

// portalWrite wraps a public write: same origin, then the per-IP and per-token rate limits. The
// token itself never reaches a log line (requests and audit keep its 8-character prefix).
func (a *api) portalWrite(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.sameOrigin(r) {
			httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "Requête refusée : origine non autorisée.")
			return
		}
		token := r.PathValue("token")
		th := sha256.Sum256([]byte(token))
		now := time.Now()
		if !a.portalIPLimit.allow(clientIP(r), now) || !a.portalTokenLimit.allow(hex.EncodeToString(th[:]), now) {
			w.Header().Set("Retry-After", strconv.Itoa(int(portalWriteWindow.Seconds())))
			httpx.Fail(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "Trop de modifications, réessayez dans quelques minutes.")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		h(w, r, token)
	}
}

func portalID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathID(r)
	if !ok {
		notFound(w)
	}
	return id, ok
}

func (a *api) portalFail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrGone) {
		err = domain.ErrNotFound
	}
	a.fail(w, r, err)
}

func (a *api) portalReply(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	switch {
	case err != nil:
		a.portalFail(w, r, err)
	case status == http.StatusNoContent:
		w.WriteHeader(status)
	default:
		httpx.JSON(w, status, v)
	}
}

// ---------- portal endpoints ----------

func (a *api) publicActivity(w http.ResponseWriter, r *http.Request) {
	if !a.shareLimit.allow(clientIP(r), time.Now()) {
		w.Header().Set("Retry-After", "60")
		httpx.Fail(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "Trop de consultations, réessayez dans une minute.")
		return
	}
	id, ok := portalID(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	v, err := a.s.PublicActivity(r.Context(), r.PathValue("token"), id)
	a.portalReply(w, r, http.StatusOK, v, err)
}

func (a *api) putDirective(w http.ResponseWriter, r *http.Request, token string) {
	var in domain.ShareDirectiveInput
	if err := httpx.Decode(w, r, &in, portalBodyLimit); err != nil {
		badRequest(w, "Corps JSON invalide ({authorName?, authorRole?, body, validUntil?, restrictions?} attendu).")
		return
	}
	v, err := a.s.PutShareDirective(r.Context(), token, in)
	a.portalReply(w, r, http.StatusOK, v, err)
}

func (a *api) deleteDirective(w http.ResponseWriter, r *http.Request, token string) {
	a.portalReply(w, r, http.StatusNoContent, nil, a.s.DeleteShareDirective(r.Context(), token))
}

func (a *api) putRemark(w http.ResponseWriter, r *http.Request, token string) {
	id, ok := portalID(w, r)
	if !ok {
		return
	}
	var in domain.ShareRemarkInput
	if err := httpx.Decode(w, r, &in, portalBodyLimit); err != nil {
		badRequest(w, "Corps JSON invalide ({authorName?, authorRole?, body} attendu).")
		return
	}
	v, err := a.s.PutShareRemark(r.Context(), token, id, in)
	a.portalReply(w, r, http.StatusOK, v, err)
}

func (a *api) deleteRemark(w http.ResponseWriter, r *http.Request, token string) {
	if id, ok := portalID(w, r); ok {
		a.portalReply(w, r, http.StatusNoContent, nil, a.s.DeleteShareRemark(r.Context(), token, id))
	}
}

func (a *api) portalCreatePlanned(w http.ResponseWriter, r *http.Request, token string) {
	if body, ok := readBody(w, r, portalBodyLimit); ok {
		v, err := a.s.CreateSharePlanned(r.Context(), token, body)
		a.portalReply(w, r, http.StatusCreated, v, err)
	}
}

func (a *api) portalPatchPlanned(w http.ResponseWriter, r *http.Request, token string) {
	id, ok := portalID(w, r)
	if !ok {
		return
	}
	if body, ok := readBody(w, r, portalBodyLimit); ok {
		v, err := a.s.PatchSharePlanned(r.Context(), token, id, body)
		a.portalReply(w, r, http.StatusOK, v, err)
	}
}

func (a *api) portalDeletePlanned(w http.ResponseWriter, r *http.Request, token string) {
	if id, ok := portalID(w, r); ok {
		a.portalReply(w, r, http.StatusNoContent, nil, a.s.DeleteSharePlanned(r.Context(), token, id))
	}
}

func (a *api) portalCreateEvent(w http.ResponseWriter, r *http.Request, token string) {
	if body, ok := readBody(w, r, portalBodyLimit); ok {
		v, err := a.s.CreateShareEvent(r.Context(), token, body)
		a.portalReply(w, r, http.StatusCreated, v, err)
	}
}

func (a *api) portalPatchEvent(w http.ResponseWriter, r *http.Request, token string) {
	id, ok := portalID(w, r)
	if !ok {
		return
	}
	if body, ok := readBody(w, r, portalBodyLimit); ok {
		v, err := a.s.PatchShareEvent(r.Context(), token, id, body)
		a.portalReply(w, r, http.StatusOK, v, err)
	}
}

func (a *api) portalDeleteEvent(w http.ResponseWriter, r *http.Request, token string) {
	if id, ok := portalID(w, r); ok {
		a.portalReply(w, r, http.StatusNoContent, nil, a.s.DeleteShareEvent(r.Context(), token, id))
	}
}

// ---------- Smart Workout Morphing ----------

func (a *api) morphPreview(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		v, err := a.s.MorphPreview(r.Context(), uid, id)
		a.reply(w, r, v, err)
	})
}

func (a *api) morphRefine(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		var body domain.MorphRefineInput
		if r.ContentLength != 0 {
			if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
				badRequest(w, "Corps JSON invalide ({note?} attendu).")
				return
			}
		}
		v, err := a.s.RefineMorph(r.Context(), uid, id, body)
		a.replyJob(w, r, v, err)
	})
}

func (a *api) morphApply(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		var body domain.MorphApplyInput
		if err := httpx.Decode(w, r, &body, 64<<10); err != nil {
			badRequest(w, "Corps JSON invalide ({proposal} attendu).")
			return
		}
		v, err := a.s.ApplyMorph(r.Context(), uid, id, body)
		a.reply(w, r, v, err)
	})
}

func (a *api) morphRevert(w http.ResponseWriter, r *http.Request) {
	a.withID(w, r, func(uid, id int64) {
		v, err := a.s.RevertMorph(r.Context(), uid, id)
		a.reply(w, r, v, err)
	})
}
