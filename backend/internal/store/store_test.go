package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenMigratesOnceWithWAL(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := s.DB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode=%q err=%v", mode, err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO users (display_name, created_at) VALUES ('a', 1)`); err != nil {
		t.Fatal(err)
	}
}
