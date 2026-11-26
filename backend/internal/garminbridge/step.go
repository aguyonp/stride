package garminbridge

import (
	"errors"
	"fmt"
)

// The steps of a sign-in, in order. A failure names its step so a log line or a message says *where*
// Garmin refused instead of a generic « identifiants refusés » — the difference between « wrong
// password » and « the ticket exchange broke » is the whole debugging session.
const (
	stepForm    = "formulaire"
	stepLogin   = "identifiants"
	stepMFA     = "code"
	stepTicket  = "ticket"
	stepTokens  = "jetons"
	stepProfile = "profil"
)

// ErrFlow is an unexpected answer: a page that is neither a form, nor a code challenge nor a
// ticket. It is a broken flow (Garmin changed something, or a proxy rewrote the page), not a wrong
// password, and not a block either.
var ErrFlow = errors.New("garmin bridge: réponse inattendue de Garmin, le pont doit être mis à jour (le fichier .FIT reste disponible)")

// stepError attaches the failing step to a bridge error, keeping errors.Is working for callers.
type stepError struct {
	step string
	err  error
}

func stepErr(step string, err error) error { return &stepError{step: step, err: err} }

func (e *stepError) Error() string { return fmt.Sprintf("étape « %s » : %v", e.step, e.err) }
func (e *stepError) Unwrap() error { return e.err }

// StepOf returns the step an error happened in, or "" when it carries none.
func StepOf(err error) string {
	var se *stepError
	if errors.As(err, &se) {
		return se.step
	}
	return ""
}
