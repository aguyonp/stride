// Package feedback collects in-app user feedback (bug, idea, other) and exposes it to administrators.
// Messages and contact e-mails are personal data: they are never logged, only stored and shown to admins.
package feedback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"stride/internal/auth"
	"stride/internal/httpx"
)

const (
	MinMessage   = 5
	MaxMessage   = 4000
	MaxNote      = 2000
	MaxEmail     = 254
	maxBody      = 16 << 10
	RateLimit    = 5
	RateWindow   = time.Hour
	listLimit    = 200
	tokenKeepLen = 8
)

var (
	categories = map[string]bool{"bug": true, "idea": true, "other": true}
	statuses   = map[string]bool{"new": true, "read": true, "done": true}
)

// Context is the non-sensitive environment attached by the app (allowlisted fields only).
type Context struct {
	AppVersion string `json:"appVersion"`
	Route      string `json:"route"`
	Viewport   string `json:"viewport"`
	UserAgent  string `json:"userAgent"`
}

// Item is a feedback entry as administrators see it.
type Item struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"userId"`
	UserName     string    `json:"userName"`
	Category     string    `json:"category"`
	Message      string    `json:"message"`
	ContactOK    bool      `json:"contactOk"`
	ContactEmail *string   `json:"contactEmail"`
	Context      Context   `json:"context"`
	Status       string    `json:"status"`
	AdminNote    string    `json:"adminNote"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Counts struct {
	New   int `json:"new"`
	Read  int `json:"read"`
	Done  int `json:"done"`
	Total int `json:"total"`
}

type Deps struct {
	DB   *sql.DB
	Auth *auth.Service
	Log  *slog.Logger
	Now  func() time.Time
}

type Handler struct{ d Deps }

func New(d Deps) *Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Handler{d: d}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/feedback", h.d.Auth.RequireUser(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/admin/feedback", h.d.Auth.RequireAdmin(http.HandlerFunc(h.list)))
	mux.Handle("PATCH /api/admin/feedback/{id}", h.d.Auth.RequireAdmin(http.HandlerFunc(h.patch)))
}

func (h *Handler) internal(w http.ResponseWriter, what string, err error) {
	h.d.Log.Error("❌ "+what, "err", err.Error())
	httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Erreur interne du serveur.")
}

func bad(w http.ResponseWriter, msg string) {
	httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, msg)
}

