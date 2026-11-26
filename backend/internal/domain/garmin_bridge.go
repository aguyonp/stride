package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/garmin"
	"stride/internal/garminbridge"
	"stride/internal/workout"
)

// Experimental Garmin Connect bridge (§23, issue #147).
//
// The Personal API is read-only, so the only way to get a workout onto the watch without a cable is
// the web API the official app uses. That path lives in `internal/garminbridge`; this file is the
// boundary: token storage (encrypted, own provider row), the publisher the domain talks to, the
// per-user lock, the circuit breaker and the notifications.
//
// The whole feature is gated by one switch, the admin setting `garminBridgeEnabled`.

const (
	// bridgeProvider is the provider_connections row holding the sealed tokens: never the same row
	// as the Personal API credentials.
	bridgeProvider = "garmin_web"
	// bridgeFailWindow / bridgeFailLimit trip the per-user breaker.
	bridgeFailWindow = 10 * time.Minute
	bridgeFailLimit  = 3
)

// bridgeTokens adapts the encrypted store to the gateway's TokenStore interface.
type bridgeTokens struct{ s *Services }

func (b bridgeTokens) Load(ctx context.Context, uid int64) (*garminbridge.Tokens, error) {
	blob, err := b.s.deps.Store.BridgeTokens(ctx, uid)
	if err != nil || len(blob) == 0 {
		return nil, err
	}
	plain, err := b.s.open(bridgeProvider, uid, "token", blob)
	if err != nil {
		return nil, err
	}
	var t garminbridge.Tokens
	if err := json.Unmarshal([]byte(plain), &t); err != nil {
		return nil, nil // an unreadable blob is a disconnected bridge, not an error for the user
	}
	return &t, nil
}

func (b bridgeTokens) Save(ctx context.Context, uid int64, t garminbridge.Tokens) error {
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	sealed, err := b.s.seal(bridgeProvider, uid, "token", string(raw))
	if err != nil {
		return err
	}
	return b.s.deps.Store.SaveBridgeTokens(ctx, uid, sealed, t.DisplayName, b.s.now())
}

func (b bridgeTokens) Delete(ctx context.Context, uid int64) error {
	return b.s.deps.Store.DeleteBridgeTokens(ctx, uid)
}

// bridgeService returns the gateway when the admin switch is on. Off means off: no caller may use
// the client, and the routes answer 409 rather than 404 — the feature exists, it is disabled.
func (s *Services) bridgeService(ctx context.Context) *garminbridge.Service {
	if s.bridge == nil || s.deps.Store == nil || !s.deps.Store.GarminBridgeEnabled(ctx) {
		return nil
	}
	return s.bridge
}

// BridgeAvailable reports whether an administrator switched the bridge on for this instance.
func (s *Services) BridgeAvailable(ctx context.Context) bool { return s.bridgeService(ctx) != nil }

// BridgeStatus is what the settings card displays. The user switch is the presence of tokens.
// `Available` tells the interface whether the instance allows the bridge at all, so the card can
// stay hidden instead of guessing from a 404.
func (s *Services) BridgeStatus(ctx context.Context, uid int64) garminbridge.Status {
	svc := s.bridgeService(ctx)
	if svc == nil {
		return garminbridge.Status{State: garminbridge.StateDisconnected, Available: false}
	}
	st := svc.Status(ctx, uid)
	st.Available = true
	return st
}

// BridgeLogin starts (or completes) the in-app sign-in. The password is passed straight through and
// never stored; only the resulting tokens are sealed.
func (s *Services) BridgeLogin(ctx context.Context, uid int64, email, password string) (garminbridge.LoginResult, error) {
	svc := s.bridgeService(ctx)
	if svc == nil {
		return garminbridge.LoginResult{State: garminbridge.StateDisconnected}, garminbridge.ErrDisabled
	}
	res, err := svc.Login(ctx, uid, email, password)
	if err != nil {
		s.logBridge(uid, err)
		s.bridgeFailure(ctx, uid, err, "connexion")
		return res, err
	}
	return res, nil
}

