package store

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// The bridge tokens live in their own provider row (§23, issue #147): the migration widened the
// CHECK, and the presence of the row is the user switch.
func TestBridgeTokensRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	if _, err := s.DB.Exec(`INSERT INTO users (id, display_name, role, created_at) VALUES (1, 'u', 'user', 1)`); err != nil {
		t.Fatal(err)
	}

	if blob, err := s.BridgeTokens(ctx, 1); err != nil || blob != nil {
		t.Fatalf("empty: %v %v", blob, err)
	}
	if s.BridgeConnected(ctx, 1) {
		t.Fatal("connected without tokens")
	}
	sealed := []byte{0xde, 0xad, 0xbe, 0xef}
	if err := s.SaveBridgeTokens(ctx, 1, sealed, "Antoine", time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := s.BridgeTokens(ctx, 1)
	if err != nil || !bytes.Equal(got, sealed) {
		t.Fatalf("blob: %v %v", got, err)
	}
	if !s.BridgeConnected(ctx, 1) {
		t.Fatal("not connected with tokens")
	}
	var name string
	if err := s.R.QueryRow(`SELECT display_name FROM provider_connections WHERE user_id = 1 AND provider = 'garmin_web'`).Scan(&name); err != nil || name != "Antoine" {
		t.Fatalf("display name: %q %v", name, err)
	}

	// A second save replaces the blob (reconnection with new tokens).
	other := []byte{1, 2, 3}
	if err := s.SaveBridgeTokens(ctx, 1, other, "Antoine", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.BridgeTokens(ctx, 1); !bytes.Equal(got, other) {
		t.Fatalf("second save: %v", got)
	}

	if err := s.DeleteBridgeTokens(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if s.BridgeConnected(ctx, 1) {
		t.Fatal("still connected after delete")
	}
	// The Personal API row is untouched by the bridge: a seeded one proves it.
	if err := s.SaveBridgeTokens(ctx, 1, sealed, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO provider_connections (user_id, provider, status, secret_enc, created_at, updated_at)
		VALUES (1, 'garmin', 'ok', x'00', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBridgeTokens(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.R.QueryRow(`SELECT COUNT(*) FROM provider_connections WHERE user_id = 1 AND provider = 'garmin'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("personal row: %d %v", n, err)
	}
}

// The admin switch defaults to off: the bridge is experimental.
func TestGarminBridgeSetting(t *testing.T) {
	ctx := context.Background()
	s := pfOpenTest(t)
	if s.GarminBridgeEnabled(ctx) {
		t.Fatal("bridge enabled by default")
	}
	if err := s.SetSetting(ctx, SettingGarminBridgeEnabled, "true", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !s.GarminBridgeEnabled(ctx) {
		t.Fatal("switch not read")
	}
	if err := s.SetSetting(ctx, SettingGarminBridgeEnabled, "peut-être", time.Now()); err != nil {
		t.Fatal(err)
	}
	if s.GarminBridgeEnabled(ctx) {
		t.Fatal("a garbled value must fall back to off")
	}
}
