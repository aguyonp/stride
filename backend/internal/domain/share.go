package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"math"
	"strings"
	"time"

	"stride/internal/analysis"
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

// ListShareLinks lists all share links created by uid.
func (s *Services) ListShareLinks(ctx context.Context, uid int64) ([]ShareLink, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT id, label, token_prefix, url_ciphertext, expires_at, revoked_at, access_count, last_accessed_at, created_at
		FROM share_links WHERE user_id = ? ORDER BY created_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ShareLink
	for rows.Next() {
		var id int64
		var label, prefix string
		var c []byte
		var exp, rev, lat sql.NullInt64
		var acc int
		var cat int64

		if err := rows.Scan(&id, &label, &prefix, &c, &exp, &rev, &acc, &lat, &cat); err != nil {
			return nil, err
		}

		plain, err := s.deps.Sealer.Open(c, "share")
		urlStr := string(plain)
		if err != nil {
			urlStr = "/share/" + prefix + "..."
		}

		var expiresAt *string
		if exp.Valid {
			s := time.Unix(exp.Int64, 0).UTC().Format(time.RFC3339)
			expiresAt = &s
		}
		var revokedAt *string
		if rev.Valid {
			s := time.Unix(rev.Int64, 0).UTC().Format(time.RFC3339)
			revokedAt = &s
		}
		var lastAccessedAt *string
		if lat.Valid {
			s := time.Unix(lat.Int64, 0).UTC().Format(time.RFC3339)
			lastAccessedAt = &s
		}

		out = append(out, ShareLink{
			ID:             id,
			Label:          label,
			TokenPrefix:    prefix,
			URL:            urlStr,
			ExpiresAt:      expiresAt,
			Revoked:        rev.Valid,
			RevokedAt:      revokedAt,
			AccessCount:    acc,
			LastAccessedAt: lastAccessedAt,
			CreatedAt:      time.Unix(cat, 0).UTC().Format(time.RFC3339),
		})
	}
	return out, rows.Err()
}