// BridgeLoginMFA finishes a sign-in challenged with a code.
func (s *Services) BridgeLoginMFA(ctx context.Context, uid int64, code string) (garminbridge.LoginResult, error) {
	svc := s.bridgeService(ctx)
	if svc == nil {
		return garminbridge.LoginResult{State: garminbridge.StateDisconnected}, garminbridge.ErrDisabled
	}
	res, err := svc.LoginMFA(ctx, uid, code)
	if err != nil {
		s.logBridge(uid, err)
	}
	if err != nil && !isWall(err) {
		// A wrong code (or an expired pending session) is a normal retry, not an incident.
		return res, err
	}
	if err != nil {
		s.bridgeFailure(ctx, uid, err, "code")
	}
	return res, nil
}

// BridgeDisconnect forgets the tokens.
func (s *Services) BridgeDisconnect(ctx context.Context, uid int64) error {
	svc := s.bridgeService(ctx)
	if svc == nil {
		return garminbridge.ErrDisabled
	}
	s.resetBreaker(uid)
	return svc.Disconnect(ctx, uid)
}

// isWall reports the answers a retry cannot fix: a rate limit or a CAPTCHA.
func isWall(err error) bool {
	return errors.Is(err, garminbridge.ErrBlocked) || errors.Is(err, garminbridge.ErrChallenge)
}

// logBridge names the failing step of a sign-in. It never logs the e-mail, the password or a token:
// only the step and the error, which is what makes the next report actionable.
func (s *Services) logBridge(uid int64, err error) {
	if err == nil {
		return
	}
	s.deps.Log.Warn("🔗 Pont Garmin : connexion refusée",
		"user", uid, "etape", garminbridge.StepOf(err), "erreur", err.Error())
}

// bridgeFailure records an entry in the notification centre for the failures that need the user's
// attention (expired session, blocked login); the toast stays the immediate signal.
func (s *Services) bridgeFailure(ctx context.Context, uid int64, err error, op string) {
	level, code, title, detail := "warn", "", "", ""
	switch {
	case errors.Is(err, garminbridge.ErrBlocked):
		code = "GARMIN_BRIDGE_BLOCKED"
		title = "Garmin rate-limits the bridge sign-in"
		detail = "Garmin answered 429/427 (anti-bot protection). Try again later, or use the .FIT file."
	case errors.Is(err, garminbridge.ErrExpired):
		code = "GARMIN_BRIDGE_EXPIRED"
		title = "Garmin bridge expired"
		detail = "The Garmin session is no longer valid: reconnect the bridge in Settings, or send the .FIT file."
	case errors.Is(err, garminbridge.ErrChallenge):
		code = "GARMIN_BRIDGE_CHALLENGE"
		title = "Garmin asks for an anti-robot check"
		detail = "Garmin wants a human check the bridge cannot answer. Send the .FIT file instead."
	case errors.Is(err, garminbridge.ErrCredentials):
		if op != "code" {
			return // a wrong password is corrected in place, no history needed
		}
		code = "GARMIN_BRIDGE_MFA"
		title = "Garmin code refused"
		detail = "The verification code was not accepted. Start the sign-in again."
	default:
		if op != "envoi" {
			return
		}
		code = "GARMIN_BRIDGE_PUSH"
		title = "Garmin push refused"
		detail = errText(err) + " The .FIT file stays available."
	}
	s.writeInbox(ctx, uid, inboxInput{
		Kind:   InboxKindGarminBridge,
		Level:  level,
		Title:  title,
		Detail: detail,
		Code:   code,
		// Straight to the card that can sign in again: an expired session is fixed there and nowhere
		// else, and the card opens its flow on that section.
		URL:      "/settings#integrations",
		DedupKey: "garmin_bridge:" + code,
	})
}

// ---------- publisher ----------

// WorkoutPublisher is the seam between the domain and the way a session reaches the watch.
type WorkoutPublisher interface {
	// Push creates or updates the workout in Garmin Connect and schedules it; it returns the
	// workout id and an optional note when the workout was created but not scheduled.
	Push(ctx context.Context, uid int64, ps athlete.PlannedSession) (int64, string, error)
}

