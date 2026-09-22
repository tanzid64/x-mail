// Package providers defines the boundary between the mail domain and whichever
// service actually carries the mail.
//
// Resend is the first implementation, but nothing above this package may depend
// on it: adding SES or SMTP later must mean writing one more implementation of
// MailProvider, not touching threads, messages or sending.
package providers

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Provider names the supported transports. It is stored on mail_accounts, so
// the values are part of the database contract.
type Provider string

const (
	ProviderResend Provider = "resend"
)

func (p Provider) Valid() bool {
	return p == ProviderResend
}

// Address is a single mail participant.
type Address struct {
	Email string
	Name  string
}

// String renders the address in RFC 5322 form, which is what providers expect.
func (a Address) String() string {
	if a.Name == "" {
		return a.Email
	}
	return fmt.Sprintf("%s <%s>", a.Name, a.Email)
}

// Attachment is a file carried with an outbound message.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
	// ContentID marks an attachment as inline, referenced by cid: in the HTML.
	ContentID string
}

// Email is a provider-agnostic outbound message.
type Email struct {
	From    Address
	To      []Address
	Cc      []Address
	Bcc     []Address
	ReplyTo []Address

	Subject string
	HTML    string
	Text    string

	// Headers carries the threading headers (In-Reply-To, References) that make
	// a reply attach to its conversation in the recipient's client.
	Headers map[string]string

	Attachments []Attachment

	// IdempotencyKey lets the provider collapse a duplicate submission caused by
	// a worker retry into a single delivery.
	IdempotencyKey string

	// ScheduledAt asks the provider to hold the message until this time.
	ScheduledAt *time.Time
}

// SendResult is what the provider returns once it accepts a message.
type SendResult struct {
	// ProviderMessageID is the provider's own id, used to correlate the
	// delivery, bounce and complaint webhooks that follow.
	ProviderMessageID string
	ScheduledAt       *time.Time
}

// DomainStatus is the verification state of a sending domain.
type DomainStatus string

const (
	DomainPending  DomainStatus = "pending"
	DomainVerified DomainStatus = "verified"
	DomainFailed   DomainStatus = "failed"
)

// DNSRecord is one record the user must publish to verify a domain.
type DNSRecord struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	TTL      string `json:"ttl,omitempty"`
	Priority *int   `json:"priority,omitempty"`
	Status   string `json:"status,omitempty"`
}

// Domain is a sending domain as the provider sees it.
type Domain struct {
	ID        string
	Name      string
	Status    DomainStatus
	Region    string
	Records   []DNSRecord
	CreatedAt *time.Time
}

// MailProvider is the transport contract. Implementations must be safe for
// concurrent use: one instance serves every job for a mail account.
type MailProvider interface {
	// Name identifies the implementation for logging and storage.
	Name() Provider

	// VerifyCredentials checks the configured credentials work, so a bad API
	// key is reported when the account is connected rather than when the user's
	// first message silently fails.
	VerifyCredentials(ctx context.Context) error

	// Send submits a message for delivery.
	Send(ctx context.Context, email Email) (*SendResult, error)

	// ListDomains returns the sending domains the credentials can use.
	ListDomains(ctx context.Context) ([]Domain, error)

	// GetDomain returns one domain, including the DNS records to publish.
	GetDomain(ctx context.Context, id string) (*Domain, error)
}

// ---------------------------------------------------------------- errors ----

// ErrorKind classifies a provider failure so callers can react without
// parsing messages or status codes from a specific vendor.
type ErrorKind string

const (
	// KindAuth means the credentials are rejected. Retrying cannot help.
	KindAuth ErrorKind = "auth"
	// KindInvalidRequest means the message itself is unacceptable — an
	// unverified sender, a malformed address. Retrying cannot help.
	KindInvalidRequest ErrorKind = "invalid_request"
	// KindRateLimited means the provider is throttling us. Retry later.
	KindRateLimited ErrorKind = "rate_limited"
	// KindTransient covers network failures and provider 5xx. Retry later.
	KindTransient ErrorKind = "transient"
	// KindNotFound means the referenced provider resource does not exist.
	KindNotFound ErrorKind = "not_found"
)

// Error is a provider failure carrying enough context for the worker to decide
// between retrying and failing the message permanently.
type Error struct {
	Provider Provider
	Kind     ErrorKind
	// Status is the HTTP status, when the failure came from an API response.
	Status  int
	Message string
	err     error
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %s: %s: %v", e.Provider, e.Kind, e.Message, e.err)
	}
	return fmt.Sprintf("%s: %s: %s", e.Provider, e.Kind, e.Message)
}

func (e *Error) Unwrap() error { return e.err }

// Retryable reports whether trying the same operation again could succeed.
func (e *Error) Retryable() bool {
	return e.Kind == KindRateLimited || e.Kind == KindTransient
}

// IsRetryable reports whether err is a provider failure worth retrying.
// Anything that is not a provider error is treated as retryable, because an
// unclassified failure is more likely a transient fault than a permanent one.
func IsRetryable(err error) bool {
	var provErr *Error
	if errors.As(err, &provErr) {
		return provErr.Retryable()
	}
	return err != nil
}

// IsKind reports whether err is a provider failure of the given kind.
func IsKind(err error, kind ErrorKind) bool {
	var provErr *Error
	return errors.As(err, &provErr) && provErr.Kind == kind
}

func NewError(provider Provider, kind ErrorKind, status int, message string, cause error) *Error {
	return &Error{Provider: provider, Kind: kind, Status: status, Message: message, err: cause}
}
