package domainapi_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"stride/internal/analysis"
	"stride/internal/domain"
)

func TestCyclesEngine(t *testing.T) {
	e := setup(t)

	// 1. Create plan with autoScaffold: true
	planIn := domain.TrainingPlanInput{
		Name:               "Marathon de Paris 2026",
		StartDate:          "2026-01-05",
		EndDate:            "2026-04-12",
		AutoScaffold:       func() *bool { b := true; return &b }(),
		PeakWeeklyVolumeKm: func() *float64 { v := 85.0; return &v }(),
		PeakWeeklyLoad:     func() *float64 { v := 550.0; return &v }(),
	}

	var overview domain.TrainingCycleOverview
	code, raw := e.do(2, "POST", "/api/cycles/plans", planIn, &overview)
	if code != http.StatusCreated {
		t.Fatalf("POST /api/cycles/plans: %d (%s)", code, raw)
	}
	if overview.ActivePlan == nil || overview.ActivePlan.ID <= 0 {
		t.Fatalf("expected active plan created: %+v", overview.ActivePlan)
	}
	planID := overview.ActivePlan.ID
	if len(overview.Phases) != 4 {
		t.Fatalf("expected 4 auto-scaffolded phases, got %d", len(overview.Phases))
	}
	if len(overview.Phases[0].Weeks) == 0 {
		t.Fatalf("expected generated weeks in phase 0, got 0")
	}

	// 2. GET /api/cycles
	var fetched domain.TrainingCycleOverview
	code, raw = e.do(2, "GET", "/api/cycles", nil, &fetched)
	if code != http.StatusOK {
		t.Fatalf("GET /api/cycles: %d (%s)", code, raw)
	}
	if fetched.ActivePlan == nil || fetched.ActivePlan.ID != planID {
		t.Fatalf("GET /api/cycles expected plan ID %d, got %+v", planID, fetched.ActivePlan)
	}

	// 3. PATCH /api/cycles/plans/{id}
	newName := "Marathon de Paris 2026 - Objectif Sub 3h15"
	statusCompleted := domain.PlanStatusCompleted
	patchIn := domain.TrainingPlanInput{
		Name:   newName,
		Status: &statusCompleted,
	}
	var patched domain.TrainingPlan
	code, raw = e.do(2, "PATCH", "/api/cycles/plans/"+int64ToStr(planID), patchIn, &patched)
	if code != http.StatusOK {
		t.Fatalf("PATCH /api/cycles/plans/%d: %d (%s)", planID, code, raw)
	}
	if patched.Name != newName || patched.Status != domain.PlanStatusCompleted {
		t.Fatalf("unexpected patched plan: %+v", patched)
	}

	// 4. POST /api/cycles/phases
	phaseIn := domain.TrainingPhaseInput{
		PlanID:         &planID,
		Name:           "Récupération post-marathon",
		StartDate:      "2026-04-13",
		EndDate:        "2026-04-26",
		WeeklyVolumeKm: func() *float64 { v := 20.0; return &v }(),
	}
	var createdPhase domain.TrainingPhase
	code, raw = e.do(2, "POST", "/api/cycles/phases", phaseIn, &createdPhase)
	if code != http.StatusCreated {
		t.Fatalf("POST /api/cycles/phases: %d (%s)", code, raw)
	}
	if createdPhase.ID <= 0 {
		t.Fatalf("expected phase ID > 0: %+v", createdPhase)
	}

	// 5. PATCH /api/cycles/phases/{id}
	newPhaseName := "Régénération & Balnéothérapie"
	focus := "Footings légers sur herbe"
	patchPhaseIn := domain.TrainingPhaseInput{
		Name:  newPhaseName,
		Focus: &focus,
	}
	var patchedPhase domain.TrainingPhase
	code, raw = e.do(2, "PATCH", "/api/cycles/phases/"+int64ToStr(createdPhase.ID), patchPhaseIn, &patchedPhase)
	if code != http.StatusOK {
		t.Fatalf("PATCH /api/cycles/phases/%d: %d (%s)", createdPhase.ID, code, raw)
	}
	if patchedPhase.Name != newPhaseName || patchedPhase.Focus != focus {
		t.Fatalf("unexpected patched phase: %+v", patchedPhase)
	}

	// 6. PUT /api/cycles/weeks
	targetVol := 45.0
	isAssim := true
	weekNote := "Semaine d'assimilation avancée"
	weekIn := domain.TrainingWeekInput{
		PlanID:         &planID,
		PhaseID:        &overview.Phases[0].ID,
		WeekStart:      "2026-01-19",
		IsAssimilation: &isAssim,
		TargetVolumeKm: &targetVol,
		Note:           &weekNote,
	}
	var putWeek domain.TrainingWeek
	code, raw = e.do(2, "PUT", "/api/cycles/weeks", weekIn, &putWeek)
	if code != http.StatusOK {
		t.Fatalf("PUT /api/cycles/weeks: %d (%s)", code, raw)
	}
	if !putWeek.IsAssimilation || putWeek.TargetVolumeKm == nil || *putWeek.TargetVolumeKm != targetVol {
		t.Fatalf("unexpected putWeek: %+v", putWeek)
	}

	// 7. DELETE /api/cycles/phases/{id}
	code, raw = e.do(2, "DELETE", "/api/cycles/phases/"+int64ToStr(createdPhase.ID), nil, nil)
	if code != http.StatusNoContent {
		t.Fatalf("DELETE /api/cycles/phases/%d: %d (%s)", createdPhase.ID, code, raw)
	}

	// 8. DELETE /api/cycles/plans/{id}
	code, raw = e.do(2, "DELETE", "/api/cycles/plans/"+int64ToStr(planID), nil, nil)
	if code != http.StatusNoContent {
		t.Fatalf("DELETE /api/cycles/plans/%d: %d (%s)", planID, code, raw)
	}
}

