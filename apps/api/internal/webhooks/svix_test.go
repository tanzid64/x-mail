package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"
)

const testSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// sign produces the headers a genuine Svix webhook would carry.
func sign(t *testing.T, secret, id string, sentAt time.Time, body []byte) http.Header {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(secret[len("whsec_"):])
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}

	timestamp := strconv.FormatInt(sentAt.Unix(), 10)
	mac := hmac.New(sha256.New, raw)
	mac.Write([]byte(fmt.Sprintf("%s.%s.%s", id, timestamp, body)))

	headers := http.Header{}
	headers.Set("svix-id", id)
	headers.Set("svix-timestamp", timestamp)
	headers.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return headers
}

func TestVerifyAcceptsAGenuineSignature(t *testing.T) {
	verifier, err := NewSvixVerifier(testSecret)
	if err != nil {
		t.Fatalf("NewSvixVerifier: %v", err)
	}

	body := []byte(`{"type":"email.received","data":{"to":["support@company.com"]}}`)
	headers := sign(t, testSecret, "msg_123", time.Now(), body)

	if err := verifier.Verify(headers, body); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

// The signature covers the exact bytes, so any change to the body invalidates
// it — this is what stops a forged or tampered webhook injecting mail.
func TestVerifyRejectsATamperedBody(t *testing.T) {
	verifier, _ := NewSvixVerifier(testSecret)

	body := []byte(`{"type":"email.received","data":{"to":["support@company.com"]}}`)
	headers := sign(t, testSecret, "msg_123", time.Now(), body)

	tampered := []byte(`{"type":"email.received","data":{"to":["attacker@evil.com"]}}`)
	if err := verifier.Verify(headers, tampered); !errors.Is(err, ErrSignatureFailed) {
		t.Errorf("err = %v, want ErrSignatureFailed", err)
	}
}

func TestVerifyRejectsTheWrongSecret(t *testing.T) {
	verifier, _ := NewSvixVerifier("whsec_" + base64.StdEncoding.EncodeToString([]byte("a different secret")))

	body := []byte(`{"type":"email.received"}`)
	headers := sign(t, testSecret, "msg_123", time.Now(), body)

	if err := verifier.Verify(headers, body); !errors.Is(err, ErrSignatureFailed) {
		t.Errorf("err = %v, want ErrSignatureFailed", err)
	}
}

// A captured request must not stay valid forever.
func TestVerifyRejectsStaleTimestamps(t *testing.T) {
	verifier, _ := NewSvixVerifier(testSecret)
	body := []byte(`{"type":"email.received"}`)

	for name, sentAt := range map[string]time.Time{
		"too old":       time.Now().Add(-10 * time.Minute),
		"too far ahead": time.Now().Add(10 * time.Minute),
	} {
		t.Run(name, func(t *testing.T) {
			headers := sign(t, testSecret, "msg_123", sentAt, body)
			if err := verifier.Verify(headers, body); !errors.Is(err, ErrTimestampStale) {
				t.Errorf("err = %v, want ErrTimestampStale", err)
			}
		})
	}

	// Ordinary clock drift is tolerated.
	headers := sign(t, testSecret, "msg_123", time.Now().Add(-2*time.Minute), body)
	if err := verifier.Verify(headers, body); err != nil {
		t.Errorf("a two-minute-old webhook was rejected: %v", err)
	}
}

func TestVerifyRequiresAllHeaders(t *testing.T) {
	verifier, _ := NewSvixVerifier(testSecret)
	body := []byte(`{}`)
	complete := sign(t, testSecret, "msg_123", time.Now(), body)

	for _, missing := range []string{"svix-id", "svix-timestamp", "svix-signature"} {
		headers := complete.Clone()
		headers.Del(missing)
		if err := verifier.Verify(headers, body); !errors.Is(err, ErrMissingHeaders) {
			t.Errorf("without %s: err = %v, want ErrMissingHeaders", missing, err)
		}
	}

	if err := verifier.Verify(http.Header{}, body); !errors.Is(err, ErrMissingHeaders) {
		t.Errorf("with no headers: err = %v, want ErrMissingHeaders", err)
	}
}

// A secret being rotated means two signatures are valid at once.
func TestVerifyAcceptsAnyOfSeveralSignatures(t *testing.T) {
	verifier, _ := NewSvixVerifier(testSecret)

	body := []byte(`{"type":"email.received"}`)
	headers := sign(t, testSecret, "msg_123", time.Now(), body)
	valid := headers.Get("svix-signature")

	headers.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString([]byte("not it"))+" "+valid)
	if err := verifier.Verify(headers, body); err != nil {
		t.Errorf("a valid signature alongside an old one was rejected: %v", err)
	}
}

func TestVerifyAcceptsStandardWebhookHeaders(t *testing.T) {
	verifier, _ := NewSvixVerifier(testSecret)

	body := []byte(`{"type":"email.received"}`)
	svixHeaders := sign(t, testSecret, "msg_123", time.Now(), body)

	standard := http.Header{}
	standard.Set("webhook-id", svixHeaders.Get("svix-id"))
	standard.Set("webhook-timestamp", svixHeaders.Get("svix-timestamp"))
	standard.Set("webhook-signature", svixHeaders.Get("svix-signature"))

	if err := verifier.Verify(standard, body); err != nil {
		t.Errorf("standard webhook headers were rejected: %v", err)
	}
}
