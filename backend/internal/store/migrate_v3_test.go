package store

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"
)

// A V2 database (0001 + 0002 with data) upgrades to V3 without losing rows.
func TestMigrateV2ToV3(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2.db")
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_platform.sql", "0002_domain.sql"} {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations VALUES (?, 1)`, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, display_name, created_at) VALUES (1, 'v2', 1)`,
		`INSERT INTO activities (id, user_id, garmin_id, start_utc, start_local, local_date, type, sport, name, distance_m, created_at, updated_at)
			VALUES (10, 1, 555, 1000, '2026-11-01T08:00:00', '2026-11-01', 'running', 'run', 'Footing', 5000, 1, 7)`,
		`INSERT INTO activity_blobs (activity_id, user_id, kind, data, fetched_at) VALUES (10, 1, 'fit', x'0102', 1), (10, 1, 'weather', x'03', 2)`,
		`INSERT INTO shoes (id, user_id, name, created_at, updated_at) VALUES (3, 1, 'Pegasus', 1, 1)`,
		`INSERT INTO activity_notes (activity_id, user_id, shoe_id, pain_during, updated_at) VALUES (10, 1, 3, 2.5, 1)`,
		`INSERT INTO planned_sessions (id, user_id, date, sport, title, created_at, updated_at) VALUES (4, 1, '2026-11-10', 'run', 'Seuil', 1, 1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	db.Close()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer s.Close()
	var blobs, notified int
	var kind string
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activity_blobs WHERE activity_id = 10`).Scan(&blobs)
	_ = s.DB.QueryRow(`SELECT notified_at FROM activities WHERE id = 10`).Scan(&notified)
	if blobs != 2 || notified != 7 {
		t.Fatalf("blobs=%d notified=%d", blobs, notified)
	}
	var steps, source, gstatus string
	var auto, shoe int
	if err := s.DB.QueryRow(`SELECT steps, source, garmin_status FROM planned_sessions WHERE id = 4`).Scan(&steps, &source, &gstatus); err != nil ||
		steps != "[]" || source != "manual" || gstatus != "none" {
		t.Fatalf("planned: %v %s %s %s", err, steps, source, gstatus)
	}
	if err := s.DB.QueryRow(`SELECT shoe_id, shoe_auto FROM activity_notes WHERE activity_id = 10`).Scan(&shoe, &auto); err != nil || shoe != 3 || auto != 0 {
		t.Fatalf("notes: %v", err)
	}
	if _, err := s.DB.Exec(`INSERT INTO activity_blobs (activity_id, user_id, kind, data, fetched_at) VALUES (10, 1, 'import_gpx', x'00', 3)`); err != nil {
		t.Fatalf("new blob kind: %v", err)
	}
	if _, err := s.DB.Exec(`INSERT INTO activity_blobs (activity_id, user_id, kind, data, fetched_at) VALUES (10, 1, 'bogus', x'00', 3)`); err == nil {
		t.Fatal("CHECK constraint lost")
	}
	_ = s.DB.QueryRow(`SELECT kind FROM activity_blobs WHERE activity_id = 10 AND kind = 'fit'`).Scan(&kind)
	if _, err := s.DB.Exec(`DELETE FROM activities WHERE id = 10`); err != nil {
		t.Fatal(err)
	}
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activity_blobs`).Scan(&blobs)
	if kind != "fit" || blobs != 0 {
		t.Fatalf("cascade after rebuild: kind=%q blobs=%d", kind, blobs)
	}
	for _, tbl := range []string{"llm_configs", "llm_analyses", "notification_targets", "activity_metrics"} {
		if _, err := s.DB.Exec(`SELECT COUNT(*) FROM ` + tbl); err != nil {
			t.Fatalf("%s: %v", tbl, err)
		}
	}
}

// A 3.0 database (0001–0004 with data) upgrades to 3.1 at startup.
func TestMigrate30To31(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v30.db")
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_platform.sql", "0002_domain.sql", "0003_v3.sql", "0004_logs.sql"} {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations VALUES (?, 1)`, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, display_name, created_at) VALUES (1, 'v30', 1)`,
		`INSERT INTO activities (id, user_id, garmin_id, start_utc, start_local, local_date, type, sport, name, distance_m, created_at, updated_at)
			VALUES (10, 1, 555, 1000, '2026-11-01T08:00:00', '2026-11-01', 'running', 'run', 'Footing', 5000, 1, 7)`,
		`INSERT INTO activity_metrics (activity_id, user_id, version, bundle_at, data) VALUES (10, 1, 1, 0, '{}')`,
		`INSERT INTO planned_sessions (id, user_id, date, sport, title, created_at, updated_at) VALUES (4, 1, '2026-11-10', 'run', 'Seuil', 1, 1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	db.Close()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer s.Close()
	var manual int
	var kudos sql.NullInt64
	if err := s.DB.QueryRow(`SELECT manual, kudos FROM activities WHERE id = 10`).Scan(&manual, &kudos); err != nil || manual != 0 || kudos.Valid {
		t.Fatalf("activities: %v manual=%d kudos=%v", err, manual, kudos)
	}
	for _, stmt := range []string{
		`INSERT INTO activity_records (activity_id, user_id, version, bundle_at, data) VALUES (10, 1, 1, 0, '{}')`,
		`INSERT INTO personal_records (user_id, key, distance_m, time_s, updated_at) VALUES (1, '5k', 5000, 1300, 1)`,
		`INSERT INTO calendar_events (user_id, kind, start_date, end_date, severity, created_at, updated_at) VALUES (1, 'illness', '2026-11-02', '2026-11-03', 2, 1, 1)`,
		`INSERT INTO plan_phases (user_id, name, start_date, end_date, created_at, updated_at) VALUES (1, 'Développement', '2026-11-02', '2026-11-30', 1, 1)`,
	} {
		if _, err := s.DB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if _, err := s.DB.Exec(`INSERT INTO calendar_events (user_id, kind, start_date, end_date, severity, created_at, updated_at) VALUES (1, 'bogus', 'a', 'b', 2, 1, 1)`); err == nil {
		t.Fatal("event kind CHECK missing")
	}
	if _, err := s.DB.Exec(`DELETE FROM activities WHERE id = 10`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activity_records`).Scan(&n)
	if n != 0 {
		t.Fatalf("activity_records cascade: %d", n)
	}
}