func TestBodyStressMapAndCorrelations(t *testing.T) {
	e := setup(t)

	// 1. Initial GET /api/bodymap - should return all 6 canonical zones
	var initial analysis.BodyMapOverview
	code, raw := e.do(2, "GET", "/api/bodymap?days=90", nil, &initial)
	if code != http.StatusOK {
		t.Fatalf("GET /api/bodymap: %d (%s)", code, raw)
	}
	if len(initial.Zones) != 6 {
		t.Fatalf("expected 6 canonical zones, got %d", len(initial.Zones))
	}
	foundKnee := false
	for _, z := range initial.Zones {
		if z.Zone == analysis.ZoneKneePFPS {
			foundKnee = true
			if z.ZoneLabel != "Rotule / Genou (SFP)" {
				t.Fatalf("expected UI label 'Rotule / Genou (SFP)', got %q", z.ZoneLabel)
			}
		}
	}
	if !foundKnee {
		t.Fatalf("ZoneKneePFPS not found in zones")
	}

	// 2. POST /api/bodymap/entries
	today := time.Now().UTC().Format("2006-01-02")
	note := "Légère gêne sur bitume en descente"
	coldScore := 2.5
	side := analysis.SideLeft
	in := analysis.BodyStressInput{
		Date:             today,
		Zone:             analysis.ZoneKneePFPS,
		Side:             &side,
		PainScore:        4.5,
		ColdNextDayScore: &coldScore,
		Note:             &note,
	}
	var created analysis.BodyStressEntry
	code, raw = e.do(2, "POST", "/api/bodymap/entries", in, &created)
	if code != http.StatusCreated {
		t.Fatalf("POST /api/bodymap/entries: %d (%s)", code, raw)
	}
	if created.ID <= 0 || created.PainScore != 4.5 || created.Side != analysis.SideLeft {
		t.Fatalf("unexpected created entry: %+v", created)
	}
	if created.ZoneLabel != "Rotule / Genou (SFP)" {
		t.Fatalf("unexpected entry ZoneLabel: %s", created.ZoneLabel)
	}

	// 3. GET /api/bodymap?days=90
	var overview analysis.BodyMapOverview
	code, raw = e.do(2, "GET", "/api/bodymap?days=90", nil, &overview)
	if code != http.StatusOK {
		t.Fatalf("GET /api/bodymap: %d (%s)", code, raw)
	}
	if len(overview.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(overview.Entries))
	}
	for _, z := range overview.Zones {
		if z.Zone == analysis.ZoneKneePFPS {
			if z.LatestScore == nil || *z.LatestScore != 4.5 {
				t.Fatalf("expected latest score 4.5, got %v", z.LatestScore)
			}
		}
	}

	// 4. PATCH /api/bodymap/entries/{id}
	newPain := 2.0
	newNote := "Disparition progressive de la douleur"
	patchIn := analysis.BodyStressInput{
		Date:      today,
		Zone:      analysis.ZoneKneePFPS,
		PainScore: newPain,
		Note:      &newNote,
	}
	var patched analysis.BodyStressEntry
	code, raw = e.do(2, "PATCH", "/api/bodymap/entries/"+int64ToStr(created.ID), patchIn, &patched)
	if code != http.StatusOK {
		t.Fatalf("PATCH /api/bodymap/entries/%d: %d (%s)", created.ID, code, raw)
	}
	if patched.PainScore != newPain || patched.Note != newNote {
		t.Fatalf("unexpected patched entry: %+v", patched)
	}

	// 5. DELETE /api/bodymap/entries/{id}
	code, raw = e.do(2, "DELETE", "/api/bodymap/entries/"+int64ToStr(created.ID), nil, nil)
	if code != http.StatusNoContent {
		t.Fatalf("DELETE /api/bodymap/entries/%d: %d (%s)", created.ID, code, raw)
	}

	// 6. Verify entry was deleted
	code, raw = e.do(2, "GET", "/api/bodymap?days=90", nil, &overview)
	if code != http.StatusOK {
		t.Fatalf("GET /api/bodymap after delete: %d (%s)", code, raw)
	}
	if len(overview.Entries) != 0 {
		t.Fatalf("expected 0 entries after deletion, got %d", len(overview.Entries))
	}
}

