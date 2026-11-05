package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"stride/internal/cache"
	"stride/internal/store"
)

const (
	kindRegister = "register"
	kindLogin    = "login"
	kindAdd      = "add"
)

var errCeremony = errors.New("webauthn ceremony missing or expired")

// ceremony is the server-side state between begin and finish, kept in the cache only.
type ceremony struct {
	Kind        string               `json:"kind"`
	Session     webauthn.SessionData `json:"session"`
	DisplayName string               `json:"displayName,omitempty"`
	Handle      []byte               `json:"handle,omitempty"`
	InviteHash  []byte               `json:"inviteHash,omitempty"`
	UserID      int64                `json:"userId,omitempty"`
	Name        string               `json:"name,omitempty"`
}

func ceremonyKey(id string) string { return cache.Prefix + "webauthn:" + id }

func (s *Service) saveCeremony(ctx context.Context, w http.ResponseWriter, c ceremony) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	id := randomToken(32)
	if err := s.cache.Set(ctx, ceremonyKey(id), b, ceremonyTTL); err != nil {
		return err
	}
	s.setCookie(w, ceremonyCookie, id, "/api/", ceremonyTTL, http.SameSiteStrictMode)
	return nil
}

// takeCeremony loads and deletes the ceremony bound to the request cookie (single use).
func (s *Service) takeCeremony(ctx context.Context, w http.ResponseWriter, r *http.Request, kind string) (*ceremony, error) {
	ck, err := r.Cookie(ceremonyCookie)
	if err != nil || ck.Value == "" || len(ck.Value) > 128 {
		return nil, errCeremony
	}
	s.setCookie(w, ceremonyCookie, "", "/api/", 0, http.SameSiteStrictMode)
	key := ceremonyKey(ck.Value)
	b, ok, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errCeremony
	}
	_ = s.cache.Del(ctx, key)
	var c ceremony
	if err := json.Unmarshal(b, &c); err != nil || c.Kind != kind {
		return nil, errCeremony
	}
	return &c, nil
}

// waUser adapts a Stride user to webauthn.User.
type waUser struct {
	handle []byte
	name   string
	creds  []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return u.handle }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.name }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *Service) loadWAUser(ctx context.Context, u *store.User) (*waUser, error) {
	rows, err := s.store.ListCredentials(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	wu := &waUser{handle: u.WebAuthnID, name: u.DisplayName}
	for _, row := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal(row.Data, &c); err != nil {
			return nil, fmt.Errorf("credential decode: %w", err)
		}
		wu.creds = append(wu.creds, c)
	}
	return wu, nil
}

func newStoredCredential(c *webauthn.Credential, name string) (store.NewCredential, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return store.NewCredential{}, err
	}
	return store.NewCredential{ID: c.ID, Name: name, Data: data, BackupEligible: c.Flags.BackupEligible, BackupState: c.Flags.BackupState}, nil
}

// defaultPasskeyName labels a passkey from the browser user agent, e.g. "Chrome · macOS".
func defaultPasskeyName(ua string, now time.Time) string {
	browser := ""
	for _, b := range []struct{ token, label string }{
		{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"CriOS/", "Chrome"}, {"FxiOS/", "Firefox"},
		{"Chrome/", "Chrome"}, {"Safari/", "Safari"},
	} {
		if strings.Contains(ua, b.token) {
			browser = b.label
			break
		}
	}
	os := ""
	for _, o := range []struct{ token, label string }{
		{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"Mac OS X", "macOS"}, {"Macintosh", "macOS"},
		{"Windows", "Windows"}, {"CrOS", "ChromeOS"}, {"Linux", "Linux"},
	} {
		if strings.Contains(ua, o.token) {
			os = o.label
			break
		}
	}
	switch {
	case browser != "" && os != "":
		return browser + " · " + os
	case browser != "" || os != "":
		return browser + os
	default:
		return "Clé d'accès du " + now.Format("02/01/2006")
	}
}
