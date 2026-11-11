package backup

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"database/sql"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"stride/internal/env"
	"stride/internal/observability"
	"stride/internal/secret"
	"stride/internal/store"
)

func writeFile(path string, data []byte) error { return os.WriteFile(path, data, 0o600) }

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dataDir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dataDir, "stride.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.DB.Exec(`INSERT INTO users (id, display_name, created_at) VALUES (1, 'Zoé', 1)`); err != nil {
		t.Fatal(err)
	}
	return st, dataDir
}

func testService(t *testing.T, st *store.Store, dataDir string, now *time.Time) *Service {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	sealer, _ := secret.New(key, nil)
	return New(Options{Store: st, Sealer: sealer, DataDir: dataDir, Version: "4.2.0", Env: env.Config{AllowPrivateEgress: true},
		Metrics: observability.New(), Log: quietLog(), Now: func() time.Time { return *now }})
}

// The snapshot is a complete, consistent SQLite file: restored (gunzip), it passes integrity_check
// and holds the data; files are owner-only and the sidecar checksum matches.
func TestSnapshotIntegrity(t *testing.T) {
	st, dataDir := testStore(t)
	now := time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC)
	s := testService(t, st, dataDir, &now)
	it, err := s.Create(context.Background(), TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if it.Name != "stride-20261107T030000Z-4.2.0.sqlite.gz" || it.Trigger != TriggerManual || it.S3.Status != S3None || len(it.SHA256) != 64 {
		t.Fatalf("item: %+v", it)
	}
	path, got, err := s.File(it.Name)
	if err != nil || got.SHA256 != it.SHA256 {
		t.Fatalf("file: %v %+v", err, got)
	}
	if sum, _ := fileSHA256(path); sum != it.SHA256 {
		t.Fatal("sidecar checksum differs from the file")
	}
	for p, want := range map[string]os.FileMode{path: 0o600, path + sidecarExt: 0o600, Dir(dataDir): 0o700} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Fatalf("%s: %v %v", p, fi.Mode().Perm(), err)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(Dir(dataDir), ".tmp-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}

	restored := filepath.Join(t.TempDir(), "restored.db")
	in, _ := os.Open(path)
	zr, err := gzip.NewReader(in)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.Create(restored)
	if _, err := io.Copy(out, zr); err != nil {
		t.Fatal(err)
	}
	out.Close()
	in.Close()
	db, err := sql.Open("sqlite", "file:"+restored)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var check, name string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatalf("integrity_check: %q %v", check, err)
	}
	if err := db.QueryRow(`SELECT display_name FROM users WHERE id = 1`).Scan(&name); err != nil || name != "Zoé" {
		t.Fatalf("restored row: %q %v", name, err)
	}
}

