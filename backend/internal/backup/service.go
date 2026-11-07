package backup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"stride/internal/egress"
	"stride/internal/env"
	"stride/internal/observability"
	"stride/internal/queue"
	"stride/internal/scheduler"
	"stride/internal/secret"
	"stride/internal/store"
)

const (
	settingS3       = "backup_s3"
	settingS3Secret = "backup_s3_secret"
	// secretAAD binds the sealed secret key to its use (AAD <scope>:<uid>:<field>).
	secretAAD = "backup:0:s3_secret_key"

	uploadAttempts = 4
	s3Timeout      = 10 * time.Minute
	testTimeout    = 15 * time.Second
	// DefaultHour is the local hour of the daily snapshot.
	DefaultHour = 3
)

var (
	ErrS3NotConfigured = errors.New("S3 non configuré")
	bucketRe           = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	regionRe           = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	prefixRe           = regexp.MustCompile(`^[A-Za-z0-9._/-]{0,200}$`)
)

// ConfigError is an invalid S3 configuration (400, French message).
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

// S3Config is the stored S3 target; the secret key is never returned (SecretKeySet only).
type S3Config struct {
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	Prefix        string `json:"prefix"`
	AccessKeyID   string `json:"accessKeyId"`
	SecretKeySet  bool   `json:"secretKeySet"`
	PathStyle     bool   `json:"pathStyle"`
	AutoUpload    bool   `json:"autoUpload"`
	RetentionDays int    `json:"retentionDays"`
}

// S3ConfigInput is the body of PUT /api/admin/backups/s3 (empty SecretKey keeps the stored one).
type S3ConfigInput struct {
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	Prefix        string `json:"prefix"`
	AccessKeyID   string `json:"accessKeyId"`
	SecretKey     string `json:"secretKey"`
	PathStyle     bool   `json:"pathStyle"`
	AutoUpload    bool   `json:"autoUpload"`
	RetentionDays int    `json:"retentionDays"`
}

// LastSuccess holds the RFC 3339 time of the latest local snapshot and of the latest S3 upload.
type LastSuccess struct {
	Local *string `json:"local"`
	S3    *string `json:"s3"`
}

// Overview is GET /api/admin/backups.
type Overview struct {
	Items           []Item      `json:"items"`
	S3              S3Config    `json:"s3"`
	LastSuccess     LastSuccess `json:"lastSuccess"`
	NextScheduledAt string      `json:"nextScheduledAt"`
}

// TestResult is POST /api/admin/backups/s3/test.
type TestResult struct {
	OK        bool    `json:"ok"`
	LatencyMs int64   `json:"latencyMs"`
	Error     *string `json:"error,omitempty"`
}

type Options struct {
	Store   *store.Store
	Sealer  *secret.Sealer
	DataDir string
	Version string
	Env     env.Config
	Metrics *observability.Metrics
	Log     *slog.Logger
	Now     func() time.Time
	// HTTP overrides the egress-guarded S3 client (tests).
	HTTP     *http.Client
	Hour     int
	Location *time.Location
}

// Service manages the snapshots of one instance.
type Service struct {
	o     Options
	dir   string
	http  *http.Client
	mu    sync.Mutex // serializes snapshots, sidecar updates and deletions
	queue *queue.Manager
}

type runPayload struct {
	Trigger string `json:"trigger"`
	Upload  bool   `json:"upload"`
}

type uploadPayload struct {
	Name string `json:"name"`
}

// Dir returns the backup directory of a data directory.
func Dir(dataDir string) string { return filepath.Join(dataDir, DirName) }

func New(o Options) *Service {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.Hour <= 0 || o.Hour > 23 {
		o.Hour = DefaultHour
	}
	s := &Service{o: o, dir: Dir(o.DataDir), http: o.HTTP}
	if s.http == nil {
		s.http = egress.Client(o.Env.AllowPrivateEgress, s3Timeout)
	}
	return s
}

// Register binds the backup job kinds to the queue (backup group, one worker).
func (s *Service) Register(q *queue.Manager) {
	s.queue = q
	q.Register(queue.KindBackupRun, s.runJob)
	q.Register(queue.KindBackupUpload, s.uploadJob)
}

