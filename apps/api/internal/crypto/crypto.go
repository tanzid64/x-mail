// Package crypto seals provider credentials at the application layer.
//
// Mail account API keys are the most sensitive rows in the database: anyone
// holding one can send mail as the user. They are therefore encrypted before
// they reach PostgreSQL, with a master key that lives outside the database
// (environment in development, a secrets manager in production), so a database
// dump alone is not enough to use them.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrInvalidKeyLength = errors.New("crypto: master key must be exactly 32 bytes")
	// ErrDecryptionFailed covers both a wrong key and tampered ciphertext —
	// GCM cannot tell them apart, and neither should the caller.
	ErrDecryptionFailed = errors.New("crypto: could not decrypt value")
)

// Sealer encrypts and decrypts short secrets with AES-256-GCM.
type Sealer struct {
	aead cipher.AEAD
}

func NewSealer(masterKey []byte) (*Sealer, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("%w, got %d", ErrInvalidKeyLength, len(masterKey))
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create gcm: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext. A fresh random nonce is generated per call and
// prefixed to the ciphertext, so encrypting the same key twice never produces
// the same bytes and identical credentials are not visible in a dump.
func (s *Sealer) Seal(plaintext string) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	return s.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open reverses Seal. Because GCM authenticates the ciphertext, a value that
// has been altered in the database fails here rather than decrypting to
// garbage.
func (s *Sealer) Open(ciphertext []byte) (string, error) {
	nonceSize := s.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", ErrDecryptionFailed
	}

	nonce, sealed := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := s.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}
	return string(plaintext), nil
}

// Hint returns the masked form of a secret that is safe to show in the UI and
// return from the API, e.g. "re_1234…wxyz". It never reveals enough to be
// usable, and deliberately keeps the prefix because that is how a user tells
// two of their own keys apart.
func Hint(secret string) string {
	const (
		prefixLen = 6
		suffixLen = 4
	)

	if len(secret) <= prefixLen+suffixLen {
		// Too short to mask meaningfully; show nothing rather than most of it.
		return strings.Repeat("•", len(secret))
	}
	return secret[:prefixLen] + "…" + secret[len(secret)-suffixLen:]
}