func TestLocalRetention(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (v INTEGER)`); err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(dir, "backups")
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 13; i++ {
		trigger := TriggerDaily
		if i%3 == 0 {
			trigger = TriggerPreMigration
		}
		if _, err := Snapshot(context.Background(), db, backups, "4.2.0", trigger, t0.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := Prune(backups)
	if err != nil {
		t.Fatal(err)
	}
	items, _ := List(backups)
	pre, regular := 0, 0
	for _, it := range items {
		if it.Trigger == TriggerPreMigration {
			pre++
		} else {
			regular++
		}
	}
	if pre != 3 || regular != 7 || len(deleted) != 3 || items[0].Name < items[len(items)-1].Name {
		t.Fatalf("retention: %d pre, %d regular, deleted %v", pre, regular, deleted)
	}
	// The oldest pre-migration snapshot (hour 0) went away, the newest regular one stayed.
	if _, err := os.Stat(filepath.Join(backups, "stride-20260101T000000Z-4.2.0.sqlite.gz")); !os.IsNotExist(err) {
		t.Fatal("oldest pre-migration snapshot kept")
	}
	if _, err := os.Stat(filepath.Join(backups, "stride-20260101T000000Z-4.2.0.sqlite.gz.json")); !os.IsNotExist(err) {
		t.Fatal("sidecar of a pruned snapshot kept")
	}
}

func TestNameValidationRejectsTraversal(t *testing.T) {
	st, dataDir := testStore(t)
	now := time.Now()
	s := testService(t, st, dataDir, &now)
	for _, bad := range []string{"../stride.db", "..%2Fstride.db", "stride-20261107T030000Z-4.2.0.sqlite.gz/../../x",
		"stride-20261107T030000Z-..sqlite.gz", "/etc/passwd", "stride-20261107T030000Z-4.2.0.sqlite", "stride.db",
		"stride-2026117T030000Z-4.2.0.sqlite.gz", "stride-20261107T030000Z-4.2.0.sqlite.gz.json", ""} {
		if ValidName(bad) {
			t.Errorf("accepted %q", bad)
		}
		if _, _, err := s.File(bad); err != ErrInvalidName {
			t.Errorf("File(%q): %v", bad, err)
		}
		if err := s.Delete(context.Background(), bad, false); err != ErrInvalidName {
			t.Errorf("Delete(%q): %v", bad, err)
		}
	}
	if _, _, err := s.File("stride-20261107T030000Z-4.2.0.sqlite.gz"); err != ErrNotFound {
		t.Fatalf("missing snapshot: %v", err)
	}
	// Any build version yields a name the API accepts.
	for _, v := range []string{"4.2.0+abc/../x y", "", "..", strings.Repeat("9", 80)} {
		if got := cleanVersion(v); !ValidName("stride-20261107T030000Z-" + got + ".sqlite.gz") {
			t.Errorf("version %q cleaned to %q", v, got)
		}
	}
}

// The pre-migration hook snapshots a non-empty database only when migrations are pending.
func TestPreMigrateHook(t *testing.T) {
	dataDir := t.TempDir()
	hook := PreMigrate(dataDir, "4.2.0", quietLog())
	path := filepath.Join(dataDir, "stride.db")
	st, err := store.OpenWith(context.Background(), path, store.Options{PreMigrate: hook})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if items, _ := List(Dir(dataDir)); len(items) != 0 {
		t.Fatalf("fresh database snapshotted: %+v", items)
	}
	db, _ := sql.Open("sqlite", "file:"+path)
	if _, err := db.Exec(`DELETE FROM goose_db_version WHERE version_id = (SELECT MAX(version_id) FROM goose_db_version)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var sawPending bool
	st, err = store.OpenWith(context.Background(), path, store.Options{PreMigrate: func(ctx context.Context, db *sql.DB, pending bool) error {
		sawPending = pending
		return hook(ctx, db, pending)
	}})
	if err == nil {
		st.Close()
	}
	items, _ := List(Dir(dataDir))
	if !sawPending || len(items) != 1 || items[0].Trigger != TriggerPreMigration {
		t.Fatalf("pending=%v items=%+v err=%v", sawPending, items, err)
	}
}

