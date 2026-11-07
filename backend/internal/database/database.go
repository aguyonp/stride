// Package database manages embedded SQLite schema migrations via goose/v3,
// lossless legacy v3.x migration state upgrades, and startup schema verification.
package database

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var rawMigrationsFS embed.FS

// RawMigration returns the raw bytes of an embedded migration file (e.g. "00001_platform.up.sql").
func RawMigration(name string) ([]byte, error) {
	return rawMigrationsFS.ReadFile(path.Join("migrations", name))
}

type memFile struct {
	name string
	data []byte
}

type memFileInfo struct {
	name string
	size int64
	dir  bool
}

func (fi memFileInfo) Name() string       { return fi.name }
func (fi memFileInfo) Size() int64        { return fi.size }
func (fi memFileInfo) Mode() fs.FileMode  { return 0o444 }
func (fi memFileInfo) ModTime() time.Time { return time.Time{} }
func (fi memFileInfo) IsDir() bool        { return fi.dir }
func (fi memFileInfo) Sys() any           { return nil }

type memDirEntry struct{ fi memFileInfo }

func (d memDirEntry) Name() string               { return d.fi.name }
func (d memDirEntry) IsDir() bool                { return d.fi.dir }
func (d memDirEntry) Type() fs.FileMode          { return d.fi.Mode().Type() }
func (d memDirEntry) Info() (fs.FileInfo, error) { return d.fi, nil }

type openMemFile struct {
	fi memFileInfo
	r  *bytes.Reader
}

func (f *openMemFile) Stat() (fs.FileInfo, error) { return f.fi, nil }
func (f *openMemFile) Read(b []byte) (int, error) { return f.r.Read(b) }
func (f *openMemFile) Close() error               { return nil }

type openMemDir struct {
	fi      memFileInfo
	entries []fs.DirEntry
	offset  int
}

func (d *openMemDir) Stat() (fs.FileInfo, error) { return d.fi, nil }
func (d *openMemDir) Read([]byte) (int, error)   { return 0, io.EOF }
func (d *openMemDir) Close() error               { return nil }
func (d *openMemDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.offset >= len(d.entries) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	if n <= 0 || d.offset+n > len(d.entries) {
		out := d.entries[d.offset:]
		d.offset = len(d.entries)
		return out, nil
	}
	out := d.entries[d.offset : d.offset+n]
	d.offset += n
	return out, nil
}

// combinedFS synthesizes goose "-- +goose Up" / "-- +goose Down" virtual SQL files
// from "<NNNNN>_<name>.up.sql" and "<NNNNN>_<name>.down.sql" pairs.
type combinedFS struct {
	files map[string]memFile
	order []string
}

func (c *combinedFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		entries := make([]fs.DirEntry, 0, len(c.order))
		for _, k := range c.order {
			f := c.files[k]
			entries = append(entries, memDirEntry{fi: memFileInfo{name: f.name, size: int64(len(f.data))}})
		}
		return &openMemDir{fi: memFileInfo{name: ".", dir: true}, entries: entries}, nil
	}
	f, ok := c.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &openMemFile{
		fi: memFileInfo{name: f.name, size: int64(len(f.data))},
		r:  bytes.NewReader(f.data),
	}, nil
}

func (c *combinedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries := make([]fs.DirEntry, 0, len(c.order))
	for _, k := range c.order {
		f := c.files[k]
		entries = append(entries, memDirEntry{fi: memFileInfo{name: f.name, size: int64(len(f.data))}})
	}
	return entries, nil
}

var (
	fsOnce   sync.Once
	cachedFS fs.FS
	fsErr    error
)

// MigrationsFS returns the virtual fs.FS formatted for goose.NewProvider.
func MigrationsFS() (fs.FS, error) {
	fsOnce.Do(func() {
		entries, err := rawMigrationsFS.ReadDir("migrations")
		if err != nil {
			fsErr = err
			return
		}
		ups := map[string][]byte{}
		downs := map[string][]byte{}
		bases := map[string]bool{}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			switch {
			case strings.HasSuffix(n, ".up.sql"):
				base := strings.TrimSuffix(n, ".up.sql")
				b, err := rawMigrationsFS.ReadFile(path.Join("migrations", n))
				if err != nil {
					fsErr = err
					return
				}
				ups[base] = b
				bases[base] = true
			case strings.HasSuffix(n, ".down.sql"):
				base := strings.TrimSuffix(n, ".down.sql")
				b, err := rawMigrationsFS.ReadFile(path.Join("migrations", n))
				if err != nil {
					fsErr = err
					return
				}
				downs[base] = b
				bases[base] = true
			}
		}
		cfs := &combinedFS{files: make(map[string]memFile, len(bases))}
		for base := range bases {
			up, ok := ups[base]
			if !ok {
				fsErr = fmt.Errorf("database: missing .up.sql for %s", base)
				return
			}
			var buf bytes.Buffer
			buf.WriteString("-- +goose Up\n")
			buf.Write(up)
			if !bytes.HasSuffix(up, []byte("\n")) {
				buf.WriteByte('\n')
			}
			if down, ok := downs[base]; ok && len(bytes.TrimSpace(down)) > 0 {
				buf.WriteString("\n-- +goose Down\n")
				buf.Write(down)
				if !bytes.HasSuffix(down, []byte("\n")) {
					buf.WriteByte('\n')
				}
			}
			fname := base + ".sql"
			cfs.files[fname] = memFile{name: fname, data: buf.Bytes()}
			cfs.order = append(cfs.order, fname)
		}
		sort.Strings(cfs.order)
		cachedFS = cfs
	})
	return cachedFS, fsErr
}

