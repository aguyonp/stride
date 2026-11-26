// inbox.go carries the per-user notification centre (§21, issue #142): the persistent memory of
// the messages that are otherwise only toasts.
//
// Two families feed it for now — the biomechanical / load alerts (emitAlert) and the writes of
// the physio/coach portal (share_notes.go). The toast stays the immediate signal; this table is
// what survives a page change or a reload. Writes happen in the domain so the history exists even
// when no tab is open. Synced activities and background jobs are deliberately out of scope, and
// so are the acknowledgement toasts (« profil enregistré »…): the centre is not a log.
package domain

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"stride/internal/sse"
	"stride/internal/store"
)

// Kinds of entries. `update` is fed by the release checker (§22, issue #143).
const (
	InboxKindAlert  = "alert"
	InboxKindShare  = "share"
	InboxKindUpdate = "update"
)

const (
	// inboxPageDefault / inboxPageMax bound the list the panel asks for.
	inboxPageDefault = 50
	inboxPageMax     = 200
	// Lengths of the stored text (the panel renders one or two lines).
	inboxTitleMax  = 160
	inboxDetailMax = 600
)

// InboxEntry is one message of the centre, as the panel renders it.
type InboxEntry struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`  // alert | share | update
	Level string `json:"level"` // success | info | warn | error
	Title string `json:"title"`
	// Code is the stable identifier the client localizes from (share kind, release version, alert
	// code): the stored title is an English fallback, the interface decides the wording.
	Code string `json:"code,omitempty"`
	// Detail is contextual text: the alert message, the share label, the release date.
	Detail     string  `json:"detail"`
	ActivityID *int64  `json:"activityId,omitempty"`
	Date       *string `json:"date,omitempty"`
	URL        string  `json:"url,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	ReadAt     *string `json:"readAt,omitempty"`
}

// InboxPage is the list answer: the newest entries, the unread count and whether older ones exist.
type InboxPage struct {
	Items   []InboxEntry `json:"items"`
	Unread  int          `json:"unread"`
	HasMore bool         `json:"hasMore"`
}

// inboxColumns is the column order scanInbox expects: one entry, as the panel renders it.
const inboxColumns = `id, kind, level, title, code, detail, url, activity_id, date, created_at, read_at`

// inboxInput is what an emitter provides; the id, the read state and the timestamp are ours.
type inboxInput struct {
	Kind       string
	Level      string
	Title      string
	Detail     string
	Code       string
	URL        string
	ActivityID *int64
	Date       *string
	// DedupKey is stable per (kind, target): refreshing the same target updates the entry
	// instead of stacking a duplicate.
	DedupKey string
	// KeepRead keeps the read state on refresh. An alert that fires again is news (it becomes
	// unread); a release notice is not (§22): re-checking must not bring the banner back.
	KeepRead bool
}

