package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"stride/internal/store"
)

var recoveryCodeRe = regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){3}$`)

func (b *browser) post(path string, v any) (int, []byte) {
	b.f.t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		b.f.t.Fatal(err)
	}
	return b.do("POST", path, raw)
}

type recoverySet struct {
	Codes []string `json:"codes"`
}

type recoveryStatusBody struct {
	Remaining int `json:"remaining"`
}

func (b *browser) loginRecovery(t *testing.T, code string) (int, []byte) {
	t.Helper()
	return b.post("/api/auth/login/recovery", map[string]string{"code": code})
}

func TestRecoveryCodesLifecycle(t *testing.T) {
	f := newFixture(t)
	alice := f.browser()
	_, body, _ := alice.register("Alice", "")
	aliceID := decode[Me](t, body).ID

	// no codes yet
	if code, body := alice.do("GET", "/api/me/recovery", nil); code != http.StatusOK || decode[recoveryStatusBody](t, body).Remaining != 0 {
		t.Fatalf("initial status: %d %s", code, body)
	}
	// anonymous access is refused
	if code, _ := f.browser().do("GET", "/api/me/recovery", nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous status: %d", code)
	}

	// generation: 10 well-formed codes
	code, body := alice.do("POST", "/api/me/recovery", nil)
	set := decode[recoverySet](t, body)
	if code != http.StatusOK || len(set.Codes) != recoveryCodeSetSize {
		t.Fatalf("generate: %d %s", code, body)
	}
	for i, c := range set.Codes {
		if !recoveryCodeRe.MatchString(c) {
			t.Fatalf("code %d malformed: %q", i, c)
		}
	}
	if code, body := alice.do("GET", "/api/me/recovery", nil); code != http.StatusOK || decode[recoveryStatusBody](t, body).Remaining != recoveryCodeSetSize {
		t.Fatalf("status after generation: %d %s", code, body)
	}

	// a fresh browser signs in with a code and gets a session
	fresh := f.browser()
	code, body = fresh.loginRecovery(t, set.Codes[0])
	if code != http.StatusOK || decode[Me](t, body).ID != aliceID {
		t.Fatalf("recovery login: %d %s", code, body)
	}
	if code, _ := fresh.do("GET", "/api/me", nil); code != http.StatusOK {
		t.Fatalf("session after recovery login: %d", code)
	}
	if code, body := fresh.do("GET", "/api/me/recovery", nil); code != http.StatusOK || decode[recoveryStatusBody](t, body).Remaining != recoveryCodeSetSize-1 {
		t.Fatalf("consumed code not counted: %d %s", code, body)
	}

	// a code is strictly single-use, unknown codes and malformed codes are 401
	if code, _ := f.browser().loginRecovery(t, set.Codes[0]); code != http.StatusUnauthorized {
		t.Fatalf("replay: %d", code)
	}
	if code, _ := f.browser().loginRecovery(t, "AAAAAAAA-BBBBBBBB-CCCCCCCC-DDDDDDDD"); code != http.StatusUnauthorized {
		t.Fatalf("unknown code: %d", code)
	}
	if code, _ := f.browser().loginRecovery(t, "abc"); code != http.StatusUnauthorized {
		t.Fatalf("malformed code: %d", code)
	}

	// regeneration: atomically voids the old set, case/separators are normalized on input
	code, body = alice.do("POST", "/api/me/recovery", nil)
	newSet := decode[recoverySet](t, body)
	if code != http.StatusOK || len(newSet.Codes) != recoveryCodeSetSize {
		t.Fatalf("regenerate: %d %s", code, body)
	}
	if code, _ := f.browser().loginRecovery(t, set.Codes[1]); code != http.StatusUnauthorized {
		t.Fatalf("old code after regeneration: %d", code)
	}
	if code, body := f.browser().loginRecovery(t, strings.ToLower(newSet.Codes[0])); code != http.StatusOK || decode[Me](t, body).ID != aliceID {
		t.Fatalf("new code (lowercase): %d %s", code, body)
	}
}

func TestRecoveryLoginDisabledAccount(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	alice, bob := f.browser(), f.browser()
	_, body, _ := alice.register("Alice", "")
	if _, err := f.store.CreateInvitation(ctx, store.Invitation{CodePrefix: "ABCD", MaxUses: 1, CreatedAt: time.Now()}, InviteHash("ABCD-EFGH")); err != nil {
		t.Fatal(err)
	}
	code, body, _ := bob.register("Bob", "abcdefgh")
	bobID := decode[Me](t, body).ID
	if code != http.StatusOK {
		t.Fatalf("invited register: %d %s", code, body)
	}

	code, body = bob.do("POST", "/api/me/recovery", nil)
	set := decode[recoverySet](t, body)
	if code != http.StatusOK || len(set.Codes) != recoveryCodeSetSize {
		t.Fatalf("generate: %d %s", code, body)
	}

	disabled := store.StatusDisabled
	if err := f.store.UpdateUserAccess(ctx, bobID, nil, &disabled); err != nil {
		t.Fatal(err)
	}
	if code, body := f.browser().loginRecovery(t, set.Codes[0]); code != http.StatusForbidden {
		t.Fatalf("disabled account: %d %s", code, body)
	}
}

func TestRecoveryLoginRateLimited(t *testing.T) {
	f := newFixture(t)
	_, _, _ = f.browser().register("Alice", "")
	probe := "AAAAAAAA-BBBBBBBB-CCCCCCCC-DDDDDDDD"
	for i := 0; i < recoveryLoginLimit; i++ {
		if code, _ := f.browser().loginRecovery(t, probe); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	if code, body := f.browser().loginRecovery(t, probe); code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d: %d %s", recoveryLoginLimit+1, code, body)
	}
}

func TestRecoveryCodesCascadeOnUserDeletion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	alice := f.browser()
	_, body, _ := alice.register("Alice", "")
	aliceID := decode[Me](t, body).ID
	if code, _ := alice.do("POST", "/api/me/recovery", nil); code != http.StatusOK {
		t.Fatalf("generate: %d", code)
	}
	if code, _ := alice.do("DELETE", "/api/me", nil); code != http.StatusNoContent {
		t.Fatalf("delete me: %d", code)
	}
	if n, err := f.store.CountUnusedRecoveryCodes(ctx, aliceID); err != nil || n != 0 {
		t.Fatalf("codes after deletion: %d (%v)", n, err)
	}
}
