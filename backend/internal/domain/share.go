package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/garmin"
)

type ShareLink struct {
	ID             int64   `json:"id"`
	Label          string  `json:"label"`
	TokenPrefix    string  `json:"tokenPrefix"`
	URL            string  `json:"url"`
	ExpiresAt      *string `json:"expiresAt"`
	Revoked        bool    `json:"revoked"`
	RevokedAt      *string `json:"revokedAt"`
	AccessCount    int     `json:"accessCount"`
	LastAccessedAt *string `json:"lastAccessedAt"`
	CreatedAt      string  `json:"createdAt"`
	ShareScope
	Permissions SharePermissions `json:"permissions"`
}

// SharePermissions opens a link to writes (§15.1): directive and session remarks, planning, and
// the per-session detail. All off by default, so links created before 4.2 stay read-only.
type SharePermissions struct {
	Comments        bool `json:"comments"`
	Planning        bool `json:"planning"`
	ActivityDetails bool `json:"activityDetails"`
}

// ShareScope selects what a public view shows: the period and the allowed sections.
type ShareScope struct {
	PeriodWeeks int      `json:"periodWeeks"`
	Sections    []string `json:"sections"`
	Anonymize   bool     `json:"anonymize"`
}

// ShareSections in display order; a link created before 4.1 allows them all.
var ShareSections = []string{"activities", "health", "recovery", "load", "records", "plan", "shoes", "notes"}

var (
	sharePeriods  = []int{4, 8, 12, 26, 52}
	shareExpiries = []int{1, 7, 30, 90}
)

func defaultScope() ShareScope { return ShareScope{PeriodWeeks: 12, Sections: ShareSections} }

func (sc ShareScope) allows(section string) bool { return slices.Contains(sc.Sections, section) }

func parseScope(raw string) ShareScope {
	sc := defaultScope()
	var stored ShareScope
	if json.Unmarshal([]byte(raw), &stored) == nil {
		if slices.Contains(sharePeriods, stored.PeriodWeeks) {
			sc.PeriodWeeks = stored.PeriodWeeks
		}
		if stored.Sections != nil {
			sc.Sections = stored.Sections
		}
		sc.Anonymize = stored.Anonymize
	}
	return sc
}

type PublicActivityItem struct {
	ID              int64    `json:"id"`
	Date            string   `json:"date"`
	StartLocal      string   `json:"startLocal"`
	Name            string   `json:"name"`
	Sport           string   `json:"sport"`
	TypeLabel       string   `json:"typeLabel"`
	DistanceM       *float64 `json:"distanceM"`
	DurationS       *float64 `json:"durationS"`
	AvgPaceSecPerKm *float64 `json:"avgPaceSecPerKm"`
	GapSecPerKm     *float64 `json:"gapSecPerKm"`
	AvgHR           *float64 `json:"avgHR"`
	AvgCadenceSpm   *float64 `json:"avgCadenceSpm"`
	GctMs           *float64 `json:"gctMs"`
	ElevGainM       *float64 `json:"elevGainM"`
	ElevLossM       *float64 `json:"elevLossM"`
	TrainingLoad    *float64 `json:"trainingLoad"`
	ImpactLoad      *float64 `json:"impactLoad"`
	JointIndex      *float64 `json:"jointIndex"`
	EfSpeed         *float64 `json:"efSpeed"`
	ShoeName        *string  `json:"shoeName"`
	ShoeDropMm      *float64 `json:"shoeDropMm"`
	PainDuring      *float64 `json:"painDuring,omitempty"`
	PainNextMorning *float64 `json:"painNextMorning,omitempty"`
	Note            *string  `json:"note,omitempty"`
	// Remark is this link's remark on the session (null when none).
	Remark *athlete.ShareNote `json:"remark"`
}

