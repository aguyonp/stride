// Package store owns the SQLite database (WAL) and delegates migrations to internal/database.
// Platform queries live in platform_*.go.
//
// Two pools share the file (§15.3): DB is the single-connection write pool (BEGIN IMMEDIATE), R a
// query_only read pool sized to the CPUs. A read never waits for the writer (WAL); a write never
// races another one. Code running inside a Tx callback must only use its *sql.Tx: a query on DB
// there would wait forever for the connection the transaction holds.
package store

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"

	"stride/internal/database"
)

type Store struct {
	// DB is the write pool (one connection): writes and read-modify-write paths.
	DB *sql.DB
	// R is the read pool (query_only); it is DB itself for an in-memory database.
	R *sql.DB
	// Path is the database file (":memory:" in tests).
	Path string
}

// Options tunes Open.
type Options struct {
	// PreMigrate runs on the write pool after the file is opened and before pending migrations are
	// applied (pre-migration snapshot); pending tells whether goose has migrations to run.
	PreMigrate func(ctx context.Context, db *sql.DB, pending bool) error
}

// pragmas common to both pools (§15.3).
var pragmas = []string{"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(1)", "synchronous(NORMAL)", "temp_store(MEMORY)",
	"cache_size(-64000)", "mmap_size(268435456)"}

func dsn(path string, extra []string, txlock string) string {
	q := url.Values{}
	for _, p := range append(append([]string{}, pragmas...), extra...) {
		q.Add("_pragma", p)
	}
	if txlock != "" {
		q.Set("_txlock", txlock)
	}
	return "file:" + path + "?" + q.Encode()
}

func Open(ctx context.Context, path string) (*Store, error) { return OpenWith(ctx, path, Options{}) }

func OpenWith(ctx context.Context, path string, o Options) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", dsn(path, nil, "immediate"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if o.PreMigrate != nil {
		pending, err := database.HasPending(ctx, db)
		if err == nil {
			err = o.PreMigrate(ctx, db, pending)
		}
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := database.Migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := database.VerifySchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{DB: db, R: db, Path: path}
	if path == ":memory:" {
		return s, nil
	}
	// The read pool is opened once the schema exists; query_only comes last so the shared pragmas apply.
	r, err := sql.Open("sqlite", dsn(path, []string{"query_only(1)"}, ""))
	if err != nil {
		db.Close()
		return nil, err
	}
	r.SetMaxOpenConns(max(4, runtime.GOMAXPROCS(0)))
	r.SetMaxIdleConns(max(4, runtime.GOMAXPROCS(0)))
	r.SetConnMaxIdleTime(5 * time.Minute)
	s.R = r
	return s, nil
}

func (s *Store) Close() error {
	if s.R != nil && s.R != s.DB {
		_ = s.R.Close()
	}
	return s.DB.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	var one int
	if err := s.R.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return err
	}
	return s.DB.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

// Tx runs fn in a write transaction. fn must use tx only (never DB, see the package comment).
func (s *Store) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