// NewProvider creates an isolated goose Provider bound to db without mutating global state.
func NewProvider(db *sql.DB) (*goose.Provider, error) {
	fsys, err := MigrationsFS()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectSQLite3, db, fsys)
}

var legacyVersionMap = map[string]int64{
	"0001_platform.sql":    1,
	"0002_domain.sql":      2,
	"0003_v3.sql":          3,
	"0004_logs.sql":        4,
	"0004_v3_backfill.sql": 4,
	"0005_v31.sql":         5,
}

// bootstrapLegacyMigrations imports applied rows from the pre-v4 schema_migrations table
// into goose_db_version so existing v2/v3.x SQLite databases upgrade losslessly.
func bootstrapLegacyMigrations(ctx context.Context, db *sql.DB, p *goose.Provider) error {
	var hasLegacy int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&hasLegacy); err != nil {
		return err
	}
	if hasLegacy == 0 {
		return nil
	}
	cur, err := p.GetDBVersion(ctx)
	if err != nil {
		return err
	}
	if cur > 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	var versions []int64
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if v, ok := legacyVersionMap[name]; ok && !seen[v] {
			seen[v] = true
			versions = append(versions, v)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(versions) == 0 {
		return nil
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, v := range versions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, 1)`, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Migrate upgrades db to the latest schema version.
func Migrate(ctx context.Context, db *sql.DB) error {
	p, err := NewProvider(db)
	if err != nil {
		return err
	}
	if err := bootstrapLegacyMigrations(ctx, db, p); err != nil {
		return fmt.Errorf("database legacy bootstrap: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("database migrate up: %w", err)
	}
	return nil
}

// HasPending reports whether migrations remain to apply on a non-empty database: a fresh file
// (no table besides goose's own) answers false, there is nothing worth a snapshot.
func HasPending(ctx context.Context, db *sql.DB) (bool, error) {
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'
		AND name NOT IN ('goose_db_version', 'sqlite_sequence')`).Scan(&tables); err != nil {
		return false, err
	}
	if tables == 0 {
		return false, nil
	}
	p, err := NewProvider(db)
	if err != nil {
		return false, err
	}
	if err := bootstrapLegacyMigrations(ctx, db, p); err != nil {
		return false, fmt.Errorf("database legacy bootstrap: %w", err)
	}
	return p.HasPending(ctx)
}

// DownTo rolls back migrations down to targetVersion (0 rolls back all migrations).
func DownTo(ctx context.Context, db *sql.DB, targetVersion int64) error {
	p, err := NewProvider(db)
	if err != nil {
		return err
	}
	if _, err := p.DownTo(ctx, targetVersion); err != nil {
		return fmt.Errorf("database migrate down to %d: %w", targetVersion, err)
	}
	return nil
}