// publisher picks the bridge when it is on and connected, the Personal API otherwise.
func (s *Services) publisher(ctx context.Context, uid int64) WorkoutPublisher {
	if svc := s.bridgeService(ctx); svc != nil {
		if st := svc.Status(ctx, uid); st.State == garminbridge.StateConnected || st.State == garminbridge.StateExpired {
			return webPublisher{s: s, bridge: svc}
		}
	}
	return personalPublisher{s: s}
}

// personalPublisher is the historical path: the Personal API. It stays "unsupported" because Garmin
// does not expose the workout service on that tier.
type personalPublisher struct{ s *Services }

func (p personalPublisher) Push(ctx context.Context, uid int64, ps athlete.PlannedSession) (int64, string, error) {
	s := p.s
	g, err := s.garminClient(ctx, uid)
	if err != nil {
		return 0, "", err
	}
	// The 404/403 of the Personal API is cached 24 h: no point asking again, the answer is the same
	// and the .FIT fallback is what the user should see.
	if _, ok := s.cacheGet(ctx, capKey(uid)); ok {
		return 0, "", &PushUnsupportedError{Session: ps}
	}
	title := ps.Title
	if title == "" {
		title = "Séance Stride " + ps.Date
	}
	payload, _ := json.Marshal(workout.GarminWorkout(title, ps.Instructions, ps.Sport, ps.Steps))
	ctx2 := apimeter.WithUser(ctx, uid)
	body, err := g.CreateWorkout(ctx2, payload)
	if st := garmin.Status(err); st == http.StatusNotFound || st == http.StatusForbidden {
		s.cacheSet(ctx, capKey(uid), []byte("unsupported"), garminCapTTL)
		return 0, "", &PushUnsupportedError{Session: ps}
	}
	if err != nil {
		return 0, "", providerErr(pGarmin, err)
	}
	var created struct {
		WorkoutID int64 `json:"workoutId"`
	}
	if json.Unmarshal(body, &created) != nil || created.WorkoutID == 0 {
		return 0, "", &ProviderError{Provider: pGarmin, Status: http.StatusOK, Detail: "identifiant de séance absent", Err: errors.New("no workoutId")}
	}
	if _, err := g.ScheduleWorkout(ctx2, created.WorkoutID, ps.Date); err != nil {
		return created.WorkoutID, "Séance créée dans Garmin Connect, mais sa planification au " + ps.Date + " a échoué : planifiez-la depuis Garmin Connect.", nil
	}
	return created.WorkoutID, "", nil
}

// webPublisher is the experimental bridge.
type webPublisher struct {
	s      *Services
	bridge *garminbridge.Service
}

func (p webPublisher) Push(ctx context.Context, uid int64, ps athlete.PlannedSession) (int64, string, error) {
	title := ps.Title
	if title == "" {
		title = "Séance Stride " + ps.Date
	}
	payload, err := json.Marshal(workout.GarminWorkout(title, ps.Instructions, ps.Sport, ps.Steps))
	if err != nil {
		return 0, "", err
	}
	var existing int64
	if ps.Garmin.WorkoutID != nil {
		existing = *ps.Garmin.WorkoutID
	}
	res, err := p.bridge.Push(ctx, uid, garminbridge.PushRequest{
		Name:      garminbridge.WorkoutName(title, ps.ID, ps.Date),
		Date:      ps.Date,
		Payload:   payload,
		WorkoutID: existing,
	})
	if err != nil {
		return 0, "", err
	}
	note := ""
	if !res.Scheduled {
		note = "Séance enregistrée dans Garmin Connect, mais non planifiée : " + res.ScheduleErr
	}
	return res.WorkoutID, note, nil
}

// pushResult is what a push attempt records in `planned_sessions.garmin_*`.
type pushResult struct {
	workoutID int64
	note      string
	err       error
	// session is the reloaded row: the caller and the error payload answer with the recorded state
	// (`pushed`, `unsupported`, `failed`), not with what it was before the attempt.
	session athlete.PlannedSession
}