// Inbox lists the newest entries of the user, newest first. before is the id of the last entry of
// the previous page (0 for the first page).
func (s *Services) Inbox(ctx context.Context, uid int64, limit int, before int64) (*InboxPage, error) {
	if limit <= 0 || limit > inboxPageMax {
		limit = inboxPageDefault
	}
	q := `SELECT ` + inboxColumns + ` FROM inbox_entries WHERE user_id = ?`
	args := []any{uid}
	if before > 0 {
		// Same order as the list (created_at DESC, id DESC); the cursor row is resolved on the
		// server so a refreshed entry cannot shift the page under the client.
		q += ` AND (created_at, id) < (SELECT created_at, id FROM inbox_entries WHERE id = ? AND user_id = ?)`
		args = append(args, before, uid)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.rdb().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &InboxPage{Items: []InboxEntry{}}
	for rows.Next() {
		e, err := scanInbox(rows)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	if out.Unread, err = s.UnreadInbox(ctx, uid); err != nil {
		return nil, err
	}
	return out, nil
}

type rowScanner interface{ Scan(...any) error }

func scanInbox(sc rowScanner) (InboxEntry, error) {
	var (
		e          InboxEntry
		activityID sql.NullInt64
		date       sql.NullString
		created    int64
		readAt     sql.NullInt64
	)
	if err := sc.Scan(&e.ID, &e.Kind, &e.Level, &e.Title, &e.Code, &e.Detail, &e.URL, &activityID, &date, &created, &readAt); err != nil {
		return e, err
	}
	if activityID.Valid {
		v := activityID.Int64
		e.ActivityID = &v
	}
	if date.Valid && date.String != "" {
		v := date.String
		e.Date = &v
	}
	e.CreatedAt = *rfc3339(created)
	if readAt.Valid {
		e.ReadAt = rfc3339(readAt.Int64)
	}
	return e, nil
}

// UnreadInbox counts the unread entries of the user (the badge).
func (s *Services) UnreadInbox(ctx context.Context, uid int64) (int, error) {
	var n int
	err := s.rdb().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_entries WHERE user_id = ? AND read_at IS NULL`, uid).Scan(&n)
	return n, err
}

// MarkInboxRead marks the given entries as read and returns the new unread count. Ids of another
// user are silently ignored: the WHERE clause scopes every update to the caller.
func (s *Services) MarkInboxRead(ctx context.Context, uid int64, ids []int64) (int, error) {
	if len(ids) == 0 {
		return s.UnreadInbox(ctx, uid)
	}
	// At most one page of ids per call.
	if len(ids) > inboxPageMax {
		ids = ids[:inboxPageMax]
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := []any{s.now().Unix(), uid}
	for _, id := range ids {
		if id <= 0 {
			return 0, badRequest("Identifiant de notification invalide.")
		}
		args = append(args, id)
	}
	if _, err := s.db().ExecContext(ctx, `UPDATE inbox_entries SET read_at = ? WHERE user_id = ? AND read_at IS NULL AND id IN (`+placeholders+`)`, args...); err != nil {
		return 0, err
	}
	unread, err := s.UnreadInbox(ctx, uid)
	if err != nil {
		return 0, err
	}
	s.publishInboxUpdated(uid, unread)
	return unread, nil
}

// MarkAllInboxRead empties the badge and returns 0.
func (s *Services) MarkAllInboxRead(ctx context.Context, uid int64) (int, error) {
	if _, err := s.db().ExecContext(ctx, `UPDATE inbox_entries SET read_at = ? WHERE user_id = ? AND read_at IS NULL`, s.now().Unix(), uid); err != nil {
		return 0, err
	}
	s.publishInboxUpdated(uid, 0)
	return 0, nil
}

// PurgeInbox applies the admin retention to every user (called when the settings change).
func (s *Services) PurgeInbox(ctx context.Context) (int64, error) {
	days, max := s.inboxRetention(ctx)
	return s.deps.Store.PurgeInbox(ctx, days, max, s.now())
}

// inboxRetention reads the admin settings; a missing Store keeps the compiled defaults.
func (s *Services) inboxRetention(ctx context.Context) (int, int) {
	if s.deps.Store == nil {
		return store.DefaultInboxRetentionDays, store.DefaultInboxRetentionMax
	}
	return s.deps.Store.InboxRetention(ctx)
}

// writeInbox persists one entry and publishes it. Failures are logged, never returned to the
// emitter: a notification must not fail the alert or the portal write that triggered it.
func (s *Services) writeInbox(ctx context.Context, uid int64, in inboxInput) *InboxEntry {
	if uid <= 0 || in.Title == "" || s.deps.Store == nil {
		return nil
	}
	level := in.Level
	switch level {
	case "success", "info", "warn", "error":
	default:
		level = "info"
	}
	title, detail := truncateRunes(in.Title, inboxTitleMax), truncateRunes(in.Detail, inboxDetailMax)
	now := s.now()
	var activityID, date any
	if in.ActivityID != nil && *in.ActivityID > 0 {
		activityID = *in.ActivityID
	}
	if in.Date != nil && *in.Date != "" {
		date = *in.Date
	}
	key := strings.TrimSpace(in.DedupKey)
	// A refreshed entry moves back to the top of the list and becomes unread again; its id (and
	// therefore its place in an open page) never changes.
	q := `INSERT INTO inbox_entries (user_id, kind, level, title, code, detail, url, dedup_key, activity_id, date, created_at, read_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
		ON CONFLICT(user_id, dedup_key) WHERE dedup_key <> '' DO UPDATE SET
			kind = excluded.kind, level = excluded.level, title = excluded.title, code = excluded.code,
			detail = excluded.detail, url = excluded.url, activity_id = excluded.activity_id,
			date = excluded.date, created_at = excluded.created_at`
	if !in.KeepRead {
		q += `, read_at = NULL`
	}
	res, err := s.db().ExecContext(ctx, q, uid, in.Kind, level, title, in.Code, detail, in.URL, key, activityID, date, now.Unix())
	if err != nil {
		s.log.Warn("⚠️ Centre de notifications : écriture impossible", "err", err.Error(), "kind", in.Kind)
		return nil
	}
	id, _ := res.LastInsertId()
	if id == 0 && key != "" {
		// The upsert updated an existing row: read its id back.
		if err := s.rdb().QueryRowContext(ctx, `SELECT id FROM inbox_entries WHERE user_id = ? AND dedup_key = ?`, uid, key).Scan(&id); err != nil {
			s.log.Warn("⚠️ Centre de notifications : entrée introuvable", "err", err.Error())
		}
	}
	entry := &InboxEntry{
		ID:         id,
		Kind:       in.Kind,
		Level:      level,
		Title:      title,
		Code:       in.Code,
		Detail:     detail,
		ActivityID: in.ActivityID,
		Date:       in.Date,
		URL:        in.URL,
		CreatedAt:  now.UTC().Format(time.RFC3339),
	}
	if s.deps.SSE != nil {
		unread, _ := s.UnreadInbox(ctx, uid)
		s.deps.SSE.PublishToUser(uid, sse.EventNotificationCreated, sse.StreamNotificationEvent{Entry: entry, Unread: unread})
	}
	// Retention on write: bounded work, and no scheduled sweep to maintain.
	days, max := s.inboxRetention(ctx)
	if _, err := s.deps.Store.PurgeUserInbox(ctx, uid, days, max, now); err != nil {
		s.log.Warn("⚠️ Centre de notifications : purge impossible", "err", err.Error())
	}
	return entry
}

// WriteUpdateInbox records the release notice for one user (SPEC §22, issue #143). The dedup key
// carries the version: one entry per admin and per version, whatever the number of checks.
func (s *Services) WriteUpdateInbox(ctx context.Context, uid int64, version, publishedAt, url string) *InboxEntry {
	return s.writeInbox(ctx, uid, inboxInput{
		Kind:     InboxKindUpdate,
		Level:    "info",
		Title:    "New version available",
		Code:     version,
		Detail:   publishedAt,
		URL:      url,
		DedupKey: "update:" + version,
		KeepRead: true,
	})
}

// UpdateInboxEntry returns the caller's entry for a version, nil when there is none.
func (s *Services) UpdateInboxEntry(ctx context.Context, uid int64, version string) (*InboxEntry, error) {
	row := s.rdb().QueryRowContext(ctx, `SELECT `+inboxColumns+` FROM inbox_entries WHERE user_id = ? AND dedup_key = ?`, uid, "update:"+version)
	e, err := scanInbox(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Services) publishInboxUpdated(uid int64, unread int) {
	if s.deps.SSE == nil {
		return
	}
	s.deps.SSE.PublishToUser(uid, sse.EventNotificationUpdated, sse.StreamNotificationEvent{Unread: unread})
}

// alertInboxLevel maps the SSE alert levels (info|warn|critical) to the centre's (success|info|warn|error).
func alertInboxLevel(level string) string {
	switch level {
	case "danger", "critical":
		return "error"
	case "warn":
		return "warn"
	default:
		return "info"
	}
}

// alertInboxKey is the stable identity of one alert: same day, same session (or day) and same
// code → one entry, refreshed when the alert fires again after the 6 h SSE window.
func alertInboxKey(date string, activityID *int64, code string) string {
	part := "day"
	if activityID != nil {
		part = "act-" + strconv.FormatInt(*activityID, 10)
	}
	return "alert:" + date + ":" + part + ":" + code
}

// shareInboxKey identifies one portal target: the link plus what it touched. A remark is per
// session, so editing it twice refreshes the same entry.
func shareInboxKey(linkID, activityID int64, kind string) string {
	if activityID > 0 {
		return "share:" + strconv.FormatInt(linkID, 10) + ":" + kind + ":" + strconv.FormatInt(activityID, 10)
	}
	return "share:" + strconv.FormatInt(linkID, 10) + ":" + kind
}

// shareInboxTitle is the English fallback title of a portal write: the panel renders the wording
// from the stored code and its own language (§15.2), exactly like the `share.updated` toast.
func shareInboxTitle(kind string) string {
	switch kind {
	case "directive":
		return "Physio/coach instruction updated"
	case "remark":
		return "New remark on a session"
	case "plan":
		return "Session prescribed by your physio or coach"
	case "event":
		return "Constraint added to your plan"
	default:
		return "Shared file updated"
	}
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
