package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestSealOpenRoundTrip(t *testing.T) {
	sealer, err := NewSealer(newKey(t))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	const secret = "re_123456789_abcdefghijklmnopqrstuvwxyz"

	sealed, err := sealer.Seal(secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, []byte(secret)) {
		t.Fatal("ciphertext contains the plaintext")
	}

	got, err := sealer.Open(sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != secret {
		t.Errorf("Open = %q, want %q", got, secret)
	}
}

// Sealing the same secret twice must produce different bytes, otherwise equal
// credentials would be identifiable in a database dump.
func TestSealUsesFreshNonce(t *testing.T) {
	sealer, err := NewSealer(newKey(t))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	a, err := sealer.Seal("same-secret")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	b, err := sealer.Seal("same-secret")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("sealing the same value twice produced identical ciphertext")
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	sealer, err := NewSealer(newKey(t))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	sealed, err := sealer.Seal("re_a_real_looking_key")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 0xff

	if _, err := sealer.Open(tampered); !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("Open(tampered) error = %v, want ErrDecryptionFailed", err)
	}
	if _, err := sealer.Open([]byte("too-short")); !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("Open(short) error = %v, want ErrDecryptionFailed", err)
	}
}

func TestOpenRejectsWrongKey(t *testing.T) {
	first, err := NewSealer(newKey(t))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	second, err := NewSealer(newKey(t))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	sealed, err := first.Seal("re_secret")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := second.Open(sealed); !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("Open with the wrong key = %v, want ErrDecryptionFailed", err)
	}
}

func TestNewSealerRejectsWrongKeyLength(t *testing.T) {
	for _, size := range []int{0, 16, 31, 33, 64} {
		if _, err := NewSealer(make([]byte, size)); !errors.Is(err, ErrInvalidKeyLength) {
			t.Errorf("NewSealer(%d bytes) error = %v, want ErrInvalidKeyLength", size, err)
		}
	}
}

func TestHintMasksTheSecret(t *testing.T) {
	const secret = "re_123456789_abcdefghijklmnopqrstuvwxyz"

	hint := Hint(secret)
	if hint != "re_123…wxyz" {
		t.Errorf("Hint = %q, want %q", hint, "re_123…wxyz")
	}

	// A short value must not leak most of itself through the prefix rule.
	if got := Hint("short"); got != "•••••" {
		t.Errorf("Hint(short) = %q, want fully masked", got)
	}
}
