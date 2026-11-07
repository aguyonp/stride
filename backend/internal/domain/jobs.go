package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/egress"
	"stride/internal/frfmt"
	"stride/internal/provider/strava"
	"stride/internal/queue"
	"stride/internal/report"
	"stride/internal/sse"
	"stride/internal/view"
)

type syncProviderPayload struct {
	Morning bool `json:"morning"`
}

type stravaWebhookPayload struct {
	Event   strava.WebhookEvent `json:"event"`
	Attempt int                 `json:"attempt"`
}

type importJobPayload struct {
	Filename string `json:"filename"`
	Data     []byte `json:"data"`
}

type llmAnalyzeJobPayload struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Prompt string `json:"prompt"`
	Async  bool   `json:"async"`
}

type llmPlanGenerateJobPayload struct {
	Ref    string `json:"ref"`
	Prompt string `json:"prompt"`
	Async  bool   `json:"async"`
}

type llmPlanAdaptJobPayload struct {
	From         string  `json:"from"`
	To           string  `json:"to"`
	Prompt       string  `json:"prompt"`
	RemainingIDs []int64 `json:"remainingIds"`
	Async        bool    `json:"async"`
}

type webhookDeliverPayload struct {
	TargetID     int64          `json:"targetId"`
	DiscordText  string         `json:"discordText"`
	TelegramText string         `json:"telegramText"`
	JSONPayload  map[string]any `json:"jsonPayload"`
}

// syncJobWait bounds synchronous waits on a job well below the HTTP WriteTimeout (150 s).
const syncJobWait = 100 * time.Second

// awaitJob enqueues req and, unless async, waits for its *T result. It returns the job status
// instead (HTTP 202) when async or when the job outlives syncJobWait.
func awaitJob[T any](ctx context.Context, s *Services, req queue.EnqueueRequest, async bool) (any, error) {
	if async {
		st, err := s.queue.Enqueue(ctx, req)
		if err != nil {
			return nil, err
		}
		return st, nil
	}
	st, res, err := s.queue.EnqueueAndWait(ctx, req, syncJobWait)
	switch {
	case errors.Is(err, queue.ErrPending):
		return st, nil
	case err != nil:
		return nil, err
	}
	if v, ok := res.(*T); ok {
		return v, nil
	}
	return nil, fmt.Errorf("job %s: unexpected result %T", req.Kind, res)
}

// ListJobs returns recent async jobs owned by uid.
func (s *Services) ListJobs(ctx context.Context, uid int64, limit int) (queue.JobListResponse, error) {
	return s.queue.ListJobs(ctx, uid, limit)
}

// GetJob returns a single async job owned by uid (ErrNotFound when absent or owned by another user).
func (s *Services) GetJob(ctx context.Context, uid int64, jobID string) (*queue.JobStatus, error) {
	st, err := s.queue.GetJob(ctx, uid, jobID)
	if errors.Is(err, queue.ErrNotFound) {
		return nil, ErrNotFound
	}
	return st, err
}

// jobErr turns a domain error into a job failure carrying a public message only.
func jobErr(err error, retryable bool) error {
	if retryable {
		return queue.Transient(publicText(err), err)
	}
	return queue.Permanent(publicText(err), err)
}

// publicText is the French message shown for a failed job; raw error texts never leave the process
// (they may embed webhook URLs or bot tokens).
func publicText(err error) string {
	var pe *ProviderError
	var br *BadRequestError
	var ce *ConflictError
	switch {
	case errors.As(err, &pe):
		return pe.Message()
	case errors.As(err, &br):
		return br.Msg
	case errors.As(err, &ce):
		return ce.Msg
	case errors.Is(err, ErrLLMNotConfigured):
		return "IA non configurée : renseignez votre fournisseur dans Réglages › IA."
	case errors.Is(err, ErrNotConnected):
		return "Fournisseur non connecté."
	case errors.Is(err, ErrNotFound):
		return "Ressource introuvable."
	case errors.Is(err, context.DeadlineExceeded):
		return "Délai dépassé."
	}
	return "La tâche a échoué (détails dans les journaux du serveur)."
}

