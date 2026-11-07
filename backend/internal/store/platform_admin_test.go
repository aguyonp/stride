package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLastAdminGuardAndSessionRevocation(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	now := time.Now()
	a, _ := s.RegisterUser(ctx, pfNewUser("alice", "c1", nil), now)
	user, admin := RoleUser, RoleAdmin
	disabled := StatusDisabled
	if err := s.UpdateUserAccess(ctx, a.ID, &user, nil); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", err)
	}
	if err := s.EnsureNotLastAdmin(ctx, a.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin: %v", err)
	}
	if err := s.UpdateUserAccess(ctx, a.ID, &admin, nil); err != nil {
		t.Fatalf("keeping the admin role is allowed: %v", err)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users (display_name, role, status, created_at) VALUES ('bob', 'admin', 'disabled', ?)`, now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := res.LastInsertId()
	if err := s.UpdateUserAccess(ctx, a.ID, nil, &disabled); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("a disabled admin does not count: %v", err)
	}
	if err := s.UpdateUserAccess(ctx, bob, &user, nil); err != nil {
		t.Fatalf("demoting an inactive admin is allowed: %v", err)
	}
	if err := s.EnsureNotLastAdmin(ctx, 999); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("unknown: %v", err)
	}

	for _, tok := range []string{"t1", "t2"} {
		if err := s.CreateSession(ctx, Session{TokenHash: pfHash(tok), UserID: bob, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.DeleteUserSessions(ctx, bob); err != nil || n != 2 {
		t.Fatalf("revoke: %d %v", n, err)
	}
	if n, err := s.DeleteUserSessions(ctx, bob); err != nil || n != 0 {
		t.Fatalf("revoke again: %d %v", n, err)
	}
}
