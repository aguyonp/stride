// Package store owns the SQLite database (WAL) and its embedded migrations.
// Platform queries live in platform_*.go, domain queries in domain_*.go.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	DB *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	q := url.Values{}
	for _, p := range []string{"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(1)", "synchronous(NORMAL)", "temp_store(MEMORY)"} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(8)
	}
	db.SetConnMaxIdleTime(5 * time.Minute)
	s := &Store{DB: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Ping(ctx context.Context) error {
	var one int
	return s.DB.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

// Tx runs fn in a write transaction.
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

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		base := strings.TrimPrefix(name, "migrations/")
		var done int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, base).Scan(&done); err != nil {
			return err
		}
		if done > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}
		err = s.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, base, time.Now().Unix())
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", base, err)
		}
	}
	return nil
}
