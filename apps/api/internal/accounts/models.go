package accounts

import (
	"time"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/providers"
)

// Account status values. These live in a CHECK constraint, so they are part of
// the database contract.
const (
	StatusActive             = "active"
	StatusInvalidCredentials = "invalid_credentials"
	StatusDisabled           = "disabled"
)

// MailAccount is one provider connection owned by a user.
//
// The encrypted API key is deliberately absent: this struct is serialised
// straight to the client, and the only credential-shaped thing it may carry is
// the masked hint.
type MailAccount struct {
	ID                uuid.UUID          `json:"id"`
	UserID            uuid.UUID          `json:"-"`
	Name              string             `json:"name"`
	Provider          providers.Provider `json:"provider"`
	ProviderAccountID *string            `json:"provider_account_id"`
	APIKeyHint        string             `json:"api_key_hint"`
	Status            string             `json:"status"`
	StatusMessage     string             `json:"status_message"`
	LastCheckedAt     *time.Time         `json:"last_checked_at"`
	Color             string             `json:"color"`
	IsDefault         bool               `json:"is_default"`
	IsActive          bool               `json:"is_active"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// Domain is a sending domain belonging to a mail account.
type Domain struct {
	ID               uuid.UUID             `json:"id"`
	MailAccountID    uuid.UUID             `json:"mail_account_id"`
	Domain           string                `json:"domain"`
	ProviderDomainID string                `json:"provider_domain_id"`
	Status           string                `json:"status"`
	Region           string                `json:"region"`
	Records          []providers.DNSRecord `json:"records"`
	LastSyncedAt     *time.Time            `json:"last_synced_at"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

// MailAddress is a sender identity on a domain.
type MailAddress struct {
	ID          uuid.UUID `json:"id"`
	DomainID    uuid.UUID `json:"domain_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	IsDefault   bool      `json:"is_default"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Denormalised for the compose "From" picker, which needs to group
	// addresses by domain and account without a second request.
	DomainName    string    `json:"domain"`
	MailAccountID uuid.UUID `json:"mail_account_id"`
}

// Route is the result of resolving an inbound recipient address back to the
// account that owns it (docs/PLAN.md §22).
type Route struct {
	AddressID     uuid.UUID
	Email         string
	DomainID      uuid.UUID
	MailAccountID uuid.UUID
	UserID        uuid.UUID
	AccountActive bool
}