// pushOnce runs one session through the selected publisher, recording the outcome. Callers hold the
// per-user lock (PushGarmin, PushToday) so two clicks cannot race.
func (s *Services) pushOnce(ctx context.Context, uid int64, ps athlete.PlannedSession) pushResult {
	r := s.repo()
	pub := s.publisher(ctx, uid)
	workoutID, note, err := pub.Push(ctx, uid, ps)
	if err != nil {
		var unsupported *PushUnsupportedError
		if errors.As(err, &unsupported) {
			msg := unsupportedMsg
			_ = r.SetGarminPush(ctx, uid, ps.ID, "unsupported", nil, &msg)
			cur, _ := s.plannedWithMatch(ctx, uid, ps.ID)
			return pushResult{err: &PushUnsupportedError{Session: cur}, session: cur}
		}
		msg := "Échec de l'envoi : " + errText(err)
		_ = r.SetGarminPush(ctx, uid, ps.ID, "failed", nil, &msg)
		s.noteBridgePushFailure(ctx, uid, err)
		cur, _ := s.plannedWithMatch(ctx, uid, ps.ID)
		return pushResult{err: err, session: cur}
	}
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	if err := r.SetGarminPush(ctx, uid, ps.ID, "pushed", &workoutID, notePtr); err != nil {
		return pushResult{err: repoErr(err)}
	}
	cur, _ := s.plannedWithMatch(ctx, uid, ps.ID)
	s.log.Info("⌚ Séance envoyée vers Garmin Connect", "user", uid, "session", ps.ID, "workout", workoutID, "bridge", s.usingBridge(ctx, uid))
	s.resetBreaker(uid)
	return pushResult{workoutID: workoutID, note: note, session: cur}
}

func (s *Services) usingBridge(ctx context.Context, uid int64) bool {
	svc := s.bridgeService(ctx)
	if svc == nil {
		return false
	}
	st := svc.Status(ctx, uid)
	return st.State == garminbridge.StateConnected || st.State == garminbridge.StateExpired
}

// noteBridgePushFailure turns a bridge failure into a notification (Personal API failures keep their
// own status only: they are documented as unsupported).
func (s *Services) noteBridgePushFailure(ctx context.Context, uid int64, err error) {
	if !s.usingBridge(ctx, uid) {
		return
	}
	if isWall(err) || errors.Is(err, garminbridge.ErrExpired) {
		s.tripBreaker(uid)
	}
	s.bridgeFailure(ctx, uid, err, "envoi")
}

// ---------- per-user lock and breaker ----------

type breakerState struct {
	fails int
	last  time.Time
	until time.Time
}

var breakerMu sync.Mutex

func (s *Services) breakerMap() map[int64]*breakerState {
	if s.garminBreaker == nil {
		s.garminBreaker = map[int64]*breakerState{}
	}
	return s.garminBreaker
}

func (s *Services) tripBreaker(uid int64) {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	b := s.breakerMap()[uid]
	if b == nil {
		b = &breakerState{}
		s.breakerMap()[uid] = b
	}
	b.fails++
	b.last = s.now()
	if b.fails >= bridgeFailLimit {
		// A quarter of an hour without trying: Garmin's block is not a minute-long affair.
		b.until = s.now().Add(15 * time.Minute)
		b.fails = 0
	}
}

func (s *Services) resetBreaker(uid int64) {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	delete(s.breakerMap(), uid)
}

// breakerOpen reports whether the user's bridge is paused after repeated failures.
func (s *Services) breakerOpen(uid int64) bool {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	b := s.breakerMap()[uid]
	return b != nil && !b.until.IsZero() && s.now().Before(b.until)
}

// BridgeLocked is the message shown when the breaker is open.
func (s *Services) BridgeLocked(uid int64) bool { return s.breakerOpen(uid) }

// ---------- routing of PushGarmin ----------

// pushable checks the session can be pushed at all (steps, sport).
func pushable(ps athlete.PlannedSession) error {
	if len(ps.Steps) == 0 {
		return badRequest("Cette séance n'a pas d'étapes structurées à envoyer.")
	}
	if ps.Sport != "run" && ps.Sport != "bike" {
		return badRequest("Seules les séances de course ou de vélo peuvent être envoyées.")
	}
	return nil
}

