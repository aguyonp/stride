package domainapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"stride/internal/athlete"
	"stride/internal/domain"
)

const portalOrigin = "http://localhost:8080"

// portal sends a public (token-scoped, session-less) call with the given Origin header.
func (e *env2) portal(method, path, origin string, body any, out any) (int, string) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("%s %s: decode: %v (%s)", method, path, err, raw)
		}
	}
	return resp.StatusCode, string(raw)
}

func shareToken(l domain.ShareLink) string { return strings.TrimPrefix(l.URL, "/share/") }

func TestSharePortalPermissionsNotesAndPrescriptions(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	aid := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)

	// A link created without permissions stays read-only: 403 SHARE_FORBIDDEN on every write.
	var ro domain.ShareLink
	if code, body := e.do(1, "POST", "/api/share", m{"label": "Lecture"}, &ro); code != 201 || ro.Permissions != (domain.SharePermissions{}) {
		t.Fatalf("create: %d %s", code, body)
	}
	roTok := shareToken(ro)
	if code, body := e.portal("PUT", "/api/public/share/"+roTok+"/directive", portalOrigin, m{"body": "Repos"}, nil); code != 403 || !strings.Contains(body, "SHARE_FORBIDDEN") {
		t.Fatalf("read-only link wrote: %d %s", code, body)
	}
	if code, _ := e.portal("GET", fmt.Sprintf("/api/public/share/%s/activities/%d", roTok, aid), "", nil, nil); code != 403 {
		t.Fatalf("detail without permission: %d", code)
	}

	var link domain.ShareLink
	in := m{"label": "Kiné Martin", "permissions": m{"comments": true, "planning": true, "activityDetails": true}}
	if code, body := e.do(1, "POST", "/api/share", in, &link); code != 201 || !link.Permissions.Comments || !link.Permissions.Planning {
		t.Fatalf("create with permissions: %d %s", code, body)
	}
	if code, _ := e.do(1, "PATCH", fmt.Sprintf("/api/share/%d", link.ID), m{"permissions": m{"bogus": true}}, nil); code != 400 {
		t.Fatalf("unknown permission accepted: %d", code)
	}
	tok := shareToken(link)
	base := "/api/public/share/" + tok

	// Same origin only.
	directive := m{"authorName": "M. Martin", "authorRole": "physio", "body": "Pas de fractionné​ cette semaine.\x07",
		"restrictions": m{"noIntervals": true, "maxIntensity": "easy", "cadenceTargetSpm": 172}}
	for _, origin := range []string{"", "https://evil.example"} {
		if code, _ := e.portal("PUT", base+"/directive", origin, directive, nil); code != 403 {
			t.Fatalf("origin %q accepted: %d", origin, code)
		}
	}
	if code, _ := e.portal("PUT", base+"/directive", portalOrigin, `{"body":"x","admin":true}`, nil); code != 400 {
		t.Fatalf("unknown field accepted: %d", code)
	}
	if code, _ := e.portal("PUT", base+"/directive", portalOrigin, m{"body": strings.Repeat("a", 17<<10)}, nil); code != 400 && code != 413 {
		t.Fatalf("oversized body accepted: %d", code)
	}
	if code, _ := e.portal("PUT", base+"/directive", portalOrigin, m{"body": strings.Repeat("é", 2001)}, nil); code != 400 {
		t.Fatalf("2001-rune directive accepted: %d", code)
	}

	// Directive upsert: one per link, text sanitized.
	var d athlete.ShareNote
	if code, body := e.portal("PUT", base+"/directive", portalOrigin, directive, &d); code != 200 {
		t.Fatalf("directive: %d %s", code, body)
	}
	if d.Body != "Pas de fractionné cette semaine." || d.AuthorRole != "physio" || d.Restrictions == nil || !d.Restrictions.NoIntervals ||
		d.LinkID == nil || *d.LinkID != link.ID || d.LinkLabel != "Kiné Martin" {
		t.Fatalf("directive: %+v", d)
	}
	directive["body"] = "Footing uniquement."
	var d2 athlete.ShareNote
	e.portal("PUT", base+"/directive", portalOrigin, directive, &d2)
	if d2.ID != d.ID || d2.Body != "Footing uniquement." {
		t.Fatalf("directive not upserted: %+v", d2)
	}

	// Remarks: in-period activity of the owner only, upsert per (link, activity).
	var r athlete.ShareNote
	if code, body := e.portal("PUT", fmt.Sprintf("%s/activities/%d/remark", base, aid), portalOrigin, m{"body": "Bonne cadence"}, &r); code != 200 {
		t.Fatalf("remark: %d %s", code, body)
	}
	other := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1003`) // 10 days ago: in the 12-week period
	if code, _ := e.portal("PUT", fmt.Sprintf("%s/activities/%d/remark", base, other+999), portalOrigin, m{"body": "x"}, nil); code != 404 {
		t.Fatalf("unknown activity: %d", code)
	}
	e.do(1, "PATCH", fmt.Sprintf("/api/share/%d", link.ID), m{"periodWeeks": 4}, nil)
	if _, err := e.st.DB.Exec(`UPDATE activities SET local_date = date(local_date, '-60 days') WHERE id = ?`, other); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.portal("PUT", fmt.Sprintf("%s/activities/%d/remark", base, other), portalOrigin, m{"body": "x"}, nil); code != 404 {
		t.Fatalf("activity outside the period: %d", code)
	}

	// The public view carries permissions, the directive and the remark; the cache was invalidated.
	var v domain.PublicShareView
	if code, body := publicGet(t, e, tok, &v); code != 200 || v.LinkID != link.ID || v.Directive == nil || v.Directive.Body != "Footing uniquement." || !v.Permissions.Planning || v.Calendar == nil {
		t.Fatalf("public view: %d %s", code, body)
	}
	found := false
	for _, it := range *v.RecentSessions {
		if it.ID == aid && it.Remark != nil && it.Remark.Body == "Bonne cadence" {
			found = true
		}
	}
	if !found {
		t.Fatal("remark missing from the public view")
	}

	// Planning: prescribed sessions and events, only within the window, editable by their link only.
	var ps athlete.PlannedSession
	session := m{"date": e.fx.day(1), "sport": "run", "title": "Footing 40'", "targetDurationS": 2400, "source": "llm",
		"steps": []m{{"type": "run", "durationS": 2400}}}
	if code, body := e.portal("POST", base+"/plan", portalOrigin, session, &ps); code != 201 {
		t.Fatalf("prescribe: %d %s", code, body)
	}
	if ps.Source != "share" || ps.PrescribedBy == nil || ps.PrescribedBy.Label != "Kiné Martin" || ps.PrescribedBy.AuthorName == nil ||
		*ps.PrescribedBy.AuthorName != "M. Martin" || ps.PrescribedBy.LinkID == nil || *ps.PrescribedBy.LinkID != link.ID {
		t.Fatalf("prescription: %+v", ps)
	}
	session["date"] = e.fx.day(121)
	if code, _ := e.portal("POST", base+"/plan", portalOrigin, session, nil); code != 400 {
		t.Fatalf("date beyond +120 d accepted: %d", code)
	}
	var mine athlete.PlannedSession
	e.do(1, "POST", "/api/plan", m{"date": e.fx.day(2), "sport": "run", "title": "Seuil"}, &mine)
	if code, _ := e.portal("PATCH", fmt.Sprintf("%s/plan/%d", base, mine.ID), portalOrigin, m{"title": "Repos"}, nil); code != 403 {
		t.Fatalf("link edited the athlete's session: %d", code)
	}
	if code, _ := e.portal("DELETE", fmt.Sprintf("%s/plan/%d", base, mine.ID), portalOrigin, nil, nil); code != 403 {
		t.Fatalf("link deleted the athlete's session: %d", code)
	}
	var patched athlete.PlannedSession
	if code, body := e.portal("PATCH", fmt.Sprintf("%s/plan/%d", base, ps.ID), portalOrigin, m{"title": "Footing 35'"}, &patched); code != 200 || patched.Title != "Footing 35'" || patched.PrescribedBy == nil {
		t.Fatalf("patch own: %d %s", code, body)
	}
	var ev athlete.CalendarEvent
	if code, body := e.portal("POST", base+"/events", portalOrigin, m{"kind": "pain", "startDate": e.fx.day(0), "severity": 2, "note": "Genou"}, &ev); code != 201 || ev.PrescribedBy == nil {
		t.Fatalf("event: %d %s", code, body)
	}
	// The athlete sees the prescription; the athlete's own edit keeps the attribution.
	var plan []athlete.PlannedSession
	e.do(1, "GET", "/api/plan?from="+e.fx.day(0)+"&to="+e.fx.day(3), nil, &plan)
	seen := false
	for _, p := range plan {
		if p.ID == ps.ID && p.PrescribedBy != nil {
			seen = true
		}
	}
	if !seen {
		t.Fatal("prescribedBy missing from the athlete plan")
	}
	if code, _ := e.do(1, "PATCH", fmt.Sprintf("/api/plan/%d", ps.ID), m{"title": "Footing", "prescribedBy": nil}, nil); code != 200 {
		t.Fatalf("athlete patch: %d", code)
	}
	var again athlete.PlannedSession
	e.do(1, "GET", fmt.Sprintf("/api/plan?from=%s&to=%s", e.fx.day(1), e.fx.day(1)), nil, &plan)
	for _, p := range plan {
		if p.ID == ps.ID {
			again = p
		}
	}
	if again.PrescribedBy == nil || again.Title != "Footing" {
		t.Fatalf("attribution lost on athlete edit: %+v", again)
	}
	if code, _ := e.portal("DELETE", fmt.Sprintf("%s/events/%d", base, ev.ID), portalOrigin, nil, nil); code != 204 {
		t.Fatalf("delete own event: %d", code)
	}

	// Session detail (activityDetails).
	var det domain.PublicActivityDetail
	if code, body := e.portal("GET", fmt.Sprintf("%s/activities/%d", base, aid), "", nil, &det); code != 200 || det.Remark == nil || det.ID != aid {
		t.Fatalf("detail: %d %s", code, body)
	}

	// Athlete side: notes listing, remark count, AI injection first, delete.
	var notes []athlete.ShareNote
	if code, _ := e.do(1, "GET", "/api/share/notes?active=1", nil, &notes); code != 200 || len(notes) != 2 || notes[0].Kind != "directive" {
		t.Fatalf("active notes: %+v", notes)
	}
	var list struct {
		Items []struct {
			ID          int64 `json:"id"`
			RemarkCount int   `json:"remarkCount"`
		} `json:"items"`
	}
	e.do(1, "GET", "/api/activities", nil, &list)
	for _, it := range list.Items {
		if it.ID == aid && it.RemarkCount != 1 {
			t.Fatalf("remarkCount: %+v", it)
		}
	}
	code, md := e.do(1, "GET", fmt.Sprintf("/api/activities/%d/report", aid), nil, nil)
	coach := strings.Index(md, "### CONSIGNES KINÉ / COACH (PRIORITAIRES)")
	if code != 200 || coach < 0 || !strings.Contains(md, "```text\nFooting uniquement.\n```") || !strings.Contains(md, "Bonne cadence") {
		t.Fatalf("coach section missing: %d %s", code, md)
	}
	if first := strings.Index(md, "### "); first != coach {
		t.Fatalf("coach section is not first: %s", md[:min(len(md), 600)])
	}

	// Revocation: the portal answers 404 (same as unknown), the directive is no longer active.
	e.do(1, "DELETE", fmt.Sprintf("/api/share/%d", link.ID), nil, nil)
	if code, _ := e.portal("PUT", base+"/directive", portalOrigin, directive, nil); code != 404 {
		t.Fatalf("revoked link wrote: %d", code)
	}
	if code, _ := e.portal("PUT", "/api/public/share/st4_unknown/directive", portalOrigin, directive, nil); code != 404 {
		t.Fatalf("unknown token: %d", code)
	}
	e.do(1, "GET", "/api/share/notes?active=1", nil, &notes)
	if len(notes) != 1 || notes[0].Kind != "remark" {
		t.Fatalf("revoked directive still active: %+v", notes)
	}
	if code, _ := e.do(2, "DELETE", fmt.Sprintf("/api/share/notes/%d", notes[0].ID), nil, nil); code != 404 {
		t.Fatalf("another user deleted a note: %d", code)
	}
	if code, _ := e.do(1, "DELETE", fmt.Sprintf("/api/share/notes/%d", notes[0].ID), nil, nil); code != 204 {
		t.Fatalf("delete note: %d", code)
	}
}

func TestSharePortalWriteRateLimit(t *testing.T) {
	e := setup(t)
	var link domain.ShareLink
	e.do(1, "POST", "/api/share", m{"label": "Coach", "permissions": m{"comments": true}}, &link)
	path := "/api/public/share/" + shareToken(link) + "/directive"
	limited := false
	for i := 0; i < 31; i++ {
		code, _ := e.portal("PUT", path, portalOrigin, m{"body": fmt.Sprintf("Consigne %d", i)}, nil)
		if code == http.StatusTooManyRequests {
			limited = i == 30
			break
		}
	}
	if !limited {
		t.Fatal("31st write within 10 minutes was not rate limited")
	}
}

func TestMorphPreviewApplyRevertFIT(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	run := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`) // yesterday 18:00: within 48 h
	if code, body := e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", run), m{"painDuring": 4}, nil); code != 200 {
		t.Fatalf("notes: %d %s", code, body)
	}
	session := m{"date": e.fx.day(0), "sport": "run", "title": "6×3'", "targetDurationS": 3600, "steps": []m{
		{"type": "warmup", "durationS": 900},
		{"type": "repeat", "repeat": 6, "steps": []m{{"type": "interval", "durationS": 180, "target": m{"kind": "pace", "min": 240, "max": 250}}, {"type": "recovery", "durationS": 120}}},
		{"type": "cooldown", "durationS": 600}}}
	var ps athlete.PlannedSession
	e.do(1, "POST", "/api/plan", session, &ps)
	var later athlete.PlannedSession
	session["date"] = e.fx.day(3)
	e.do(1, "POST", "/api/plan", session, &later)
	if code, _ := e.do(1, "GET", fmt.Sprintf("/api/plan/%d/morph", later.ID), nil, nil); code != 400 {
		t.Fatalf("J+3 session morphable: %d", code)
	}
	if code, _ := e.do(2, "GET", fmt.Sprintf("/api/plan/%d/morph", ps.ID), nil, nil); code != 404 {
		t.Fatalf("another user's session: %d", code)
	}

	var prev domain.MorphPreview
	if code, body := e.do(1, "GET", fmt.Sprintf("/api/plan/%d/morph", ps.ID), nil, &prev); code != 200 || prev.Proposal == nil {
		t.Fatalf("preview: %d %s", code, body)
	}
	if len(prev.Constraints) == 0 || prev.Constraints[0].Kind != "pain" || len(prev.Changes) == 0 || prev.SessionID != ps.ID {
		t.Fatalf("preview content: %+v", prev)
	}
	var applied athlete.PlannedSession
	if code, body := e.do(1, "POST", fmt.Sprintf("/api/plan/%d/morph/apply", ps.ID), m{"proposal": prev.Proposal}, &applied); code != 200 {
		t.Fatalf("apply: %d %s", code, body)
	}
	if applied.Morph == nil || len(applied.Morph.Original.Steps) != 3 || applied.Morph.Original.Title != "6×3'" || len(applied.Steps) != 1 ||
		len(applied.Morph.Reasons) == 0 || applied.Date != ps.Date {
		t.Fatalf("applied: %+v", applied)
	}
	// Applying again keeps the very first planned trace.
	e.do(1, "POST", fmt.Sprintf("/api/plan/%d/morph/apply", ps.ID), m{"proposal": m{"title": "Footing 30'", "targetDurationS": 1800, "date": "2001-01-01"}}, &applied)
	if applied.Title != "Footing 30'" || applied.Date != ps.Date || applied.Morph.Original.Title != "6×3'" {
		t.Fatalf("re-apply: %+v", applied)
	}
	if code, _ := e.do(1, "GET", fmt.Sprintf("/api/plan/%d/workout.fit", ps.ID), nil, nil); code != 200 {
		t.Fatalf("FIT of the morphed steps: %d", code)
	}
	var reverted athlete.PlannedSession
	if code, _ := e.do(1, "POST", fmt.Sprintf("/api/plan/%d/morph/revert", ps.ID), nil, &reverted); code != 200 || reverted.Morph != nil ||
		reverted.Title != "6×3'" || len(reverted.Steps) != 3 || *reverted.TargetDurationS != 3600 {
		t.Fatalf("revert: %v %+v", *reverted.TargetDurationS, reverted)
	}
	if code, _ := e.do(1, "POST", fmt.Sprintf("/api/plan/%d/morph/revert", ps.ID), nil, nil); code != 409 {
		t.Fatalf("revert of a plain session: %d", code)
	}
	if code, _ := e.do(1, "POST", fmt.Sprintf("/api/plan/%d/morph/refine", ps.ID), m{"note": "Je préfère le matin"}, nil); code != 412 {
		t.Fatalf("refine without LLM: %d", code)
	}
}
