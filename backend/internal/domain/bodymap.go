package domain

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"stride/internal/analysis"
)

// BodyMap builds the full anatomical body stress overview with canonical zones, entries, and correlations.
func (s *Services) BodyMap(ctx context.Context, uid int64, days int, zoneFilter string) (*analysis.BodyMapOverview, error) {
	if days <= 0 {
		days = 90
	}

	today := s.now().Format("2006-01-02")
	cutoffWindow := addDays(today, -days)
	cutoff30 := addDays(today, -30)
	cutoff90 := addDays(today, -90)

	// Fetch entries in 90 days window for zone summaries & correlations
	q := `SELECT e.id, e.date, e.activity_id, a.name, e.zone, e.side, e.pain_score, e.cold_next_day_score, e.note, e.created_at, e.updated_at
		FROM body_stress_entries e
		LEFT JOIN activities a ON e.activity_id = a.id
		WHERE e.user_id = ? AND e.date >= ?
		ORDER BY e.date DESC, e.id DESC`
	rows, err := s.db().QueryContext(ctx, q, uid, cutoff90)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all90 []analysis.BodyStressEntry
	for rows.Next() {
		e, err := scanBodyStressEntry(rows)
		if err != nil {
			return nil, err
		}
		all90 = append(all90, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Compute summaries for all 6 canonical zones
	zones := make([]analysis.BodyZoneSummary, 0, len(analysis.AllBodyZones))
	for _, z := range analysis.AllBodyZones {
		var z90 []analysis.BodyStressEntry
		var z30 []analysis.BodyStressEntry
		for _, e := range all90 {
			if e.Zone == z {
				z90 = append(z90, e)
				if e.Date >= cutoff30 {
					z30 = append(z30, e)
				}
			}
		}

		var latest *analysis.BodyStressEntry
		var prev *analysis.BodyStressEntry
		if len(z90) > 0 {
			latest = &z90[0]
			if len(z90) > 1 {
				prev = &z90[1]
			}
		}

		var latestScore *float64
		var latestCold *float64
		var latestSide *analysis.BodySide
		var latestDate *string
		if latest != nil {
			s := latest.PainScore
			latestScore = &s
			latestCold = latest.ColdNextDayScore
			side := latest.Side
			latestSide = &side
			d := latest.Date
			latestDate = &d
		}

		var max30 *float64
		if len(z30) > 0 {
			m := 0.0
			for _, e := range z30 {
				if e.PainScore > m {
					m = e.PainScore
				}
				if e.ColdNextDayScore != nil && *e.ColdNextDayScore > m {
					m = *e.ColdNextDayScore
				}
			}
			max30 = &m
		}

		trend := "none"
		if latest != nil {
			if prev == nil {
				trend = "stable"
			} else if latest.PainScore-prev.PainScore >= 1.0 {
				trend = "worsening"
			} else if prev.PainScore-latest.PainScore >= 1.0 {
				trend = "improving"
			} else {
				trend = "stable"
			}
		}

		zones = append(zones, analysis.BodyZoneSummary{
			Zone:                   z,
			ZoneLabel:              analysis.BodyZoneLabel(z),
			LatestScore:            latestScore,
			LatestColdNextDayScore: latestCold,
			LatestSide:             latestSide,
			LatestDate:             latestDate,
			MaxScore30d:            max30,
			EntriesCount90d:        len(z90),
			Trend:                  trend,
		})
	}

	// Filter entries for the requested window and optional zone
	var entries []analysis.BodyStressEntry
	for _, e := range all90 {
		if e.Date >= cutoffWindow {
			if zoneFilter == "" || string(e.Zone) == zoneFilter {
				entries = append(entries, e)
			}
		}
	}

	// Active zones for correlation engine
	var activeZones []analysis.BodyZoneSummary
	for _, z := range zones {
		if z.EntriesCount90d > 0 && (zoneFilter == "" || string(z.Zone) == zoneFilter) {
			activeZones = append(activeZones, z)
		}
	}

	// Query shoes for shoe correlations
	shoes, err := s.Shoes(ctx, uid)
	if err != nil {
		shoes = nil
	}
	shoeSummaries := make([]analysis.ShoeBioSummary, 0, len(shoes))
	for _, sh := range shoes {
		shoeSummaries = append(shoeSummaries, analysis.ShoeBioSummary{
			ID:         sh.ID,
			Name:       sh.Name,
			DropMm:     sh.DropMm,
			DistanceKm: sh.DistanceKm,
			Retired:    sh.Retired,
		})
	}

	// Query running activities in the last 90 days for correlation baselines
	acts, err := s.activitiesBetween(ctx, uid, cutoff90, today)
	if err != nil {
		acts = nil
	}
	actSummaries := make([]analysis.ActivityBioSummary, 0, len(acts))
	for _, a := range acts {
		actSummaries = append(actSummaries, analysis.ActivityBioSummary{
			ID:         a.ID,
			Date:       a.LocalDate,
			IsRunning:  a.isRunning(),
			ElevLossM:  a.ElevLoss,
			GctMs:      a.AvgGCT,
			ImpactLoad: a.Load,
		})
	}

	corrs := analysis.ComputeCorrelations(activeZones, all90, shoeSummaries, actSummaries)

	return &analysis.BodyMapOverview{
		Zones:        zones,
		Entries:      entries,
		Correlations: corrs,
		WindowDays:   days,
	}, nil
}

// CreateBodyStressEntry logs a new pain or discomfort score.
func (s *Services) CreateBodyStressEntry(ctx context.Context, uid int64, in analysis.BodyStressInput) (*analysis.BodyStressEntry, error) {
	if !isValidDate(in.Date) {
		return nil, badRequest("Date invalide (format AAAA-MM-JJ attendu).")
	}
	if !isValidZone(in.Zone) {
		return nil, badRequest("Zone anatomique inconnue.")
	}
	if in.PainScore < 0 || in.PainScore > 10 {
		return nil, badRequest("Score de douleur hors limites [0, 10].")
	}
	if in.ColdNextDayScore != nil && (*in.ColdNextDayScore < 0 || *in.ColdNextDayScore > 10) {
		return nil, badRequest("Score à froid J+1 hors limites [0, 10].")
	}

	side := analysis.SideBoth
	if in.Side != nil && (*in.Side == analysis.SideLeft || *in.Side == analysis.SideRight || *in.Side == analysis.SideBoth) {
		side = *in.Side
	}

	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	now := s.now().Unix()
	res, err := s.db().ExecContext(ctx, `INSERT INTO body_stress_entries
		(user_id, date, activity_id, zone, side, pain_score, cold_next_day_score, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, in.Date, in.ActivityID, string(in.Zone), string(side), in.PainScore, in.ColdNextDayScore, note, now, now)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return s.getBodyStressEntry(ctx, uid, id)
}

// PatchBodyStressEntry updates an existing pain entry.
func (s *Services) PatchBodyStressEntry(ctx context.Context, uid, id int64, in analysis.BodyStressInput) (*analysis.BodyStressEntry, error) {
	entry, err := s.getBodyStressEntry(ctx, uid, id)
	if err != nil {
		return nil, err
	}

	date := entry.Date
	if in.Date != "" {
		if !isValidDate(in.Date) {
			return nil, badRequest("Date invalide (format AAAA-MM-JJ attendu).")
		}
		date = in.Date
	}

	zone := entry.Zone
	if in.Zone != "" {
		if !isValidZone(in.Zone) {
			return nil, badRequest("Zone anatomique inconnue.")
		}
		zone = in.Zone
	}

	side := entry.Side
	if in.Side != nil {
		if *in.Side == analysis.SideLeft || *in.Side == analysis.SideRight || *in.Side == analysis.SideBoth {
			side = *in.Side
		} else {
			return nil, badRequest("Côté anatomique invalide (left | right | both).")
		}
	}

	painScore := entry.PainScore
	if in.PainScore > 0 || in.PainScore == 0 {
		if in.PainScore < 0 || in.PainScore > 10 {
			return nil, badRequest("Score de douleur hors limites [0, 10].")
		}
		painScore = in.PainScore
	}

	coldScore := entry.ColdNextDayScore
	if in.ColdNextDayScore != nil {
		if *in.ColdNextDayScore < 0 || *in.ColdNextDayScore > 10 {
			return nil, badRequest("Score à froid J+1 hors limites [0, 10].")
		}
		coldScore = in.ColdNextDayScore
	}

	actID := entry.ActivityID
	if in.ActivityID != nil {
		actID = in.ActivityID
	}

	note := entry.Note
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	now := s.now().Unix()
	_, err = s.db().ExecContext(ctx, `UPDATE body_stress_entries
		SET date = ?, activity_id = ?, zone = ?, side = ?, pain_score = ?, cold_next_day_score = ?, note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		date, actID, string(zone), string(side), painScore, coldScore, note, now, uid, id)
	if err != nil {
		return nil, err
	}

	return s.getBodyStressEntry(ctx, uid, id)
}

// DeleteBodyStressEntry removes a body stress entry.
func (s *Services) DeleteBodyStressEntry(ctx context.Context, uid, id int64) error {
	res, err := s.db().ExecContext(ctx, `DELETE FROM body_stress_entries WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Services) getBodyStressEntry(ctx context.Context, uid, id int64) (*analysis.BodyStressEntry, error) {
	row := s.db().QueryRowContext(ctx, `SELECT e.id, e.date, e.activity_id, a.name, e.zone, e.side, e.pain_score, e.cold_next_day_score, e.note, e.created_at, e.updated_at
		FROM body_stress_entries e
		LEFT JOIN activities a ON e.activity_id = a.id
		WHERE e.user_id = ? AND e.id = ?`, uid, id)
	e, err := scanBodyStressEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &e, err
}

func (s *Services) recentBodyStress(ctx context.Context, uid int64, days int) ([]analysis.BodyStressEntry, error) {
	cutoff := addDays(s.now().Format("2006-01-02"), -days)
	rows, err := s.db().QueryContext(ctx, `SELECT e.id, e.date, e.activity_id, a.name, e.zone, e.side, e.pain_score, e.cold_next_day_score, e.note, e.created_at, e.updated_at
		FROM body_stress_entries e
		LEFT JOIN activities a ON e.activity_id = a.id
		WHERE e.user_id = ? AND e.date >= ?
		ORDER BY e.date DESC, e.id DESC`, uid, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []analysis.BodyStressEntry
	for rows.Next() {
		e, err := scanBodyStressEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanBodyStressEntry(sc interface{ Scan(...any) error }) (analysis.BodyStressEntry, error) {
	var e analysis.BodyStressEntry
	var aid sql.NullInt64
	var aname sql.NullString
	var zone, side, note string
	var pain float64
	var cold sql.NullFloat64
	var cat, uat int64

	err := sc.Scan(&e.ID, &e.Date, &aid, &aname, &zone, &side, &pain, &cold, &note, &cat, &uat)
	if err != nil {
		return e, err
	}

	if aid.Valid {
		e.ActivityID = &aid.Int64
	}
	if aname.Valid {
		e.ActivityName = &aname.String
	}
	e.Zone = analysis.BodyZone(zone)
	e.ZoneLabel = analysis.BodyZoneLabel(e.Zone)
	e.Side = analysis.BodySide(side)
	e.PainScore = pain
	if cold.Valid {
		v := cold.Float64
		e.ColdNextDayScore = &v
	}
	e.Note = note
	e.CreatedAt = time.Unix(cat, 0).UTC().Format(time.RFC3339)
	e.UpdatedAt = time.Unix(uat, 0).UTC().Format(time.RFC3339)
	return e, nil
}

func isValidDate(d string) bool {
	if len(d) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", d)
	return err == nil
}

func isValidZone(z analysis.BodyZone) bool {
	for _, k := range analysis.AllBodyZones {
		if k == z {
			return true
		}
	}
	return false
}