// ---------- local snapshots ----------

// Create takes a snapshot now and applies the local retention.
func (s *Service) Create(ctx context.Context, trigger string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, done, err := s.snapshotConn()
	if err != nil {
		return Item{}, err
	}
	defer done()
	it, err := Snapshot(ctx, db, s.dir, s.o.Version, trigger, s.o.Now())
	if err != nil {
		s.o.Metrics.ObserveBackup("local", "error", 0, time.Time{})
		return it, err
	}
	s.o.Metrics.ObserveBackup("local", "success", it.SizeBytes, s.o.Now())
	s.o.Log.Info("💾 Sauvegarde créée", "nom", it.Name, "octets", it.SizeBytes, "déclencheur", trigger)
	if deleted, err := Prune(s.dir); err != nil {
		s.o.Log.Warn("💾 Rétention locale des sauvegardes en échec", "err", err.Error())
	} else if len(deleted) > 0 {
		s.o.Log.Info("🧹 Anciennes sauvegardes supprimées", "nombre", len(deleted))
	}
	return it, nil
}

// snapshotConn opens a dedicated connection to the database file: VACUUM INTO then reads a WAL
// snapshot without holding the single writer connection (the read pool is query_only, which
// SQLite refuses for VACUUM INTO). An in-memory store falls back to its write pool.
func (s *Service) snapshotConn() (*sql.DB, func(), error) {
	path := s.o.Store.Path
	if path == "" || path == ":memory:" {
		return s.o.Store.DB, func() {}, nil
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	return db, func() { db.Close() }, nil
}

// List returns the local snapshots, newest first.
func (s *Service) List() ([]Item, error) { return List(s.dir) }

// File returns the path of a snapshot after strict name validation.
func (s *Service) File(name string) (string, Item, error) {
	it, err := readItem(s.dir, name)
	if err != nil {
		return "", it, err
	}
	return filepath.Join(s.dir, name), it, nil
}

// Delete removes a snapshot, and its S3 copy when remote is set.
func (s *Service) Delete(ctx context.Context, name string, remote bool) error {
	it, err := readItem(s.dir, name)
	if err != nil {
		return err
	}
	if remote && it.S3.Key != nil {
		c, cfg, err := s.client(ctx, nil)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(*it.S3.Key, normPrefix(cfg.Prefix)) {
			return &ConfigError{Msg: "La copie S3 n'appartient pas au préfixe configuré."}
		}
		dctx, cancel := context.WithTimeout(ctx, testTimeout)
		defer cancel()
		if err := c.Delete(dctx, *it.S3.Key); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return removeLocal(s.dir, name)
}

// InitMetrics publishes the last successes found on disk (after a restart).
func (s *Service) InitMetrics() {
	items, err := s.List()
	if err != nil {
		return
	}
	last := lastSuccess(items)
	if last.Local != nil {
		t, _ := time.Parse(time.RFC3339, *last.Local)
		s.o.Metrics.SetBackupLastSuccess("local", t)
		s.o.Metrics.SetBackupSize(items[0].SizeBytes)
	}
	if last.S3 != nil {
		t, _ := time.Parse(time.RFC3339, *last.S3)
		s.o.Metrics.SetBackupLastSuccess("s3", t)
	}
}

func lastSuccess(items []Item) LastSuccess {
	var l LastSuccess
	for _, it := range items {
		if l.Local == nil || it.CreatedAt > *l.Local {
			v := it.CreatedAt
			l.Local = &v
		}
		if it.S3.Status == S3Synced && it.S3.SyncedAt != nil && (l.S3 == nil || *it.S3.SyncedAt > *l.S3) {
			v := *it.S3.SyncedAt
			l.S3 = &v
		}
	}
	return l
}

// NextScheduledAt is the next daily snapshot.
func (s *Service) NextScheduledAt() time.Time {
	return scheduler.NextRun(s.o.Now(), s.o.Hour, s.o.Location)
}

// Overview is the admin panel payload.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	items, err := s.List()
	if err != nil {
		return Overview{}, err
	}
	cfg, err := s.S3Config(ctx)
	if err != nil {
		return Overview{}, err
	}
	return Overview{Items: items, S3: cfg, LastSuccess: lastSuccess(items), NextScheduledAt: s.NextScheduledAt().UTC().Format(time.RFC3339)}, nil
}

// ---------- jobs ----------

// Enqueue starts a snapshot job (and an upload job after it when upload is set).
func (s *Service) Enqueue(ctx context.Context, uid int64, trigger string, upload bool) (*queue.JobStatus, error) {
	if upload {
		if _, cfg, err := s.client(ctx, nil); err != nil || cfg.Bucket == "" {
			return nil, ErrS3NotConfigured
		}
	}
	return s.queue.Enqueue(ctx, queue.EnqueueRequest{UserID: uid, Kind: queue.KindBackupRun, Payload: runPayload{Trigger: trigger, Upload: upload}})
}

// EnqueueUpload starts the upload job of an existing snapshot (retried with backoff).
func (s *Service) EnqueueUpload(ctx context.Context, uid int64, name string) (*queue.JobStatus, error) {
	if _, err := readItem(s.dir, name); err != nil {
		return nil, err
	}
	if _, _, err := s.client(ctx, nil); err != nil {
		return nil, err
	}
	return s.queue.Enqueue(ctx, queue.EnqueueRequest{UserID: uid, Kind: queue.KindBackupUpload, MaxAttempts: uploadAttempts,
		Payload: uploadPayload{Name: name}})
}

// RunDaily is the scheduler hook: daily snapshot, then the upload when auto-upload is on.
func (s *Service) RunDaily(ctx context.Context) error {
	it, err := s.Create(ctx, TriggerDaily)
	if err != nil {
		return err
	}
	cfg, err := s.S3Config(ctx)
	if err != nil || !cfg.AutoUpload || !cfg.configured() || s.queue == nil {
		return err
	}
	_, err = s.EnqueueUpload(ctx, 0, it.Name)
	return err
}

func (s *Service) runJob(ctx context.Context, jc *queue.JobContext) (any, error) {
	var p runPayload
	if err := json.Unmarshal(jc.Payload, &p); err != nil {
		return nil, queue.Permanent("Tâche de sauvegarde illisible.", err)
	}
	if p.Trigger == "" {
		p.Trigger = TriggerManual
	}
	jc.UpdateProgress("Sauvegarde de la base en cours…", nil, nil, nil)
	it, err := s.Create(ctx, p.Trigger)
	if err != nil {
		s.o.Log.Error("💾 Sauvegarde en échec", "err", err.Error())
		return nil, queue.Permanent("La sauvegarde a échoué (détails dans les journaux du serveur).", err)
	}
	if p.Upload {
		if _, err := s.EnqueueUpload(context.WithoutCancel(ctx), jc.UserID, it.Name); err != nil {
			s.o.Log.Warn("💾 Envoi S3 non planifié", "nom", it.Name, "err", publicError(err))
		}
	}
	return it, nil
}

func (s *Service) uploadJob(ctx context.Context, jc *queue.JobContext) (any, error) {
	var p uploadPayload
	if err := json.Unmarshal(jc.Payload, &p); err != nil {
		return nil, queue.Permanent("Tâche d'envoi illisible.", err)
	}
	jc.UpdateProgress("Envoi de la sauvegarde vers S3…", nil, nil, nil)
	it, err := s.Upload(ctx, p.Name)
	if err != nil {
		msg := "Envoi S3 en échec : " + publicError(err)
		if Retryable(err) && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrS3NotConfigured) && jc.Attempt < jc.MaxAttempts {
			return nil, queue.Transient(msg, err)
		}
		return nil, queue.Permanent(msg, err)
	}
	return it, nil
}

