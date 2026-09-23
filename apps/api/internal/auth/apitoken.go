package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TokenPrefix marks a value as an x-mail API token. It exists so a leaked key
// is recognisable on sight — by its owner, and by the secret scanners that
// watch public repositories for exactly this kind of string.
const TokenPrefix = "xm_"

var (
	ErrAPITokenNotFound = errors.New("auth: api token not found")
	ErrAPITokenExpired  = errors.New("auth: api token has expired")
	ErrAPITokenRevoked  = errors.New("auth: api token was revoked")
	ErrBadLifetime      = errors.New("auth: unsupported token lifetime")
)

// APIToken is a credential a third party uses in place of a browser session.
//
// The secret itself exists exactly once, in the response that creates it. What
// is stored is its digest, so a dump of this table cannot be replayed.
type APIToken struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"user_id"`
	// MailAccountID nil means the token reaches every account the user owns.
	MailAccountID *uuid.UUID `json:"mail_account_id"`
	Name          string     `json:"name"`
	Hint          string     `json:"hint"`
	// ExpiresAt nil means the token never expires.
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Lifetime is how long a new token lasts. These four are the whole set: an
// arbitrary duration would invite tokens that outlive the integration they were
// made for without anyone choosing that.
type Lifetime string

const (
	Lifetime15Days Lifetime = "15d"
	Lifetime30Days Lifetime = "30d"
	Lifetime1Year  Lifetime = "1y"
	LifetimeNever  Lifetime = "never"
)

// Lifetimes is the set offered by the API, in the order the UI shows them.
var Lifetimes = []Lifetime{Lifetime15Days, Lifetime30Days, Lifetime1Year, LifetimeNever}

// ExpiresAt turns a lifetime into an absolute instant, or nil for "never".
//
// Calendar arithmetic rather than a fixed number of hours: "30 days" should
// mean the same date next month regardless of a daylight-saving change in
// between, which is what the person picking it expects.
func (l Lifetime) ExpiresAt(now time.Time) (*time.Time, error) {
	var at time.Time
	switch l {
	case Lifetime15Days:
		at = now.AddDate(0, 0, 15)
	case Lifetime30Days:
		at = now.AddDate(0, 0, 30)
	case Lifetime1Year:
		at = now.AddDate(1, 0, 0)
	case LifetimeNever:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrBadLifetime, string(l))
	}
	return &at, nil
}

// Valid reports whether a lifetime is one of the four offered.
func (l Lifetime) Valid() bool {
	_, err := l.ExpiresAt(time.Now())
	return err == nil
}

// NewAPIToken returns the secret to show the user once, and the digest to store.
func NewAPIToken() (token string, digest []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("auth: generate api token: %w", err)
	}
	token = TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// TokenHint is the masked form kept for the list.
func TokenHint(token string) string {
	const (
		prefixLen = len(TokenPrefix) + 4
		suffixLen = 4
	)
	if len(token) <= prefixLen+suffixLen {
		return strings.Repeat("•", len(token))
	}
	return token[:prefixLen] + "…" + token[len(token)-suffixLen:]
}

// IsExpired reports whether the token may no longer be used.
func (t *APIToken) IsExpired(now time.Time) bool {
	return t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)
}

// BearerToken extracts an API token from the Authorization header.
//
// Only the Bearer scheme is accepted. Taking the key from a query string would
// leak it into access logs, proxy logs and browser history.
func BearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}