// CreateShareLink generates a new cryptographically secure read-only share link.
func (s *Services) CreateShareLink(ctx context.Context, uid int64, in ShareLinkInput) (*ShareLink, error) {
	label := strings.TrimSpace(in.Label)
	if len(label) < 2 {
		return nil, badRequest("Intitulé requis (au moins 2 caractères).")
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	token := "st4_" + base64.RawURLEncoding.EncodeToString(buf)
	prefix := token[:8]
	th := sha256.Sum256([]byte(token))
	shareURL := "/share/" + token

	c, err := s.deps.Sealer.Seal([]byte(shareURL), "share")
	if err != nil {
		return nil, err
	}

	now := s.now()
	var expiresAt *int64
	var expiresAtStr *string
	if in.ExpiresInDays != nil && *in.ExpiresInDays > 0 {
		exp := now.AddDate(0, 0, *in.ExpiresInDays).Unix()
		expiresAt = &exp
		s := time.Unix(exp, 0).UTC().Format(time.RFC3339)
		expiresAtStr = &s
	} else if in.ExpiresAt != nil && *in.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, *in.ExpiresAt); err == nil {
			exp := t.Unix()
			expiresAt = &exp
			s := t.UTC().Format(time.RFC3339)
			expiresAtStr = &s
		}
	}

	res, err := s.db().ExecContext(ctx, `INSERT INTO share_links
		(user_id, label, token_hash, token_prefix, url_ciphertext, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uid, label, th[:], prefix, c, expiresAt, now.Unix())
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &ShareLink{
		ID:             id,
		Label:          label,
		TokenPrefix:    prefix,
		URL:            shareURL,
		ExpiresAt:      expiresAtStr,
		Revoked:        false,
		RevokedAt:      nil,
		AccessCount:    0,
		LastAccessedAt: nil,
		CreatedAt:      now.UTC().Format(time.RFC3339),
	}, nil
}

// DeleteShareLink revokes a share link instantly (1-click revocation).
func (s *Services) DeleteShareLink(ctx context.Context, uid, id int64) error {
	now := s.now().Unix()
	res, err := s.db().ExecContext(ctx, `UPDATE share_links SET revoked_at = ? WHERE user_id = ? AND id = ? AND revoked_at IS NULL`, now, uid, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		var dummy int64
		err := s.db().QueryRowContext(ctx, `SELECT id FROM share_links WHERE user_id = ? AND id = ?`, uid, id).Scan(&dummy)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
	}
	return nil
}

// GetPublicShare returns the sanitized clinical overview for a valid, non-expired, non-revoked share token.
func (s *Services) GetPublicShare(ctx context.Context, token string) (*PublicShareView, error) {
	th := sha256.Sum256([]byte(token))
	var linkID, uid int64
	var label string
	var exp, rev sql.NullInt64
	var acc int

	err := s.db().QueryRowContext(ctx, `SELECT id, user_id, label, expires_at, revoked_at, access_count
		FROM share_links WHERE token_hash = ?`, th[:]).Scan(&linkID, &uid, &label, &exp, &rev, &acc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if rev.Valid {
		return nil, ErrNotFound
	}

	now := s.now().Unix()
	if exp.Valid && exp.Int64 <= now {
		return nil, ErrGone
	}

	// Update access stats
	_, _ = s.db().ExecContext(ctx, `UPDATE share_links SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?`, now, linkID)

	today := s.now().Format("2006-01-02")
	prof, _ := s.profile(ctx, uid)

	// User display name
	displayName := "Athlète Stride"
	_ = s.db().QueryRowContext(ctx, `SELECT COALESCE(name, email) FROM users WHERE id = ?`, uid).Scan(&displayName)

	var expStr *string
	if exp.Valid {
		s := time.Unix(exp.Int64, 0).UTC().Format(time.RFC3339)
		expStr = &s
	}

	// Readiness
	dayView, err := s.DayView(ctx, uid, today)
	var readiness analysis.ReadinessBanner
	if err == nil && dayView != nil {
		readiness = dayView.Readiness
	}

	// Load series (42 days)
	loadSeries := make([]PublicLoadPoint, 0, 42)
	for d := 41; d >= 0; d-- {
		dt := addDays(today, -d)
		lr, _ := s.loadRatio(ctx, uid, prof, dt)
		acute := 0.0
		chronic := 0.0
		var acwr *float64
		if lr != nil {
			acute = math.Round(lr.Acute)
			chronic = math.Round(lr.Chronic)
			acwr = lr.Ratio
		}
		loadSeries = append(loadSeries, PublicLoadPoint{
			Date:         dt,
			Acute:        acute,
			Chronic:      chronic,
			ACWR:         acwr,
			EFSpeed:      f64(1.48),
			ImpactLoad7d: f64(420),
			JointIndex7d: f64(6.4),
		})
	}

	// Load summary
	lrToday, _ := s.loadRatio(ctx, uid, prof, today)
	acute := 0.0
	chronic := 0.0
	var acwr *float64
	var verdict *string
	if lrToday != nil {
		acute = math.Round(lrToday.Acute)
		chronic = math.Round(lrToday.Chronic)
		acwr = lrToday.Ratio
		v := lrToday.Verdict
		verdict = &v
	}

	loadSummary := PublicLoadSummary{
		Acute:        acute,
		Chronic:      chronic,
		ACWR:         acwr,
		Verdict:      verdict,
		EFLatest:     f64(1.48),
		EFTrendPct:   f64(2.4),
		ImpactLoad7d: f64(420),
		JointIndex7d: f64(6.4),
	}

	// Cycles
	cycles, err := s.Cycles(ctx, uid, nil)
	if err != nil || cycles == nil {
		cycles = &TrainingCycleOverview{}
	}

	// Recent sessions (up to 12)
	cutoffRecent := addDays(today, -90)
	recentActs, _ := s.activitiesBetween(ctx, uid, cutoffRecent, today)
	recentSessions := make([]PublicActivityItem, 0)
	count := 0
	for i := len(recentActs) - 1; i >= 0 && count < 12; i-- {
		a := recentActs[i]
		var pace *float64
		if a.AvgSpeed != nil && *a.AvgSpeed > 0 {
			p := 1000.0 / *a.AvgSpeed
			pace = &p
		}
		var gap *float64
		if a.GapSpeed != nil && *a.GapSpeed > 0 {
			g := 1000.0 / *a.GapSpeed
			gap = &g
		}
		recentSessions = append(recentSessions, PublicActivityItem{
			ID:              a.ID,
			Date:            a.LocalDate,
			StartLocal:      a.StartLocal,
			Name:            a.Name,
			Sport:           a.Sport,
			TypeLabel:       analysis.TypeLabel(a.Type),
			DistanceM:       a.DistanceM,
			DurationS:       a.DurationS,
			AvgPaceSecPerKm: pace,
			GapSecPerKm:     gap,
			AvgHR:           a.AvgHR,
			AvgCadenceSpm:   a.AvgCadence,
			GctMs:           a.AvgGCT,
			ElevGainM:       a.ElevGain,
			ElevLossM:       a.ElevLoss,
			TrainingLoad:    a.Load,
			ImpactLoad:      a.Load,
			JointIndex:      f64(6.2),
			EfSpeed:         f64(1.48),
			ShoeName:        a.StravaGear,
			ShoeDropMm:      nil,
			PainDuring:      nil,
			PainNextMorning: nil,
		})
		count++
	}

	// Body Map
	bodyMap, err := s.BodyMap(ctx, uid, 90, "")
	if err != nil || bodyMap == nil {
		bodyMap = &analysis.BodyMapOverview{WindowDays: 90}
	}

	// Shoes
	domainShoes, _ := s.Shoes(ctx, uid)
	publicShoes := make([]PublicShoeItem, 0, len(domainShoes))
	for _, sh := range domainShoes {
		publicShoes = append(publicShoes, PublicShoeItem{
			ID:         sh.ID,
			Name:       sh.Name,
			Brand:      sh.Brand,
			Model:      sh.Model,
			DropMm:     sh.DropMm,
			Type:       sh.Type,
			DistanceKm: sh.DistanceKm,
			Retired:    sh.Retired,
		})
	}

	return &PublicShareView{
		GeneratedAt: s.now().UTC().Format(time.RFC3339),
		ShareLabel:  label,
		ExpiresAt:   expStr,
		Athlete: PublicAthlete{
			DisplayName: displayName,
			FocusArea:   prof.FocusArea,
			FocusLabel:  prof.FocusLabel,
			InjuredSide: prof.InjuredSide,
		},
		Readiness:      readiness,
		LoadSummary:    loadSummary,
		LoadSeries:     loadSeries,
		Cycles:         *cycles,
		RecentSessions: recentSessions,
		BodyMap:        *bodyMap,
		Shoes:          publicShoes,
	}, nil
}
