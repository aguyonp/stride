package domain

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"stride/internal/analysis"
)

const bodyEntrySelect = `SELECT e.id, e.date, e.activity_id, a.name, e.zone, e.side, e.pain_score, e.cold_next_day_score, e.note, e.created_at, e.updated_at
	FROM body_stress_entries e LEFT JOIN activities a ON a.id = e.activity_id AND a.user_id = e.user_id`

// BodyMap builds the body stress overview: zone summaries (30/90 days), the entries of the last
// days (7 to 365) and the correlations computed on the runs of the last 90 days.
func (s *Services) BodyMap(ctx context.Context, uid int64, days int, zoneFilter string) (*analysis.BodyMapOverview, error) {
	if days == 0 {
		days = 90
	}
	if days < 7 || days > 3650 {
		return nil, badRequest("days doit être compris entre 7 et 3650.")
	}
	if zoneFilter != "" && !isValidZone(analysis.BodyZone(zoneFilter)) {
		return nil, badRequest("Zone anatomique inconnue.")
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	cutoff90 := addDays(today, -90)
	all, err := s.bodyStressSince(ctx, uid, min(addDays(today, -days), cutoff90))
	if err != nil {
		return nil, err
	}
	var entries, last90 []analysis.BodyStressEntry
	for _, e := range all {
		if e.Date >= addDays(today, -days) && (zoneFilter == "" || string(e.Zone) == zoneFilter) {
			entries = append(entries, e)
		}
		if e.Date >= cutoff90 {
			last90 = append(last90, e)
		}
	}
	zones := make([]analysis.BodyZoneSummary, 0, len(analysis.AllBodyZones))
	var active []analysis.BodyZoneSummary
	for _, z := range analysis.AllBodyZones {
		zs := zoneSummary(z, last90, addDays(today, -30))
		zones = append(zones, zs)
		if zs.EntriesCount90d > 0 && (zoneFilter == "" || string(z) == zoneFilter) {
			active = append(active, zs)
		}
	}
	corrs := []analysis.BodyCorrelationFactor{}
	if len(active) > 0 {
		if corrs, err = s.bodyCorrelations(ctx, uid, active, last90, cutoff90, today); err != nil {
			return nil, err
		}
	}
	if entries == nil {
		entries = []analysis.BodyStressEntry{}
	}
	return &analysis.BodyMapOverview{Zones: zones, Entries: entries, Correlations: corrs, WindowDays: days}, nil
}

// zoneSummary summarizes the entries of one zone (entries are sorted newest first).
func zoneSummary(z analysis.BodyZone, entries []analysis.BodyStressEntry, cutoff30 string) analysis.BodyZoneSummary {
	out := analysis.BodyZoneSummary{Zone: z, ZoneLabel: analysis.BodyZoneLabel(z), Trend: "none"}
	var latest, prev *analysis.BodyStressEntry
	for i := range entries {
		e := &entries[i]
		if e.Zone != z {
			continue
		}
		out.EntriesCount90d++
		switch {
		case latest == nil:
			latest = e
		case prev == nil:
			prev = e
		}
		if e.Date >= cutoff30 {
			m := e.PainScore
			if e.ColdNextDayScore != nil {
				m = max(m, *e.ColdNextDayScore)
			}
			if out.MaxScore30d == nil || m > *out.MaxScore30d {
				out.MaxScore30d = &m
			}
		}
	}
	if latest == nil {
		return out
	}
	score, side, date := latest.PainScore, latest.Side, latest.Date
	out.LatestScore, out.LatestColdNextDayScore, out.LatestSide, out.LatestDate = &score, latest.ColdNextDayScore, &side, &date
	out.Trend = "stable"
	if prev != nil {
		switch d := latest.PainScore - prev.PainScore; {
		case d >= 1:
			out.Trend = "worsening"
		case d <= -1:
			out.Trend = "improving"
		}
	}
	return out
}

// bodyCorrelations feeds the correlation engine with the user's shoes and activities since from.
func (s *Services) bodyCorrelations(ctx context.Context, uid int64, active []analysis.BodyZoneSummary, entries []analysis.BodyStressEntry, from, today string) ([]analysis.BodyCorrelationFactor, error) {
	shoes, err := s.Shoes(ctx, uid)
	if err != nil {
		return nil, err
	}
	shoeSums := make([]analysis.ShoeBioSummary, 0, len(shoes))
	for _, sh := range shoes {
		shoeSums = append(shoeSums, analysis.ShoeBioSummary{ID: sh.ID, Name: sh.Name, DropMm: sh.DropMm, DistanceKm: sh.DistanceKm})
	}
	acts, err := s.activitiesBetween(ctx, uid, from, today)
	if err != nil {
		return nil, err
	}
	shoeOf, err := s.activityShoes(ctx, uid, from)
	if err != nil {
		return nil, err
	}
	actSums := make([]analysis.ActivityBioSummary, 0, len(acts))
	for _, a := range acts {
		sum := analysis.ActivityBioSummary{ID: a.ID, Date: a.LocalDate, IsRunning: a.isRunning(), ElevLossM: a.ElevLoss, GctMs: a.AvgGCT}
		if a.DistanceM != nil {
			km := *a.DistanceM / 1000
			sum.DistanceKm = &km
		}
		if id, ok := shoeOf[a.ID]; ok {
			sum.ShoeID = &id
		}
		actSums = append(actSums, sum)
	}
	return analysis.ComputeCorrelations(active, entries, shoeSums, actSums), nil
}

// activityShoes maps activities since from to their shoe: the note's shoe, else the shoe matching
// the Strava gear (same rule as the shoe mileage).
func (s *Services) activityShoes(ctx context.Context, uid int64, from string) (map[int64]int64, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT a.id, COALESCE(n.shoe_id,
			(SELECT sh.id FROM shoes sh WHERE sh.user_id = a.user_id AND sh.strava_gear_id IS NOT NULL AND sh.strava_gear_id = a.strava_gear))
		FROM activities a LEFT JOIN activity_notes n ON n.activity_id = a.id AND n.user_id = a.user_id
		WHERE a.user_id = ? AND a.local_date >= ?`, uid, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id int64
		var shoe sql.NullInt64
		if err := rows.Scan(&id, &shoe); err != nil {
			return nil, err
		}
		if shoe.Valid {
			out[id] = shoe.Int64
		}
	}
	return out, rows.Err()
}

// ownActivity returns ErrNotFound unless the activity exists and belongs to uid.
func (s *Services) ownActivity(ctx context.Context, uid int64, id *int64) error {
	if id == nil {
		return nil
	}
	var one int
	err := s.db().QueryRowContext(ctx, `SELECT 1 FROM activities WHERE id = ? AND user_id = ?`, *id, uid).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// CreateBodyStressEntry logs a pain score; an entry linked to an activity replaces the one of the
// same activity and zone (the Ressenti form writes one entry per zone).
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
	if err := s.ownActivity(ctx, uid, in.ActivityID); err != nil {
		return nil, err
	}
	if in.ActivityID != nil {
		in.Side = &side
		id, err := s.upsertActivityEntry(ctx, uid, in)
		if err != nil {
			return nil, err
		}
		return s.getBodyStressEntry(ctx, uid, id)
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

	if in.PainScore < 0 || in.PainScore > 10 {
		return nil, badRequest("Score de douleur hors limites [0, 10].")
	}
	painScore := in.PainScore

	coldScore := entry.ColdNextDayScore
	if in.ColdNextDayScore != nil {
		if *in.ColdNextDayScore < 0 || *in.ColdNextDayScore > 10 {
			return nil, badRequest("Score à froid J+1 hors limites [0, 10].")
		}
		coldScore = in.ColdNextDayScore
	}

	actID := entry.ActivityID
	if in.ActivityID != nil {
		if err := s.ownActivity(ctx, uid, in.ActivityID); err != nil {
			return nil, err
		}
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
	e, err := scanBodyStressEntry(s.db().QueryRowContext(ctx, bodyEntrySelect+` WHERE e.user_id = ? AND e.id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &e, err
}

// bodyStressSince lists the entries dated from since, newest first.
func (s *Services) bodyStressSince(ctx context.Context, uid int64, since string) ([]analysis.BodyStressEntry, error) {
	rows, err := s.db().QueryContext(ctx, bodyEntrySelect+` WHERE e.user_id = ? AND e.date >= ? ORDER BY e.date DESC, e.id DESC`, uid, since)
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

// bodyPromptContext returns the pain entries of the last 30 days and the conclusive correlations.
func (s *Services) bodyPromptContext(ctx context.Context, uid int64) ([]analysis.BodyStressEntry, []analysis.BodyCorrelationFactor, error) {
	bm, err := s.BodyMap(ctx, uid, 30, "")
	if err != nil {
		return nil, nil, err
	}
	var corrs []analysis.BodyCorrelationFactor
	for _, c := range bm.Correlations {
		if !c.InsufficientData {
			corrs = append(corrs, c)
		}
	}
	return bm.Entries, corrs, nil
}
