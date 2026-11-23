package env

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestEnvExampleDocumentsAllReadVars (issue #90): every environment variable read by the
// binary must be documented in .env.example at the repository root, so the file can never
// drift from env.go again. "Read" means:
//   - os.Getenv / os.LookupEnv with a string literal in any non-test *.go file;
//   - the first string-literal argument of get() / decodeKey() in internal/env/env.go
//     (the only file that builds the Config).
//
// Vars read only from _test.go files (e.g. STRIDE_LIVE_ENV behind the live tag) are out of
// scope, and the check is one-directional: compose.yml variables stay documented in
// .env.example without being read by Go.
func TestEnvExampleDocumentsAllReadVars(t *testing.T) {
	backend, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(backend)

	read := map[string]bool{}
	testOnly := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(backend, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(backend, path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		isEnvFile := filepath.ToSlash(rel) == filepath.Join("internal", "env", "env.go")
		into := read
		if strings.HasSuffix(path, "_test.go") {
			into = testOnly
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			arg, ok := call.Args[0].(*ast.BasicLit)
			if !ok || arg.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(arg.Value)
			if err != nil || name == "" {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == "os" &&
					(fn.Sel.Name == "Getenv" || fn.Sel.Name == "LookupEnv") {
					into[name] = true
				}
			case *ast.Ident:
				if isEnvFile && (fn.Name == "get" || fn.Name == "decodeKey") {
					into[name] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("parsing Go sources: %v", err)
	}
	if len(read) == 0 {
		t.Fatal("no env var read detected — the scan is broken, refusing to pass")
	}

	examplePath := filepath.Join(repoRoot, ".env.example")
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}
	lineVar := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=`)
	documented := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if m := lineVar.FindStringSubmatch(line); m != nil {
			documented[m[1]] = true
		}
	}

	missing := []string{}
	for name := range read {
		if !documented[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("env vars read by the binary but missing from .env.example: %s", strings.Join(missing, ", "))
	}
	t.Logf("checked %d read vars against .env.example (%d documented, %d test-only)", len(read), len(documented), len(testOnly))
}