var expectedSchema = map[string][]string{
	"users":                {"id", "display_name", "role", "status"},
	"webauthn_credentials": {"id", "user_id", "credential"},
	"sessions":             {"token_hash", "user_id", "expires_at"},
	"invitations":          {"id", "code_hash", "code_prefix"},
	"app_settings":         {"key", "value"},
	"api_events":           {"id", "at_ms", "provider", "status"},
	"app_logs":             {"id", "at_ms", "category", "level"},
	"provider_connections": {"user_id", "provider", "status"},
	"activities":           {"id", "user_id", "garmin_id", "strava_id", "import_format", "notified_at", "kudos", "manual"},
	"activity_blobs":       {"activity_id", "user_id", "kind", "data"},
	"wellness_days":        {"user_id", "date", "kind", "data"},
	"athlete_profiles":     {"user_id", "focus_area", "timezone"},
	"goals":                {"id", "user_id", "title", "kind", "status"},
	"shoes":                {"id", "user_id", "name", "drop_mm", "strava_distance_m", "strava_state"},
	"activity_notes":       {"activity_id", "user_id", "shoe_id", "shoe_auto"},
	"daily_checkins":       {"user_id", "date", "training_readiness"},
	"llm_configs":          {"user_id", "provider", "model", "key_enc"},
	"llm_analyses":         {"id", "user_id", "kind", "ref", "markdown"},
	"notification_targets": {"id", "user_id", "kind", "secret_enc"},
	"activity_metrics":     {"activity_id", "user_id", "version", "data"},
	"activity_records":     {"activity_id", "user_id", "version", "data"},
	"personal_records":     {"user_id", "key", "distance_m"},
	"calendar_events":      {"id", "user_id", "kind", "start_date", "end_date", "severity", "share_link_id", "prescribed_label", "prescribed_author"},
	"training_plans":       {"id", "user_id", "goal_id", "name", "sport", "start_date", "end_date", "target_event_label", "status", "note", "created_at", "updated_at"},
	"plan_phases":          {"id", "user_id", "plan_id", "name", "phase_type", "order_index", "start_date", "end_date", "focus", "weekly_volume_km", "target_weekly_load", "target_weekly_duration_s", "note"},
	"training_weeks":       {"id", "user_id", "plan_id", "phase_id", "week_start", "week_end", "week_number", "is_assimilation", "target_volume_km", "target_load", "target_duration_s", "note", "created_at", "updated_at"},
	"planned_sessions":     {"id", "user_id", "date", "sport", "title", "steps", "source", "garmin_status", "plan_id", "phase_id", "week_id", "share_link_id", "prescribed_label", "prescribed_author", "morph_json"},
	"body_stress_entries":  {"id", "user_id", "date", "activity_id", "zone", "side", "pain_score", "cold_next_day_score", "note", "created_at", "updated_at"},
	"share_links":          {"id", "user_id", "label", "token_hash", "token_prefix", "url_ciphertext", "expires_at", "revoked_at", "access_count", "last_accessed_at", "created_at", "scope_json", "allow_comments", "allow_planning", "allow_activity_details"},
	"share_notes":          {"id", "user_id", "share_link_id", "link_label", "kind", "activity_id", "author_name", "author_role", "body", "restrictions_json", "valid_until", "created_at", "updated_at"},
	"health_status":        {"user_id", "zones", "updated_at"},
	"feedback":             {"id", "user_id", "category", "message", "contact_ok", "contact_email", "context_json", "status", "admin_note", "created_at", "updated_at"},
	"async_jobs":           {"id", "user_id", "kind", "queue", "state", "label", "progress_pct", "processed", "total", "attempts", "max_attempts", "error", "result_json", "created_at", "updated_at", "payload_json"},
}

// CheckIntegrity runs the full PRAGMA integrity_check and foreign_key_check (whole-database scans,
// for `stride doctor`) and returns the problems found.
func CheckIntegrity(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return nil, fmt.Errorf("pragma integrity_check: %w", err)
	}
	var problems []string
	for rows.Next() {
		var res string
		if err := rows.Scan(&res); err != nil {
			rows.Close()
			return nil, err
		}
		if res != "ok" {
			problems = append(problems, res)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	fk, err := ForeignKeyViolations(ctx, db)
	return append(problems, fk...), err
}

// ForeignKeyViolations lists the rows whose referenced parent row is missing.
func ForeignKeyViolations(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, fmt.Errorf("pragma foreign_key_check: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var tbl, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&tbl, &rowid, &parent, &fkid); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%s(rowid=%d) -> %s", tbl, rowid.Int64, parent))
	}
	return out, rows.Err()
}

// VerifySchema asserts that every expected table and column exists (fast: schema only).
func VerifySchema(ctx context.Context, db *sql.DB) error {
	tRows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return err
	}
	tables := map[string]bool{}
	for tRows.Next() {
		var name string
		if err := tRows.Scan(&name); err != nil {
			tRows.Close()
			return err
		}
		tables[name] = true
	}
	tRows.Close()
	if err := tRows.Err(); err != nil {
		return err
	}

	names := make([]string, 0, len(expectedSchema))
	for tbl := range expectedSchema {
		names = append(names, tbl)
	}
	sort.Strings(names)

	for _, tbl := range names {
		if !tables[tbl] {
			return fmt.Errorf("schema verification failed: missing table %q", tbl)
		}
		cRows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tbl))
		if err != nil {
			return fmt.Errorf("pragma table_info(%s): %w", tbl, err)
		}
		cols := map[string]bool{}
		for cRows.Next() {
			var cid, notnull, pk int
			var cname, ctype string
			var dflt sql.NullString
			if err := cRows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err != nil {
				cRows.Close()
				return err
			}
			cols[cname] = true
		}
		cRows.Close()
		if err := cRows.Err(); err != nil {
			return err
		}
		for _, wantCol := range expectedSchema[tbl] {
			if !cols[wantCol] {
				return fmt.Errorf("schema verification failed: table %q missing column %q", tbl, wantCol)
			}
		}
	}
	return nil
}
