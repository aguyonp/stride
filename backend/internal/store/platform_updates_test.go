package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// insertUser writes a user with an explicit role and status (the release notice targets active
// administrators only, §22 / issue #143).
func insertUser(t *testing.T, s *Store, id int64, role, status string) {
	t.Helper()
	if _, err := s.DB.Exec(`INSERT INTO users (id, display_name, role, status, created_at) VALUES (?, ?, ?, ?, 1)`,
		id, fmt.Sprintf("u%d", id), role, status); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func TestUpdateCheckEnabledDefaultsAndStorage(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)

	// Nothing stored: the check is on, otherwise the feature would be dead by default.
	if !s.UpdateCheckEnabled(ctx) {
		t.Fatal("default is off")
	}
	if err := s.SetSetting(ctx, SettingUpdateCheckEnabled, "false", time.Now()); err != nil {
		t.Fatal(err)
	}
	if s.UpdateCheckEnabled(ctx) {
		t.Fatal("stored false ignored")
	}
	if err := s.SetSetting(ctx, SettingUpdateCheckEnabled, "true", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !s.UpdateCheckEnabled(ctx) {
		t.Fatal("stored true ignored")
	}
	// A garbled value must not turn the feature off nor fail a page load.
	if err := s.SetSetting(ctx, SettingUpdateCheckEnabled, "peut-être", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !s.UpdateCheckEnabled(ctx) {
		t.Fatal("garbled value should fall back to the default")
	}
}

func TestAdminUserIDs(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	insertUser(t, s, 1, "admin", "active")
	insertUser(t, s, 2, "user", "active")
	insertUser(t, s, 3, "admin", "active")
	insertUser(t, s, 4, "admin", "disabled")

	ids, err := s.AdminUserIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Active administrators only, in a stable order: a disabled admin has no session to read it.
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("admins: %v", ids)
	}
}

func TestAdminUserIDsEmpty(t *testing.T) {
	s := pfOpenTest(t)
	ids, err := s.AdminUserIDs(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatalf("no admin: %v %v", ids, err)
	}
}