// Upload sends a snapshot to S3, records the outcome in its sidecar, then applies the remote retention.
func (s *Service) Upload(ctx context.Context, name string) (Item, error) {
	it, err := readItem(s.dir, name)
	if err != nil {
		return it, err
	}
	c, cfg, err := s.client(ctx, nil)
	if err != nil {
		return it, err
	}
	key := normPrefix(cfg.Prefix) + name
	s.setS3(name, S3State{Status: S3Pending, Key: &key})
	uctx, cancel := context.WithTimeout(ctx, s3Timeout)
	defer cancel()
	if err := c.Put(uctx, key, filepath.Join(s.dir, name), it.SHA256); err != nil {
		msg := publicError(err)
		s.setS3(name, S3State{Status: S3Failed, Key: &key, Error: &msg})
		s.o.Metrics.ObserveBackup("s3", "error", 0, time.Time{})
		s.o.Log.Warn("☁️ Envoi S3 de la sauvegarde en échec", "nom", name, "err", msg)
		return it, err
	}
	at := s.o.Now().UTC()
	synced := at.Format(time.RFC3339)
	it.S3 = S3State{Status: S3Synced, Key: &key, SyncedAt: &synced}
	s.setS3(name, it.S3)
	s.o.Metrics.ObserveBackup("s3", "success", it.SizeBytes, at)
	s.o.Log.Info("☁️ Sauvegarde envoyée vers S3", "nom", name, "octets", it.SizeBytes)
	if cfg.RetentionDays > 0 {
		s.pruneRemote(ctx, c, cfg, key)
	}
	return it, nil
}

