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
	if err := os.WriteFile(p, []byte("PATELLA_TEST_X=fichier\nPATELLA_TEST_Y=fichier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATELLA_TEST_X", "env")
	os.Unsetenv("PATELLA_TEST_Y")
	t.Cleanup(func() { os.Unsetenv("PATELLA_TEST_Y") })
	n, err := LoadDotEnv(p)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if os.Getenv("PATELLA_TEST_X") != "env" || os.Getenv("PATELLA_TEST_Y") != "fichier" {
		t.Error("priorité env/fichier incorrecte")
	}
}

func TestKeyPriorityAndPersistence(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(dir, Config{EnvAPIKey: "cle-env-synthetique", Timezone: "Europe/Paris"})
	if err != nil {
		t.Fatal(err)
	}
	if k, src := st.APIKey(); k != "cle-env-synthetique" || src != "env" {
		t.Errorf("source env attendue : %s", src)
	}
	if err := st.SetAPIKey("cle-ui-synthetique"); err != nil {
		t.Fatal(err)
	}
	if _, src := st.APIKey(); src != "ui" {
		t.Errorf("source ui attendue : %s", src)
	}
	cad := 176.0
	if err := st.UpdateProfile(&ProfilePatch{CadenceTargetMin: &cad}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "settings.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("settings.json : %v", err)
	}
	st2, err := NewStore(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := st2.APIKey(); k != "cle-ui-synthetique" || st2.Profile().CadenceTargetMin != 176 || st2.Profile().CadenceTargetMax != 182 {
		t.Error("persistance incorrecte")
	}
	_ = st2.SetAPIKey("")
	if _, src := st2.APIKey(); src != "none" {
		t.Errorf("source none attendue : %s", src)
	}
}

func TestProfileValidation(t *testing.T) {
	p := DefaultProfile()
	bad := 300.0
	if _, err := p.Apply(&ProfilePatch{CadenceTargetMin: &bad}); err == nil {
		t.Error("cadence hors limites acceptée")
	}
	side := "droite"
	if _, err := p.Apply(&ProfilePatch{InjuredSide: &side}); err == nil {
		t.Error("côté invalide accepté")
	}
	min, max := 185.0, 180.0
	if _, err := p.Apply(&ProfilePatch{CadenceTargetMin: &min, CadenceTargetMax: &max}); err == nil {
		t.Error("min > max accepté")
	}
	right := "RIGHT"
	np, err := p.Apply(&ProfilePatch{InjuredSide: &right})
	if err != nil || np.InjuredSide != "right" {
		t.Errorf("côté droit : %v %v", np.InjuredSide, err)
	}
}