func (s *Services) registerJobs() {
	s.queue.Register(queue.KindSyncProvider, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		var p syncProviderPayload
		if err := json.Unmarshal(jc.Payload, &p); err != nil {
			return nil, jobErr(err, false)
		}
		cctx, cancel := context.WithTimeout(apimeter.WithUser(ctx, jc.UserID), syncTimeout)
		defer cancel()
		err := s.runSyncWithProgress(cctx, jc.UserID, p.Morning, jc)
		if errors.Is(err, errSyncRunning) {
			return map[string]any{"skipped": true}, nil
		}
		if err != nil {
			s.log.Warn("🔄 Synchronisation en échec", "user", jc.UserID, "err", errText(err))
			return nil, jobErr(err, false)
		}
		return map[string]any{"synced": true}, nil
	})

	s.queue.Register(queue.KindSyncResync, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		cctx, cancel := context.WithTimeout(apimeter.WithUser(ctx, jc.UserID), syncTimeout)
		defer cancel()
		jc.UpdateProgress("Resynchronisation complète en cours…", nil, nil, nil)
		err := s.runSyncWithProgress(cctx, jc.UserID, false, jc)
		if errors.Is(err, errSyncRunning) {
			return map[string]any{"skipped": true}, nil
		}
		if err != nil {
			return nil, jobErr(err, false)
		}
		return map[string]any{"resynced": true}, nil
	})

	s.queue.Register(queue.KindSyncHistory, s.runHistory)

	s.queue.Register(queue.KindSyncStravaWebhook, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		var p stravaWebhookPayload
		if err := json.Unmarshal(jc.Payload, &p); err != nil {
			return nil, jobErr(err, false)
		}
		cctx, cancel := context.WithTimeout(ctx, webhookTimeout)
		defer cancel()
		if err := s.processStravaEvent(cctx, p.Event, p.Attempt); err != nil {
			permanent := errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotConnected)
			if !permanent {
				s.log.Warn("🔗 Événement Strava non traité", "type", p.Event.ObjectType, "aspect", p.Event.AspectType, "err", errText(err))
			}
			return nil, jobErr(err, !permanent)
		}
		return map[string]any{"ok": true}, nil
	})

	s.queue.Register(queue.KindImportActivity, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		var p importJobPayload
		if err := json.Unmarshal(jc.Payload, &p); err != nil {
			return nil, jobErr(err, false)
		}
		pct := 25.0
		jc.UpdateProgress("Décodage et analyse de la trace…", nil, nil, &pct)
		res, err := s.importFileExec(ctx, jc.UserID, p.Filename, p.Data)
		if err != nil {
			return nil, jobErr(err, false)
		}
		return res, nil
	})

	s.queue.Register(queue.KindLLMAnalyze, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		var p llmAnalyzeJobPayload
		if err := json.Unmarshal(jc.Payload, &p); err != nil {
			return nil, jobErr(err, false)
		}
		a, err := s.execAnalyzeLLM(ctx, jc, p)
		if err != nil {
			s.emitLLMFailed(jc, p.Kind, p.Ref, err)
			return nil, jobErr(err, false)
		}
		s.emitLLMDone(jc.UserID, sse.StreamLLMDoneEvent{JobID: jc.ID, Kind: p.Kind, Ref: p.Ref, Analysis: a})
		return a, nil
	})

	s.queue.Register(queue.KindLLMPlanGenerate, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		var p llmPlanGenerateJobPayload
		if err := json.Unmarshal(jc.Payload, &p); err != nil {
			return nil, jobErr(err, false)
		}
		text, err := s.runLLMJob(ctx, jc, "Génération du plan d'entraînement IA…", p.Prompt, p.Async, "plan_generate", p.Ref)
		if err != nil {
			return nil, err
		}
		drafts, phases, errs := athlete.ParsePlan(text, "llm")
		out := &PlanDraftsResult{Drafts: drafts, Phases: phases, Errors: errs, Raw: text}
		s.emitLLMDone(jc.UserID, sse.StreamLLMDoneEvent{JobID: jc.ID, Kind: "plan_generate", Ref: p.Ref, PlanDrafts: out})
		return out, nil
	})

	s.queue.Register(queue.KindLLMPlanAdapt, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		var p llmPlanAdaptJobPayload
		if err := json.Unmarshal(jc.Payload, &p); err != nil {
			return nil, jobErr(err, false)
		}
		text, err := s.runLLMJob(ctx, jc, "Adaptation du plan IA aux imprévus…", p.Prompt, p.Async, "plan_adapt", p.From)
		if err != nil {
			return nil, err
		}
		out := adaptFromLLM(text, p)
		s.emitLLMDone(jc.UserID, sse.StreamLLMDoneEvent{JobID: jc.ID, Kind: "plan_adapt", Ref: p.From, AdaptPreview: out})
		return out, nil
	})

	s.queue.Register(queue.KindWebhookDeliver, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		var p webhookDeliverPayload
		if err := json.Unmarshal(jc.Payload, &p); err != nil {
			return nil, jobErr(err, false)
		}
		t, err := s.notifyTarget(ctx, jc.UserID, p.TargetID)
		if err != nil {
			return nil, jobErr(err, false)
		}
		sctx, cancel := context.WithTimeout(ctx, notifyTimeout)
		defer cancel()
		status, err := s.deliverPrepared(sctx, jc.UserID, t, p)
		retry := err != nil && webhookRetryable(status, err) && jc.Attempt < jc.MaxAttempts
		if !retry {
			s.recordSend(context.WithoutCancel(ctx), jc.UserID, t.ID, err)
		}
		if err != nil {
			s.log.Warn("🔔 Notification en échec", "user", jc.UserID, "kind", t.Kind, "attempt", jc.Attempt, "err", sendErrText(err))
			msg := "Échec de l'envoi : " + sendErrText(err)
			if retry {
				return nil, queue.Transient(msg, err)
			}
			return nil, queue.Permanent(msg, err)
		}
		return map[string]any{"status": status}, nil
	})
}

