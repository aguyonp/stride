package database_test

import (
	"context"
	"testing"

	"stride/internal/database"
)

// 00011 is additive (share permissions off for existing links, notes, prescriptions, morph trace);
// its down migration rebuilds planned_sessions and calendar_events and keeps every 4.1 column.
func TestPortalMigrationUpDownLossless(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, "portal.db")
	p, err := database.NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 10); err != nil {
		t.Fatalf("up to 10: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, display_name, created_at) VALUES (1, 'alice', 100)`,
		`INSERT INTO activities (id, user_id, start_utc, start_local, local_date, type, sport, name, created_at, updated_at)
			VALUES (5, 1, 1000, '2026-11-01T10:00:00', '2026-11-01', 'running', 'run', 'Footing', 100, 100)`,
		`INSERT INTO training_plans (id, user_id, name, sport, start_date, end_date, created_at, updated_at) VALUES (1, 1, 'Semi', 'run', '2026-11-02', '2026-12-27', 100, 100)`,
		`INSERT INTO share_links (id, user_id, label, token_hash, token_prefix, url_ciphertext, created_at, scope_json) VALUES (3, 1, 'Kiné', x'01', 'st4_abcd', x'02', 100, '{}')`,
		`INSERT INTO planned_sessions (id, user_id, plan_id, date, sport, title, steps, source, garmin_status, garmin_workout_id, created_at, updated_at)
			VALUES (7, 1, 1, '2026-11-03', 'run', 'Seuil', '[{"type":"run","durationS":600}]', 'llm', 'pushed', 42, 100, 200)`,
		`INSERT INTO calendar_events (id, user_id, kind, start_date, end_date, severity, note, created_at, updated_at)
			VALUES (9, 1, 'pain', '2026-11-04', '2026-11-05', 3, 'genou', 100, 200)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	var c, pl, ad int
	if err := db.QueryRowContext(ctx, `SELECT allow_comments, allow_planning, allow_activity_details FROM share_links WHERE id = 3`).Scan(&c, &pl, &ad); err != nil || c+pl+ad != 0 {
		t.Fatalf("existing link gained permissions: %d %d %d %v", c, pl, ad, err)
	}
	for _, stmt := range []string{
		`INSERT INTO share_notes (user_id, share_link_id, link_label, kind, author_role, body, created_at, updated_at) VALUES (1, 3, 'Kiné', 'directive', 'physio', 'Pas de fractionné', 1, 1)`,
		`INSERT INTO share_notes (user_id, share_link_id, link_label, kind, activity_id, body, created_at, updated_at) VALUES (1, 3, 'Kiné', 'remark', 5, 'Bien', 1, 1)`,
		`UPDATE planned_sessions SET share_link_id = 3, prescribed_label = 'Kiné', source = 'share', morph_json = '{}' WHERE id = 7`,
		`UPDATE calendar_events SET share_link_id = 3, prescribed_label = 'Kiné' WHERE id = 9`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("4.2 write: %v", err)
		}
	}
	for _, bad := range []string{
		`INSERT INTO share_notes (user_id, share_link_id, kind, body, created_at, updated_at) VALUES (1, 3, 'directive', 'doublon', 1, 1)`,
		`INSERT INTO share_notes (user_id, share_link_id, kind, activity_id, body, created_at, updated_at) VALUES (1, 3, 'remark', 5, 'doublon', 1, 1)`,
		`INSERT INTO share_notes (user_id, kind, body, created_at, updated_at) VALUES (1, 'remark', 'sans séance', 1, 1)`,
		`INSERT INTO share_notes (user_id, kind, author_role, body, created_at, updated_at) VALUES (1, 'directive', 'doctor', 'x', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, bad); err == nil {
			t.Fatalf("accepted: %s", bad)
		}
	}
	// Deleting the link keeps the notes and prescriptions (label snapshot), without the link id.
	if _, err := db.ExecContext(ctx, `DELETE FROM share_links WHERE id = 3`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM share_notes WHERE share_link_id IS NULL`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("notes after link delete: %d %v", n, err)
	}
	var label string
	if err := db.QueryRowContext(ctx, `SELECT prescribed_label FROM planned_sessions WHERE id = 7 AND share_link_id IS NULL`).Scan(&label); err != nil || label != "Kiné" {
		t.Fatalf("prescription after link delete: %q %v", label, err)
	}

	if err := database.DownTo(ctx, db, 10); err != nil {
		t.Fatalf("down to 10: %v", err)
	}
	var (
		date, title, steps, source, gstatus string
		planID, wid, created, updated       int64
	)
	if err := db.QueryRowContext(ctx, `SELECT date, title, steps, source, garmin_status, garmin_workout_id, plan_id, created_at, updated_at
		FROM planned_sessions WHERE id = 7`).Scan(&date, &title, &steps, &source, &gstatus, &wid, &planID, &created, &updated); err != nil {
		t.Fatal(err)
	}
	if date != "2026-11-03" || title != "Seuil" || steps != `[{"type":"run","durationS":600}]` || source != "manual" || gstatus != "pushed" ||
		wid != 42 || planID != 1 || created != 100 || updated == 0 {
		t.Fatalf("planned row changed: %s %s %s %s %s %d %d %d", date, title, steps, source, gstatus, wid, planID, created)
	}
	var note string
	var sev int
	if err := db.QueryRowContext(ctx, `SELECT note, severity FROM calendar_events WHERE id = 9`).Scan(&note, &sev); err != nil || note != "genou" || sev != 3 {
		t.Fatalf("event row changed: %q %d %v", note, sev, err)
	}
	var idx int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name IN
		('planned_sessions_user_date', 'planned_sessions_plan', 'planned_sessions_week', 'calendar_events_user')`).Scan(&idx); err != nil || idx != 4 {
		t.Fatalf("indexes: %d %v", idx, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM training_plans WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var orphan *int64
	if err := db.QueryRowContext(ctx, `SELECT plan_id FROM planned_sessions WHERE id = 7`).Scan(&orphan); err != nil || orphan != nil {
		t.Fatalf("plan_id lost its ON DELETE SET NULL: %v %v", orphan, err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
}
