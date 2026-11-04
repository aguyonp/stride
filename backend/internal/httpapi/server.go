// Package httpapi expose l'API REST (/api/*) et sert le SPA embarqué.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/gzhttp"

	"patella/internal/analysis"
	"patella/internal/config"
	"patella/internal/garmin"
)

// Server regroupe les dépendances HTTP.
type Server struct {
	Cfg      config.Config
	Settings *config.Store
	G        *garmin.Client
	Svc      *analysis.Service
	Log      *slog.Logger
	Version  string
	Static   http.Handler
}

// Handler construit la chaîne complète (recover → logs → auth → gzip → routes).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/settings/test", s.testSettings)
	mux.HandleFunc("GET /api/activities", s.listActivities)
	mux.HandleFunc("GET /api/activities/{id}/analysis", s.activityAnalysis)
	mux.HandleFunc("GET /api/activities/{id}/report", s.activityReport)
	mux.HandleFunc("GET /api/days/{date}/analysis", s.dayAnalysis)
	mux.HandleFunc("GET /api/days/{date}/report", s.dayReport)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Route d'API inconnue.")
	})
	if s.Static != nil {
		mux.Handle("/", s.Static)
	}

	var h http.Handler = mux
	h = gzhttp.GzipHandler(h)
	if s.Cfg.AuthEnabled() {
		h = withBasicAuth(s.Cfg.AuthUser, s.Cfg.AuthPassword, h)
	}
	h = withSecurityHeaders(h)
	h = withLogging(s.Log, h)
	h = withRecover(s.Log, h)
	return h
}

// ---------- réponses ----------

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"error":"Erreur d'encodage JSON.","code":"INTERNAL"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Error: msg, Code: code})
}

func writeText(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, s)
}

// garminError traduit une erreur en réponse HTTP + code du contrat.
func (s *Server) garminError(w http.ResponseWriter, r *http.Request, err error) {
	var ae *garmin.APIError
	switch {
	case errors.Is(err, context.Canceled) && r.Context().Err() != nil:
		return // client parti
	case errors.Is(err, garmin.ErrNoAPIKey):
		writeError(w, http.StatusPreconditionFailed, "NO_API_KEY", "Aucune clé API Garmin configurée : renseignez-la dans les réglages.")
	case garmin.IsUnauthorized(err):
		writeError(w, http.StatusUnauthorized, "GARMIN_UNAUTHORIZED", "Clé API Garmin refusée ou expirée.")
	case garmin.IsNotFound(err):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Ressource introuvable côté Garmin.")
	case garmin.IsTimeout(err):
		writeError(w, http.StatusGatewayTimeout, "GARMIN_UNAVAILABLE", "Garmin ne répond pas (délai dépassé).")
	case errors.As(err, &ae):
		msg := "Garmin est indisponible (erreur réseau)."
		if ae.Status > 0 {
			msg = fmt.Sprintf("Garmin est indisponible (HTTP %d).", ae.Status)
		}
		writeError(w, http.StatusBadGateway, "GARMIN_UNAVAILABLE", msg)
	case errors.Is(err, analysis.ErrAllSourcesFailed):
		writeError(w, http.StatusBadGateway, "GARMIN_UNAVAILABLE", "Aucune donnée Garmin n'a pu être récupérée pour cette date.")
	default:
		s.Log.Error(fmt.Sprintf("❌ %s %s : %v", r.Method, r.URL.Path, err))
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Erreur interne du serveur.")
	}
}

// ---------- handlers ----------

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"version":          s.Version,
		"garminConfigured": s.G.Configured(),
	})
}

type settingsView struct {
	APIKeyConfigured bool           `json:"apiKeyConfigured"`
	APIKeySource     string         `json:"apiKeySource"`
	APIKeyHint       *string        `json:"apiKeyHint"`
	TokenExpiresAt   *string        `json:"tokenExpiresAt"`
	Timezone         string         `json:"timezone"`
	Profile          config.Profile `json:"profile"`
}

// keyHint : "••••" + 4 derniers caractères (clé ≥ 12 caractères), sinon "••••".
func keyHint(key string) *string {
	if key == "" {
		return nil
	}
	h := "••••"
	if r := []rune(key); len(r) >= 12 {
		h += string(r[len(r)-4:])
	}
	return &h
}

func (s *Server) settingsView() settingsView {
	key, src := s.Settings.APIKey()
	v := settingsView{
		APIKeyConfigured: key != "",
		APIKeySource:     src,
		APIKeyHint:       keyHint(key),
		Timezone:         s.Settings.Timezone(),
		Profile:          s.Settings.Profile(),
	}
	if exp := s.G.TokenExpiresAt(); exp != nil {
		t := exp.UTC().Format(time.RFC3339)
		v.TokenExpiresAt = &t
	}
	return v
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsView())
}