func TestSecureShareLinks(t *testing.T) {
	e := setup(t)

	// 1. Create a share link
	in := m{"label": "Partage Médecin du Sport", "expiresInDays": 30}
	var link domain.ShareLink
	code, raw := e.do(2, "POST", "/api/share", in, &link)
	if code != http.StatusCreated {
		t.Fatalf("POST /api/share: %d (%s)", code, raw)
	}
	if link.ID <= 0 {
		t.Fatalf("expected link ID > 0")
	}
	if !strings.HasPrefix(link.TokenPrefix, "st4_") {
		t.Fatalf("expected TokenPrefix starting with st4_, got %q", link.TokenPrefix)
	}
	if !strings.HasPrefix(link.URL, "/share/st4_") {
		t.Fatalf("expected URL starting with /share/st4_, got %q", link.URL)
	}
	token := strings.TrimPrefix(link.URL, "/share/")

	// 2. GET /api/share (authenticated)
	var links []domain.ShareLink
	code, raw = e.do(2, "GET", "/api/share", nil, &links)
	if code != http.StatusOK {
		t.Fatalf("GET /api/share: %d (%s)", code, raw)
	}
	if len(links) != 1 || links[0].ID != link.ID {
		t.Fatalf("expected 1 share link with ID %d, got %+v", link.ID, links)
	}

	// 3. GET /api/public/share/{token} (unauthenticated, user = 0)
	var pubView domain.PublicShareView
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/public/share/"+token, nil)
	cl := &http.Client{}
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatalf("public share request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public share status: %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("expected Cache-Control: private, no-store, got %q", cc)
	}
	_ = pubView

	// 4. Test expired link returns 410 Gone
	pastTime := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	expiredIn := m{"label": "Lien Expiré", "expiresAt": pastTime}
	var expLink domain.ShareLink
	code, raw = e.do(2, "POST", "/api/share", expiredIn, &expLink)
	if code != http.StatusCreated {
		t.Fatalf("POST /api/share expired: %d (%s)", code, raw)
	}
	expToken := strings.TrimPrefix(expLink.URL, "/share/")
	reqExp, _ := http.NewRequest("GET", e.srv.URL+"/api/public/share/"+expToken, nil)
	respExp, err := cl.Do(reqExp)
	if err != nil {
		t.Fatalf("expired link request: %v", err)
	}
	defer respExp.Body.Close()
	if respExp.StatusCode != http.StatusGone {
		t.Fatalf("expected 410 Gone for expired link, got %d", respExp.StatusCode)
	}

	// 5. DELETE /api/share/{id} (1-click revocation)
	code, raw = e.do(2, "DELETE", "/api/share/"+int64ToStr(link.ID), nil, nil)
	if code != http.StatusNoContent {
		t.Fatalf("DELETE /api/share/%d: %d (%s)", link.ID, code, raw)
	}

	// 6. After revocation, public access must return 404
	reqRevoked, _ := http.NewRequest("GET", e.srv.URL+"/api/public/share/"+token, nil)
	respRevoked, err := cl.Do(reqRevoked)
	if err != nil {
		t.Fatalf("revoked link request: %v", err)
	}
	defer respRevoked.Body.Close()
	if respRevoked.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for revoked link, got %d", respRevoked.StatusCode)
	}
}

func TestJobsAPI(t *testing.T) {
	e := setup(t)

	// 1. GET /api/jobs
	var jobList struct {
		Items []any `json:"items"`
	}
	code, raw := e.do(2, "GET", "/api/jobs", nil, &jobList)
	if code != http.StatusOK {
		t.Fatalf("GET /api/jobs: %d (%s)", code, raw)
	}

	// 2. GET /api/jobs/{id} for non-existent job
	code, _ = e.do(2, "GET", "/api/jobs/missing-job-uuid", nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing job, got %d", code)
	}
}

func int64ToStr(n int64) string {
	return strconv.FormatInt(n, 10)
}
