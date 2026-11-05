package secret

import (
	"bytes"
	"testing"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func TestRoundTripAndAAD(t *testing.T) {
	s, err := New(key(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.SealString("token", "garmin:1")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("token")) {
		t.Fatal("plaintext leaked")
	}
	if got, err := s.OpenString(sealed, "garmin:1"); err != nil || got != "token" {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := s.Open(sealed, "garmin:2"); err == nil {
		t.Fatal("aad mismatch must fail")
	}
}

func TestRotation(t *testing.T) {
	old, _ := New(key(1), nil)
	sealed, _ := old.SealString("refresh", "strava:7")
	rotated, _ := New(key(2), key(1))
	if got, err := rotated.OpenString(sealed, "strava:7"); err != nil || got != "refresh" {
		t.Fatalf("previous key: %q %v", got, err)
	}
	if !rotated.NeedsRotation(sealed, "strava:7") {
		t.Fatal("expected rotation flag")
	}
	fresh, _ := New(key(2), nil)
	if _, err := fresh.Open(sealed, "strava:7"); err == nil {
		t.Fatal("unknown key must fail")
	}
}
