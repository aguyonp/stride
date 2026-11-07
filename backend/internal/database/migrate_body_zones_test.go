package database_test

import (
	"context"
	"testing"

	"stride/internal/database"
)

// 00010 rebuilds body_stress_entries to accept the 4.1 zones: every row survives the upgrade,
// the down migration keeps the 4.0 rows and drops only the added zones (entries and tracking).
func TestBodyZonesMigrationPreservesRows(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, "zones.db")
	p, err := database.NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 9); err != nil {
		t.Fatalf("up to 9: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, display_name, created_at) VALUES (1, 'alice', 100)`,
		`INSERT INTO activities (id, user_id, start_utc, start_local, local_date, type, sport, name, created_at, updated_at)
			VALUES (5, 1, 1000, '2026-11-01T10:00:00', '2026-11-01', 'running', 'run', 'Footing', 100, 100)`,
		`INSERT INTO body_stress_entries (id, user_id, date, activity_id, zone, side, pain_score, cold_next_day_score, note, created_at, updated_at)
			VALUES (11, 1, '2026-11-01', 5, 'knee_pfps', 'left', 4, 2.5, 'escaliers', 100, 200)`,
		`INSERT INTO body_stress_entries (id, user_id, date, zone, side, pain_score, created_at, updated_at)
			VALUES (12, 1, '2026-11-02', 'achilles', 'both', 3, 100, 100)`,
		`INSERT INTO health_status (user_id, zones, updated_at) VALUES (1, '[{"zone":"achilles","side":"left","since":null,"note":""}]', 100)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO body_stress_entries (user_id, date, zone, pain_score, created_at, updated_at) VALUES (1, '2026-11-03', 'ankle', 5, 1, 1)`); err == nil {
		t.Fatal("ankle accepted before 00010")
	}

	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	var (
		date, zone, side, note string
		aid                    int64
		pain, cold             float64
		created, updated       int64
	)
	if err := db.QueryRowContext(ctx, `SELECT date, activity_id, zone, side, pain_score, cold_next_day_score, note, created_at, updated_at
		FROM body_stress_entries WHERE id = 11`).Scan(&date, &aid, &zone, &side, &pain, &cold, &note, &created, &updated); err != nil {
		t.Fatal(err)
	}
	if date != "2026-11-01" || aid != 5 || zone != "knee_pfps" || side != "left" || pain != 4 || cold != 2.5 || note != "escaliers" || created != 100 || updated != 200 {
		t.Fatalf("row changed: %s %d %s %s %v %v %s %d %d", date, aid, zone, side, pain, cold, note, created, updated)
	}
	for _, z := range []string{"ankle", "forefoot", "hip", "lower_back"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO body_stress_entries (user_id, date, zone, pain_score, created_at, updated_at) VALUES (1, '2026-11-04', ?, 5, 1, 1)`, z); err != nil {
			t.Fatalf("%s refused after 00010: %v", z, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO body_stress_entries (user_id, date, zone, pain_score, created_at, updated_at) VALUES (1, '2026-11-04', 'elbow', 5, 1, 1)`); err == nil {
		t.Fatal("unknown zone accepted")
	}
	var idx int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name IN ('body_stress_entries_user_date', 'body_stress_entries_activity')`).Scan(&idx); err != nil || idx != 2 {
		t.Fatalf("indexes: %d %v", idx, err)
	}
	// activity_id keeps its ON DELETE SET NULL
	if _, err := db.ExecContext(ctx, `DELETE FROM activities WHERE id = 5`); err != nil {
		t.Fatal(err)
	}
	var orphan *int64
	if err := db.QueryRowContext(ctx, `SELECT activity_id FROM body_stress_entries WHERE id = 11`).Scan(&orphan); err != nil || orphan != nil {
		t.Fatalf("activity link: %v %v", orphan, err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE health_status SET zones = '[{"zone":"ankle","side":"left","condition":"ankle_fracture"},{"zone":"achilles","side":"right","condition":"achilles_tendinopathy"}]' WHERE user_id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := database.DownTo(ctx, db, 9); err != nil {
		t.Fatalf("down to 9: %v", err)
	}
	var n, added int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE zone NOT IN ('knee_pfps', 'achilles')) FROM body_stress_entries`).Scan(&n, &added); err != nil || n != 2 || added != 0 {
		t.Fatalf("down rows: %d (%d added) %v", n, added, err)
	}
	var zones string
	if err := db.QueryRowContext(ctx, `SELECT zones FROM health_status WHERE user_id = 1`).Scan(&zones); err != nil ||
		zones != `[{"zone":"achilles","side":"right","condition":"achilles_tendinopathy"}]` {
		t.Fatalf("down health: %s %v", zones, err)
	}
}