type PublicLoadPoint struct {
	Date         string   `json:"date"`
	Acute        float64  `json:"acute"`
	Chronic      float64  `json:"chronic"`
	ACWR         *float64 `json:"acwr"`
	EFSpeed      *float64 `json:"efSpeed"`
	ImpactLoad7d *float64 `json:"impactLoad7d"`
	JointIndex7d *float64 `json:"jointIndex7d"`
}

// PublicShoeItem never carries an internal id.
type PublicShoeItem struct {
	Name       string   `json:"name"`
	Brand      string   `json:"brand"`
	Model      string   `json:"model"`
	DropMm     *float64 `json:"dropMm"`
	Type       string   `json:"type"`
	DistanceKm float64  `json:"distanceKm"`
	Retired    bool     `json:"retired"`
}

type PublicAthlete struct {
	DisplayName string  `json:"displayName"`
	FocusArea   *string `json:"focusArea,omitempty"`
	FocusLabel  *string `json:"focusLabel,omitempty"`
	InjuredSide *string `json:"injuredSide,omitempty"`
}

type PublicLoadSummary struct {
	Acute        float64  `json:"acute"`
	Chronic      float64  `json:"chronic"`
	ACWR         *float64 `json:"acwr"`
	Verdict      *string  `json:"verdict"`
	EFLatest     *float64 `json:"efLatest"`
	EFTrendPct   *float64 `json:"efTrendPct"`
	ImpactLoad7d *float64 `json:"impactLoad7d"`
	JointIndex7d *float64 `json:"jointIndex7d"`
}

// PublicShareView only carries the sections the link allows (others are absent, not empty).
type PublicShareView struct {
	GeneratedAt    string                    `json:"generatedAt"`
	ShareLabel     string                    `json:"shareLabel"`
	ExpiresAt      *string                   `json:"expiresAt"`
	PeriodWeeks    int                       `json:"periodWeeks"`
	Sections       []string                  `json:"sections"`
	Athlete        PublicAthlete             `json:"athlete"`
	Readiness      *analysis.ReadinessBanner `json:"readiness,omitempty"`
	LoadSummary    *PublicLoadSummary        `json:"loadSummary,omitempty"`
	LoadSeries     *[]PublicLoadPoint        `json:"loadSeries,omitempty"`
	Cycles         *TrainingCycleOverview    `json:"cycles,omitempty"`
	RecentSessions *[]PublicActivityItem     `json:"recentSessions,omitempty"`
	BodyMap        *analysis.BodyMapOverview `json:"bodyMap,omitempty"`
	Records        *[]athlete.PersonalRecord `json:"records,omitempty"`
	Shoes          *[]PublicShoeItem         `json:"shoes,omitempty"`
	// LinkID is this link's id: the portal edits the sessions and events whose prescribedBy.linkId matches.
	LinkID      int64            `json:"linkId"`
	Permissions SharePermissions `json:"permissions"`
	// Directive is this link's directive (null when none), shown even when no longer valid so it can be edited.
	Directive *athlete.ShareNote `json:"directive"`
	// Calendar is present with the planning permission (today −7 d → +42 d).
	Calendar *PublicCalendar `json:"calendar,omitempty"`
}

type PublicCalendar struct {
	Planned []athlete.PlannedSession `json:"planned"`
	Events  []athlete.CalendarEvent  `json:"events"`
}

const (
	shareCacheTTL     = 5 * time.Minute
	shareMaxSessions  = 200
	shareHistoryExtra = 7*trendWeeksActivity + 27 // EF trend and load ratio need days before the period
)

// PublicURL is STRIDE_PUBLIC_URL: the only origin allowed to write through the portal.
func (s *Services) PublicURL() string { return s.deps.Env.PublicURL }

func shareAAD(uid int64) string { return fmt.Sprintf("share:%d:url", uid) }

func shareCacheKey(uid int64, tokenHash []byte) string {
	return userKey(uid, "share", hex.EncodeToString(tokenHash))
}

const shareCols = `id, label, token_prefix, url_ciphertext, expires_at, revoked_at, access_count, last_accessed_at, created_at, scope_json,
	allow_comments, allow_planning, allow_activity_details`

