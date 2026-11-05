package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
)

// softAuthn is a minimal platform authenticator (ES256, "none" attestation) for handler tests.
type softAuthn struct {
	rpID   string
	origin string
}

type softCred struct {
	id     []byte
	key    *ecdsa.PrivateKey
	handle []byte
	count  uint32
}

const (
	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagBS = 0x10
	flagAT = 0x40
)

var b64 = base64.RawURLEncoding

func (a softAuthn) authData(flags byte, count uint32, attested []byte) []byte {
	h := sha256.Sum256([]byte(a.rpID))
	out := append(h[:], flags)
	out = binary.BigEndian.AppendUint32(out, count)
	return append(out, attested...)
}

func (a softAuthn) clientData(t *testing.T, typ, challenge string) []byte {
	b, err := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": a.origin, "crossOrigin": false})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// create answers a {"publicKey": PublicKeyCredentialCreationOptionsJSON} document.
func (a softAuthn) create(t *testing.T, options []byte) ([]byte, *softCred) {
	t.Helper()
	var opt struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
			AuthenticatorSelection struct {
				ResidentKey string `json:"residentKey"`
			} `json:"authenticatorSelection"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opt); err != nil {
		t.Fatalf("creation options: %v (%s)", err, options)
	}
	if opt.PublicKey.AuthenticatorSelection.ResidentKey != "required" {
		t.Fatalf("resident key must be required: %s", options)
	}
	handle, err := b64.DecodeString(opt.PublicKey.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	pub, err := key.PublicKey.ECDH()
	if err != nil {
		t.Fatal(err)
	}
	raw := pub.Bytes()
	cose, err := webauthncbor.Marshal(map[int64]any{1: int64(2), 3: int64(-7), -1: int64(1), -2: raw[1:33], -3: raw[33:65]})
	if err != nil {
		t.Fatal(err)
	}
	attested := make([]byte, 16)
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(id)))
	attested = append(append(attested, id...), cose...)
	ad := a.authData(flagUP|flagUV|flagBE|flagBS|flagAT, 0, attested)
	att, err := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": ad})
	if err != nil {
		t.Fatal(err)
	}
	cd := a.clientData(t, "webauthn.create", opt.PublicKey.Challenge)
	body, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(id), "rawId": b64.EncodeToString(id), "type": "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"attestationObject": b64.EncodeToString(att),
			"clientDataJSON":    b64.EncodeToString(cd),
			"transports":        []string{"internal"},
		},
		"clientExtensionResults": map[string]any{"credProps": map[string]any{"rk": true}},
	})
	return body, &softCred{id: id, key: key, handle: handle}
}

// get answers a {"publicKey": PublicKeyCredentialRequestOptionsJSON} document with a discoverable credential.
func (a softAuthn) get(t *testing.T, options []byte, c *softCred) []byte {
	t.Helper()
	var opt struct {
		PublicKey struct {
			Challenge        string            `json:"challenge"`
			AllowCredentials []json.RawMessage `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opt); err != nil {
		t.Fatalf("request options: %v", err)
	}
	if len(opt.PublicKey.AllowCredentials) != 0 {
		t.Fatalf("discoverable login must not list credentials: %s", options)
	}
	c.count++
	ad := a.authData(flagUP|flagUV|flagBE|flagBS, c.count, nil)
	cd := a.clientData(t, "webauthn.get", opt.PublicKey.Challenge)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, c.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(c.id), "rawId": b64.EncodeToString(c.id), "type": "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"authenticatorData": b64.EncodeToString(ad),
			"clientDataJSON":    b64.EncodeToString(cd),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(c.handle),
		},
		"clientExtensionResults": map[string]any{},
	})
	return body
}
