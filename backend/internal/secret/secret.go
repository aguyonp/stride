// Package secret encrypts provider credentials at rest with AES-256-GCM.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const version byte = 1

var ErrDecrypt = errors.New("secret: cannot decrypt (wrong master key or corrupted data)")

type Sealer struct {
	current  cipher.AEAD
	previous cipher.AEAD
}

// New takes the 32-byte master key and an optional previous key accepted for decryption only (rotation).
func New(key, previous []byte) (*Sealer, error) {
	cur, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	s := &Sealer{current: cur}
	if len(previous) > 0 {
		if s.previous, err = newAEAD(previous); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal returns version || nonce || ciphertext. aad binds the value to its context (e.g. "garmin:42").
func (s *Sealer) Seal(plaintext []byte, aad string) ([]byte, error) {
	nonce := make([]byte, s.current.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+s.current.Overhead())
	out = append(out, version)
	out = append(out, nonce...)
	return s.current.Seal(out, nonce, plaintext, []byte(aad)), nil
}

func (s *Sealer) Open(sealed []byte, aad string) ([]byte, error) {
	if pt, err := open(s.current, sealed, aad); err == nil {
		return pt, nil
	}
	if s.previous != nil {
		if pt, err := open(s.previous, sealed, aad); err == nil {
			return pt, nil
		}
	}
	return nil, ErrDecrypt
}

// NeedsRotation reports whether sealed data only opens with the previous key.
func (s *Sealer) NeedsRotation(sealed []byte, aad string) bool {
	_, err := open(s.current, sealed, aad)
	return err != nil && s.previous != nil
}

func (s *Sealer) SealString(v, aad string) ([]byte, error) { return s.Seal([]byte(v), aad) }

func (s *Sealer) OpenString(sealed []byte, aad string) (string, error) {
	b, err := s.Open(sealed, aad)
	return string(b), err
}

func open(aead cipher.AEAD, sealed []byte, aad string) ([]byte, error) {
	n := aead.NonceSize()
	if len(sealed) < 1+n+aead.Overhead() || sealed[0] != version {
		return nil, ErrDecrypt
	}
	return aead.Open(nil, sealed[1:1+n], sealed[1+n:], []byte(aad))
}
