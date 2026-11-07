package database_test

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"stride/internal/database"
)

func openTestDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrateUpDownReUpAndVerify(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, "fresh.db")

	// 1. Fresh Up to v6 + VerifySchema
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("fresh migrate up: %v", err)
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		t.Fatalf("verify after fresh up: %v", err)
	}

	// Insert live v4 rows before rolling back to verify Down migrations handle data cleanly.
	for _, stmt := range []string{
		`INSERT INTO users (id, display_name, created_at) VALUES (1, 'alice', 100)`,
		`INSERT INTO training_plans (id, user_id, name, sport, start_date, end_date, created_at, updated_at) VALUES (1, 1, 'Semi', 'run', '2026-11-02', '2026-12-27', 100, 100)`,
		`INSERT INTO plan_phases (id, user_id, plan_id, name, phase_type, order_index, start_date, end_date, created_at, updated_at) VALUES (1, 1, 1, 'Fondation', 'foundation', 0, '2026-11-02', '2026-11-22', 100, 100)`,
		`INSERT INTO training_weeks (id, user_id, plan_id, phase_id, week_start, week_end, week_number, created_at, updated_at) VALUES (1, 1, 1, 1, '2026-11-02', '2026-11-08', 1, 100, 100)`,
		`INSERT INTO planned_sessions (id, user_id, plan_id, phase_id, week_id, date, sport, title, created_at, updated_at) VALUES (1, 1, 1, 1, 1, '2026-11-03', 'run', 'Footing', 100, 100)`,
		`INSERT INTO body_stress_entries (user_id, date, zone, side, pain_score, created_at, updated_at) VALUES (1, '2026-11-03', 'knee_pfps', 'left', 4, 100, 100)`,
		`INSERT INTO share_links (user_id, label, token_hash, token_prefix, url_ciphertext, created_at) VALUES (1, 'Kiné', x'01020304', 'st4_abcd', x'0908', 100)`,
		`INSERT INTO async_jobs (id, user_id, kind, queue, state, created_at, updated_at) VALUES ('job-1', 1, 'sync.provider', 'default', 'completed', 100, 100)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed stmt %s: %v", stmt, err)
		}
	}

	// 2. Full Down to 0
	if err := database.DownTo(ctx, db, 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	var userTables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'`).Scan(&userTables); err != nil || userTables != 0 {
		t.Fatalf("expected users table dropped at v0, got count=%d err=%v", userTables, err)
	}

	// 3. Re-Up to latest + VerifySchema
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("re-migrate up: %v", err)
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		t.Fatalf("verify after re-up: %v", err)
	}
}

func TestLegacyV31UpgradePreservesData(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, "legacy31.db")

	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		legacy, file string
	}{
		{"0001_platform.sql", "00001_platform.up.sql"},
		{"0002_domain.sql", "00002_domain.up.sql"},
		{"0003_v3.sql", "00003_v3.up.sql"},
		{"0004_logs.sql", "00004_v3_backfill.up.sql"},
		{"0005_v31.sql", "00005_v31.up.sql"},
	} {
		raw, err := database.RawMigration(pair.file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", pair.legacy, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, 1)`, pair.legacy); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, display_name, created_at) VALUES (7, 'legacy', 42)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO activities (id, user_id, start_utc, start_local, local_date, type, sport, name, created_at, updated_at)
		VALUES (99, 7, 1000, '2026-11-01T10:00:00', '2026-11-01', 'running', 'run', 'Sortie V3.1', 42, 42)`); err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("legacy upgrade: %v", err)
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		t.Fatalf("verify after legacy upgrade: %v", err)
	}

	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM activities WHERE id = 99 AND user_id = 7`).Scan(&name); err != nil || name != "Sortie V3.1" {
		t.Fatalf("preserved row missing: %v %q", err, name)
	}
	p, err := database.NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := p.GetDBVersion(ctx)
	if err != nil || ver != 6 {
		t.Fatalf("expected goose version 6, got %d (%v)", ver, err)
	}
}

func TestVerifySchemaDetectsTampering(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, "tampered.db")
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	// Tamper 1: drop a required v4 column
	if _, err := db.ExecContext(ctx, `ALTER TABLE training_plans DROP COLUMN target_event_label`); err != nil {
		t.Fatal(err)
	}
	err := database.VerifySchema(ctx, db)
	if err == nil || !strings.Contains(err.Error(), "target_event_label") {
		t.Fatalf("expected missing column error, got %v", err)
	}

	// Tamper 2: drop a required v4 table
	if _, err := db.ExecContext(ctx, `DROP TABLE share_links`); err != nil {
		t.Fatal(err)
	}
	err = database.VerifySchema(ctx, db)
	if err == nil || !strings.Contains(err.Error(), "share_links") {
		t.Fatalf("expected missing table error, got %v", err)
	}
}

func TestOrphanRowsNeverBlockVerifyAndShowInDoctor(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, "orphans.db")
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if p, err := database.CheckIntegrity(ctx, db); err != nil || len(p) != 0 {
		t.Fatalf("clean database: %v %v", p, err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO goals (user_id, title, kind, created_at, updated_at) VALUES (404, 'orphan', 'race', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		t.Fatalf("orphan rows must not fail the boot check: %v", err)
	}
	fk, err := database.ForeignKeyViolations(ctx, db)
	if err != nil || len(fk) != 1 || !strings.Contains(fk[0], "goals") {
		t.Fatalf("foreign keys: %v %v", fk, err)
	}
	if p, _ := database.CheckIntegrity(ctx, db); len(p) != 1 {
		t.Fatalf("doctor: %v", p)
	}
}
