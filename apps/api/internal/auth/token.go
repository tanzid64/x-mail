package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// tokenBytes is the entropy behind session cookies and one-time links.
// 32 bytes is far beyond guessable and keeps the URL-safe encoding compact.
const tokenBytes = 32

// NewToken returns a URL-safe secret together with the SHA-256 digest that
// should be stored. Only the digest ever reaches the database, so a dump of the
// sessions table cannot be replayed against the API.
//
// SHA-256 (rather than Argon2) is the right choice here: these tokens are
// full-entropy random values, not user-chosen secrets, so there is nothing for
// an attacker to brute force and no reason to pay a slow-hash cost on every
// authenticated request.
func NewToken() (token string, digest []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("auth: generate token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken returns the digest stored for a token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