// pruneRemote deletes the snapshots of the prefix older than the retention, never the one just sent.
func (s *Service) pruneRemote(ctx context.Context, c *S3Client, cfg S3Config, keep string) {
	objs, err := c.List(ctx, normPrefix(cfg.Prefix))
	if err != nil {
		s.o.Log.Warn("☁️ Rétention S3 : listing impossible", "err", publicError(err))
		return
	}
	cutoff := s.o.Now().Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)
	n := 0
	for _, o := range objs {
		base := strings.TrimPrefix(o.Key, normPrefix(cfg.Prefix))
		if o.Key == keep || strings.Contains(base, "/") || !ValidName(base) || o.LastModified.IsZero() || !o.LastModified.Before(cutoff) {
			continue
		}
		if err := c.Delete(ctx, o.Key); err != nil {
			s.o.Log.Warn("☁️ Rétention S3 : suppression impossible", "clé", o.Key, "err", publicError(err))
			continue
		}
		n++
	}
	if n > 0 {
		s.o.Log.Info("🧹 Anciennes sauvegardes S3 supprimées", "nombre", n)
	}
}

func (s *Service) setS3(name string, st S3State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := readItem(s.dir, name)
	if err != nil {
		return
	}
	it.S3 = st
	if err := writeSidecar(s.dir, it); err != nil {
		s.o.Log.Warn("💾 Fichier de description de sauvegarde non mis à jour", "nom", name, "err", err.Error())
	}
}

// publicError is a log- and API-safe description (never URLs, hosts or credentials).
func publicError(err error) string {
	var se *S3Error
	var ce *ConfigError
	switch {
	case errors.As(err, &se):
		return se.Error()
	case errors.As(err, &ce):
		return ce.Msg
	case errors.Is(err, ErrS3NotConfigured), errors.Is(err, ErrNotFound):
		return err.Error()
	case egress.IsForbidden(err):
		return "point de terminaison refusé (réseau privé : STRIDE_ALLOW_PRIVATE_EGRESS)"
	case errors.Is(err, context.DeadlineExceeded):
		return "délai dépassé"
	}
	return "S3 injoignable (erreur réseau)"
}

// PublicError exposes publicError to the HTTP layer.
func PublicError(err error) string { return publicError(err) }

// ---------- S3 configuration ----------