// runLLMJob calls the athlete's LLM for a plan job, streaming tokens over SSE.
func (s *Services) runLLMJob(ctx context.Context, jc *queue.JobContext, label, prompt string, async bool, kind, ref string) (string, error) {
	c, err := s.llmConfig(ctx, jc.UserID)
	if err == nil && c == nil {
		err = ErrLLMNotConfigured
	}
	if err != nil {
		s.emitLLMFailed(jc, kind, ref, err)
		return "", jobErr(err, false)
	}
	cctx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	jc.UpdateProgress(label, nil, nil, nil)
	text, err := s.callLLMStream(cctx, jc.UserID, c, llmRequest{system: systemDirective(), prompt: prompt, maxTokens: llmMaxTokens},
		async || s.hasSSESubscribers(jc.UserID), func(delta string) { s.emitLLMToken(jc.UserID, jc.ID, kind, ref, delta) })
	if err != nil {
		s.emitLLMFailed(jc, kind, ref, err)
		return "", jobErr(err, false)
	}
	return text, nil
}

func adaptFromLLM(text string, p llmPlanAdaptJobPayload) *AdaptResult {
	drafts, _, errs := athlete.ParsePlan(text, "llm")
	out := &AdaptResult{Sessions: []athlete.PlannedSession{}, RemoveIDs: []int64{}}
	for _, d := range drafts {
		if d.Date >= p.From && d.Date <= p.To {
			out.Sessions = append(out.Sessions, d)
		} else {
			errs = append(errs, fmt.Sprintf("Séance du %s hors période ignorée", frfmt.Date(d.Date)))
		}
	}
	if len(out.Sessions) == 0 {
		out.Summary = "L'IA n'a proposé aucune séance exploitable : plan inchangé."
	} else {
		out.RemoveIDs = append(out.RemoveIDs, p.RemainingIDs...)
		out.Summary = fmt.Sprintf("Proposition de l'IA : %s du %s au %s, en remplacement des %s.",
			plural(len(out.Sessions), "séance"), frfmt.Date(p.From), frfmt.Date(p.To), plural(len(p.RemainingIDs), "séance prévue"))
	}
	if len(errs) > 0 {
		out.Summary += " Avertissements : " + strings.Join(errs, " ; ") + "."
	}
	return out
}

// webhookRetryable retries only what was certainly not delivered: refused connections, 429 and 5xx.
// A timeout after the request left may have been delivered, so it is not retried.
func webhookRetryable(status int, err error) bool {
	if status == http.StatusTooManyRequests || status >= 500 {
		return true
	}
	if status > 0 || egress.IsForbidden(err) {
		return false
	}
	var cd *apimeter.CooldownError
	var op *net.OpError
	return errors.As(err, &cd) || (errors.As(err, &op) && op.Op == "dial")
}

func (s *Services) emitLLMFailed(jc *queue.JobContext, kind, ref string, err error) {
	msg := publicText(err)
	s.emitLLMDone(jc.UserID, sse.StreamLLMDoneEvent{JobID: jc.ID, Kind: kind, Ref: ref, Error: &msg})
}