func TestS3ConfigUploadAndRemoteRetention(t *testing.T) {
	st, dataDir := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	s := testService(t, st, dataDir, &now)
	ctx := context.Background()
	f, srv := newFakeS3(t, "stride-bk", "s3cr3t", true)
	s.http = srv.Client()

	if _, err := s.PutS3Config(ctx, S3ConfigInput{Endpoint: srv.URL, Bucket: "stride-bk", AccessKeyID: "AKTEST", PathStyle: true}); err == nil {
		t.Fatal("configuration without secret accepted")
	}
	for _, bad := range []S3ConfigInput{
		{Endpoint: "ftp://x", Bucket: "stride-bk", AccessKeyID: "AKTEST", SecretKey: "s"},
		{Endpoint: srv.URL, Bucket: "UP", AccessKeyID: "AKTEST", SecretKey: "s"},
		{Endpoint: srv.URL, Bucket: "stride-bk", Prefix: "../x", AccessKeyID: "AKTEST", SecretKey: "s"},
		{Endpoint: srv.URL + "/path", Bucket: "stride-bk", AccessKeyID: "AKTEST", SecretKey: "s"},
	} {
		if _, err := s.PutS3Config(ctx, bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	cfg, err := s.PutS3Config(ctx, S3ConfigInput{Endpoint: srv.URL, Bucket: "stride-bk", Prefix: "/prod/", AccessKeyID: "AKTEST",
		SecretKey: "s3cr3t", PathStyle: true, AutoUpload: true, RetentionDays: 30})
	if err != nil || !cfg.SecretKeySet || cfg.Prefix != "prod" || cfg.Region != "us-east-1" {
		t.Fatalf("config: %+v %v", cfg, err)
	}
	raw, _, _ := st.Setting(ctx, settingS3Secret)
	stored, _, _ := st.Setting(ctx, settingS3)
	if strings.Contains(raw, "s3cr3t") || strings.Contains(stored, "s3cr3t") || strings.Contains(stored, "secret") {
		t.Fatalf("secret stored in clear: %s / %s", raw, stored)
	}
	// An empty secret keeps the stored one.
	if cfg, err = s.PutS3Config(ctx, S3ConfigInput{Endpoint: srv.URL, Bucket: "stride-bk", Prefix: "prod", AccessKeyID: "AKTEST",
		PathStyle: true, RetentionDays: 30}); err != nil || !cfg.SecretKeySet {
		t.Fatalf("keep secret: %+v %v", cfg, err)
	}
	if res := s.TestS3(ctx, nil); !res.OK || res.Error != nil {
		t.Fatalf("test: %+v", res)
	}
	wrong := S3ConfigInput{Endpoint: srv.URL, Bucket: "stride-bk", AccessKeyID: "AKTEST", SecretKey: "nope", PathStyle: true}
	if res := s.TestS3(ctx, &wrong); res.OK || res.Error == nil || strings.Contains(*res.Error, "nope") || !strings.Contains(*res.Error, "403") {
		t.Fatalf("test with a wrong secret: %+v", res)
	}

	// An old remote snapshot exists; the upload succeeds, then the remote retention removes it.
	old := "prod/stride-20250101T030000Z-4.1.0.sqlite.gz"
	f.objects[old], f.modified[old] = []byte("old"), now.AddDate(0, 0, -40)
	foreign := "prod/notes.txt"
	f.objects[foreign], f.modified[foreign] = []byte("keep"), now.AddDate(0, 0, -400)
	it, err := s.Create(ctx, TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	up, err := s.Upload(ctx, it.Name)
	if err != nil || up.S3.Status != S3Synced || *up.S3.Key != "prod/"+it.Name {
		t.Fatalf("upload: %+v %v", up, err)
	}
	if _, ok := f.objects[old]; ok {
		t.Fatal("remote retention kept an expired snapshot")
	}
	if _, ok := f.objects[foreign]; !ok {
		t.Fatal("remote retention deleted a foreign object")
	}
	ov, err := s.Overview(ctx)
	if err != nil || len(ov.Items) != 1 || ov.Items[0].S3.Status != S3Synced || ov.LastSuccess.S3 == nil || ov.LastSuccess.Local == nil {
		t.Fatalf("overview: %+v %v", ov, err)
	}
	if err := s.Delete(ctx, it.Name, true); err != nil {
		t.Fatal(err)
	}
	if len(f.objects) != 1 {
		t.Fatalf("remote copy kept: %v", f.objects)
	}
	if items, _ := s.List(); len(items) != 0 {
		t.Fatal("local copy kept")
	}
}

func TestSeedFromEnvironmentOnce(t *testing.T) {
	st, dataDir := testStore(t)
	now := time.Now()
	s := testService(t, st, dataDir, &now)
	s.o.Env.S3 = env.S3Seed{Endpoint: "https://s3.example.com", Region: "eu-west-3", Bucket: "stride-bk", AccessKeyID: "AKENV",
		SecretAccessKey: "env-secret", RetentionDays: 14}
	s.o.Env.AllowPrivateEgress = false
	if err := s.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg, _ := s.S3Config(context.Background())
	if cfg.Bucket != "stride-bk" || cfg.Region != "eu-west-3" || !cfg.SecretKeySet || cfg.RetentionDays != 14 {
		t.Fatalf("seeded: %+v", cfg)
	}
	s.o.Env.S3.Bucket = "other-bucket"
	_ = s.Seed(context.Background())
	if cfg, _ = s.S3Config(context.Background()); cfg.Bucket != "stride-bk" {
		t.Fatal("environment overrode the stored configuration")
	}
	if k, err := s.secretKey(context.Background()); err != nil || k != "env-secret" {
		t.Fatalf("sealed secret: %v", err)
	}
	u, _ := url.Parse(cfg.Endpoint)
	if u.Host != "s3.example.com" {
		t.Fatal(cfg.Endpoint)
	}
}
