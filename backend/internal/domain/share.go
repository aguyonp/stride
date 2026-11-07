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
}

type ShareLinkInput struct {
	Label         string  `json:"label"`
	ExpiresInDays *int    `json:"expiresInDays,omitempty"`
	ExpiresAt     *string `json:"expiresAt,omitempty"`
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
	PainDuring      *float64 `json:"painDuring"`
	PainNextMorning *float64 `json:"painNextMorning"`
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

type PublicShoeItem struct {
	ID         int64    `json:"id"`
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
	FocusArea   string  `json:"focusArea"`
	FocusLabel  string  `json:"focusLabel"`
	InjuredSide *string `json:"injuredSide"`
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

type PublicShareView struct {
	GeneratedAt    string                   `json:"generatedAt"`
	ShareLabel     string                   `json:"shareLabel"`
	ExpiresAt      *string                  `json:"expiresAt"`
	Athlete        PublicAthlete            `json:"athlete"`
	Readiness      analysis.ReadinessBanner `json:"readiness"`
	LoadSummary    PublicLoadSummary        `json:"loadSummary"`
	LoadSeries     []PublicLoadPoint        `json:"loadSeries"`
	Cycles         TrainingCycleOverview    `json:"cycles"`
	RecentSessions []PublicActivityItem     `json:"recentSessions"`
	BodyMap        analysis.BodyMapOverview `json:"bodyMap"`
	Shoes          []PublicShoeItem         `json:"shoes"`
}

const (
	shareCacheTTL = 5 * time.Minute
	shareWindow   = 90 // days of activities behind a public view
	shareSeries   = 42 // days of the load series
	shareSessions = 12
)

func shareAAD(uid int64) string { return fmt.Sprintf("share:%d:url", uid) }

func shareCacheKey(uid int64, tokenHash []byte) string {
	return userKey(uid, "share", hex.EncodeToString(tokenHash))
}

// ListShareLinks lists all share links created by uid.
func (s *Services) ListShareLinks(ctx context.Context, uid int64) ([]ShareLink, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT id, label, token_prefix, url_ciphertext, expires_at, revoked_at, access_count, last_accessed_at, created_at
		FROM share_links WHERE user_id = ? ORDER BY created_at DESC, id DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ShareLink{}
	for rows.Next() {
		var l ShareLink
		var sealed []byte
		var exp, rev, last sql.NullInt64
		var created int64
		if err := rows.Scan(&l.ID, &l.Label, &l.TokenPrefix, &sealed, &exp, &rev, &l.AccessCount, &last, &created); err != nil {
			return nil, err
		}
		plain, err := s.deps.Sealer.Open(sealed, shareAAD(uid))
		if err != nil {
			s.log.Warn("🔗 Lien de partage indéchiffrable", "user", uid, "lien", l.ID)
			l.URL = "/share/" + l.TokenPrefix + "…"
		} else {
			l.URL = string(plain)
		}
		l.ExpiresAt, l.RevokedAt, l.LastAccessedAt = nullTime(exp), nullTime(rev), nullTime(last)
		l.Revoked, l.CreatedAt = rev.Valid, *rfc3339(created)
		out = append(out, l)
	}
	return out, rows.Err()
}

func nullTime(v sql.NullInt64) *string {
	if !v.Valid {
		return nil
	}
	return rfc3339(v.Int64)
}

// CreateShareLink generates a new read-only share link; only the token hash is stored, the URL is sealed.
func (s *Services) CreateShareLink(ctx context.Context, uid int64, in ShareLinkInput) (*ShareLink, error) {
	label := strings.TrimSpace(in.Label)
	if len(label) < 2 {
		return nil, badRequest("Intitulé requis (au moins 2 caractères).")
	}
	now := s.now()
	var expiresAt *int64
	switch {
	case in.ExpiresInDays != nil && (*in.ExpiresInDays < 0 || *in.ExpiresInDays > 3650):
		return nil, badRequest("expiresInDays doit être compris entre 0 (sans expiration) et 3650.")
	case in.ExpiresInDays != nil && *in.ExpiresInDays > 0:
		exp := now.AddDate(0, 0, *in.ExpiresInDays).Unix()
		expiresAt = &exp
	case in.ExpiresAt != nil && *in.ExpiresAt != "":
		t, err := time.Parse(time.RFC3339, *in.ExpiresAt)
		if err != nil {
			return nil, badRequest("expiresAt attendu au format RFC 3339 (ex. 2026-12-31T23:59:59Z).")
		}
		exp := t.Unix()
		expiresAt = &exp
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	token := "st4_" + base64.RawURLEncoding.EncodeToString(buf)
	th := sha256.Sum256([]byte(token))
	link := ShareLink{Label: label, TokenPrefix: token[:8], URL: "/share/" + token, CreatedAt: *rfc3339(now.Unix())}
	sealed, err := s.deps.Sealer.Seal([]byte(link.URL), shareAAD(uid))
	if err != nil {
		return nil, err
	}
	res, err := s.db().ExecContext(ctx, `INSERT INTO share_links (user_id, label, token_hash, token_prefix, url_ciphertext, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, uid, label, th[:], link.TokenPrefix, sealed, expiresAt, now.Unix())
	if err != nil {
		return nil, err
	}
	if link.ID, err = res.LastInsertId(); err != nil {
		return nil, err
	}
	if expiresAt != nil {
		link.ExpiresAt = rfc3339(*expiresAt)
	}
	return &link, nil
}

// DeleteShareLink revokes a share link at once and drops its cached public view.
func (s *Services) DeleteShareLink(ctx context.Context, uid, id int64) error {
	var hash []byte
	err := s.db().QueryRowContext(ctx, `SELECT token_hash FROM share_links WHERE user_id = ? AND id = ?`, uid, id).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := s.db().ExecContext(ctx, `UPDATE share_links SET revoked_at = ? WHERE user_id = ? AND id = ? AND revoked_at IS NULL`,
		s.now().Unix(), uid, id); err != nil {
		return err
	}
	if err := s.deps.Cache.Del(ctx, shareCacheKey(uid, hash)); err != nil {
		s.log.Warn("⚠️ Cache indisponible (suppression)", "err", err.Error())
	}
	return nil
}

type shareLinkRow struct {
	id, uid     int64
	label, name string
	hash        []byte
	expiresAt   *string
}

// GetPublicShare returns the read-only overview behind a valid token: unknown, revoked or
// disabled-owner links are ErrNotFound, expired ones ErrGone. The view is cached for 5 minutes.
func (s *Services) GetPublicShare(ctx context.Context, token string) (*PublicShareView, error) {
	th := sha256.Sum256([]byte(token))
	l := shareLinkRow{hash: th[:]}
	var exp, rev sql.NullInt64
	var status string
	err := s.db().QueryRowContext(ctx, `SELECT l.id, l.user_id, l.label, l.expires_at, l.revoked_at, u.display_name, u.status
		FROM share_links l JOIN users u ON u.id = l.user_id WHERE l.token_hash = ?`, l.hash).
		Scan(&l.id, &l.uid, &l.label, &exp, &rev, &l.name, &status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, err
	case rev.Valid || status != "active":
		return nil, ErrNotFound
	case exp.Valid && exp.Int64 <= s.now().Unix():
		return nil, ErrGone
	}
	l.expiresAt = nullTime(exp)
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

func (s *Services) buildPublicShare(ctx context.Context, l shareLinkRow) (*PublicShareView, error) {
	p, err := s.profile(ctx, l.uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	dv, err := s.DayView(ctx, l.uid, today)
	if err != nil {
		return nil, err
	}
	acts, err := s.shareActivities(ctx, l.uid, p, addDays(today, -shareWindow+1), today)
	if err != nil {
		return nil, err
	}
	cycles, err := s.Cycles(ctx, l.uid, nil)
	if err != nil {
		return nil, err
	}
	bodyMap, err := s.BodyMap(ctx, l.uid, shareWindow, "")
	if err != nil {
		return nil, err
	}
	shoes, err := s.Shoes(ctx, l.uid)
	if err != nil {
		return nil, err
	}
	sessions, err := s.shareRecentSessions(ctx, l.uid, acts, shoes)
	if err != nil {
		return nil, err
	}
	series, summary := shareLoad(acts, today, p.Thresholds.LoadRatioMax)
	v := &PublicShareView{
		GeneratedAt: s.now().UTC().Format(time.RFC3339),
		ShareLabel:  l.label,
		ExpiresAt:   l.expiresAt,
		Athlete:     PublicAthlete{DisplayName: l.name, FocusArea: p.FocusArea, FocusLabel: p.FocusLabel, InjuredSide: p.InjuredSide},
		Readiness:   dv.Readiness, LoadSummary: summary, LoadSeries: series, Cycles: *cycles,
		RecentSessions: sessions, BodyMap: *bodyMap, Shoes: make([]PublicShoeItem, 0, len(shoes)),
	}
	for _, sh := range shoes {
		v.Shoes = append(v.Shoes, PublicShoeItem{ID: sh.ID, Name: sh.Name, Brand: sh.Brand, Model: sh.Model, DropMm: sh.DropMm,
			Type: sh.Type, DistanceKm: sh.DistanceKm, Retired: sh.Retired})
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
func shareLoad(acts []shareAct, today string, maxRatio float64) ([]PublicLoadPoint, PublicLoadSummary) {
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
	series := make([]PublicLoadPoint, 0, shareSeries)
	for i := shareSeries - 1; i >= 0; i-- {
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

// shareRecentSessions lists the latest sessions with their measured metrics, shoe and pain notes.
func (s *Services) shareRecentSessions(ctx context.Context, uid int64, acts []shareAct, shoes []athlete.Shoe) ([]PublicActivityItem, error) {
	recent := make([]shareAct, 0, shareSessions)
	for i := len(acts) - 1; i >= 0 && len(recent) < shareSessions; i-- {
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
		if n, ok := notes[a.ID]; ok {
			it.PainDuring, it.PainNextMorning = n.PainDuring, n.PainNextMorning
		}
		out = append(out, it)
	}
	return out, nil
}