func (s *Services) execAnalyzeLLM(ctx context.Context, jc *queue.JobContext, p llmAnalyzeJobPayload) (*view.LLMAnalysis, error) {
	c, err := s.llmConfig(ctx, jc.UserID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrLLMNotConfigured
	}
	cctx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	jc.UpdateProgress("Analyse coach IA en cours…", nil, nil, nil)
	t0 := time.Now()
	wantStream := p.Async || s.hasSSESubscribers(jc.UserID)
	text, err := s.callLLMStream(cctx, jc.UserID, c, llmRequest{
		system: systemDirective(), prompt: p.Prompt, maxTokens: llmMaxTokens,
	}, wantStream, func(delta string) {
		s.emitLLMToken(jc.UserID, jc.ID, p.Kind, p.Ref, delta)
	})
	if err != nil {
		return nil, err
	}
	a := view.LLMAnalysis{
		Kind:                 p.Kind,
		Ref:                  p.Ref,
		Provider:             c.Provider,
		Model:                c.Model,
		Markdown:             text,
		PromptTokensEstimate: report.EstimateTokens(p.Prompt),
		DurationMs:           time.Since(t0).Milliseconds(),
	}
	now := s.now().Unix()
	res, err := s.db().ExecContext(context.WithoutCancel(ctx), `INSERT INTO llm_analyses (user_id, kind, ref, provider, model, markdown, prompt_tokens, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, jc.UserID, a.Kind, a.Ref, a.Provider, a.Model, a.Markdown, a.PromptTokensEstimate, a.DurationMs, now)
	if err != nil {
		return nil, err
	}
	a.ID, _ = res.LastInsertId()
	a.CreatedAt = *rfc3339(now)
	_, _ = s.db().ExecContext(context.WithoutCancel(ctx), `DELETE FROM llm_analyses WHERE user_id = ? AND id NOT IN
		(SELECT id FROM llm_analyses WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT 200)`, jc.UserID, jc.UserID)
	s.log.Info("🤖 Analyse IA générée", "user", jc.UserID, "kind", a.Kind, "provider", a.Provider, "ms", a.DurationMs)
	return &a, nil
}

// ---------- SSE real-time broadcast helpers ----------

func (s *Services) hasSSESubscribers(uid int64) bool {
	return s.deps.SSE != nil && s.deps.SSE.HasSubscribers(uid)
}

func (s *Services) emitSyncStatus(ctx context.Context, uid int64) {
	if s.deps.SSE == nil || uid <= 0 {
		return
	}
	if st, err := s.SyncStatus(ctx, uid); err == nil {
		s.deps.SSE.PublishToUser(uid, sse.EventSyncUpdated, st)
	}
}

func (s *Services) emitLLMToken(uid int64, jobID, kind, ref, delta string) {
	if s.deps.SSE == nil || uid <= 0 || delta == "" {
		return
	}
	s.deps.SSE.PublishToUser(uid, sse.EventLLMToken, sse.StreamLLMTokenEvent{
		JobID: jobID, Kind: kind, Ref: ref, Delta: delta,
	})
}

func (s *Services) emitLLMDone(uid int64, ev sse.StreamLLMDoneEvent) {
	if s.deps.SSE == nil || uid <= 0 {
		return
	}
	s.deps.SSE.PublishToUser(uid, sse.EventLLMDone, ev)
}

func mapAlertLevel(l string) string {
	switch l {
	case "danger", "critical":
		return "critical"
	case "warn":
		return "warn"
	default:
		return "info"
	}
}

func (s *Services) emitAlert(ctx context.Context, uid int64, activityID *int64, date, level, code, title, detail string) {
	if s.deps.SSE == nil || uid <= 0 || code == "" {
		return
	}
	actPart := "day"
	if activityID != nil {
		actPart = fmt.Sprint(*activityID)
	}
	dedupKey := userKey(uid, "sse-alert", date, actPart, code)
	if _, seen := s.cacheGet(ctx, dedupKey); seen {
		return
	}
	s.cacheSet(ctx, dedupKey, []byte("1"), 6*time.Hour)

	var idBytes [6]byte
	_, _ = rand.Read(idBytes[:])
	s.deps.SSE.PublishToUser(uid, sse.EventAlertTriggered, sse.StreamAlertEvent{
		ID:         "al_" + hex.EncodeToString(idBytes[:]),
		ActivityID: activityID,
		Date:       date,
		Level:      mapAlertLevel(level),
		Code:       code,
		Title:      title,
		Detail:     detail,
		CreatedAt:  s.now().UTC().Format(time.RFC3339),
	})
}

// emitPostSyncAlerts inspects post-sync ACWR, HRV/readiness, and newly synced sessions to push live SSE alerts.
func (s *Services) emitPostSyncAlerts(ctx context.Context, uid int64, today string, newActivityIDs []int64) {
	if s.deps.SSE == nil {
		return
	}
	if dv, err := s.DayView(ctx, uid, today); err == nil {
		for _, al := range dv.Alerts {
			if al.Level == "warn" || al.Level == "danger" {
				s.emitAlert(ctx, uid, nil, today, al.Level, al.Code, al.Title, al.Message)
			}
		}
	}
	for _, aid := range newActivityIDs {
		v, err := s.ActivityView(ctx, uid, aid)
		if err != nil {
			continue
		}
		idCopy := aid
		for _, al := range v.Alerts {
			if al.Level == "warn" || al.Level == "danger" {
				s.emitAlert(ctx, uid, &idCopy, v.Activity.Date, al.Level, al.Code, al.Title, al.Message)
			}
		}
	}
}
