package domain

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/apimeter"
	"stride/internal/egress"
	f "stride/internal/frfmt"
	"stride/internal/queue"
	"stride/internal/view"
)

const (
	notifyWindow  = 48 * time.Hour
	notifyTimeout = 15 * time.Second
)

var notifyKinds = []string{"discord", "telegram", "json"}

var (
	telegramTokenRe = regexp.MustCompile(`^\d{5,}:[A-Za-z0-9_-]{20,}$`)
	telegramChatRe  = regexp.MustCompile(`^(-?\d{3,}|@[A-Za-z0-9_]{4,})$`)
)

type notifySecret struct {
	URL      string `json:"url,omitempty"`
	BotToken string `json:"botToken,omitempty"`
	ChatID   string `json:"chatId,omitempty"`
}

type NotificationInput struct {
	Kind     *string `json:"kind"`
	Label    *string `json:"label"`
	Enabled  *bool   `json:"enabled"`
	URL      *string `json:"url"`
	BotToken *string `json:"botToken"`
	ChatID   *string `json:"chatId"`
}

type notifyTarget struct {
	view.NotificationTarget
	secret notifySecret
}

func notifyField(id int64) string { return strconv.FormatInt(id, 10) }

func (s *Services) notifyTargets(ctx context.Context, uid int64, onlyEnabled bool, id int64) ([]notifyTarget, error) {
	q := `SELECT id, kind, label, enabled, hint, last_status, last_sent_at, last_error, secret_enc FROM notification_targets WHERE user_id = ?`
	args := []any{uid}
	if onlyEnabled {
		q += ` AND enabled = 1`
	}
	if id > 0 {
		q += ` AND id = ?`
		args = append(args, id)
	}
	rows, err := s.db().QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []notifyTarget{}
	for rows.Next() {
		var t notifyTarget
		var enabled int
		var status, lastErr sql.NullString
		var sent sql.NullInt64
		var sealed []byte
		if err := rows.Scan(&t.ID, &t.Kind, &t.Label, &enabled, &t.Hint, &status, &sent, &lastErr, &sealed); err != nil {
			return nil, err
		}
		t.Enabled = enabled != 0
		if status.Valid {
			t.LastStatus = &status.String
		}
		if lastErr.Valid {
			t.LastError = &lastErr.String
		}
		if sent.Valid {
			t.LastSentAt = rfc3339(sent.Int64)
		}
		if raw, err := s.open("notify", uid, notifyField(t.ID), sealed); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), &t.secret)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Services) Notifications(ctx context.Context, uid int64) ([]view.NotificationTarget, error) {
	ts, err := s.notifyTargets(ctx, uid, false, 0)
	if err != nil {
		return nil, err
	}
	out := make([]view.NotificationTarget, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.NotificationTarget)
	}
	return out, nil
}

func (s *Services) notifyTarget(ctx context.Context, uid, id int64) (notifyTarget, error) {
	ts, err := s.notifyTargets(ctx, uid, false, id)
	if err != nil {
		return notifyTarget{}, err
	}
	if len(ts) == 0 {
		return notifyTarget{}, ErrNotFound
	}
	return ts[0], nil
}

func trimPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// applyInput validates in onto t (secrets kept when omitted) and returns the new hint.
func (s *Services) applyInput(t *notifyTarget, in NotificationInput, creating bool) error {
	if in.Kind != nil {
		if !creating && *in.Kind != t.Kind {
			return badRequest("Le type d'une notification ne peut pas être modifié.")
		}
		t.Kind = *in.Kind
	}
	if !oneOfStr(t.Kind, notifyKinds) {
		return badRequest("kind invalide (discord, telegram, json).")
	}
	if in.Label != nil {
		t.Label = strings.TrimSpace(*in.Label)
	}
	if len([]rune(t.Label)) > 60 {
		return badRequest("Libellé trop long (60 caractères max).")
	}
	if in.Enabled != nil {
		t.Enabled = *in.Enabled
	}
	switch t.Kind {
	case "discord", "json":
		if in.URL != nil {
			u, err := egress.CheckURL(*in.URL)
			if err != nil {
				return badRequest("%s", err.Error())
			}
			if err := egress.PreCheck(u, s.deps.Env.AllowPrivateEgress); err != nil {
				return badRequest("Adresse refusée : les réseaux privés ou locaux sont interdits (STRIDE_ALLOW_PRIVATE_EGRESS).")
			}
			t.secret.URL = u.String()
		}
		if t.secret.URL == "" {
			return badRequest("url requise pour une notification %s.", t.Kind)
		}
		u, _ := egress.CheckURL(t.secret.URL)
		t.Hint = u.Host + " · " + keyHint(t.secret.URL)
	case "telegram":
		if in.BotToken != nil {
			t.secret.BotToken = trimPtr(in.BotToken)
		}
		if in.ChatID != nil {
			t.secret.ChatID = trimPtr(in.ChatID)
		}
		if !telegramTokenRe.MatchString(t.secret.BotToken) {
			return badRequest("botToken Telegram invalide (format 123456:ABC…).")
		}
		if !telegramChatRe.MatchString(t.secret.ChatID) {
			return badRequest("chatId Telegram invalide (identifiant numérique ou @canal).")
		}
		t.Hint = "chat " + t.secret.ChatID + " · bot " + keyHint(t.secret.BotToken)
	}
	if t.Label == "" {
		t.Label = map[string]string{"discord": "Discord", "telegram": "Telegram", "json": "Webhook JSON"}[t.Kind]
	}
	return nil
}

func oneOfStr(v string, set []string) bool {
	for _, x := range set {
		if v == x {
			return true
		}
	}
	return false
}

func (s *Services) saveTarget(ctx context.Context, uid int64, t *notifyTarget) error {
	raw, _ := json.Marshal(t.secret)
	now := s.now().Unix()
	return s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		if t.ID == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO notification_targets (user_id, kind, label, enabled, hint, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, uid, t.Kind, t.Label, boolInt(t.Enabled), t.Hint, now, now)
			if err != nil {
				return err
			}
			if t.ID, err = res.LastInsertId(); err != nil {
				return err
			}
		}
		sealed, err := s.seal("notify", uid, notifyField(t.ID), string(raw))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE notification_targets SET label = ?, enabled = ?, hint = ?, secret_enc = ?, updated_at = ?
			WHERE id = ? AND user_id = ?`, t.Label, boolInt(t.Enabled), t.Hint, sealed, now, t.ID, uid)
		return err
	})
}

func (s *Services) CreateNotification(ctx context.Context, uid int64, in NotificationInput) (*view.NotificationTarget, error) {
	var n int
	if err := s.db().QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_targets WHERE user_id = ?`, uid).Scan(&n); err != nil {
		return nil, err
	}
	if n >= 10 {
		return nil, badRequest("10 notifications maximum.")
	}
	t := notifyTarget{NotificationTarget: view.NotificationTarget{Enabled: true}}
	if err := s.applyInput(&t, in, true); err != nil {
		return nil, err
	}
	if err := s.saveTarget(ctx, uid, &t); err != nil {
		return nil, err
	}
	out, err := s.notifyTarget(ctx, uid, t.ID)
	return &out.NotificationTarget, err
}