// clean drops control and invisible format characters (keeping line breaks and tabs when multiline),
// normalizes line endings and trims.
func clean(s string, multiline bool) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.Map(func(r rune) rune {
		switch {
		case multiline && (r == '\n' || r == '\t'):
			return r
		case r == '\n' || r == '\t':
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == utf8.RuneError:
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// cleanRoute keeps the path only (no query or fragment, which may carry invite codes or OAuth
// parameters) and shortens share tokens to their prefix.
func cleanRoute(s string) string {
	s = clean(s, false)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if !strings.HasPrefix(s, "/") {
		return ""
	}
	if tok, ok := strings.CutPrefix(s, "/share/"); ok && len(tok) > tokenKeepLen {
		s = "/share/" + tok[:tokenKeepLen] + "…"
	}
	return truncate(s, 200)
}

func cleanContext(c Context) Context {
	return Context{
		AppVersion: truncate(clean(c.AppVersion, false), 60),
		Route:      cleanRoute(c.Route),
		Viewport:   truncate(clean(c.Viewport, false), 20),
		UserAgent:  truncate(clean(c.UserAgent, false), 300),
	}
}

func validEmail(s string) bool {
	if s == "" || len(s) > MaxEmail || strings.ContainsAny(s, " <>\"") {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && strings.Contains(s[strings.LastIndex(s, "@")+1:], ".")
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		Category     string  `json:"category"`
		Message      string  `json:"message"`
		ContactOK    bool    `json:"contactOk"`
		ContactEmail *string `json:"contactEmail"`
		Context      Context `json:"context"`
	}
	if err := httpx.Decode(w, r, &body, maxBody); err != nil {
		bad(w, "Corps JSON invalide ou trop volumineux.")
		return
	}
	if !categories[body.Category] {
		bad(w, "Catégorie invalide (bug, idea ou other).")
		return
	}
	msg := clean(body.Message, true)
	if n := utf8.RuneCountInString(msg); n < MinMessage || n > MaxMessage {
		bad(w, "Message requis : entre 5 et 4000 caractères.")
		return
	}
	var email *string
	if body.ContactOK {
		e := ""
		if body.ContactEmail != nil {
			e = strings.TrimSpace(*body.ContactEmail)
		}
		if !validEmail(e) {
			bad(w, "Adresse e-mail invalide : elle est nécessaire pour être recontacté.")
			return
		}
		email = &e
	}
	ctxJSON, _ := json.Marshal(cleanContext(body.Context))
	now := h.d.Now()
	var recent int
	if err := h.d.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM feedback WHERE user_id = ? AND created_at > ?`,
		u.ID, now.Add(-RateWindow).Unix()).Scan(&recent); err != nil {
		h.internal(w, "Comptage des avis", err)
		return
	}
	if recent >= RateLimit {
		w.Header().Set("Retry-After", strconv.Itoa(int(RateWindow.Seconds())))
		httpx.Fail(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "Merci ! Tu as déjà envoyé plusieurs avis : réessaie dans une heure.")
		return
	}
	res, err := h.d.DB.ExecContext(r.Context(), `INSERT INTO feedback (user_id, category, message, contact_ok, contact_email, context_json, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'new', ?, ?)`, u.ID, body.Category, msg, body.ContactOK, email, string(ctxJSON), now.Unix(), now.Unix())
	if err != nil {
		h.internal(w, "Enregistrement d'un avis", err)
		return
	}
	id, _ := res.LastInsertId()
	// Never log the message or the e-mail address.
	h.d.Log.Info("💬 Avis reçu", "userId", u.ID, "avis", id, "catégorie", body.Category, "contact", body.ContactOK)
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "createdAt": time.Unix(now.Unix(), 0).UTC()})
}

const itemCols = `f.id, f.user_id, COALESCE(u.display_name, ''), f.category, f.message, f.contact_ok, f.contact_email, f.context_json,
	f.status, f.admin_note, f.created_at, f.updated_at`

func scanItem(sc interface{ Scan(...any) error }) (Item, error) {
	var it Item
	var email sql.NullString
	var ctxJSON string
	var created, updated int64
	if err := sc.Scan(&it.ID, &it.UserID, &it.UserName, &it.Category, &it.Message, &it.ContactOK, &email, &ctxJSON,
		&it.Status, &it.AdminNote, &created, &updated); err != nil {
		return it, err
	}
	if it.ContactOK && email.Valid {
		it.ContactEmail = &email.String
	}
	_ = json.Unmarshal([]byte(ctxJSON), &it.Context)
	it.CreatedAt, it.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	return it, nil
}

func (h *Handler) counts(ctx context.Context) (Counts, error) {
	var c Counts
	err := h.d.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(status = 'new'), 0), COALESCE(SUM(status = 'read'), 0), COALESCE(SUM(status = 'done'), 0), COUNT(*) FROM feedback`).
		Scan(&c.New, &c.Read, &c.Done, &c.Total)
	return c, err
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where, args := []string{}, []any{}
	switch s := q.Get("status"); {
	case s == "" || s == "all":
	case statuses[s]:
		where, args = append(where, "f.status = ?"), append(args, s)
	default:
		bad(w, "Statut inconnu (new, read, done ou all).")
		return
	}
	switch c := q.Get("category"); {
	case c == "" || c == "all":
	case categories[c]:
		where, args = append(where, "f.category = ?"), append(args, c)
	default:
		bad(w, "Catégorie inconnue (bug, idea, other ou all).")
		return
	}
	sqlq := `SELECT ` + itemCols + ` FROM feedback f LEFT JOIN users u ON u.id = f.user_id`
	if len(where) > 0 {
		sqlq += " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := h.d.DB.QueryContext(r.Context(), sqlq+` ORDER BY f.created_at DESC, f.id DESC LIMIT `+strconv.Itoa(listLimit), args...)
	if err != nil {
		h.internal(w, "Liste des avis", err)
		return
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			h.internal(w, "Lecture des avis", err)
			return
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		h.internal(w, "Lecture des avis", err)
		return
	}
	c, err := h.counts(r.Context())
	if err != nil {
		h.internal(w, "Comptage des avis", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "counts": c})
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		bad(w, "Identifiant invalide.")
		return
	}
	var body struct {
		Status    *string `json:"status"`
		AdminNote *string `json:"adminNote"`
	}
	if err := httpx.Decode(w, r, &body, maxBody); err != nil {
		bad(w, "Corps JSON invalide.")
		return
	}
	if body.Status != nil && !statuses[*body.Status] {
		bad(w, "Statut invalide (new, read ou done).")
		return
	}
	var note *string
	if body.AdminNote != nil {
		n := clean(*body.AdminNote, true)
		if utf8.RuneCountInString(n) > MaxNote {
			bad(w, "Note trop longue (2000 caractères maximum).")
			return
		}
		note = &n
	}
	res, err := h.d.DB.ExecContext(r.Context(), `UPDATE feedback SET status = COALESCE(?, status), admin_note = COALESCE(?, admin_note), updated_at = ? WHERE id = ?`,
		body.Status, note, h.d.Now().Unix(), id)
	if err != nil {
		h.internal(w, "Mise à jour d'un avis", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Avis introuvable.")
		return
	}
	it, err := scanItem(h.d.DB.QueryRowContext(r.Context(), `SELECT `+itemCols+` FROM feedback f LEFT JOIN users u ON u.id = f.user_id WHERE f.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Avis introuvable.")
		return
	}
	if err != nil {
		h.internal(w, "Lecture d'un avis", err)
		return
	}
	admin, _ := auth.UserFrom(r.Context())
	h.d.Log.Info("💬 Avis mis à jour par un administrateur", "adminId", admin.ID, "avis", id, "statut", it.Status)
	httpx.JSON(w, http.StatusOK, it)
}