func normPrefix(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

func (c S3Config) configured() bool {
	return c.Endpoint != "" && c.Bucket != "" && c.AccessKeyID != "" && c.SecretKeySet
}

// S3Config returns the stored configuration (zero value when none).
func (s *Service) S3Config(ctx context.Context) (S3Config, error) {
	var cfg S3Config
	raw, ok, err := s.o.Store.Setting(ctx, settingS3)
	if err != nil || !ok {
		return S3Config{Region: "us-east-1"}, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, err
	}
	_, cfg.SecretKeySet, err = s.o.Store.Setting(ctx, settingS3Secret)
	return cfg, err
}

func (s *Service) secretKey(ctx context.Context) (string, error) {
	raw, ok, err := s.o.Store.Setting(ctx, settingS3Secret)
	if err != nil || !ok {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	return s.o.Sealer.OpenString(sealed, secretAAD)
}

// validate normalizes an input; secretSet tells whether a secret will exist once saved.
func (s *Service) validate(in S3ConfigInput, secretSet bool) (S3Config, error) {
	cfg := S3Config{Endpoint: strings.TrimRight(strings.TrimSpace(in.Endpoint), "/"), Region: strings.TrimSpace(in.Region),
		Bucket: strings.TrimSpace(in.Bucket), Prefix: strings.Trim(strings.TrimSpace(in.Prefix), "/"), AccessKeyID: strings.TrimSpace(in.AccessKeyID),
		SecretKeySet: secretSet, PathStyle: in.PathStyle, AutoUpload: in.AutoUpload, RetentionDays: in.RetentionDays}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Endpoint == "" && cfg.Bucket == "" {
		return cfg, nil // S3 disabled
	}
	u, err := egress.CheckURL(cfg.Endpoint)
	if err != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return cfg, &ConfigError{Msg: "endpoint invalide : https://hôte[:port] attendu."}
	}
	if err := egress.PreCheck(u, s.o.Env.AllowPrivateEgress); err != nil {
		return cfg, &ConfigError{Msg: "endpoint refusé : réseau privé ou local (STRIDE_ALLOW_PRIVATE_EGRESS)."}
	}
	switch {
	case !bucketRe.MatchString(cfg.Bucket):
		return cfg, &ConfigError{Msg: "bucket invalide (3 à 63 caractères a-z, 0-9, point, tiret)."}
	case !regionRe.MatchString(cfg.Region):
		return cfg, &ConfigError{Msg: "region invalide (a-z, 0-9, tiret)."}
	case !prefixRe.MatchString(cfg.Prefix) || strings.Contains(cfg.Prefix, ".."):
		return cfg, &ConfigError{Msg: "prefix invalide (A-Z, a-z, 0-9, . _ / -, 200 caractères max)."}
	case cfg.AccessKeyID == "" || len(cfg.AccessKeyID) > 128 || strings.ContainsAny(cfg.AccessKeyID, " \t\r\n"):
		return cfg, &ConfigError{Msg: "accessKeyId requis (128 caractères max, sans espace)."}
	case cfg.RetentionDays < 0 || cfg.RetentionDays > 3650:
		return cfg, &ConfigError{Msg: "retentionDays doit être compris entre 0 (jamais) et 3650."}
	case !secretSet:
		return cfg, &ConfigError{Msg: "secretKey requis."}
	}
	return cfg, nil
}

// PutS3Config validates and stores the configuration; the secret key is sealed (empty keeps it).
func (s *Service) PutS3Config(ctx context.Context, in S3ConfigInput) (S3Config, error) {
	cur, err := s.S3Config(ctx)
	if err != nil {
		return cur, err
	}
	if len(in.SecretKey) > 256 {
		return cur, &ConfigError{Msg: "secretKey trop long."}
	}
	cfg, err := s.validate(in, in.SecretKey != "" || cur.SecretKeySet)
	if err != nil {
		return cur, err
	}
	if err := s.store(ctx, cfg, in.SecretKey); err != nil {
		return cur, err
	}
	return s.S3Config(ctx)
}

func (s *Service) store(ctx context.Context, cfg S3Config, secretKey string) error {
	// secretKeySet is derived from the sealed secret row, never stored with the configuration.
	var fields map[string]any
	raw, _ := json.Marshal(cfg)
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "secretKeySet")
	raw, _ = json.Marshal(fields)
	now := s.o.Now()
	if secretKey != "" {
		sealed, err := s.o.Sealer.SealString(secretKey, secretAAD)
		if err != nil {
			return err
		}
		if err := s.o.Store.SetSetting(ctx, settingS3Secret, base64.StdEncoding.EncodeToString(sealed), now); err != nil {
			return err
		}
	}
	return s.o.Store.SetSetting(ctx, settingS3, string(raw), now)
}