func (s *Services) scanShareLink(uid int64, sc interface{ Scan(...any) error }) (ShareLink, error) {
	var l ShareLink
	var sealed []byte
	var exp, rev, last sql.NullInt64
	var created int64
	var scope string
	if err := sc.Scan(&l.ID, &l.Label, &l.TokenPrefix, &sealed, &exp, &rev, &l.AccessCount, &last, &created, &scope,
		&l.Permissions.Comments, &l.Permissions.Planning, &l.Permissions.ActivityDetails); err != nil {
		return l, err
	}
	plain, err := s.deps.Sealer.Open(sealed, shareAAD(uid))
	if err != nil {
		s.log.Warn("🔗 Lien de partage indéchiffrable", "user", uid, "lien", l.ID)
		l.URL = "/share/" + l.TokenPrefix + "…"
	} else {
		l.URL = string(plain)
	}
	l.ExpiresAt, l.RevokedAt, l.LastAccessedAt = nullTime(exp), nullTime(rev), nullTime(last)
	l.Revoked, l.CreatedAt, l.ShareScope = rev.Valid, *rfc3339(created), parseScope(scope)
	return l, nil
}

// ListShareLinks lists all share links created by uid.
func (s *Services) ListShareLinks(ctx context.Context, uid int64) ([]ShareLink, error) {
	rows, err := s.rdb().QueryContext(ctx, `SELECT `+shareCols+` FROM share_links WHERE user_id = ? ORDER BY created_at DESC, id DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ShareLink{}
	for rows.Next() {
		l, err := s.scanShareLink(uid, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Services) shareLink(ctx context.Context, uid, id int64) (*ShareLink, error) {
	l, err := s.scanShareLink(uid, s.db().QueryRowContext(ctx, `SELECT `+shareCols+` FROM share_links WHERE user_id = ? AND id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &l, err
}

func nullTime(v sql.NullInt64) *string {
	if !v.Valid {
		return nil
	}
	return rfc3339(v.Int64)
}

// shareEdit holds the validated fields of a create or patch body (nil: not sent).
type shareEdit struct {
	label     *string
	expiresAt **int64
	scope     ShareScope
	perms     SharePermissions
}

// parseShareEdit validates any subset of {label, expiresInDays, expiresAt, periodWeeks, sections, anonymize,
// permissions} onto base; expiresInDays restarts the expiry from now (null or 0: never).
func (s *Services) parseShareEdit(body map[string]json.RawMessage, base ShareScope, perms SharePermissions) (shareEdit, error) {
	e := shareEdit{scope: base, perms: perms}
	for k, raw := range body {
		var err error
		switch k {
		case "label":
			var v string
			if err = json.Unmarshal(raw, &v); err == nil {
				if v = strings.TrimSpace(v); len([]rune(v)) < 2 || len([]rune(v)) > 80 {
					return e, badRequest("Intitulé requis (2 à 80 caractères).")
				}
				e.label = &v
			}
		case "expiresInDays":
			var v *int
			if err = json.Unmarshal(raw, &v); err == nil {
				var exp *int64
				if v != nil && *v != 0 {
					if !slices.Contains(shareExpiries, *v) {
						return e, badRequest("expiresInDays doit valoir 1, 7, 30, 90 ou null.")
					}
					t := s.now().AddDate(0, 0, *v).Unix()
					exp = &t
				}
				e.expiresAt = &exp
			}
		case "expiresAt":
			var v *string
			if err = json.Unmarshal(raw, &v); err == nil && v != nil && *v != "" {
				t, perr := time.Parse(time.RFC3339, *v)
				if perr != nil {
					return e, badRequest("expiresAt attendu au format RFC 3339 (ex. 2026-12-31T23:59:59Z).")
				}
				exp := t.Unix()
				ptr := &exp
				e.expiresAt = &ptr
			}
		case "periodWeeks":
			if err = json.Unmarshal(raw, &e.scope.PeriodWeeks); err == nil && !slices.Contains(sharePeriods, e.scope.PeriodWeeks) {
				return e, badRequest("periodWeeks doit valoir 4, 8, 12, 26 ou 52.")
			}
		case "sections":
			var v []string
			if err = json.Unmarshal(raw, &v); err == nil {
				ordered := []string{}
				for _, sec := range ShareSections {
					if slices.Contains(v, sec) {
						ordered = append(ordered, sec)
					}
				}
				for _, sec := range v {
					if !slices.Contains(ShareSections, sec) {
						return e, badRequest("Section inconnue %q (attendu : %s).", sec, strings.Join(ShareSections, ", "))
					}
				}
				if len(ordered) == 0 {
					return e, badRequest("Au moins une section doit être partagée.")
				}
				e.scope.Sections = ordered
			}
		case "anonymize":
			err = json.Unmarshal(raw, &e.scope.Anonymize)
		case "permissions":
			var v struct {
				Comments        *bool `json:"comments"`
				Planning        *bool `json:"planning"`
				ActivityDetails *bool `json:"activityDetails"`
			}
			dec := json.NewDecoder(strings.NewReader(string(raw)))
			dec.DisallowUnknownFields()
			if err = dec.Decode(&v); err == nil {
				for _, f := range []struct {
					src *bool
					dst *bool
				}{{v.Comments, &e.perms.Comments}, {v.Planning, &e.perms.Planning}, {v.ActivityDetails, &e.perms.ActivityDetails}} {
					if f.src != nil {
						*f.dst = *f.src
					}
				}
			}
		default:
			return e, badRequest("Champ inconnu %q.", k)
		}
		if err != nil {
			return e, badRequest("Valeur invalide pour %s.", k)
		}
	}
	return e, nil
}

// CreateShareLink generates a read-only share link; only the token hash is stored, the URL is sealed.
func (s *Services) CreateShareLink(ctx context.Context, uid int64, body map[string]json.RawMessage) (*ShareLink, error) {
	e, err := s.parseShareEdit(body, defaultScope(), SharePermissions{})
	if err != nil {
		return nil, err
	}
	label := "Lien de partage"
	if e.label != nil {
		label = *e.label
	}
	var expiresAt *int64
	if e.expiresAt != nil {
		expiresAt = *e.expiresAt
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	token := "st4_" + base64.RawURLEncoding.EncodeToString(buf)
	th := sha256.Sum256([]byte(token))
	sealed, err := s.deps.Sealer.Seal([]byte("/share/"+token), shareAAD(uid))
	if err != nil {
		return nil, err
	}
	scope, _ := json.Marshal(e.scope)
	res, err := s.db().ExecContext(ctx, `INSERT INTO share_links (user_id, label, token_hash, token_prefix, url_ciphertext, expires_at, created_at, scope_json,
		allow_comments, allow_planning, allow_activity_details)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, label, th[:], token[:8], sealed, expiresAt, s.now().Unix(), string(scope),
		e.perms.Comments, e.perms.Planning, e.perms.ActivityDetails)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.shareLink(ctx, uid, id)
}

// PatchShareLink edits the label, expiry or scope of a link and drops its cached public view.
func (s *Services) PatchShareLink(ctx context.Context, uid, id int64, body map[string]json.RawMessage) (*ShareLink, error) {
	cur, err := s.shareLink(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	e, err := s.parseShareEdit(body, cur.ShareScope, cur.Permissions)
	if err != nil {
		return nil, err
	}
	label := cur.Label
	if e.label != nil {
		label = *e.label
	}
	scope, _ := json.Marshal(e.scope)
	q, args := `UPDATE share_links SET label = ?, scope_json = ?, allow_comments = ?, allow_planning = ?, allow_activity_details = ?`,
		[]any{label, string(scope), e.perms.Comments, e.perms.Planning, e.perms.ActivityDetails}
	if e.expiresAt != nil {
		q, args = q+`, expires_at = ?`, append(args, *e.expiresAt)
	}
	if _, err := s.db().ExecContext(ctx, q+` WHERE user_id = ? AND id = ?`, append(args, uid, id)...); err != nil {
		return nil, err
	}
	s.dropShareCache(ctx, uid, id)
	return s.shareLink(ctx, uid, id)
}

func (s *Services) dropShareCache(ctx context.Context, uid, id int64) {
	var hash []byte
	if err := s.db().QueryRowContext(ctx, `SELECT token_hash FROM share_links WHERE user_id = ? AND id = ?`, uid, id).Scan(&hash); err != nil {
		return
	}
	if err := s.deps.Cache.Del(ctx, shareCacheKey(uid, hash)); err != nil {
		s.log.Warn("⚠️ Cache indisponible (suppression)", "err", err.Error())
	}
}

// DeleteShareLink revokes a share link at once and drops its cached public view.
func (s *Services) DeleteShareLink(ctx context.Context, uid, id int64) error {
	if _, err := s.shareLink(ctx, uid, id); err != nil {
		return err
	}
	if _, err := s.db().ExecContext(ctx, `UPDATE share_links SET revoked_at = ? WHERE user_id = ? AND id = ? AND revoked_at IS NULL`,
		s.now().Unix(), uid, id); err != nil {
		return err
	}
	s.dropShareCache(ctx, uid, id)
	return nil
}

type shareLinkRow struct {
	id, uid     int64
	label, name string
	prefix      string
	hash        []byte
	expiresAt   *string
	scope       ShareScope
	perms       SharePermissions
}

// shareByToken resolves a token: unknown, revoked or disabled-owner links are ErrNotFound, expired
// ones ErrGone. Plain read on the read pool.
func (s *Services) shareByToken(ctx context.Context, token string) (shareLinkRow, error) {
	th := sha256.Sum256([]byte(token))
	l := shareLinkRow{hash: th[:]}
	var exp, rev sql.NullInt64
	var status, scope string
	err := s.rdb().QueryRowContext(ctx, `SELECT l.id, l.user_id, l.label, l.token_prefix, l.expires_at, l.revoked_at, l.scope_json,
		l.allow_comments, l.allow_planning, l.allow_activity_details, u.display_name, u.status
		FROM share_links l JOIN users u ON u.id = l.user_id WHERE l.token_hash = ?`, l.hash).
		Scan(&l.id, &l.uid, &l.label, &l.prefix, &exp, &rev, &scope, &l.perms.Comments, &l.perms.Planning, &l.perms.ActivityDetails, &l.name, &status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return l, ErrNotFound
	case err != nil:
		return l, err
	case rev.Valid || status != "active":
		return l, ErrNotFound
	case exp.Valid && exp.Int64 <= s.now().Unix():
		return l, ErrGone
	}
	l.expiresAt, l.scope = nullTime(exp), parseScope(scope)
	return l, nil
}

// portalLink resolves the token of a portal call: unknown, revoked and expired links are the same
// ErrNotFound; a permission that is off is ErrShareForbidden ("" checks none).
func (s *Services) portalLink(ctx context.Context, token, perm string) (shareLinkRow, error) {
	l, err := s.shareByToken(ctx, token)
	if errors.Is(err, ErrGone) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	allowed := map[string]bool{"": true, "comments": l.perms.Comments, "planning": l.perms.Planning, "activityDetails": l.perms.ActivityDetails}
	if !allowed[perm] {
		return l, ErrShareForbidden
	}
	return l, nil
}

// GetPublicShare returns the read-only overview behind a valid token: unknown, revoked or
// disabled-owner links are ErrNotFound, expired ones ErrGone. The view is cached for 5 minutes.
func (s *Services) GetPublicShare(ctx context.Context, token string) (*PublicShareView, error) {
	l, err := s.shareByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if _, err := s.db().ExecContext(ctx, `UPDATE share_links SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?`,
		s.now().Unix(), l.id); err != nil {
		s.log.Warn("🔗 Compteur d'accès du lien de partage non mis à jour", "lien", l.id, "err", err.Error())
	}
	key := shareCacheKey(l.uid, l.hash)
	if raw, ok := s.cacheGet(ctx, key); ok {
		var v PublicShareView
		if json.Unmarshal(raw, &v) == nil {
			return &v, nil
		}
	}
	v, err := s.buildPublicShare(ctx, l)
	if err != nil {
		return nil, err
	}
	if raw, err := json.Marshal(v); err == nil {
		s.cacheSet(ctx, key, raw, shareCacheTTL)
	}
	return v, nil
}

// buildPublicShare computes only the sections the link allows, over its period.
func (s *Services) buildPublicShare(ctx context.Context, l shareLinkRow) (*PublicShareView, error) {
	p, err := s.profile(ctx, l.uid)
	if err != nil {
		return nil, err
	}
	sc, today := l.scope, s.today(p)
	days := sc.PeriodWeeks * 7
	v := &PublicShareView{GeneratedAt: s.now().UTC().Format(time.RFC3339), ShareLabel: l.label, ExpiresAt: l.expiresAt,
		PeriodWeeks: sc.PeriodWeeks, Sections: sc.Sections, Athlete: PublicAthlete{DisplayName: l.name}, LinkID: l.id, Permissions: l.perms}
	directive, remarks, err := s.linkNotes(ctx, l.uid, l.id)
	if err != nil {
		return nil, err
	}
	v.Directive = directive
	if l.perms.Planning {
		if v.Calendar, err = s.publicCalendar(ctx, l.uid, today); err != nil {
			return nil, err
		}
	}
	if sc.Anonymize {
		v.Athlete.DisplayName = "Athlète"
	}
	if sc.allows("health") {
		v.Athlete.FocusArea, v.Athlete.FocusLabel, v.Athlete.InjuredSide = &p.FocusArea, &p.FocusLabel, p.InjuredSide
		if v.BodyMap, err = s.BodyMap(ctx, l.uid, max(7, days), ""); err != nil {
			return nil, err
		}
	}
	if sc.allows("recovery") {
		dv, err := s.DayView(ctx, l.uid, today)
		if err != nil {
			return nil, err
		}
		v.Readiness = &dv.Readiness
	}
	if sc.allows("plan") {
		if v.Cycles, err = s.Cycles(ctx, l.uid, nil); err != nil {
			return nil, err
		}
	}
	if sc.allows("records") {
		recs, err := s.records(ctx, l.uid)
		if err != nil {
			return nil, err
		}
		v.Records = &recs
	}
	var shoes []athlete.Shoe
	if sc.allows("shoes") || sc.allows("activities") {
		if shoes, err = s.Shoes(ctx, l.uid); err != nil {
			return nil, err
		}
	}
	if sc.allows("shoes") {
		list := make([]PublicShoeItem, 0, len(shoes))
		for _, sh := range shoes {
			list = append(list, PublicShoeItem{Name: sh.Name, Brand: sh.Brand, Model: sh.Model, DropMm: sh.DropMm, Type: sh.Type,
				DistanceKm: sh.DistanceKm, Retired: sh.Retired})
		}
		v.Shoes = &list
	}
	if !sc.allows("load") && !sc.allows("activities") {
		return v, nil
	}
	acts, err := s.shareActivities(ctx, l.uid, p, addDays(today, -days-shareHistoryExtra), today)
	if err != nil {
		return nil, err
	}
	if sc.allows("load") {
		series, sum := shareLoad(acts, today, days, p.Thresholds.LoadRatioMax)
		v.LoadSeries, v.LoadSummary = &series, &sum
	}
	if sc.allows("activities") {
		sessions, err := s.shareSessions(ctx, l.uid, acts, shoes, addDays(today, -days+1), sc.allows("notes"))
		if err != nil {
			return nil, err
		}
		for i := range sessions {
			if n, ok := remarks[sessions[i].ID]; ok {
				sessions[i].Remark = &n
			}
		}
		v.RecentSessions = &sessions
	}
	return v, nil
}

// shareAct is an activity with its persisted metrics: EF when eligible (structured sessions
// excluded), joint index, and Garmin's impact load.
type shareAct struct {
	row    *activityRow
	ef     *float64
	joint  *float64
	impact *float64
}

func (s *Services) shareActivities(ctx context.Context, uid int64, p athlete.Profile, from, to string) ([]shareAct, error) {
	rows, err := s.activitiesBetween(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	pm, err := s.matchPlan(ctx, uid, p, from, to)
	if err != nil {
		return nil, err
	}
	structured := structuredPlans(pm)
	out := make([]shareAct, 0, len(rows))
	for _, a := range rows {
		m, err := s.metricsFor(ctx, uid, a, nil)
		if err != nil {
			return nil, err
		}
		sa := shareAct{row: a}
		if ef := withPlanStructure(m.Efficiency, structured[a.ID]); ef.Eligible {
			sa.ef = ef.EfSpeed
		}
		if m.Impact != nil {
			sa.joint = m.Impact.JointIndex
		}
		if a.isRunning() {
			if sa.impact, err = s.garminImpactLoad(ctx, uid, a.ID); err != nil {
				return nil, err
			}
		}
		out = append(out, sa)
	}
	return out, nil
}

// garminImpactLoad reads Garmin's impact load from the stored activity detail (nil when absent).
func (s *Services) garminImpactLoad(ctx context.Context, uid, aid int64) (*float64, error) {
	raw, _, ok, err := s.blob(ctx, uid, aid, "garmin_detail")
	if err != nil || !ok {
		return nil, err
	}
	d, err := garmin.Decode[garmin.ActivityDetail](raw)
	if err != nil {
		return nil, nil
	}
	return analysis.PR(d.Summary.ImpactLoad, 0), nil
}

// shareLoad builds the 42-day series (ACWR, daily running EF, 7-day impact and joint sums) and
// today's summary; values are null when no activity carries them.
func shareLoad(acts []shareAct, today string, days int, maxRatio float64) ([]PublicLoadPoint, PublicLoadSummary) {
	var loads []analysis.DatedLoad
	var efDates []string
	var efVals []float64
	efByDay := map[string][]float64{}
	impactByDay, jointByDay := map[string]float64{}, map[string]float64{}
	impactDays, jointDays := map[string]bool{}, map[string]bool{}
	for _, a := range acts {
		d := a.row.LocalDate
		if a.row.Load != nil {
			loads = append(loads, analysis.DatedLoad{Date: d, StartLocal: a.row.StartLocal, Load: *a.row.Load})
		}
		if a.ef != nil && a.row.Sport == "run" {
			efDates, efVals = append(efDates, d), append(efVals, *a.ef)
			efByDay[d] = append(efByDay[d], *a.ef)
		}
		if a.impact != nil {
			impactByDay[d] += *a.impact
			impactDays[d] = true
		}
		if a.joint != nil {
			jointByDay[d] += *a.joint
			jointDays[d] = true
		}
	}
	sum7 := func(byDay map[string]float64, seen map[string]bool, day string, digits int) *float64 {
		total, found := 0.0, false
		for i := 0; i < 7; i++ {
			d := addDays(day, -i)
			total += byDay[d]
			found = found || seen[d]
		}
		if !found {
			return nil
		}
		return analysis.Num(total, digits)
	}
	series := make([]PublicLoadPoint, 0, days)
	for i := days - 1; i >= 0; i-- {
		d := addDays(today, -i)
		pt := PublicLoadPoint{Date: d, ImpactLoad7d: sum7(impactByDay, impactDays, d, 0), JointIndex7d: sum7(jointByDay, jointDays, d, 1)}
		if lr := analysis.ComputeLoadRatio(loads, d, "", maxRatio); lr != nil {
			pt.Acute, pt.Chronic, pt.ACWR = math.Round(lr.Acute), math.Round(lr.Chronic), lr.Ratio
		}
		if vals := efByDay[d]; len(vals) > 0 {
			total := 0.0
			for _, v := range vals {
				total += v
			}
			pt.EFSpeed = analysis.Num(total/float64(len(vals)), 3)
		}
		series = append(series, pt)
	}
	last := series[len(series)-1]
	sum := PublicLoadSummary{Acute: last.Acute, Chronic: last.Chronic, ACWR: last.ACWR, ImpactLoad7d: last.ImpactLoad7d, JointIndex7d: last.JointIndex7d}
	if lr := analysis.ComputeLoadRatio(loads, today, "", maxRatio); lr != nil {
		sum.Verdict = &lr.Verdict
	}
	if n := len(efVals); n > 0 {
		sum.EFLatest = analysis.Num(efVals[n-1], 3)
		sum.EFTrendPct = analysis.EfficiencyChangePct(efDates, efVals, addDays(today, -7*trendWeeksActivity+1), today)
	}
	return series, sum
}

// shareSessions lists the sessions since from (newest first) with their measured metrics and shoe;
// pains and note only when the notes section is shared.
func (s *Services) shareSessions(ctx context.Context, uid int64, acts []shareAct, shoes []athlete.Shoe, from string, withNotes bool) ([]PublicActivityItem, error) {
	var recent []shareAct
	for i := len(acts) - 1; i >= 0 && len(recent) < shareMaxSessions && acts[i].row.LocalDate >= from; i-- {
		recent = append(recent, acts[i])
	}
	out := make([]PublicActivityItem, 0, len(recent))
	if len(recent) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(recent))
	for _, a := range recent {
		ids = append(ids, a.row.ID)
	}
	notes, err := s.repo().Notes(ctx, uid, ids)
	if err != nil {
		return nil, err
	}
	shoeOf, err := s.activityShoes(ctx, uid, recent[len(recent)-1].row.LocalDate)
	if err != nil {
		return nil, err
	}
	shoeByID := make(map[int64]athlete.Shoe, len(shoes))
	for _, sh := range shoes {
		shoeByID[sh.ID] = sh
	}
	for _, sa := range recent {
		a := sa.row
		it := PublicActivityItem{ID: a.ID, Date: a.LocalDate, StartLocal: a.StartLocal, Name: a.Name, Sport: a.Sport,
			TypeLabel: analysis.TypeLabel(a.Type), DistanceM: a.DistanceM, DurationS: a.DurationS, AvgHR: a.AvgHR,
			AvgCadenceSpm: a.AvgCadence, GctMs: a.AvgGCT, ElevGainM: a.ElevGain, ElevLossM: a.ElevLoss, TrainingLoad: a.Load,
			ImpactLoad: sa.impact, JointIndex: sa.joint, EfSpeed: sa.ef}
		if a.AvgSpeed != nil && *a.AvgSpeed > 0 {
			it.AvgPaceSecPerKm = f64(1000 / *a.AvgSpeed)
		}
		if a.GapSpeed != nil && *a.GapSpeed > 0 {
			it.GapSecPerKm = f64(1000 / *a.GapSpeed)
		}
		if id, ok := shoeOf[a.ID]; ok {
			if sh, ok := shoeByID[id]; ok {
				it.ShoeName, it.ShoeDropMm = &sh.Name, sh.DropMm
			}
		}
		if n, ok := notes[a.ID]; ok && withNotes {
			it.PainDuring, it.PainNextMorning = n.PainDuring, n.PainNextMorning
			if n.Note != "" {
				it.Note = &n.Note
			}
		}
		out = append(out, it)
	}
	return out, nil
}
