package admin

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stride/internal/apimeter"
	"stride/internal/auth"
	"stride/internal/backup"
	"stride/internal/env"
	"stride/internal/queue"
	"stride/internal/secret"
)

func setupBackups(t *testing.T) (*controlEnv, *backup.Service, string) {
	t.Helper()
	c := setupControl(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	sealer, _ := secret.New(key, nil)
	dataDir := t.TempDir()
	b := backup.New(backup.Options{Store: c.st, Sealer: sealer, DataDir: dataDir, Version: "4.2.0", Log: log})
	b.Register(c.dom.Jobs())
	svc, err := auth.New(auth.Options{Env: env.Config{RPID: "localhost", RPOrigins: []string{"http://localhost:8080"}, SessionTTL: time.Hour},
		Store: c.st, Cache: c.cache, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(Deps{Store: c.st, Cache: c.cache, Meter: apimeter.NewLocal(apimeter.Options{Log: log}), Auth: svc, Log: log, Domain: c.dom,
		Backups: b}).Register(mux)
	c.h = mux
	return c, b, dataDir
}

func TestBackupsAdminOnlyAndNames(t *testing.T) {
	c, b, dataDir := setupBackups(t)
	_, adminTok := c.user("admin")
	_, userTok := c.user("user")
	for _, r := range [][2]string{{"GET", "/api/admin/backups"}, {"POST", "/api/admin/backups"}, {"GET", "/api/admin/backups/s3"},
		{"PUT", "/api/admin/backups/s3"}, {"POST", "/api/admin/backups/s3/test"},
		{"GET", "/api/admin/backups/stride-20261107T030000Z-4.2.0.sqlite.gz/download"},
		{"DELETE", "/api/admin/backups/stride-20261107T030000Z-4.2.0.sqlite.gz"}} {
		if code, _ := c.do(userTok, r[0], r[1], "{}"); code != http.StatusForbidden {
			t.Fatalf("non-admin %s %s: %d", r[0], r[1], code)
		}
		if code, _ := c.do("", r[0], r[1], "{}"); code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s: %d", r[0], r[1], code)
		}
	}

	code, body := c.do(adminTok, "POST", "/api/admin/backups", `{"upload":false}`)
	st := decode[queue.JobStatus](t, code, body, http.StatusAccepted)
	if st.Kind != queue.KindBackupRun {
		t.Fatalf("job: %+v", st)
	}
	c.dom.Wait()
	items, _ := b.List()
	if len(items) != 1 {
		t.Fatalf("snapshot not created: %+v", items)
	}
	name := items[0].Name
	code, body = c.do(adminTok, "GET", "/api/admin/backups", "")
	ov := decode[backup.Overview](t, code, body, http.StatusOK)
	if len(ov.Items) != 1 || ov.LastSuccess.Local == nil || ov.NextScheduledAt == "" || ov.S3.SecretKeySet {
		t.Fatalf("overview: %+v", ov)
	}
	if code, body := c.do(adminTok, "POST", "/api/admin/backups", `{"upload":true}`); code != http.StatusPreconditionFailed {
		t.Fatalf("upload without S3: %d %s", code, body)
	}
	code, raw := c.do(adminTok, "GET", "/api/admin/backups/"+name+"/download", "")
	if code != 200 || len(raw) != int(items[0].SizeBytes) {
		t.Fatalf("download: %d %d bytes", code, len(raw))
	}
	for _, bad := range []string{"..%2Fstride.db", "stride.db", "%2E%2E", "stride-20261107T030000Z-4.2.0.sqlite.gz.json"} {
		if code, _ := c.do(adminTok, "GET", "/api/admin/backups/"+bad+"/download", ""); code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Fatalf("name %q: %d", bad, code)
		}
		if code, _ := c.do(adminTok, "DELETE", "/api/admin/backups/"+bad, ""); code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Fatalf("delete %q: %d", bad, code)
		}
	}
	if code, _ := c.do(adminTok, "DELETE", "/api/admin/backups/"+name+"?remote=2", ""); code != http.StatusBadRequest {
		t.Fatalf("remote=2: %d", code)
	}

	// S3 configuration: the secret is sealed and never returned.
	code, body = c.do(adminTok, "PUT", "/api/admin/backups/s3", `{"endpoint":"https://s3.example.com","bucket":"stride-bk","accessKeyId":"AK","secretKey":"top-secret-value","retentionDays":30}`)
	cfg := decode[backup.S3Config](t, code, body, http.StatusOK)
	if !cfg.SecretKeySet || strings.Contains(string(body), "top-secret-value") {
		t.Fatalf("config: %s", body)
	}
	if code, body := c.do(adminTok, "GET", "/api/admin/backups/s3", ""); code != 200 || strings.Contains(string(body), "top-secret-value") {
		t.Fatalf("get config: %d %s", code, body)
	}
	if code, _ := c.do(adminTok, "PUT", "/api/admin/backups/s3", `{"endpoint":"http://127.0.0.1:9000","bucket":"stride-bk","accessKeyId":"AK","secretKey":"x"}`); code != http.StatusBadRequest {
		t.Fatalf("private endpoint accepted without STRIDE_ALLOW_PRIVATE_EGRESS: %d", code)
	}
	if code, _ := c.do(adminTok, "PUT", "/api/admin/backups/s3", `{"bucket":"x","unknown":1}`); code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", code)
	}
	code, body = c.do(adminTok, "POST", "/api/admin/backups/s3/test", `{"config":{"endpoint":"http://10.0.0.1","bucket":"stride-bk","accessKeyId":"AK"}}`)
	var res backup.TestResult
	_ = json.Unmarshal(body, &res)
	if code != 200 || res.OK || res.Error == nil {
		t.Fatalf("test of a private endpoint: %d %s", code, body)
	}

	if code, _ := c.do(adminTok, "DELETE", "/api/admin/backups/"+name, ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if left, _ := filepath.Glob(filepath.Join(backup.Dir(dataDir), "stride-*")); len(left) != 0 {
		t.Fatalf("files left: %v", left)
	}
}