type settingsUpdate struct {
	APIKey   *string              `json:"apiKey"`
	Timezone *string              `json:"timezone"`
	Profile  *config.ProfilePatch `json:"profile"`
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body settingsUpdate
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Corps JSON invalide.")
		return
	}
	// 1) validations sans effet de bord
	if body.Timezone != nil && strings.TrimSpace(*body.Timezone) != "" {
		if _, err := time.LoadLocation(strings.TrimSpace(*body.Timezone)); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("Fuseau horaire inconnu : %q.", *body.Timezone))
			return
		}
	}
	if body.Profile != nil {
		if _, err := s.Settings.Profile().Apply(body.Profile); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Profil invalide : "+err.Error())
			return
		}
	}
	// 2) clé API : validée par échange de jeton avant d'être persistée
	if body.APIKey != nil {
		key := strings.TrimSpace(*body.APIKey)
		if key == "" {
			if err := s.Settings.SetAPIKey(""); err != nil {
				s.garminError(w, r, err)
				return
			}
			s.G.InvalidateToken()
			_, src := s.Settings.APIKey()
			s.Log.Info("🔑 Clé API saisie dans l'UI supprimée", "source", src)
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			tok, exp, err := s.G.Exchange(ctx, key)
			cancel()
			if err != nil {
				if garmin.IsUnauthorized(err) {
					s.Log.Warn("🔑 Nouvelle clé API refusée par Garmin")
					writeError(w, http.StatusBadRequest, "GARMIN_UNAUTHORIZED", "Clé API refusée par Garmin.")
					return
				}
				s.garminError(w, r, err)
				return
			}
			if err := s.Settings.SetAPIKey(key); err != nil {
				s.garminError(w, r, err)
				return
			}
			s.G.InvalidateToken()
			s.G.SetToken(tok, exp, key)
			s.Log.Info("🔑 Nouvelle clé API enregistrée (source ui)", "jetonExpire", exp.In(s.Settings.Location()).Format("2006-01-02 15:04"))
		}
	}
	if body.Timezone != nil {
		if err := s.Settings.SetTimezone(*body.Timezone); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
	}
	if body.Profile != nil {
		if err := s.Settings.UpdateProfile(body.Profile); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Profil invalide : "+err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, s.settingsView())
}

func (s *Server) testSettings(w http.ResponseWriter, r *http.Request) {
	key, _ := s.Settings.APIKey()
	if key == "" {
		s.garminError(w, r, garmin.ErrNoAPIKey)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	tok, exp, err := s.G.Exchange(ctx, key)
	if err != nil {
		s.garminError(w, r, err)
		return
	}
	s.G.SetToken(tok, exp, key)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"message":        "Connexion Garmin OK",
		"tokenExpiresAt": exp.UTC().Format(time.RFC3339),
	})
}

func queryInt(r *http.Request, name string, def, min, max int) (int, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("paramètre %s invalide (entier entre %d et %d attendu)", name, min, max)
	}
	return n, nil
}

func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func (s *Server) listActivities(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20, 1, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	start, err := queryInt(r, "start", 0, 0, 100000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if (from != "" && !validDate(from)) || (to != "" && !validDate(to)) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Dates from/to attendues au format AAAA-MM-JJ.")
		return
	}
	resp, err := s.Svc.ListActivities(r.Context(), start, limit, from, to)
	if err != nil {
		s.garminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) parseActivityParams(w http.ResponseWriter, r *http.Request) (int64, analysis.Mode, int, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Identifiant d'activité invalide.")
		return 0, "", 0, false
	}
	mode, ok := analysis.ParseMode(r.URL.Query().Get("mode"))
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Mode invalide (sfp, recovery ou performance).")
		return 0, "", 0, false
	}
	hist, err := queryInt(r, "history", 0, 0, analysis.MaxHistoryDays)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return 0, "", 0, false
	}
	return id, mode, hist, true
}

func (s *Server) activityAnalysis(w http.ResponseWriter, r *http.Request) {
	id, mode, hist, ok := s.parseActivityParams(w, r)
	if !ok {
		return
	}
	a, err := s.Svc.AnalyzeActivity(r.Context(), id, mode, hist)
	if err != nil {
		s.garminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) activityReport(w http.ResponseWriter, r *http.Request) {
	id, mode, hist, ok := s.parseActivityParams(w, r)
	if !ok {
		return
	}
	a, err := s.Svc.AnalyzeActivity(r.Context(), id, mode, hist)
	if err != nil {
		s.garminError(w, r, err)
		return
	}
	writeText(w, a.Report)
}

func (s *Server) parseDayParams(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	date := r.PathValue("date")
	if date == "today" || date == "aujourdhui" {
		date = s.Svc.Today()
	}
	if !validDate(date) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Date attendue au format AAAA-MM-JJ.")
		return "", 0, false
	}
	if date > s.Svc.Today() {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "La date ne peut pas être dans le futur.")
		return "", 0, false
	}
	hist, err := queryInt(r, "history", 0, 0, analysis.MaxHistoryDays)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return "", 0, false
	}
	return date, hist, true
}

func (s *Server) dayAnalysis(w http.ResponseWriter, r *http.Request) {
	date, hist, ok := s.parseDayParams(w, r)
	if !ok {
		return
	}
	d, err := s.Svc.AnalyzeDay(r.Context(), date, hist)
	if err != nil {
		s.garminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) dayReport(w http.ResponseWriter, r *http.Request) {
	date, hist, ok := s.parseDayParams(w, r)
	if !ok {
		return
	}
	d, err := s.Svc.AnalyzeDay(r.Context(), date, hist)
	if err != nil {
		s.garminError(w, r, err)
		return
	}
	writeText(w, d.Report)
}
