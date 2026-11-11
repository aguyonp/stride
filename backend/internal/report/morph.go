package report

import (
	"strings"

	"stride/internal/analysis"
	"stride/internal/athlete"
	f "stride/internal/frfmt"
	"stride/internal/view"
)

// MorphRequest is the input of the « Affiner avec l'IA » prompt of Smart Workout Morphing (§15.5).
type MorphRequest struct {
	Original athlete.PlannedSession
	// Proposal is the deterministic adaptation (nil when no rule applied).
	Proposal *athlete.PlannedSession
	// Constraints are rendered « label : détail » lines.
	Constraints []string
	Note        string
	LoadRatio   *analysis.LoadRatio
	CoachNotes  []athlete.ShareNote
}

// Morph builds the refinement prompt: physio/coach notes first, the athlete state, the planned
// session, today's constraints and the rule-based proposal, then the one-session contract.
func Morph(v *view.DayView, prof athlete.Profile, req MorphRequest) string {
	b := newBuilder()
	writeCoachNotes(b, req.CoachNotes)
	writePlanState(b, v, prof, req.LoadRatio)
	b.section("SÉANCE PRÉVUE")
	b.raw("- " + f.Date(req.Original.Date) + " : " + describePlanned(req.Original, true))
	b.section("CONTRAINTES DU JOUR")
	if len(req.Constraints) == 0 {
		b.raw("- Aucune contrainte détectée.")
	}
	for _, c := range req.Constraints {
		b.raw("- " + c)
	}
	b.section("ADAPTATION PROPOSÉE PAR LES RÈGLES")
	if req.Proposal == nil {
		b.raw("- Aucune.")
	} else {
		b.raw("- " + describePlanned(*req.Proposal, true))
	}
	b.section("DEMANDE")
	b.line("Affine l'adaptation de la séance du %s : respecte strictement les consignes kiné / coach et les contraintes ci-dessus, garde la même date et le même sport, propose **une seule** séance (structure en étapes si elle reste pertinente) et explique brièvement chaque changement dans `instructions`.",
		f.Date(req.Original.Date))
	if n := strings.TrimSpace(req.Note); n != "" {
		b.raw("**Précision de l'athlète :** " + compact(n, 600))
	}
	writeContract(b, false)
	return b.String()
}
