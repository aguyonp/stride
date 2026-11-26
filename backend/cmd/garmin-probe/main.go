// Command garmin-probe is the field diagnostic of the experimental Garmin bridge (§23, issue #147).
//
// It replays the exact sign-in the server performs — same package, same flow, same headers — against
// Garmin, on the machine of the person who runs it, and prints the state, the failing step and the
// error. The password is read without echo, never leaves the process, is never written to disk and
// never logged; the tokens are thrown away on exit. It is how a report becomes actionable without
// anyone sharing a credential.
//
//	cd backend && go run ./cmd/garmin-probe
//
// It is a separate main package: nothing of it ships in the stride binary.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"stride/internal/garminbridge"
)

// memStore keeps the tokens in memory only: this tool must not leave a session behind.
type memStore struct{ t *garminbridge.Tokens }

func (m *memStore) Load(context.Context, int64) (*garminbridge.Tokens, error) { return m.t, nil }
func (m *memStore) Save(_ context.Context, _ int64, t garminbridge.Tokens) error {
	m.t = &t
	return nil
}
func (m *memStore) Delete(context.Context, int64) error { m.t = nil; return nil }

func main() {
	fmt.Println("Diagnostic du pont Garmin — le mot de passe est lu sans écho et n'est écrit nulle part.")
	fmt.Println()
	email := prompt("E-mail Garmin : ")
	password := promptHidden("Mot de passe     : ")

	svc := garminbridge.New(garminbridge.Options{Store: &memStore{}})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Println("\n→ connexion…")
	res, err := svc.Login(ctx, 1, email, password)
	report(res, err)
	if err != nil || res.State != garminbridge.StateMFA {
		return
	}

	code := prompt("\nCode reçu (e-mail ou application) : ")
	fmt.Println("\n→ vérification du code…")
	res, err = svc.LoginMFA(ctx, 1, code)
	report(res, err)
}

// report prints the three things that matter: the state, the step, the error.
func report(res garminbridge.LoginResult, err error) {
	fmt.Printf("état    : %s\n", res.State)
	if res.DisplayName != "" {
		fmt.Printf("compte  : %s\n", res.DisplayName)
	}
	if err == nil {
		fmt.Println("résultat : connecté ✅")
		return
	}
	step := garminbridge.StepOf(err)
	if step == "" {
		step = "(aucune étape)"
	}
	fmt.Printf("étape   : %s\n", step)
	fmt.Printf("erreur  : %v\n", err)
	fmt.Println("résultat : échec ❌ — envoyez ces lignes, jamais vos identifiants.")
}

func prompt(label string) string {
	fmt.Print(label)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// promptHidden reads a line without echoing it, so the password never lands on screen, in a scroll
// buffer or in the shell history.
func promptHidden(label string) string {
	fmt.Print(label)
	off := exec.Command("stty", "-echo")
	off.Stdin = os.Stdin
	_ = off.Run()
	defer func() {
		on := exec.Command("stty", "echo")
		on.Stdin = os.Stdin
		_ = on.Run()
		fmt.Println()
	}()
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}