// PushGarmin sends one session through the selected publisher, behind the per-user lock.
func (s *Services) PushGarmin(ctx context.Context, uid, id int64) (athlete.PlannedSession, error) {
	r := s.repo()
	ps, err := r.Planned(ctx, uid, id)
	if err != nil {
		return ps, repoErr(err)
	}
	if err := pushable(ps); err != nil {
		return ps, err
	}
	if s.usingBridge(ctx, uid) && s.breakerOpen(uid) {
		return ps, &ProviderError{Provider: pGarmin, Status: http.StatusTooManyRequests,
			Detail: "Garmin a bloqué les tentatives récentes ; réessayez dans un quart d'heure ou utilisez le fichier .FIT", Err: garminbridge.ErrBlocked}
	}
	// One push per user at a time: a double click returns the same row, not a second workout.
	res := pushResult{session: ps}
	_, err, _ = s.sf.Do(fmt.Sprintf("garmin-push:%d", uid), func() (any, error) {
		res = s.pushOnce(ctx, uid, ps)
		return nil, res.err
	})
	if err != nil {
		// The row is already recorded (unsupported/failed): answer with it, not with the old state.
		return res.session, err
	}
	if res.session.ID != 0 {
		return res.session, nil
	}
	return s.plannedWithMatch(ctx, uid, id)
}

// PushTodayResult is one line of the bulk answer.
type PushTodayResult struct {
	SessionID int64  `json:"sessionId"`
	Title     string `json:"title"`
	Status    string `json:"status"` // pushed | unsupported | failed | skipped
	WorkoutID int64  `json:"workoutId,omitempty"`
	Message   string `json:"message,omitempty"`
}

// PushToday sends every session of a day that has structured steps and is not pushed yet, serially,
// under the same per-user lock as a manual push (§23).
func (s *Services) PushToday(ctx context.Context, uid int64, date string) ([]PushTodayResult, error) {
	if strings.TrimSpace(date) == "" {
		return nil, badRequest("Date manquante.")
	}
	rows, err := s.repo().Plan(ctx, uid, date, date)
	if err != nil {
		return nil, repoErr(err)
	}
	out := make([]PushTodayResult, 0, len(rows))
	_, err, _ = s.sf.Do(fmt.Sprintf("garmin-push-today:%d", uid), func() (any, error) {
		for _, ps := range rows {
			res := PushTodayResult{SessionID: ps.ID, Title: ps.Title}
			switch {
			case len(ps.Steps) == 0 || (ps.Sport != "run" && ps.Sport != "bike"):
				res.Status = "skipped"
				res.Message = "Pas d'étapes structurées à envoyer."
			case ps.Garmin.Status == "pushed":
				res.Status = "skipped"
				res.Message = "Déjà envoyée."
				if ps.Garmin.WorkoutID != nil {
					res.WorkoutID = *ps.Garmin.WorkoutID
				}
			default:
				r := s.pushOnce(ctx, uid, ps)
				if r.err != nil {
					res.Status = "failed"
					res.Message = errText(r.err)
					var unsupported *PushUnsupportedError
					if errors.As(r.err, &unsupported) {
						res.Status = "unsupported"
					}
				} else {
					res.Status = "pushed"
					res.WorkoutID = r.workoutID
					res.Message = r.note
				}
			}
			out = append(out, res)
		}
		return nil, nil
	})
	if err != nil {
		return out, err
	}
	return out, nil
}

// Today is the athlete's current day (their timezone), the default of the bulk push.
func (s *Services) Today(uid int64) string {
	p, err := s.profile(context.Background(), uid)
	if err != nil {
		return s.now().Format("2006-01-02")
	}
	return s.today(p)
}

// BridgeTokenStore is the store the wiring hands to the gateway (main builds it).
func (s *Services) BridgeTokenStore() garminbridge.TokenStore { return bridgeTokens{s: s} }
