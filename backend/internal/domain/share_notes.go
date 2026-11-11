package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"stride/internal/athlete"
	"stride/internal/frfmt"
	"stride/internal/sse"
)

// Physio/coach notes written through a share link (§15.1): one directive per link, one remark per
// link and session. Bodies are third-party text: sanitized here, quoted and capped in the prompts.

const (
	maxDirectiveRunes = 2000
	maxRemarkRunes    = 1000
	maxAuthorRunes    = 80
	// noteRemarksDays is how far back the athlete's « active » listing keeps remarks.
	noteRemarksDays = 30
)

// ShareDirectiveInput is the body of PUT /api/public/share/{token}/directive.
type ShareDirectiveInput struct {
	AuthorName   *string                        `json:"authorName"`
	AuthorRole   *string                        `json:"authorRole"`
	Body         string                         `json:"body"`
	ValidUntil   *string                        `json:"validUntil"`
	Restrictions *athlete.DirectiveRestrictions `json:"restrictions"`
}

// ShareRemarkInput is the body of PUT /api/public/share/{token}/activities/{id}/remark.
type ShareRemarkInput struct {
	AuthorName *string `json:"authorName"`
	AuthorRole *string `json:"authorRole"`
	Body       string  `json:"body"`
}

// cleanText drops control and invisible format characters (keeping line breaks and tabs when
// multiline), normalizes line endings and trims: the rules of the feedback form.
func cleanText(s string, multiline bool) string {
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

func noteAuthor(name, role *string) (*string, string, error) {
	r := "other"
	if role != nil && *role != "" {
		if !slices.Contains(athlete.AuthorRoles, *role) {
			return nil, "", badRequest("authorRole invalide (physio, coach, other).")
		}
		r = *role
	}
	if name == nil {
		return nil, r, nil
	}
	n := cleanText(*name, false)
	if utf8.RuneCountInString(n) > maxAuthorRunes {
		return nil, "", badRequest("authorName trop long (%d caractères max).", maxAuthorRunes)
	}
	if n == "" {
		return nil, r, nil
	}
	return &n, r, nil
}

func noteBody(raw string, limit int) (string, error) {
	b := cleanText(raw, true)
	if n := utf8.RuneCountInString(b); n < 1 || n > limit {
		return "", badRequest("Texte requis (1 à %d caractères).", limit)
	}
	return b, nil
}

func validRestrictions(r *athlete.DirectiveRestrictions) error {
	if r == nil {
		return nil
	}
	if r.MaxIntensity != nil && *r.MaxIntensity != "easy" && *r.MaxIntensity != "moderate" {
		return badRequest("restrictions.maxIntensity doit valoir easy, moderate ou null.")
	}
	if r.MaxElevLossM != nil && (*r.MaxElevLossM < 0 || *r.MaxElevLossM > 5000) {
		return badRequest("restrictions.maxElevLossM hors limites [0, 5000].")
	}
	if r.CadenceTargetSpm != nil && (*r.CadenceTargetSpm < 140 || *r.CadenceTargetSpm > 220) {
		return badRequest("restrictions.cadenceTargetSpm hors limites [140, 220].")
	}
	return nil
}

const noteSelect = `SELECT n.id, n.kind, n.share_link_id, n.link_label, n.author_name, n.author_role, n.body, n.activity_id, n.valid_until,
	n.restrictions_json, n.created_at, n.updated_at, COALESCE(a.local_date, ''), COALESCE(a.name, '')
	FROM share_notes n LEFT JOIN activities a ON a.id = n.activity_id`

func scanNote(sc interface{ Scan(...any) error }) (athlete.ShareNote, error) {
	var n athlete.ShareNote
	var link, aid sql.NullInt64
	var author, until, restr sql.NullString
	var created, updated int64
	var date, name string
	if err := sc.Scan(&n.ID, &n.Kind, &link, &n.LinkLabel, &author, &n.AuthorRole, &n.Body, &aid, &until, &restr, &created, &updated,
		&date, &name); err != nil {
		return n, err
	}
	if link.Valid {
		n.LinkID = &link.Int64
	}
	if aid.Valid {
		n.ActivityID = &aid.Int64
	}
	if author.Valid {
		n.AuthorName = &author.String
	}
	if until.Valid {
		n.ValidUntil = &until.String
	}
	if restr.Valid {
		var r athlete.DirectiveRestrictions
		if json.Unmarshal([]byte(restr.String), &r) == nil {
			n.Restrictions = &r
		}
	}
	n.CreatedAt, n.UpdatedAt = *rfc3339(created), *rfc3339(updated)
	if date != "" {
		n.ActivityLabel = frfmt.Date(date) + " · " + name
	}
	return n, nil
}

func (s *Services) queryNotes(ctx context.Context, q string, args ...any) ([]athlete.ShareNote, error) {
	rows, err := s.rdb().QueryContext(ctx, noteSelect+" "+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []athlete.ShareNote{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// linkNotes returns the directive and the remarks (by activity id) written by one link.
func (s *Services) linkNotes(ctx context.Context, uid, linkID int64) (*athlete.ShareNote, map[int64]athlete.ShareNote, error) {
	notes, err := s.queryNotes(ctx, `WHERE n.user_id = ? AND n.share_link_id = ?`, uid, linkID)
	if err != nil {
		return nil, nil, err
	}
	var directive *athlete.ShareNote
	remarks := map[int64]athlete.ShareNote{}
	for _, n := range notes {
		if n.Kind == "directive" {
			directive = &n
		} else if n.ActivityID != nil {
			remarks[*n.ActivityID] = n
		}
	}
	return directive, remarks, nil
}

// activeDirectiveWhere selects directives whose link is neither revoked nor expired and whose
// validity has not ended (args: now unix, today).
const activeDirectiveWhere = `n.kind = 'directive' AND n.share_link_id IN (SELECT id FROM share_links
	WHERE user_id = n.user_id AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?))
	AND (n.valid_until IS NULL OR n.valid_until >= ?)`

func (s *Services) activeDirectives(ctx context.Context, uid int64, today string) ([]athlete.ShareNote, error) {
	return s.queryNotes(ctx, `WHERE n.user_id = ? AND `+activeDirectiveWhere+` ORDER BY n.updated_at DESC, n.id DESC LIMIT 10`,
		uid, s.now().Unix(), today)
}

// ShareNotes lists the notes written about uid: with active, the active directives and the remarks
// of the last 30 days; otherwise every note (latest 200).
func (s *Services) ShareNotes(ctx context.Context, uid int64, active bool) ([]athlete.ShareNote, error) {
	if !active {
		return s.queryNotes(ctx, `WHERE n.user_id = ? ORDER BY n.updated_at DESC, n.id DESC LIMIT 200`, uid)
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	directives, err := s.activeDirectives(ctx, uid, today)
	if err != nil {
		return nil, err
	}
	remarks, err := s.queryNotes(ctx, `WHERE n.user_id = ? AND n.kind = 'remark' AND n.updated_at >= ? ORDER BY n.updated_at DESC, n.id DESC LIMIT 100`,
		uid, s.now().Add(-noteRemarksDays*24*time.Hour).Unix())
	if err != nil {
		return nil, err
	}
	return append(directives, remarks...), nil
}

// DeleteShareNote lets the athlete remove any note written about them.
func (s *Services) DeleteShareNote(ctx context.Context, uid, id int64) error {
	var link sql.NullInt64
	if err := s.db().QueryRowContext(ctx, `DELETE FROM share_notes WHERE user_id = ? AND id = ? RETURNING share_link_id`, uid, id).Scan(&link); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	s.dropOwnerShareCache(ctx, uid)
	return nil
}

// RemarkCounts returns the number of remarks per activity among ids.
func (s *Services) remarkCounts(ctx context.Context, uid int64, ids []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(ids) == 0 {
		return out, nil
	}
	q, args := inInts(ids)
	rows, err := s.rdb().QueryContext(ctx, `SELECT activity_id, COUNT(*) FROM share_notes WHERE user_id = ? AND kind = 'remark' AND activity_id IN (`+q+`)
		GROUP BY activity_id`, append([]any{uid}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func inInts(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}

// ---------- portal writes ----------

// PutShareDirective creates or replaces the directive of a link (comments permission).
func (s *Services) PutShareDirective(ctx context.Context, token string, in ShareDirectiveInput) (*athlete.ShareNote, error) {
	l, err := s.portalLink(ctx, token, "comments")
	if err != nil {
		return nil, err
	}
	body, err := noteBody(in.Body, maxDirectiveRunes)
	if err != nil {
		return nil, err
	}
	author, role, err := noteAuthor(in.AuthorName, in.AuthorRole)
	if err != nil {
		return nil, err
	}
	var until *string
	if in.ValidUntil != nil && *in.ValidUntil != "" {
		p, err := s.profile(ctx, l.uid)
		if err != nil {
			return nil, err
		}
		if !athlete.ValidDate(*in.ValidUntil) || *in.ValidUntil < s.today(p) {
			return nil, badRequest("validUntil attendue au format AAAA-MM-JJ, aujourd'hui ou plus tard.")
		}
		until = in.ValidUntil
	}
	if err := validRestrictions(in.Restrictions); err != nil {
		return nil, err
	}
	var restr any
	if in.Restrictions != nil && !in.Restrictions.Empty() {
		b, _ := json.Marshal(in.Restrictions)
		restr = string(b)
	}
	now := s.now().Unix()
	if _, err := s.db().ExecContext(ctx, `INSERT INTO share_notes (user_id, share_link_id, link_label, kind, author_name, author_role, body,
		restrictions_json, valid_until, created_at, updated_at) VALUES (?, ?, ?, 'directive', ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (share_link_id) WHERE kind = 'directive' AND share_link_id IS NOT NULL DO UPDATE SET link_label = excluded.link_label,
		author_name = excluded.author_name, author_role = excluded.author_role, body = excluded.body, restrictions_json = excluded.restrictions_json,
		valid_until = excluded.valid_until, updated_at = excluded.updated_at`,
		l.uid, l.id, l.label, author, role, body, restr, until, now, now); err != nil {
		return nil, err
	}
	s.afterPortalWrite(ctx, l, "directive")
	d, _, err := s.linkNotes(ctx, l.uid, l.id)
	if err == nil && d == nil {
		err = ErrNotFound
	}
	return d, err
}

// DeleteShareDirective removes the directive of a link (comments permission).
func (s *Services) DeleteShareDirective(ctx context.Context, token string) error {
	l, err := s.portalLink(ctx, token, "comments")
	if err != nil {
		return err
	}
	res, err := s.db().ExecContext(ctx, `DELETE FROM share_notes WHERE user_id = ? AND share_link_id = ? AND kind = 'directive'`, l.uid, l.id)
	if err := affectedOrNotFound(res, err); err != nil {
		return err
	}
	s.afterPortalWrite(ctx, l, "directive")
	return nil
}

// portalActivity checks that an activity belongs to the link owner and lies in the link period.
func (s *Services) portalActivity(ctx context.Context, l shareLinkRow, aid int64) (*activityRow, error) {
	a, err := s.activity(ctx, l.uid, aid)
	if err != nil {
		return nil, err
	}
	p, err := s.profile(ctx, l.uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	if a.LocalDate > today || a.LocalDate < addDays(today, -l.scope.PeriodWeeks*7+1) {
		return nil, ErrNotFound
	}
	return a, nil
}

// PutShareRemark creates or replaces the link's remark on a session (comments permission).
func (s *Services) PutShareRemark(ctx context.Context, token string, aid int64, in ShareRemarkInput) (*athlete.ShareNote, error) {
	l, err := s.portalLink(ctx, token, "comments")
	if err != nil {
		return nil, err
	}
	if _, err := s.portalActivity(ctx, l, aid); err != nil {
		return nil, err
	}
	body, err := noteBody(in.Body, maxRemarkRunes)
	if err != nil {
		return nil, err
	}
	author, role, err := noteAuthor(in.AuthorName, in.AuthorRole)
	if err != nil {
		return nil, err
	}
	now := s.now().Unix()
	if _, err := s.db().ExecContext(ctx, `INSERT INTO share_notes (user_id, share_link_id, link_label, kind, activity_id, author_name, author_role,
		body, created_at, updated_at) VALUES (?, ?, ?, 'remark', ?, ?, ?, ?, ?, ?)
		ON CONFLICT (share_link_id, activity_id) WHERE kind = 'remark' AND share_link_id IS NOT NULL DO UPDATE SET link_label = excluded.link_label,
		author_name = excluded.author_name, author_role = excluded.author_role, body = excluded.body, updated_at = excluded.updated_at`,
		l.uid, l.id, l.label, aid, author, role, body, now, now); err != nil {
		return nil, err
	}
	s.afterPortalWrite(ctx, l, "remark")
	_, remarks, err := s.linkNotes(ctx, l.uid, l.id)
	if err != nil {
		return nil, err
	}
	n, ok := remarks[aid]
	if !ok {
		return nil, ErrNotFound
	}
	return &n, nil
}

// DeleteShareRemark removes the link's remark on a session (comments permission).
func (s *Services) DeleteShareRemark(ctx context.Context, token string, aid int64) error {
	l, err := s.portalLink(ctx, token, "comments")
	if err != nil {
		return err
	}
	if _, err := s.portalActivity(ctx, l, aid); err != nil {
		return err
	}
	res, err := s.db().ExecContext(ctx, `DELETE FROM share_notes WHERE user_id = ? AND share_link_id = ? AND kind = 'remark' AND activity_id = ?`,
		l.uid, l.id, aid)
	if err := affectedOrNotFound(res, err); err != nil {
		return err
	}
	s.afterPortalWrite(ctx, l, "remark")
	return nil
}

func affectedOrNotFound(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// dropOwnerShareCache drops the cached public views of every link of uid.
func (s *Services) dropOwnerShareCache(ctx context.Context, uid int64) {
	if _, err := s.deps.Cache.DelPrefix(ctx, userKey(uid, "share")); err != nil {
		s.log.Warn("⚠️ Cache indisponible (suppression)", "err", err.Error())
	}
}

// afterPortalWrite invalidates the public views of the owner and tells them over SSE.
func (s *Services) afterPortalWrite(ctx context.Context, l shareLinkRow, kind string) {
	s.dropOwnerShareCache(ctx, l.uid)
	if s.deps.SSE != nil {
		s.deps.SSE.PublishToUser(l.uid, sse.EventShareUpdated, sse.ShareUpdatedEvent{Kind: kind, LinkID: l.id})
	}
}

// coachNotesDays is the window of remarks injected into the plan prompts.
const coachNotesDays = 14

// coachNotes gathers what the prompts inject first (§15.1): the active directives, then the remarks
// on the given sessions and, when remarksFrom is set, on every session since that date.
func (s *Services) coachNotes(ctx context.Context, uid int64, today string, activityIDs []int64, remarksFrom string) ([]athlete.ShareNote, error) {
	out, err := s.activeDirectives(ctx, uid, today)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	add := func(list []athlete.ShareNote) {
		for _, n := range list {
			if !seen[n.ID] {
				seen[n.ID] = true
				out = append(out, n)
			}
		}
	}
	if len(activityIDs) > 0 {
		q, args := inInts(activityIDs)
		list, err := s.queryNotes(ctx, `WHERE n.user_id = ? AND n.kind = 'remark' AND n.activity_id IN (`+q+`)
			ORDER BY a.local_date DESC, n.updated_at DESC LIMIT 20`, append([]any{uid}, args...)...)
		if err != nil {
			return nil, err
		}
		add(list)
	}
	if remarksFrom != "" {
		list, err := s.queryNotes(ctx, `WHERE n.user_id = ? AND n.kind = 'remark' AND a.local_date >= ?
			ORDER BY a.local_date DESC, n.updated_at DESC LIMIT 20`, uid, remarksFrom)
		if err != nil {
			return nil, err
		}
		add(list)
	}
	return out, nil
}
