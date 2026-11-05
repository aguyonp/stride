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

	"stride/internal/apimeter"
	"stride/internal/egress"
	"stride/internal/report"
	"stride/internal/view"
)

const (
	llmTimeout   = 120 * time.Second
	llmMaxTokens = 4096
)

// ErrLLMNotConfigured answers 412 PROVIDER_NOT_CONNECTED on LLM routes.
var ErrLLMNotConfigured = errors.New("IA non configurée")

var llmProviders = []string{"gemini", "anthropic", "openai", "openai_compat"}

var modelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,99}$`)

type llmConfig struct {
	Provider, Model string
	BaseURL         sql.NullString
	KeyEnc          []byte
	KeyHint         sql.NullString
	UpdatedAt       int64
}

func (s *Services) llmConfig(ctx context.Context, uid int64) (*llmConfig, error) {
	c := &llmConfig{}
	err := s.db().QueryRowContext(ctx, `SELECT provider, model, base_url, key_enc, key_hint, updated_at FROM llm_configs WHERE user_id = ?`, uid).
		Scan(&c.Provider, &c.Model, &c.BaseURL, &c.KeyEnc, &c.KeyHint, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

func (c *llmConfig) view() *view.LLMConfig {
	out := &view.LLMConfig{}
	if c == nil {
		return out
	}
	out.Configured = true
	out.Provider, out.Model = &c.Provider, &c.Model
	if c.BaseURL.Valid {
		out.BaseURL = &c.BaseURL.String
	}
	if c.KeyHint.Valid {
		out.KeyHint = &c.KeyHint.String
	}
	out.UpdatedAt = rfc3339(c.UpdatedAt)
	return out
}

func (s *Services) LLMConfig(ctx context.Context, uid int64) (*view.LLMConfig, error) {
	c, err := s.llmConfig(ctx, uid)
	if err != nil {
		return nil, err
	}
	return c.view(), nil
}

type LLMConfigInput struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	BaseURL  *string `json:"baseUrl"`
	APIKey   *string `json:"apiKey"`
}

// PutLLMConfig stores the connector; apiKey omitted keeps the stored key, "" removes it.
func (s *Services) PutLLMConfig(ctx context.Context, uid int64, in LLMConfigInput) (*view.LLMConfig, error) {
	if !oneOfStr(in.Provider, llmProviders) {
		return nil, badRequest("provider invalide (%s).", strings.Join(llmProviders, ", "))
	}
	in.Model = strings.TrimPrefix(strings.TrimSpace(in.Model), "models/")
	if !modelRe.MatchString(in.Model) {
		return nil, badRequest("Modèle invalide (ex. gemini-2.5-flash, claude-sonnet-4-5, gpt-4.1, llama3.1:8b).")
	}
	cur, err := s.llmConfig(ctx, uid)
	if err != nil {
		return nil, err
	}
	var base any
	if in.Provider == "openai_compat" {
		if in.BaseURL == nil || strings.TrimSpace(*in.BaseURL) == "" {
			return nil, badRequest("baseUrl requise pour un serveur compatible OpenAI (ex. http://ollama:11434).")
		}
		u, err := egress.CheckURL(*in.BaseURL)
		if err != nil {
			return nil, badRequest("%s", err.Error())
		}
		if err := egress.PreCheck(u, s.deps.Env.AllowPrivateEgress); err != nil {
			return nil, badRequest("Adresse refusée : les réseaux privés ou locaux sont interdits (activez STRIDE_ALLOW_PRIVATE_EGRESS pour un Ollama local).")
		}
		base = strings.TrimSuffix(strings.TrimRight(u.String(), "/"), "/v1")
	}
	var keyEnc []byte
	var hint any
	switch {
	case in.APIKey == nil && cur != nil:
		keyEnc = cur.KeyEnc
		if cur.KeyHint.Valid {
			hint = cur.KeyHint.String
		}
	case in.APIKey != nil && strings.TrimSpace(*in.APIKey) != "":
		k := strings.TrimSpace(*in.APIKey)
		if len(k) > 512 {
			return nil, badRequest("Clé API trop longue.")
		}
		if keyEnc, err = s.seal("llm", uid, "key", k); err != nil {
			return nil, err
		}
		hint = keyHint(k)
	}
	if len(keyEnc) == 0 && in.Provider != "openai_compat" {
		return nil, badRequest("apiKey requise pour ce fournisseur.")
	}
	_, err = s.db().ExecContext(ctx, `INSERT INTO llm_configs (user_id, provider, model, base_url, key_enc, key_hint, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET provider = excluded.provider, model = excluded.model, base_url = excluded.base_url,
		key_enc = excluded.key_enc, key_hint = excluded.key_hint, updated_at = excluded.updated_at`,
		uid, in.Provider, in.Model, base, keyEnc, hint, s.now().Unix())
	if err != nil {
		return nil, err
	}
	s.log.Info("🤖 Connecteur IA enregistré", "user", uid, "provider", in.Provider)
	return s.LLMConfig(ctx, uid)
}

func (s *Services) DeleteLLMConfig(ctx context.Context, uid int64) error {
	_, err := s.db().ExecContext(ctx, `DELETE FROM llm_configs WHERE user_id = ?`, uid)
	return err
}

// ---------- calls ----------

type llmRequest struct {
	system, prompt string
	maxTokens      int
}

type llmError struct {
	status int
	detail string
	err    error
}

func (e *llmError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("HTTP %d: %s", e.status, e.detail)
}

func (e *llmError) Unwrap() error { return e.err }

func (s *Services) llmEndpoint(c *llmConfig) (string, *http.Client) {
	switch c.Provider {
	case "gemini":
		return s.baseURL("gemini", "https://generativelanguage.googleapis.com") + "/v1beta/models/" + url.PathEscape(c.Model) + ":generateContent", s.llmHTTP
	case "anthropic":
		return s.baseURL("anthropic", "https://api.anthropic.com") + "/v1/messages", s.llmHTTP
	case "openai":
		return s.baseURL("openai", "https://api.openai.com") + "/v1/chat/completions", s.llmHTTP
	}
	return strings.TrimSuffix(c.BaseURL.String, "/v1") + "/v1/chat/completions", s.egress
}

func llmBody(c *llmConfig, r llmRequest) any {
	switch c.Provider {
	case "gemini":
		return map[string]any{
			"systemInstruction": map[string]any{"parts": []any{map[string]string{"text": r.system}}},
			"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": r.prompt}}}},
			"generationConfig":  map[string]any{"maxOutputTokens": r.maxTokens, "temperature": 0.4},
		}
	case "anthropic":
		return map[string]any{"model": c.Model, "max_tokens": r.maxTokens, "system": r.system,
			"messages": []any{map[string]string{"role": "user", "content": r.prompt}}}
	}
	body := map[string]any{"model": c.Model, "messages": []any{
		map[string]string{"role": "system", "content": r.system}, map[string]string{"role": "user", "content": r.prompt}}}
	if c.Provider == "openai" {
		body["max_completion_tokens"] = r.maxTokens
	} else {
		body["max_tokens"] = r.maxTokens
	}
	return body
}

type llmAnswer struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error json.RawMessage `json:"error"`
}

func (a *llmAnswer) text() string {
	var b strings.Builder
	if len(a.Candidates) > 0 {
		for _, p := range a.Candidates[0].Content.Parts {
			b.WriteString(p.Text)
		}
	}
	for _, c := range a.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	if len(a.Choices) > 0 {
		b.WriteString(a.Choices[0].Message.Content)
	}
	return strings.TrimSpace(b.String())
}

func errDetail(raw []byte) string {
	var e struct {
		Error any `json:"error"`
	}
	msg := ""
	if json.Unmarshal(raw, &e) == nil {
		switch v := e.Error.(type) {
		case string:
			msg = v
		case map[string]any:
			msg, _ = v["message"].(string)
		}
	}
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200]) + "…"
	}
	return msg
}

// callLLM runs one completion through the meter (provider llm).
func (s *Services) callLLM(ctx context.Context, uid int64, c *llmConfig, r llmRequest) (string, error) {
	key, err := s.open("llm", uid, "key", c.KeyEnc)
	if err != nil {
		return "", fmt.Errorf("clé IA illisible : %w", err)
	}
	endpoint, hc := s.llmEndpoint(c)
	raw, _ := json.Marshal(llmBody(c, r))
	release, err := s.deps.Meter.Acquire(ctx, apimeter.LLM, uid)
	if err != nil {
		return "", providerErr(apimeter.LLM, err)
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	switch c.Provider {
	case "gemini":
		req.Header.Set("x-goog-api-key", key)
	case "anthropic":
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	t0 := time.Now()
	resp, err := hc.Do(req)
	call := apimeter.Call{Provider: apimeter.LLM, UserID: uid, Endpoint: c.Provider, Duration: time.Since(t0)}
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		call.Err = err
		s.deps.Meter.Observe(call)
		pe := &ProviderError{Provider: apimeter.LLM, Err: err, Timeout: errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err)}
		if egress.IsForbidden(err) {
			pe.Detail = "destination refusée (réseau privé ou local)"
		}
		return "", pe
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	call.Status = resp.StatusCode
	if resp.StatusCode == http.StatusTooManyRequests {
		if sec, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			call.RetryAfter = time.Duration(sec) * time.Second
		}
	}
	if resp.StatusCode >= 300 {
		call.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		s.deps.Meter.Observe(call)
		return "", &ProviderError{Provider: apimeter.LLM, Status: resp.StatusCode, Unauthorized: resp.StatusCode == 401 || resp.StatusCode == 403,
			RateLimited: resp.StatusCode == 429, Detail: errDetail(body), Err: &llmError{status: resp.StatusCode, detail: errDetail(body)}}
	}
	s.deps.Meter.Observe(call)
	var ans llmAnswer
	if err := json.Unmarshal(body, &ans); err != nil {
		return "", &ProviderError{Provider: apimeter.LLM, Status: resp.StatusCode, Detail: "réponse illisible", Err: err}
	}
	text := ans.text()
	if text == "" {
		return "", &ProviderError{Provider: apimeter.LLM, Status: resp.StatusCode, Detail: "réponse vide", Err: errors.New("empty answer")}
	}
	return text, nil
}

// systemDirective is §6.1 without the Markdown quote marker.
func systemDirective() string { return strings.TrimPrefix(report.Directive, "> ") }

func (s *Services) TestLLM(ctx context.Context, uid int64) (*view.LLMTestResult, error) {
	c, err := s.llmConfig(ctx, uid)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrLLMNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	t0 := time.Now()
	text, err := s.callLLM(ctx, uid, c, llmRequest{system: systemDirective(), prompt: "Test de connexion : réponds uniquement « OK ».", maxTokens: 512})
	ms := int(time.Since(t0).Milliseconds())
	if err != nil {
		var pe *ProviderError
		msg := errText(err)
		if errors.As(err, &pe) {
			msg = pe.Message()
		}
		return &view.LLMTestResult{OK: false, LatencyMs: ms, Message: msg}, nil
	}
	return &view.LLMTestResult{OK: true, LatencyMs: ms, Message: "Connexion réussie (" + c.Provider + " · " + c.Model + ") : " + compactText(text, 80)}, nil
}

func compactText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

type AnalyzeInput struct {
	Kind        string          `json:"kind"`
	ActivityID  int64           `json:"activityId"`
	Date        string          `json:"date"`
	ActivityIDs []int64         `json:"activityIds"`
	Weeks       int             `json:"weeks"`
	Include     json.RawMessage `json:"include"`
	Question    string          `json:"question"`
}

func includeOf(raw json.RawMessage) ([]string, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list, strings.Join(list, ",")
	}
	var csv string
	_ = json.Unmarshal(raw, &csv)
	if csv == "" {
		return nil, ""
	}
	return strings.Split(csv, ","), csv
}

// Analyze builds the requested prompt, sends it to the athlete's LLM and stores the answer.
func (s *Services) Analyze(ctx context.Context, uid int64, in AnalyzeInput) (*view.LLMAnalysis, error) {
	c, err := s.llmConfig(ctx, uid)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrLLMNotConfigured
	}
	if r := []rune(strings.TrimSpace(in.Question)); len(r) > 2000 {
		return nil, badRequest("Question trop longue (2000 caractères max).")
	}
	list, csv := includeOf(in.Include)
	var prompt, ref string
	switch in.Kind {
	case "activity":
		if in.ActivityID <= 0 {
			return nil, badRequest("activityId requis.")
		}
		ref = strconv.FormatInt(in.ActivityID, 10)
		prompt, err = s.ActivityReport(ctx, uid, in.ActivityID, csv)
	case "day":
		if ref, err = s.ResolveDate(ctx, uid, in.Date); err == nil {
			prompt, err = s.DayReport(ctx, uid, ref, csv)
		}
	case "multi":
		var res *view.MultiExportResult
		if res, err = s.MultiExport(ctx, uid, in.ActivityIDs, in.Weeks, list); err == nil {
			prompt = res.Markdown
		}
		if len(in.ActivityIDs) > 0 {
			ids := make([]string, len(in.ActivityIDs))
			for i, id := range in.ActivityIDs {
				ids[i] = strconv.FormatInt(id, 10)
			}
			ref = "ids:" + strings.Join(ids, ",")
		} else {
			ref = fmt.Sprintf("weeks:%d", in.Weeks)
		}
	default:
		return nil, badRequest("kind invalide (activity, day, multi).")
	}
	if err != nil {
		return nil, err
	}
	if q := strings.TrimSpace(in.Question); q != "" {
		prompt = strings.TrimRight(prompt, "\n") + "\n\n### QUESTION DE L'ATHLÈTE\n" + q + "\n"
	}
	ctx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	t0 := time.Now()
	text, err := s.callLLM(ctx, uid, c, llmRequest{system: systemDirective(), prompt: prompt, maxTokens: llmMaxTokens})
	if err != nil {
		return nil, err
	}
	a := view.LLMAnalysis{Kind: in.Kind, Ref: ref, Provider: c.Provider, Model: c.Model, Markdown: text,
		PromptTokensEstimate: report.EstimateTokens(prompt), DurationMs: time.Since(t0).Milliseconds()}
	now := s.now().Unix()
	res, err := s.db().ExecContext(context.WithoutCancel(ctx), `INSERT INTO llm_analyses (user_id, kind, ref, provider, model, markdown, prompt_tokens, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, a.Kind, a.Ref, a.Provider, a.Model, a.Markdown, a.PromptTokensEstimate, a.DurationMs, now)
	if err != nil {
		return nil, err
	}
	a.ID, _ = res.LastInsertId()
	a.CreatedAt = *rfc3339(now)
	_, _ = s.db().ExecContext(context.WithoutCancel(ctx), `DELETE FROM llm_analyses WHERE user_id = ? AND id NOT IN
		(SELECT id FROM llm_analyses WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT 200)`, uid, uid)
	s.log.Info("🤖 Analyse IA générée", "user", uid, "kind", a.Kind, "provider", a.Provider, "ms", a.DurationMs)
	return &a, nil
}

// Analyses lists the latest 20 stored analyses, optionally filtered by kind and ref.
func (s *Services) Analyses(ctx context.Context, uid int64, kind, ref string) ([]view.LLMAnalysis, error) {
	q := `SELECT id, kind, ref, provider, model, markdown, prompt_tokens, duration_ms, created_at FROM llm_analyses WHERE user_id = ?`
	args := []any{uid}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	}
	if ref != "" {
		q += ` AND ref = ?`
		args = append(args, ref)
	}
	rows, err := s.db().QueryContext(ctx, q+` ORDER BY created_at DESC, id DESC LIMIT 20`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []view.LLMAnalysis{}
	for rows.Next() {
		var a view.LLMAnalysis
		var at int64
		if err := rows.Scan(&a.ID, &a.Kind, &a.Ref, &a.Provider, &a.Model, &a.Markdown, &a.PromptTokensEstimate, &a.DurationMs, &at); err != nil {
			return nil, err
		}
		a.CreatedAt = *rfc3339(at)
		out = append(out, a)
	}
	return out, rows.Err()
}
