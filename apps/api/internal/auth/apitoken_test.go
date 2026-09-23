package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dark/x-mail/api/internal/auth"
)

func TestTheFourLifetimesAreTheOnlyOnes(t *testing.T) {
	// A fixed instant, so the expected dates are readable rather than derived
	// with the same arithmetic the code under test uses.
	now := time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		lifetime auth.Lifetime
		want     *time.Time
	}{
		{auth.Lifetime15Days, timePtr(time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC))},
		{auth.Lifetime30Days, timePtr(time.Date(2026, time.February, 9, 12, 0, 0, 0, time.UTC))},
		{auth.Lifetime1Year, timePtr(time.Date(2027, time.January, 10, 12, 0, 0, 0, time.UTC))},
		{auth.LifetimeNever, nil},
	}

	for _, tc := range cases {
		t.Run(string(tc.lifetime), func(t *testing.T) {
			got, err := tc.lifetime.ExpiresAt(now)
			if err != nil {
				t.Fatalf("ExpiresAt: %v", err)
			}
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("%q produced an expiry of %v, want never", tc.lifetime, *got)
			case tc.want != nil && got == nil:
				t.Fatalf("%q never expires, want %v", tc.lifetime, *tc.want)
			case tc.want != nil && !got.Equal(*tc.want):
				t.Errorf("%q expires %v, want %v", tc.lifetime, *got, *tc.want)
			}
		})
	}

	if len(auth.Lifetimes) != len(cases) {
		t.Errorf("auth.Lifetimes has %d entries, but %d are covered here", len(auth.Lifetimes), len(cases))
	}
}

func TestAnUnknownLifetimeIsRefused(t *testing.T) {
	for _, bad := range []auth.Lifetime{"", "7d", "1d", "forever", "90d", "1y "} {
		if _, err := bad.ExpiresAt(time.Now()); !errors.Is(err, auth.ErrBadLifetime) {
			t.Errorf("ExpiresAt(%q) = %v, want ErrBadLifetime", bad, err)
		}
		if bad.Valid() {
			t.Errorf("%q reports itself valid", bad)
		}
	}
}

func TestATokenIsPrefixedAndHighEntropy(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		token, digest, err := auth.NewAPIToken()
		if err != nil {
			t.Fatalf("NewAPIToken: %v", err)
		}
		if !strings.HasPrefix(token, auth.TokenPrefix) {
			t.Fatalf("token %q lacks the %q prefix", token, auth.TokenPrefix)
		}
		if seen[token] {
			t.Fatalf("NewAPIToken produced a duplicate: %q", token)
		}
		seen[token] = true

		// The digest is what gets stored, so it must match a fresh hash of the
		// secret and must not contain the secret itself.
		if string(digest) != string(auth.HashToken(token)) {
			t.Fatal("stored digest does not match HashToken of the secret")
		}
		if strings.Contains(string(digest), token) {
			t.Fatal("digest contains the plaintext token")
		}
	}
}

func TestTokenHintRevealsAlmostNothing(t *testing.T) {
	token, _, err := auth.NewAPIToken()
	if err != nil {
		t.Fatalf("NewAPIToken: %v", err)
	}
	hint := auth.TokenHint(token)

	if strings.Contains(token, hint) {
		t.Fatalf("hint %q is a literal substring of the token", hint)
	}
	if len(hint) > len(auth.TokenPrefix)+12 {
		t.Errorf("hint %q shows too much of the token", hint)
	}
	if !strings.HasPrefix(hint, auth.TokenPrefix) {
		t.Errorf("hint %q should keep the prefix so it is recognisable", hint)
	}
}

func TestBearerTokenParsing(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"Bearer xm_abc", "xm_abc"},
		{"bearer xm_abc", "xm_abc"},   // scheme is case-insensitive per RFC 7235
		{"Bearer  xm_abc ", "xm_abc"}, // surrounding space is not part of the value
		{"", ""},
		{"xm_abc", ""},       // no scheme
		{"Basic xm_abc", ""}, // wrong scheme
		{"Bearer", ""},       // no value
		{"Bearer   ", ""},    // blank value
	}

	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			r.Header.Set("Authorization", tc.header)
		}
		got, ok := auth.BearerToken(r)
		if tc.want == "" {
			if ok {
				t.Errorf("BearerToken(%q) = %q, want no token", tc.header, got)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Errorf("BearerToken(%q) = %q/%v, want %q", tc.header, got, ok, tc.want)
		}
	}
}

func TestExpiryIsAnInstantNotALabel(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Second)
	future := now.Add(time.Hour)

	if (&auth.APIToken{ExpiresAt: nil}).IsExpired(now) {
		t.Error("a token that never expires reported itself expired")
	}
	if !(&auth.APIToken{ExpiresAt: &past}).IsExpired(now) {
		t.Error("a token past its expiry reported itself live")
	}
	if (&auth.APIToken{ExpiresAt: &future}).IsExpired(now) {
		t.Error("a token still within its expiry reported itself expired")
	}
	// The boundary belongs to the past: at exactly the expiry, it is over.
	if !(&auth.APIToken{ExpiresAt: &now}).IsExpired(now) {
		t.Error("a token exactly at its expiry should be expired")
	}
}

func timePtr(t time.Time) *time.Time { return &t }