func (s *Services) PatchNotification(ctx context.Context, uid, id int64, in NotificationInput) (*view.NotificationTarget, error) {
	t, err := s.notifyTarget(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if err := s.applyInput(&t, in, false); err != nil {
		return nil, err
	}
	if err := s.saveTarget(ctx, uid, &t); err != nil {
		return nil, err
	}
	out, err := s.notifyTarget(ctx, uid, id)
	return &out.NotificationTarget, err
}

func (s *Services) DeleteNotification(ctx context.Context, uid, id int64) error {
	res, err := s.db().ExecContext(ctx, `DELETE FROM notification_targets WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- sending ----------

type notifyMessage struct {
	text    func(bold func(string) string, esc func(string) string) string
	payload any
}

func telegramEscape(s string) string {
	return strings.NewReplacer("_", "\\_", "*", "\\*", "`", "\\`", "[", "\\[").Replace(s)
}

func prepareWebhookPayload(targetID int64, msg notifyMessage) webhookDeliverPayload {
	discordText := msg.text(func(x string) string { return "**" + x + "**" }, func(x string) string { return x })
	if r := []rune(discordText); len(r) > 1900 {
		discordText = string(r[:1900]) + "…"
	}
	telegramText := msg.text(func(x string) string { return "*" + telegramEscape(x) + "*" }, telegramEscape)
	var jp map[string]any
	if m, ok := msg.payload.(map[string]any); ok {
		jp = m
	} else if raw, err := json.Marshal(msg.payload); err == nil {
		_ = json.Unmarshal(raw, &jp)
	}
	return webhookDeliverPayload{
		TargetID:     targetID,
		DiscordText:  discordText,
		TelegramText: telegramText,
		JSONPayload:  jp,
	}
}

func (s *Services) deliverPrepared(ctx context.Context, uid int64, t notifyTarget, p webhookDeliverPayload) (int, error) {
	var endpoint string
	var body any
	switch t.Kind {
	case "discord":
		endpoint = t.secret.URL
		body = map[string]any{"content": p.DiscordText}
	case "telegram":
		endpoint = s.telegramBase() + "/bot" + t.secret.BotToken + "/sendMessage"
		body = map[string]any{"chat_id": t.secret.ChatID, "parse_mode": "Markdown", "disable_web_page_preview": true,
			"text": p.TelegramText}
	default:
		endpoint, body = t.secret.URL, p.JSONPayload
	}
	raw, _ := json.Marshal(body)
	release, err := s.deps.Meter.Acquire(ctx, apimeter.Webhook, uid)
	if err != nil {
		return 0, err
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Stride/4")
	t0 := time.Now()
	resp, err := s.egressHTTP().Do(req)
	call := apimeter.Call{Provider: apimeter.Webhook, UserID: uid, Endpoint: "notify/" + t.Kind, Duration: time.Since(t0), Err: err}
	if err != nil {
		s.deps.Meter.Observe(call)
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	call.Status = resp.StatusCode
	s.deps.Meter.Observe(call)
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func (s *Services) send(ctx context.Context, uid int64, t notifyTarget, msg notifyMessage) (int, error) {
	return s.deliverPrepared(ctx, uid, t, prepareWebhookPayload(t.ID, msg))
}

// sendErrText never echoes the target URL (Discord and Telegram secrets live in it).
func sendErrText(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	switch {
	case egress.IsForbidden(err):
		return "Destination refusée (réseau privé ou local)."
	case errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err):
		return "Délai dépassé."
	}
	return err.Error()
}

func (s *Services) recordSend(ctx context.Context, uid, id int64, err error) {
	status, msg := "ok", any(nil)
	if err != nil {
		status, msg = "error", sendErrText(err)
	}
	_, _ = s.db().ExecContext(ctx, `UPDATE notification_targets SET last_status = ?, last_sent_at = ?, last_error = ? WHERE id = ? AND user_id = ?`,
		status, s.now().Unix(), msg, id, uid)
}

// TestNotification sends a test message to one target.
func (s *Services) TestNotification(ctx context.Context, uid, id int64) (*view.NotifyTestResult, error) {
	t, err := s.notifyTarget(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	link := s.deps.Env.PublicURL + "/"
	msg := notifyMessage{
		text: func(bold, esc func(string) string) string {
			return "✅ " + bold("Stride") + esc(" : notification de test. Les prochaines séances synchronisées arriveront ici.") + "\n" + link
		},
		payload: map[string]any{"event": "test", "message": "Notification de test Stride", "url": link},
	}
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()
	status, err := s.send(ctx, uid, t, msg)
	s.recordSend(context.WithoutCancel(ctx), uid, id, err)
	if err != nil {
		return &view.NotifyTestResult{OK: false, Status: status, Message: "Échec de l'envoi : " + sendErrText(err)}, nil
	}
	return &view.NotifyTestResult{OK: true, Status: status, Message: "Notification envoyée."}, nil
}

func activityMessage(v *view.ActivityView, link string) notifyMessage {
	a := v.Activity
	var alerts []analysis.Alert
	for _, al := range v.Alerts {
		if (al.Level == "warn" || al.Level == "danger") && len(alerts) < 3 {
			alerts = append(alerts, al)
		}
	}
	if alerts == nil {
		alerts = []analysis.Alert{}
	}
	return notifyMessage{
		text: func(bold, esc func(string) string) string {
			var b strings.Builder
			title := a.TypeLabel
			if a.Name != "" {
				title += " · " + a.Name
			}
			b.WriteString("🏃 " + bold(title) + "\n")
			b.WriteString(esc(fmt.Sprintf("%s · %s · %s", f.Km(f.P(a.DistanceM), 2), f.Duration(f.P(a.DurationS)), f.Pace(f.P(a.AvgPaceSecPerKm))) + "\n"))
			if ef := v.Efficiency; ef != nil && ef.EfSpeed != nil {
				b.WriteString(esc("EF "+f.Dec(*ef.EfSpeed, 2)+" m/min/bpm") + "\n")
			}
			if im := v.Impact; im != nil && im.Steps != nil {
				b.WriteString(esc(fmt.Sprintf("Impacts %s foulées · indice %s (7 j : %s)", f.Int(*im.Steps), f.Dec(f.P(im.JointIndex), 1),
					f.Dec(f.P(im.JointIndex7d), 1))) + "\n")
			}
			for _, al := range alerts {
				b.WriteString(esc("⚠️ "+al.Title) + "\n")
			}
			b.WriteString(link)
			return b.String()
		},
		payload: map[string]any{"event": "activity.synced", "activity": a.ActivitySummary, "efficiency": v.Efficiency, "impact": v.Impact,
			"alerts": alerts, "url": link},
	}
}

// notifyPending sends, once, the recent complete activities to the enabled targets. When both
// providers are connected, a single-source activity waits for the fusion until it is final.
func (s *Services) notifyPending(ctx context.Context, uid int64) {
	targets, err := s.notifyTargets(ctx, uid, true, 0)
	if err != nil || len(targets) == 0 {
		return
	}
	rows, err := s.queryActivities(ctx, `WHERE user_id = ? AND notified_at IS NULL AND start_utc >= ? ORDER BY start_utc`,
		uid, s.now().Add(-notifyWindow).Unix())
	if err != nil || len(rows) == 0 {
		return
	}
	gc, _ := s.getConn(ctx, uid, pGarmin)
	sc, _ := s.getConn(ctx, uid, pStrava)
	both := gc.connected() && sc.connected()
	for _, a := range rows {
		v, err := s.ActivityView(ctx, uid, a.ID)
		if err != nil {
			continue
		}
		cur, err := s.activity(ctx, uid, a.ID)
		if err != nil || !cur.Complete || (both && (cur.GarminID == nil || cur.StravaID == nil) && !cur.Final) {
			continue
		}
		res, err := s.db().ExecContext(ctx, `UPDATE activities SET notified_at = ? WHERE id = ? AND user_id = ? AND notified_at IS NULL`,
			s.now().Unix(), a.ID, uid)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		msg := activityMessage(v, fmt.Sprintf("%s/?s=%d", s.deps.Env.PublicURL, a.ID))
		for _, t := range targets {
			payload := prepareWebhookPayload(t.ID, msg)
			if _, err := s.queue.Enqueue(context.Background(), queue.EnqueueRequest{
				UserID:      uid,
				Kind:        queue.KindWebhookDeliver,
				Queue:       queue.QueueDefault,
				Label:       "Notification " + t.Label,
				MaxAttempts: 3,
				Payload:     payload,
			}); err != nil {
				s.log.Warn("🔔 Mise en file de notification en échec", "user", uid, "kind", t.Kind, "err", errText(err))
			}
		}
		s.log.Info("🔔 Séance mise en file pour notification", "user", uid, "cibles", len(targets))
	}
}
