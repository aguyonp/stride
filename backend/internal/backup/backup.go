// Package backup takes atomic SQLite snapshots (§15.4): VACUUM INTO a temporary file, gzip, SHA-256
// sidecar, into <STRIDE_DATA_DIR>/backups (0700, files 0600). The filesystem is the source of truth
// (it works before migrations); a local retention keeps the 7 latest snapshots and the 3 latest
// pre-migration ones. Uploads to S3-compatible storage run as queue jobs with retries.
package backup

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DirName = "backups"

	TriggerManual       = "manual"
	TriggerDaily        = "daily"
	TriggerPreMigration = "pre_migration"

	S3None    = "none"
	S3Pending = "pending"
	S3Synced  = "synced"
	S3Failed  = "error"

	keepLocal        = 7
	keepPreMigration = 3
	sidecarExt       = ".json"
)

var (
	ErrNotFound    = errors.New("sauvegarde introuvable")
	ErrInvalidName = errors.New("nom de sauvegarde invalide")
	nameRe         = regexp.MustCompile(`^stride-[0-9]{8}T[0-9]{6}Z-[0-9A-Za-z.-]{1,40}\.sqlite\.gz$`)
	versionClean   = regexp.MustCompile(`[^0-9A-Za-z.-]+`)
)

// S3State is the remote copy of a snapshot.
type S3State struct {
	Status   string  `json:"status"` // none | pending | synced | error
	Key      *string `json:"key"`
	SyncedAt *string `json:"syncedAt"`
	Error    *string `json:"error"`
}

// Item is a snapshot as described by its JSON sidecar (BackupItem in the API).
type Item struct {
	Name      string  `json:"name"`
	SizeBytes int64   `json:"sizeBytes"`
	SHA256    string  `json:"sha256"`
	CreatedAt string  `json:"createdAt"`
	Trigger   string  `json:"trigger"` // manual | daily | pre_migration
	S3        S3State `json:"s3"`
}

// ValidName reports whether name is a snapshot file name (no path, no traversal).
func ValidName(name string) bool { return nameRe.MatchString(name) && !strings.Contains(name, "..") }

func cleanVersion(v string) string {
	v = versionClean.ReplaceAllString(v, "-")
	for strings.Contains(v, "..") {
		v = strings.ReplaceAll(v, "..", ".")
	}
	v = strings.Trim(v, "-.")
	if v == "" {
		v = "dev"
	}
	return v[:min(len(v), 40)]
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ensureDir creates the backup directory with owner-only permissions.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// Snapshot writes a consistent copy of db into dir. It needs no table, so it also runs before
// migrations (pre-migration snapshot at boot).
func Snapshot(ctx context.Context, db *sql.DB, dir, version, trigger string, now time.Time) (Item, error) {
	if err := ensureDir(dir); err != nil {
		return Item{}, err
	}
	var name string
	for i := 0; i < 10; i++ {
		name = fmt.Sprintf("stride-%s-%s.sqlite.gz", now.UTC().Add(time.Duration(i)*time.Second).Format("20060102T150405Z"), cleanVersion(version))
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
	}
	tmpDB := filepath.Join(dir, ".tmp-"+randHex(8)+".sqlite")
	defer os.Remove(tmpDB)
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, tmpDB); err != nil {
		return Item{}, fmt.Errorf("VACUUM INTO : %w", err)
	}
	tmpGz := filepath.Join(dir, ".tmp-"+randHex(8)+".gz")
	defer os.Remove(tmpGz)
	size, sum, err := gzipFile(tmpDB, tmpGz)
	if err != nil {
		return Item{}, err
	}
	if err := os.Rename(tmpGz, filepath.Join(dir, name)); err != nil {
		return Item{}, err
	}
	it := Item{Name: name, SizeBytes: size, SHA256: sum, CreatedAt: now.UTC().Format(time.RFC3339), Trigger: trigger, S3: S3State{Status: S3None}}
	if err := writeSidecar(dir, it); err != nil {
		return Item{}, err
	}
	return it, nil
}

// gzipFile compresses src into dst (0600, fsynced) and returns the size and SHA-256 of dst.
func gzipFile(src, dst string) (int64, string, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	counter := &countWriter{}
	zw, _ := gzip.NewWriterLevel(io.MultiWriter(out, h, counter), gzip.BestSpeed)
	_, err = io.Copy(zw, in)
	if err == nil {
		err = zw.Close()
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, "", err
	}
	return counter.n, hex.EncodeToString(h.Sum(nil)), nil
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

func writeSidecar(dir string, it Item) error {
	raw, err := json.MarshalIndent(it, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".tmp-"+randHex(8)+sidecarExt)
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, it.Name+sidecarExt))
}

// readItem loads the sidecar of name; a snapshot without sidecar gets one (checksum recomputed).
func readItem(dir, name string) (Item, error) {
	if !ValidName(name) {
		return Item{}, ErrInvalidName
	}
	fi, err := os.Stat(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, name+sidecarExt))
	if err == nil {
		var it Item
		if json.Unmarshal(raw, &it) == nil && it.Name == name {
			return it, nil
		}
	}
	sum, err := fileSHA256(filepath.Join(dir, name))
	if err != nil {
		return Item{}, err
	}
	it := Item{Name: name, SizeBytes: fi.Size(), SHA256: sum, CreatedAt: nameTime(name).Format(time.RFC3339), Trigger: TriggerManual,
		S3: S3State{Status: S3None}}
	return it, writeSidecar(dir, it)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func nameTime(name string) time.Time {
	t, _ := time.Parse("20060102T150405Z", strings.TrimPrefix(name, "stride-")[:16])
	return t
}

// List returns the snapshots of dir, newest first.
func List(dir string) ([]Item, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Item{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Item{}
	for _, e := range entries {
		if e.IsDir() || !ValidName(e.Name()) {
			continue
		}
		it, err := readItem(dir, e.Name())
		if err != nil {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// Prune applies the local retention: the 7 latest manual and daily snapshots, the 3 latest
// pre-migration ones. It returns the deleted names.
func Prune(dir string) ([]string, error) {
	items, err := List(dir)
	if err != nil {
		return nil, err
	}
	var deleted []string
	regular, pre := 0, 0
	for _, it := range items {
		keep := true
		if it.Trigger == TriggerPreMigration {
			pre++
			keep = pre <= keepPreMigration
		} else {
			regular++
			keep = regular <= keepLocal
		}
		if keep {
			continue
		}
		if err := removeLocal(dir, it.Name); err != nil {
			return deleted, err
		}
		deleted = append(deleted, it.Name)
	}
	return deleted, nil
}

func removeLocal(dir, name string) error {
	if !ValidName(name) {
		return ErrInvalidName
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name+sidecarExt)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
