// Package webhooks verifies and records provider callbacks.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMissingHeaders  = errors.New("webhooks: signature headers are missing")
	ErrInvalidSecret   = errors.New("webhooks: signing secret is malformed")
	ErrTimestampStale  = errors.New("webhooks: timestamp is outside the accepted window")
	ErrSignatureFailed = errors.New("webhooks: signature does not match")
)

// tolerance is how far a webhook's timestamp may be from now.
//
// Rejecting old timestamps is what stops a captured request being replayed
// indefinitely; five minutes is Svix's own recommendation and leaves room for
// ordinary clock drift.
const tolerance = 5 * time.Minute

// SvixVerifier checks the signature Resend (and anything else built on Svix)
// puts on its webhooks.
//
// The scheme signs "{id}.{timestamp}.{body}" with HMAC-SHA256 under a shared
// secret. Verifying it is what makes the endpoint safe to expose: without it,
// anyone who knows the URL could inject mail into a user's inbox.
type SvixVerifier struct {
	secret []byte
	now    func() time.Time
}

// NewSvixVerifier decodes a "whsec_"-prefixed base64 secret.
func NewSvixVerifier(secret string) (*SvixVerifier, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil, ErrInvalidSecret
	}
	trimmed = strings.TrimPrefix(trimmed, "whsec_")

	raw, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		// Some deployments configure a raw secret rather than a base64 one.
		raw = []byte(trimmed)
	}
	if len(raw) == 0 {
		return nil, ErrInvalidSecret
	}
	return &SvixVerifier{secret: raw, now: time.Now}, nil
}

// Verify checks the signature headers against the exact bytes received.
//
// It must be given the raw body: re-encoding a decoded payload would change
// whitespace or key order and the signature would never match.
func (v *SvixVerifier) Verify(headers http.Header, body []byte) error {
	id := headers.Get("webhook-id")
	timestamp := headers.Get("webhook-timestamp")
	signature := headers.Get("webhook-signature")

	// Svix sends both the standard headers and its own prefixed ones.
	if id == "" {
		id = headers.Get("svix-id")
	}
	if timestamp == "" {
		timestamp = headers.Get("svix-timestamp")
	}
	if signature == "" {
		signature = headers.Get("svix-signature")
	}

	if id == "" || timestamp == "" || signature == "" {
		return ErrMissingHeaders
	}

	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: unparseable timestamp", ErrTimestampStale)
	}
	sent := time.Unix(seconds, 0)
	drift := v.now().Sub(sent)
	if drift > tolerance || drift < -tolerance {
		return ErrTimestampStale
	}

	signed := fmt.Sprintf("%s.%s.%s", id, timestamp, body)
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(signed))
	expected := mac.Sum(nil)

	// The header carries a space-separated list, each "v1,<base64>", because a
	// secret being rotated means two valid signatures for a while.
	for _, candidate := range strings.Split(signature, " ") {
		_, encoded, found := strings.Cut(candidate, ",")
		if !found {
			continue
		}
		provided, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		if hmac.Equal(provided, expected) {
			return nil
		}
	}
	return ErrSignatureFailed
}
