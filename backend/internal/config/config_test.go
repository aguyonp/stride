package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	src := "\ufeff# commentaire\n\nexport A=1\nB = \"deux mots\" \nC='trois'\nD=quatre # fin\nE=a#b\nsans_egal\n"
	got, err := ParseDotEnv(bufio.NewScanner(strings.NewReader(src)))
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{"A", "1"}, {"B", "deux mots"}, {"C", "trois"}, {"D", "quatre"}, {"E", "a#b"}}
	if len(got) != len(want) {
		t.Fatalf("obtenu %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d : %v, attendu %v", i, got[i], want[i])
		}
	}
}

func TestLoadDotEnvDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("STRIDE_TEST_X=fichier\nSTRIDE_TEST_Y=fichier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STRIDE_TEST_X", "env")
	os.Unsetenv("STRIDE_TEST_Y")
	t.Cleanup(func() { os.Unsetenv("STRIDE_TEST_Y") })
	n, err := LoadDotEnv(p)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if os.Getenv("STRIDE_TEST_X") != "env" || os.Getenv("STRIDE_TEST_Y") != "fichier" {
		t.Error("priorité env/fichier incorrecte")
	}
}