// Seed stores the STRIDE_S3_* configuration once, when none is stored yet.
func (s *Service) Seed(ctx context.Context) error {
	e := s.o.Env.S3
	if e.Endpoint == "" && e.Bucket == "" {
		return nil
	}
	if _, ok, err := s.o.Store.Setting(ctx, settingS3); err != nil || ok {
		return err
	}
	cfg, err := s.validate(S3ConfigInput{Endpoint: e.Endpoint, Region: e.Region, Bucket: e.Bucket, Prefix: e.Prefix, AccessKeyID: e.AccessKeyID,
		SecretKey: e.SecretAccessKey, PathStyle: e.PathStyle, AutoUpload: e.AutoUpload, RetentionDays: e.RetentionDays}, e.SecretAccessKey != "")
	if err != nil {
		return fmt.Errorf("STRIDE_S3_* : %w", err)
	}
	if err := s.store(ctx, cfg, e.SecretAccessKey); err != nil {
		return err
	}
	s.o.Log.Info("☁️ Cible S3 des sauvegardes initialisée depuis l'environnement", "bucket", cfg.Bucket)
	return nil
}

// client builds an S3 client from in (merged with the stored secret when in has none) or from
// the stored configuration.
func (s *Service) client(ctx context.Context, in *S3ConfigInput) (*S3Client, S3Config, error) {
	var cfg S3Config
	var err error
	secretKey := ""
	if in != nil {
		secretKey = in.SecretKey
		stored, err := s.S3Config(ctx)
		if err != nil {
			return nil, cfg, err
		}
		if cfg, err = s.validate(*in, secretKey != "" || stored.SecretKeySet); err != nil {
			return nil, cfg, err
		}
	} else if cfg, err = s.S3Config(ctx); err != nil {
		return nil, cfg, err
	}
	if !cfg.configured() {
		return nil, cfg, ErrS3NotConfigured
	}
	if secretKey == "" {
		if secretKey, err = s.secretKey(ctx); err != nil {
			return nil, cfg, fmt.Errorf("secret S3 illisible : %w", err)
		}
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, cfg, &ConfigError{Msg: "endpoint invalide."}
	}
	return &S3Client{Endpoint: u, Region: cfg.Region, Bucket: cfg.Bucket, AccessKey: cfg.AccessKeyID, SecretKey: secretKey,
		PathStyle: cfg.PathStyle, HTTP: s.http, Now: s.o.Now}, cfg, nil
}

// TestS3 checks the bucket with in (or the stored configuration): HEAD bucket, latency measured.
func (s *Service) TestS3(ctx context.Context, in *S3ConfigInput) TestResult {
	c, _, err := s.client(ctx, in)
	if err != nil {
		msg := publicError(err)
		return TestResult{Error: &msg}
	}
	tctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	t0 := time.Now()
	err = c.HeadBucket(tctx)
	res := TestResult{OK: err == nil, LatencyMs: time.Since(t0).Milliseconds()}
	if err != nil {
		msg := publicError(err)
		res.Error = &msg
	}
	return res
}

// PreMigrate returns the store hook taking a snapshot before pending migrations (non-empty database).
func PreMigrate(dataDir, version string, log *slog.Logger) func(ctx context.Context, db *sql.DB, pending bool) error {
	return func(ctx context.Context, db *sql.DB, pending bool) error {
		if !pending {
			return nil
		}
		dir := Dir(dataDir)
		it, err := Snapshot(ctx, db, dir, version, TriggerPreMigration, time.Now())
		if err != nil {
			return fmt.Errorf("sauvegarde avant migration impossible : %w", err)
		}
		if _, err := Prune(dir); err != nil {
			log.Warn("💾 Rétention locale des sauvegardes en échec", "err", err.Error())
		}
		log.Info("💾 Sauvegarde avant migration", "nom", it.Name, "octets", it.SizeBytes)
		return nil
	}
}
