package garminbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// The Garmin Connect web API surface the bridge needs, and nothing else: the profile (a cheap way to
// prove the tokens work), the workout service (create, update, list, schedule).

// do performs an authorised call, refreshing the tokens once on a 401.
func (s *Service) do(ctx context.Context, t *Tokens, method, path string, body []byte) ([]byte, int, error) {
	res, status, err := s.call(ctx, t, method, path, body)
	if err != nil || status != http.StatusUnauthorized {
		return res, status, err
	}
	// A 401 means the access token is gone: replay the OAuth1 exchange once, then give up.
	fresh, rerr := s.refresh(ctx, t)
	if rerr != nil {
		return res, status, rerr
	}
	*t = *fresh
	return s.call(ctx, t, method, path, body)
}

func (s *Service) call(ctx context.Context, t *Tokens, method, path string, body []byte) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.opt.APIBase+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	req.Header.Set("NK", "NT")
	req.Header.Set("DI-Backend", "connectapi.garmin.com")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	s.headers(req)
	res, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = res.Body.Close() }()
	out, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 427 {
		return nil, res.StatusCode, ErrBlocked
	}
	return out, res.StatusCode, nil
}

// refresh replays the OAuth1 → OAuth2 exchange with the stored OAuth1 token.
func (s *Service) refresh(ctx context.Context, t *Tokens) (*Tokens, error) {
	if t.RefreshToken == "" {
		return nil, ErrExpired
	}
	fresh, err := s.exchange(ctx, "oauth:"+t.AccessToken+":"+t.RefreshToken, "rafraîchissement")
	if err != nil {
		return nil, ErrExpired
	}
	fresh.DisplayName = t.DisplayName
	return fresh, nil
}

// tokensFor loads the tokens of a user, refreshing them when the stored expiry has passed.
func (s *Service) tokensFor(ctx context.Context, userID int64) (*Tokens, error) {
	t, err := s.opt.Store.Load(ctx, userID)
	if err != nil {
		return nil, err
	}
	if t == nil || t.AccessToken == "" {
		return nil, ErrNotConnected
	}
	if !t.ExpiresAt.IsZero() && s.opt.Now().After(t.ExpiresAt) {
		fresh, err := s.refresh(ctx, t)
		if err != nil {
			return nil, ErrExpired
		}
		*t = *fresh
		if err := s.opt.Store.Save(ctx, userID, *t); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// Push creates or updates the workout then schedules it on req.Date. Idempotent by construction: an
// existing workout is updated in place, never duplicated.
func (s *Service) Push(ctx context.Context, userID int64, req PushRequest) (PushResult, error) {
	if req.Name == "" || len(req.Payload) == 0 || req.Date == "" {
		return PushResult{}, fmt.Errorf("garmin bridge: nom, charge utile et date requis")
	}
	t, err := s.tokensFor(ctx, userID)
	if err != nil {
		return PushResult{}, err
	}
	id, created, err := s.upsertWorkout(ctx, t, req)
	if err != nil {
		return PushResult{}, err
	}
	out := PushResult{WorkoutID: id, Created: created}
	body, _ := json.Marshal(map[string]string{"date": req.Date})
	_, status, err := s.do(ctx, t, http.MethodPost, "/workout-service/schedule/"+strconv.FormatInt(id, 10), body)
	switch {
	case err != nil:
		out.ScheduleErr = err.Error()
	case status >= 300:
		out.ScheduleErr = fmt.Sprintf("Garmin a refusé la planification (HTTP %d)", status)
	default:
		out.Scheduled = true
	}
	// Keep the refreshed token when the call rotated it.
	if err := s.opt.Store.Save(ctx, userID, *t); err != nil {
		s.log().Warn("🔗 Pont Garmin : jetons non enregistrés", "user", userID, "err", err.Error())
	}
	if !out.Scheduled {
		s.log().Warn("🔗 Pont Garmin : séance créée mais non planifiée", "user", userID, "workout", id, "err", out.ScheduleErr)
	}
	return out, nil
}

// upsertWorkout finds an existing workout with the same name, updates it, or creates it.
func (s *Service) upsertWorkout(ctx context.Context, t *Tokens, req PushRequest) (int64, bool, error) {
	id := req.WorkoutID
	if id == 0 {
		found, err := s.findWorkout(ctx, t, req.Name)
		if err != nil {
			return 0, false, err
		}
		id = found
	}
	if id > 0 {
		_, status, err := s.do(ctx, t, http.MethodPut, "/workout-service/workout/"+strconv.FormatInt(id, 10), req.Payload)
		if err != nil {
			return 0, false, err
		}
		switch {
		case status < 300:
			return id, false, nil
		case status == http.StatusNotFound:
			// Deleted in Garmin Connect since the last push: fall through and recreate it.
		default:
			return 0, false, fmt.Errorf("garmin bridge: mise à jour refusée (HTTP %d)", status)
		}
	}
	body, status, err := s.do(ctx, t, http.MethodPost, "/workout-service/workout", req.Payload)
	if err != nil {
		return 0, false, err
	}
	if status >= 300 {
		return 0, false, fmt.Errorf("garmin bridge: création refusée (HTTP %d)", status)
	}
	id = workbookID(body)
	if id == 0 {
		return 0, false, fmt.Errorf("garmin bridge: réponse sans identifiant de séance")
	}
	return id, true, nil
}

// findWorkout looks the deterministic name up in the account.
func (s *Service) findWorkout(ctx context.Context, t *Tokens, name string) (int64, error) {
	body, status, err := s.do(ctx, t, http.MethodGet, "/workout-service/workouts?start=0&limit=200", nil)
	if err != nil {
		return 0, err
	}
	if status >= 300 {
		return 0, fmt.Errorf("garmin bridge: liste des séances indisponible (HTTP %d)", status)
	}
	var rows []struct {
		ID   int64  `json:"workoutId"`
		Name string `json:"workoutName"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, nil // an unexpected listing only costs a duplicate renamed properly
	}
	for _, r := range rows {
		if strings.TrimSpace(r.Name) == strings.TrimSpace(name) {
			return r.ID, nil
		}
	}
	return 0, nil
}

// workbookID reads the workout id of a create answer (the field moved between client versions).
func workbookID(body []byte) int64 {
	var v struct {
		WorkoutID    int64 `json:"workoutId"`
		WorkoutIDAlt int64 `json:"workout_id"`
		ID           int64 `json:"id"`
	}
	if json.Unmarshal(body, &v) != nil {
		return 0
	}
	for _, id := range []int64{v.WorkoutID, v.WorkoutIDAlt, v.ID} {
		if id > 0 {
			return id
		}
	}
	return 0
}

// WorkoutName is the deterministic name of a Stride session in Garmin Connect. Two pushes of the same
// session therefore target the same workout, and the id is not even needed to stay idempotent.
func WorkoutName(title string, sessionID int64, date string) string {
	base := strings.TrimSpace(title)
	if base == "" {
		base = "Séance Stride"
	}
	// Garmin refuses very long names; keep the identity (id + date) and a readable prefix.
	if len([]rune(base)) > 64 {
		base = string([]rune(base)[:64])
	}
	return fmt.Sprintf("%s · Stride %d · %s", base, sessionID, date)
}

// FindWorkoutName exposes the lookup for tests and for a manual "is it already there?" check.
func (s *Service) FindWorkoutName(ctx context.Context, userID int64, name string) (int64, error) {
	t, err := s.tokensFor(ctx, userID)
	if err != nil {
		return 0, err
	}
	return s.findWorkout(ctx, t, name)
}
