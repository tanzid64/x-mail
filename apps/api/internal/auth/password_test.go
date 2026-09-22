package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	const password = "correct horse battery staple"

	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Fatalf("expected PHC argon2id format, got %q", encoded)
	}

	ok, err := VerifyPassword(password, encoded)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("correct password did not verify")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	encoded, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	for _, wrong := range []string{"hunter3", "", "HUNTER2", "hunter2 "} {
		ok, err := VerifyPassword(wrong, encoded)
		if err != nil {
			t.Fatalf("VerifyPassword(%q): %v", wrong, err)
		}
		if ok {
			t.Errorf("password %q should not have verified", wrong)
		}
	}
}

// Two hashes of the same password must differ, otherwise the salt is not doing
// its job and identical passwords would be visible in a database dump.
func TestHashPasswordUsesUniqueSalt(t *testing.T) {
	a, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Fatal("hashing the same password twice produced identical output")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"not phc":          "plaintext",
		"wrong algorithm":  "$argon2i$v=19$m=65536,t=3,p=2$c29tZXNhbHQ$c29tZWtleQ",
		"missing sections": "$argon2id$v=19$m=65536,t=3,p=2",
		"bad base64":       "$argon2id$v=19$m=65536,t=3,p=2$!!!!$!!!!",
	}

	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := VerifyPassword("whatever", encoded); err == nil {
				t.Fatal("expected an error for a malformed hash")
			}
		})
	}
}

func TestNeedsRehash(t *testing.T) {
	current, err := HashPassword("pw")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if NeedsRehash(current) {
		t.Error("a freshly created hash should not need rehashing")
	}

	// Same algorithm, but half the memory cost of the current policy.
	weak := "$argon2id$v=19$m=32768,t=3,p=2$c29tZXNhbHRzb21lc2E$" +
		"0123456789abcdef0123456789abcdef0123456789ab"
	if !NeedsRehash(weak) {
		t.Error("a hash below the current memory cost should need rehashing")
	}
	if !NeedsRehash("garbage") {
		t.Error("an unparseable hash should need rehashing")
	}
}
