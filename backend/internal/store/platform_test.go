package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func pfOpenTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func pfHash(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

func pfNewUser(name string, cred string, invite []byte) NewUser {
	return NewUser{
		WebAuthnID:  []byte("handle-" + name),
		DisplayName: name,
		InviteHash:  invite,
		Credential:  NewCredential{ID: []byte(cred), Name: "Passkey", Data: []byte(`{}`), BackupEligible: true},
	}
}

func TestRegisterFirstAdminThenInvitations(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	now := time.Now()

	admin, err := s.RegisterUser(ctx, pfNewUser("alice", "c1", nil), now)
	if err != nil || admin.Role != RoleAdmin {
		t.Fatalf("first user: %+v %v", admin, err)
	}
	if _, err := s.RegisterUser(ctx, pfNewUser("bob", "c2", nil), now); !errors.Is(err, ErrInviteRequired) {
		t.Fatalf("want ErrInviteRequired, got %v", err)
	}
	inv, err := s.CreateInvitation(ctx, Invitation{Label: "x", CodePrefix: "ABCD", MaxUses: 1, CreatedBy: &admin.ID, CreatedAt: now}, pfHash("code"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CheckInvitation(ctx, pfHash("code"), now); err != nil {
		t.Fatal(err)
	}
	bob, err := s.RegisterUser(ctx, pfNewUser("bob", "c2", pfHash("code")), now)
	if err != nil || bob.Role != RoleUser {
		t.Fatalf("invited user: %+v %v", bob, err)
	}
	if _, err := s.RegisterUser(ctx, pfNewUser("carol", "c3", pfHash("code")), now); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("used-up invitation: %v", err)
	}
	if got, _ := s.GetInvitation(ctx, inv.ID); got.Uses != 1 {
		t.Fatalf("uses=%d", got.Uses)
	}
	// a failed registration must not leave a user behind
	if n, _ := s.CountUsers(ctx); n != 2 {
		t.Fatalf("users=%d", n)
	}
	if err := s.SetSetting(ctx, SettingRegistrationMode, RegistrationOpen, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterUser(ctx, pfNewUser("carol", "c3", nil), now); err != nil {
		t.Fatalf("open mode: %v", err)
	}
	if _, err := s.RegisterUser(ctx, pfNewUser("dave", "c3", nil), now); !errors.Is(err, ErrDuplicateCredential) {
		t.Fatalf("duplicate credential: %v", err)
	}
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) != 3 || users[0].Passkeys != 1 {
		t.Fatalf("list: %+v %v", users, err)
	}
}

func TestInvitationExpiryRevocationAndPurge(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	now := time.Now()
	past := now.Add(-time.Hour)
	if _, err := s.CreateInvitation(ctx, Invitation{CodePrefix: "A", MaxUses: 5, ExpiresAt: &past, CreatedAt: now.Add(-10 * 24 * time.Hour)}, pfHash("expired")); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckInvitation(ctx, pfHash("expired"), now); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expired: %v", err)
	}
	live, _ := s.CreateInvitation(ctx, Invitation{CodePrefix: "B", MaxUses: 5, CreatedAt: now}, pfHash("live"))
	if err := s.RevokeInvitation(ctx, live.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckInvitation(ctx, pfHash("live"), now); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("revoked: %v", err)
	}
	n, err := s.PurgeInvitations(ctx, now, 7*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("purge n=%d err=%v", n, err)
	}
	if err := s.RevokeInvitation(ctx, 999); !errors.Is(err, ErrUnknownInvitation) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestSessionsAndDisable(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	now := time.Now()
	u, _ := s.RegisterUser(ctx, pfNewUser("alice", "c1", nil), now)
	tok := pfHash("token")
	if err := s.CreateSession(ctx, Session{TokenHash: tok, UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	sess, su, err := s.GetSession(ctx, tok, now)
	if err != nil || su.ID != u.ID || sess.UserID != u.ID {
		t.Fatalf("get session: %v", err)
	}
	if _, _, err := s.GetSession(ctx, tok, now.Add(2*time.Hour)); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("expired session: %v", err)
	}
	if err := s.TouchSession(ctx, tok, now, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GetSession(ctx, tok, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("slid session: %v", err)
	}
	disabled := StatusDisabled
	if err := s.UpdateUserAccess(ctx, u.ID, nil, &disabled); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("the last active admin cannot be disabled: %v", err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO users (display_name, role, status, created_at) VALUES ('root', 'admin', 'active', ?)`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUserAccess(ctx, u.ID, nil, &disabled); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GetSession(ctx, tok, now); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("disabled user keeps session: %v", err)
	}
	_ = s.CreateSession(ctx, Session{TokenHash: pfHash("old"), UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Minute), LastSeenAt: now})
	if n, err := s.PurgeExpiredSessions(ctx, now); err != nil || n != 1 {
		t.Fatalf("purge n=%d err=%v", n, err)
	}
}

func TestCredentialsLastOneProtected(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	now := time.Now()
	u, _ := s.RegisterUser(ctx, pfNewUser("alice", "c1", nil), now)
	if err := s.DeleteCredential(ctx, u.ID, []byte("c1")); !errors.Is(err, ErrLastCredential) {
		t.Fatalf("last: %v", err)
	}
	if err := s.AddCredential(ctx, u.ID, NewCredential{ID: []byte("c2"), Name: "Second", Data: []byte(`{}`)}, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameCredential(ctx, u.ID, []byte("c2"), "Téléphone"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameCredential(ctx, u.ID+1, []byte("c2"), "x"); !errors.Is(err, ErrUnknownCredential) {
		t.Fatalf("foreign rename: %v", err)
	}
	if err := s.UpdateCredentialUse(ctx, []byte("c2"), []byte(`{"a":1}`), true, now); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCredential(ctx, u.ID, []byte("c1")); err != nil {
		t.Fatal(err)
	}
	creds, _ := s.ListCredentials(ctx, u.ID)
	if len(creds) != 1 || creds[0].Name != "Téléphone" || !creds[0].BackupState || creds[0].LastUsedAt == nil {
		t.Fatalf("creds: %+v", creds)
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if creds, _ := s.ListCredentials(ctx, u.ID); len(creds) != 0 {
		t.Fatal("credentials not cascaded")
	}
}

func TestSettingsAndEvents(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	now := time.Now()
	if m, _ := s.RegistrationMode(ctx); m != RegistrationInvite {
		t.Fatalf("default mode %q", m)
	}
	_ = s.EnsureSetting(ctx, SettingRegistrationMode, RegistrationOpen, now)
	_ = s.EnsureSetting(ctx, SettingRegistrationMode, RegistrationInvite, now)
	if m, _ := s.RegistrationMode(ctx); m != RegistrationOpen {
		t.Fatalf("ensure must not override: %q", m)
	}
	evs := []APIEvent{
		{At: now.Add(-8 * 24 * time.Hour), Provider: "strava", Endpoint: "GET /x", Status: 500},
		{At: now, Provider: "garmin", UserID: 3, Endpoint: "GET /y", Status: 429, Message: "rate limited"},
	}
	if err := s.InsertAPIEvents(ctx, evs); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.PurgeAPIEvents(ctx, now.Add(-7*24*time.Hour)); n != 1 {
		t.Fatalf("purge %d", n)
	}
	got, err := s.RecentAPIEvents(ctx, 10)
	if err != nil || len(got) != 1 || got[0].UserID != 3 || got[0].Status != 429 {
		t.Fatalf("recent: %+v %v", got, err)
	}
}
