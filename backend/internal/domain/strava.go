package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/provider/strava"
	"stride/internal/queue"
)

// stravaRetryDelays are the waits before re-trying to fuse a Strava activity Garmin does not have yet.
var stravaRetryDelays = []time.Duration{5 * time.Minute, 30 * time.Minute}

const webhookTimeout = 2 * time.Minute

// HandleStravaEvent enqueues webhook processing on the worker queue (the HTTP handler acks at once).
func (s *Services) HandleStravaEvent(ev strava.WebhookEvent) {
	uid, _ := s.userForAthlete(context.Background(), ev.OwnerID)
	_, err := s.queue.Enqueue(context.Background(), queue.EnqueueRequest{
		UserID:      uid,
		Kind:        queue.KindSyncStravaWebhook,
		Queue:       queue.QueueDefault,
		Label:       "Synchronisation webhook Strava",
		MaxAttempts: 3,
		Payload:     stravaWebhookPayload{Event: ev, Attempt: 0},
	})
	if err != nil {
		s.log.Warn("🔗 Mise en file de l'événement Strava impossible", "err", errText(err))
	}
}

func (s *Services) userForAthlete(ctx context.Context, athleteID int64) (int64, error) {
	var uid int64
	err := s.db().QueryRowContext(ctx, `SELECT user_id FROM provider_connections WHERE provider = 'strava' AND external_id = ?`,
		strconv.FormatInt(athleteID, 10)).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return uid, err
}

func (s *Services) processStravaEvent(ctx context.Context, ev strava.WebhookEvent, attempt int) error {
	uid, err := s.userForAthlete(ctx, ev.OwnerID)
	if err != nil {
		return err
	}
	ctx = apimeter.WithUser(ctx, uid)
	if ev.ObjectType == "athlete" {
		if ev.Updates["authorized"] == "false" {
			_, err := s.db().ExecContext(ctx, `UPDATE provider_connections SET status = 'disconnected', secret_enc = NULL, token_enc = NULL,
				last_error = 'Accès révoqué depuis Strava', last_error_at = ?, updated_at = ? WHERE user_id = ? AND provider = 'strava'`,
				s.now().Unix(), s.now().Unix(), uid)
			s.log.Info("🔗 Accès Strava révoqué par l'athlète", "user", uid)
			return err
		}
		return nil
	}
	if ev.ObjectType != "activity" {
		return nil
	}
	if ev.AspectType == "delete" {
		return s.removeStravaActivity(ctx, uid, ev.ObjectID)
	}
	tok, err := s.stravaToken(ctx, uid)
	if err != nil {
		return err
	}
	act, err := s.deps.Strava.GetActivity(ctx, tok, ev.ObjectID)
	if err != nil {
		return providerErr(pStrava, err)
	}
	if act.Manual {
		return nil
	}
	gc, _ := s.getConn(ctx, uid, pGarmin)
	garminOn := gc.connected()
	if garminOn && attempt == 0 {
		// Garmin may already list it: refresh the recent list before fusing.
		day := act.StartDate.UTC().Format("2006-01-02")
		if _, _, err := s.syncGarminList(ctx, uid, dateShift(day, -1), dateShift(day, 1)); err != nil {
			s.log.Warn("⚠️ Liste Garmin indisponible pour la fusion Strava", "user", uid, "err", errText(err))
		}
	}
	insertAlone := !garminOn || attempt >= len(stravaRetryDelays) || s.now().Sub(act.StartDate) > 48*time.Hour
	raw, _ := json.Marshal(act)
	id, _, err := s.upsertStrava(ctx, uid, act, raw, insertAlone)
	if err != nil {
		return err
	}
	if id == 0 {
		delay := stravaRetryDelays[attempt]
		s.log.Info("🔗 Activité Strava en attente de Garmin, nouvelle tentative programmée", "user", uid, "dans", delay.String())
		_, _ = s.queue.Enqueue(context.Background(), queue.EnqueueRequest{
			UserID:      uid,
			Kind:        queue.KindSyncStravaWebhook,
			Queue:       queue.QueueDefault,
			Label:       "Nouvelle tentative de fusion Strava",
			MaxAttempts: 3,
			Delay:       delay,
			Payload:     stravaWebhookPayload{Event: ev, Attempt: attempt + 1},
		})
		return nil
	}
	if st, err := s.deps.Strava.GetStreams(ctx, tok, act.ID); err == nil {
		b, _ := json.Marshal(st)
		_ = s.putBlob(ctx, uid, id, "strava_streams", b)
		if st != nil {
			s.deps.Metrics.AddTelemetryPoints("strava_stream", len(st.TimeS))
		}
	} else {
		s.log.Warn("⚠️ Flux Strava indisponibles", "user", uid, "err", errText(providerErr(pStrava, err)))
	}
	_, _ = s.db().ExecContext(ctx, `UPDATE activities SET bundle_at = MAX(?, COALESCE(bundle_at, 0) + 1), updated_at = ? WHERE id = ? AND user_id = ?`, s.now().Unix(), s.now().Unix(), id, uid)
	s.log.Info("🔗 Activité Strava enregistrée", "user", uid, "aspect", ev.AspectType)
	if err := s.autoAssignShoes(ctx, uid); err != nil {
		s.log.Warn("⚠️ Association automatique des chaussures en échec", "user", uid, "err", errText(err))
	}
	s.notifyPending(ctx, uid)
	return nil
}

