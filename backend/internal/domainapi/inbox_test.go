package domainapi_test

import (
	"fmt"
	"testing"

	"stride/internal/domain"
)

// inboxKind filters a page by entry kind: the sync triggered by bootGarmin already feeds the centre
// with real analysis alerts, so the assertions count the portal entries rather than the total.
func inboxKind(page domain.InboxPage, kind string) []domain.InboxEntry {
	var out []domain.InboxEntry
	for _, e := range page.Items {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// TestInboxRoutes covers the notification centre API (§21, issue #142): the portal writes feed it,
// the owner alone sees its entries, and marking read only touches the caller's rows.
func TestInboxRoutes(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	aid := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)
	var day string
	if err := e.st.R.QueryRow(`SELECT local_date FROM activities WHERE user_id = 1 AND id = ?`, aid).Scan(&day); err != nil {
		t.Fatal(err)
	}

	// Every list goes into a fresh value: json.Unmarshal reuses the elements of an existing slice,
	// and the omitempty pointer fields (activityId, date, readAt) would keep the previous answer.
	list := func() domain.InboxPage {
		t.Helper()
		var p domain.InboxPage
		if code, body := e.do(1, "GET", "/api/inbox?limit=100", nil, &p); code != 200 {
			t.Fatalf("list: %d %s", code, body)
		}
		return p
	}

	// The sync of bootGarmin produced analysis alerts: they are persisted, and nothing else is.
	page := list()
	if len(page.Items) == 0 || len(inboxKind(page, domain.InboxKindShare)) != 0 {
		t.Fatalf("alerts after sync: %+v", page.Items)
	}
	for _, it := range page.Items {
		if it.Title == "" || it.CreatedAt == "" || (it.Level != "info" && it.Level != "warn" && it.Level != "error") {
			t.Fatalf("malformed alert entry: %+v", it)
		}
	}
	baselineUnread := page.Unread
	if baselineUnread == 0 {
		t.Fatalf("sync alerts are not unread: %+v", page)
	}

	// A physio writes a directive and a remark through the portal.
	var link domain.ShareLink
	if code, body := e.do(1, "POST", "/api/share", m{"label": "Kiné Martin", "permissions": m{"comments": true, "planning": true}}, &link); code != 201 {
		t.Fatalf("create link: %d %s", code, body)
	}
	base := "/api/public/share/" + shareToken(link)
	if code, body := e.portal("PUT", base+"/directive", portalOrigin, m{"body": "Pas de fractionné cette semaine."}, nil); code != 200 {
		t.Fatalf("directive: %d %s", code, body)
	}
	if code, body := e.portal("PUT", fmt.Sprintf("%s/activities/%d/remark", base, aid), portalOrigin, m{"body": "Bonne cadence"}, nil); code != 200 {
		t.Fatalf("remark: %d %s", code, body)
	}

	page = list()
	shares := inboxKind(page, domain.InboxKindShare)
	if len(shares) != 2 || page.Unread != baselineUnread+2 {
		t.Fatalf("after two portal writes: unread %d (was %d) shares %+v", page.Unread, baselineUnread, shares)
	}
	remark, directive := shares[0], shares[1]
	// The panel renders the wording from the code; the stored title is the English fallback.
	if remark.Code != "remark" || remark.Title != "New remark on a session" {
		t.Fatalf("remark entry: %+v", remark)
	}
	// The remark points at the session it comments, with its date.
	if remark.ActivityID == nil || *remark.ActivityID != aid || remark.Date == nil || *remark.Date != day {
		t.Fatalf("remark target: %+v", remark)
	}
	if remark.Detail != "Kiné Martin" {
		t.Fatalf("remark detail: %q", remark.Detail)
	}
	if directive.Code != "directive" || directive.ActivityID != nil || directive.Date != nil || directive.ReadAt != nil {
		t.Fatalf("directive entry: %+v", directive)
	}

	// Editing the same target does not stack a duplicate.
	if code, _ := e.portal("PUT", base+"/directive", portalOrigin, m{"body": "Footing uniquement."}, nil); code != 200 {
		t.Fatalf("directive update: %d", code)
	}
	if len(inboxKind(list(), domain.InboxKindShare)) != 2 {
		t.Fatal("upsert duplicated the entry")
	}

	// Another user sees none of it, and cannot mark these entries read.
	var other domain.InboxPage
	if code, _ := e.do(2, "GET", "/api/inbox?limit=100", nil, &other); code != 200 || len(inboxKind(other, domain.InboxKindShare)) != 0 {
		t.Fatalf("isolation read: %+v", other)
	}
	var counts map[string]int
	before := other.Unread
	if code, body := e.do(2, "POST", "/api/inbox/read", m{"ids": []int64{remark.ID}}, &counts); code != 200 || counts["unread"] != before {
		t.Fatalf("isolation write: %d %s", code, body)
	}
	if page = list(); len(inboxKind(page, domain.InboxKindShare)) != 2 || page.Unread != baselineUnread+2 {
		t.Fatalf("another user marked my entries: %+v", page)
	}

	// Marking read is per caller and idempotent; the history stays.
	if code, body := e.do(1, "POST", "/api/inbox/read", m{"ids": []int64{remark.ID}}, &counts); code != 200 || counts["unread"] != baselineUnread+1 {
		t.Fatalf("mark one: %d %s", code, body)
	}
	if code, body := e.do(1, "POST", "/api/inbox/read", m{"ids": []int64{remark.ID}}, &counts); code != 200 || counts["unread"] != baselineUnread+1 {
		t.Fatalf("mark twice: %d %s", code, body)
	}
	if code, body := e.do(1, "POST", "/api/inbox/read-all", m{}, &counts); code != 200 || counts["unread"] != 0 {
		t.Fatalf("mark all: %d %s", code, body)
	}
	page = list()
	if len(inboxKind(page, domain.InboxKindShare)) != 2 || page.Unread != 0 {
		t.Fatalf("after read all: %+v", page)
	}
	for _, it := range inboxKind(page, domain.InboxKindShare) {
		if it.ReadAt == nil {
			t.Fatalf("entry not read: %+v", it)
		}
	}

	// No unauthenticated access, no delete route.
	if code, _ := e.do(0, "GET", "/api/inbox", nil, nil); code != 401 {
		t.Fatalf("anonymous read: %d", code)
	}
	if code, _ := e.do(1, "DELETE", fmt.Sprintf("/api/inbox/%d", remark.ID), nil, nil); code != 404 && code != 405 {
		t.Fatalf("delete route exists: %d", code)
	}
}
