package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. These are the interactive-login settings: 64 MiB of
// memory makes GPU cracking expensive while keeping a login around 50ms on
// commodity hardware. Changing them does not invalidate existing hashes —
// every hash encodes the parameters it was created with, and NeedsRehash
// reports when a stored hash is weaker than the current policy.
const (
	argonMemory      uint32 = 64 * 1024
	argonTime        uint32 = 3
	argonParallelism uint8  = 2
	argonSaltLength  uint32 = 16
	argonKeyLength   uint32 = 32
)

var (
	// ErrInvalidHash means the stored value is not a hash this package produced.
	ErrInvalidHash = errors.New("auth: password hash is malformed")
	// ErrIncompatibleVersion means the hash came from a different argon2 version.
	ErrIncompatibleVersion = errors.New("auth: incompatible argon2 version")
)

type argonParams struct {
	memory      uint32
	time        uint32
	parallelism uint8
	keyLength   uint32
}

// HashPassword derives an Argon2id hash with a fresh random salt and returns it
// in the standard PHC string format, which carries the parameters and salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonParallelism, argonKeyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory, argonTime, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches encoded. Comparison is
// constant time so that a timing signal cannot reveal how much of a guess was
// correct.
func VerifyPassword(password, encoded string) (bool, error) {
	params, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}

	got := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.parallelism, params.keyLength)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NeedsRehash reports whether a stored hash was created with weaker parameters
// than the current policy, so it can be upgraded on the user's next successful
// login.
func NeedsRehash(encoded string) bool {
	params, _, _, err := decodeHash(encoded)
	if err != nil {
		return true
	}
	return params.memory < argonMemory ||
		params.time < argonTime ||
		params.parallelism != argonParallelism ||
		params.keyLength != argonKeyLength
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	var p argonParams

	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return p, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return p, nil, nil, ErrIncompatibleVersion
	}

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.parallelism); err != nil {
		return p, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return p, nil, nil, ErrInvalidHash
	}
	p.keyLength = uint32(len(key))

	return p, salt, key, nil
}