func (s *Services) removeStravaActivity(ctx context.Context, uid, stravaID int64) error {
	return s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		var id int64
		var gid sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT id, garmin_id FROM activities WHERE user_id = ? AND strava_id = ?`, uid, stravaID).Scan(&id, &gid)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !gid.Valid {
			_, err = tx.ExecContext(ctx, `DELETE FROM activities WHERE id = ? AND user_id = ?`, id, uid)
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM activity_blobs WHERE activity_id = ? AND user_id = ? AND kind IN ('strava_detail', 'strava_streams')`, id, uid); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE activities SET strava_id = NULL, strava_gear = NULL, updated_at = ? WHERE id = ? AND user_id = ?`, s.now().Unix(), id, uid)
		return err
	})
}

// ImportStravaShoes creates or updates local shoes from the athlete's Strava gear.
func (s *Services) ImportStravaShoes(ctx context.Context, uid int64) ([]athlete.Shoe, error) {
	tok, err := s.stravaToken(ctx, uid)
	if err != nil {
		return nil, err
	}
	gear, err := s.deps.Strava.ListAthleteGear(apimeter.WithUser(ctx, uid), tok)
	if err != nil {
		return nil, providerErr(pStrava, err)
	}
	r := s.repo()
	existing, err := r.Shoes(ctx, uid)
	if err != nil {
		return nil, err
	}
	for _, g := range gear {
		if g.Kind != "" && g.Kind != "shoe" {
			continue
		}
		gid := g.ID
		name := g.Name
		if name == "" {
			name = strings.TrimSpace(g.BrandName + " " + g.ModelName)
		}
		var cur *athlete.Shoe
		for i := range existing {
			if existing[i].StravaGearID != nil && *existing[i].StravaGearID == gid {
				cur = &existing[i]
			}
		}
		if cur == nil {
			for i := range existing {
				if existing[i].StravaGearID == nil && sameShoe(existing[i], name, g.BrandName, g.ModelName) {
					cur = &existing[i]
					cur.StravaGearID = &gid
					break
				}
			}
		}
		if cur == nil {
			sh := athlete.Shoe{Name: name, Brand: g.BrandName, Model: g.ModelName, Type: "soft", StravaGearID: &gid, Retired: g.Retired, Note: g.Description}
			if err := r.CreateShoe(ctx, uid, &sh); err != nil {
				return nil, err
			}
			existing = append(existing, sh)
			cur = &existing[len(existing)-1]
		} else {
			cur.Name, cur.Retired = name, g.Retired
			if g.BrandName != "" {
				cur.Brand = g.BrandName
			}
			if g.ModelName != "" {
				cur.Model = g.ModelName
			}
			if err := r.UpdateShoe(ctx, uid, cur); err != nil {
				return nil, err
			}
		}
		if _, err := s.db().ExecContext(ctx, `UPDATE shoes SET strava_distance_m = ? WHERE id = ? AND user_id = ?`, g.DistanceM, cur.ID, uid); err != nil {
			return nil, err
		}
	}
	if err := s.autoAssignShoes(ctx, uid); err != nil {
		return nil, err
	}
	return r.Shoes(ctx, uid)
}

func dateShift(d string, n int) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}
