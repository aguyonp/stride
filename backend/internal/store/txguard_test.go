package store

import (
	"context"
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The write pool has a single connection: a query on DB (or through a helper that reaches it,
// such as s.db() or s.repo()) inside a Tx callback waits forever for the connection the
// transaction holds. This test walks every Tx callback of the backend and the methods it calls on
// the same receiver, and fails on any such access.
func TestNoPoolQueryInsideTx(t *testing.T) {
	root := filepath.Join("..")
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	var problems []string
	for _, dir := range dirs {
		problems = append(problems, scanTxCallbacks(t, dir)...)
	}
	if len(problems) > 0 {
		t.Fatalf("pool access inside a Tx callback (deadlock with the single writer):\n%s", strings.Join(problems, "\n"))
	}
}

// poolSelectors are the fields and helper methods that reach a connection pool instead of the tx.
var poolSelectors = map[string]bool{"DB": true, "R": true, "db": true, "rdb": true, "repo": true, "readRepo": true}

func scanTxCallbacks(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	var out []string
	for _, pkg := range pkgs {
		methods := map[string]*ast.FuncDecl{}
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Body != nil {
					methods[fd.Name.Name] = fd
				}
			}
		}
		for _, f := range pkg.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Tx" {
					return true
				}
				fn, ok := call.Args[len(call.Args)-1].(*ast.FuncLit)
				if !ok {
					return true
				}
				pos := fset.Position(call.Pos())
				for _, bad := range poolUses(fn.Body, methods, map[string]bool{}) {
					out = append(out, pos.String()+": "+bad)
				}
				return true
			})
		}
	}
	return out
}

// poolUses lists the pool accesses of body, following calls to methods of the same package.
func poolUses(body ast.Node, methods map[string]*ast.FuncDecl, seen map[string]bool) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if poolSelectors[x.Sel.Name] {
				if id, ok := x.X.(*ast.Ident); !ok || id.Name != "tx" {
					out = append(out, "uses ."+x.Sel.Name)
				}
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				// Follow only calls on the method's own receiver (s.helper()), not on a value of
				// another type sharing the name (athlete.Repo{DB: tx}.CreatePlanned()).
				id, isIdent := sel.X.(*ast.Ident)
				if fd, ok := methods[sel.Sel.Name]; ok && isIdent && receiverName(fd) == id.Name && !seen[sel.Sel.Name] {
					seen[sel.Sel.Name] = true
					for _, u := range poolUses(fd.Body, methods, seen) {
						out = append(out, sel.Sel.Name+"() "+u)
					}
				}
			}
		}
		return true
	})
	return out
}

func receiverName(fd *ast.FuncDecl) string {
	if len(fd.Recv.List) > 0 && len(fd.Recv.List[0].Names) > 0 {
		return fd.Recv.List[0].Names[0].Name
	}
	return ""
}

// The guard above is static; this documents why it matters: with one writer connection, a pool
// query issued while a transaction is open cannot complete.
func TestPoolQueryInsideTxBlocks(t *testing.T) {
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = s.Tx(context.Background(), func(tx *sql.Tx) error {
		var n int
		return s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the nested pool query to block until the deadline, got %v", err)
	}
	// The read pool is independent and query_only.
	var n int
	if err := s.R.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if _, err := s.R.ExecContext(context.Background(), `INSERT INTO users (display_name, created_at) VALUES ('x', 1)`); err == nil {
		t.Fatal("read pool accepted a write")
	}
}

// The guard must catch a direct and an indirect pool access (it is not vacuous).
func TestTxGuardDetectsViolations(t *testing.T) {
	dir := t.TempDir()
	src := `package x

import ("context"; "database/sql")

type S struct{ st interface{ Tx(context.Context, func(*sql.Tx) error) error }; pool *sql.DB }

func (s *S) db() *sql.DB { return s.pool }
func (s *S) helper(ctx context.Context) { s.db().ExecContext(ctx, "SELECT 1") }
func (s *S) ok(ctx context.Context, tx *sql.Tx) { tx.ExecContext(ctx, "SELECT 1") }

func (s *S) Direct(ctx context.Context) error {
	return s.st.Tx(ctx, func(tx *sql.Tx) error { _, err := s.db().ExecContext(ctx, "SELECT 1"); return err })
}
func (s *S) Indirect(ctx context.Context) error {
	return s.st.Tx(ctx, func(tx *sql.Tx) error { s.ok(ctx, tx); s.helper(ctx); return nil })
}
`
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	got := scanTxCallbacks(t, dir)
	if len(got) != 2 || !strings.Contains(got[1], "helper()") {
		t.Fatalf("violations not detected: %q", got)
	}
}
