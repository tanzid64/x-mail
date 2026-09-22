package accounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/crypto"
	"github.com/dark/x-mail/api/internal/providers"
	"github.com/dark/x-mail/api/internal/providers/resend"
)

// ErrInvalidCredentials means the provider rejected the supplied API key.
var ErrInvalidCredentials = errors.New("accounts: the provider rejected those credentials")

// ErrAddressNotOnDomain means the address does not belong to the domain it was
// filed under, which would produce mail the provider refuses to send.
var ErrAddressNotOnDomain = errors.New("accounts: address does not belong to that domain")

// ProviderFactory builds a transport client from a provider name and a
// decrypted credential. It is a field on Service so tests can substitute a fake
// without reaching the network.
type ProviderFactory func(provider providers.Provider, apiKey string) (providers.MailProvider, error)

// NewProviderFactory maps a stored provider name onto its implementation.
//
// resendBaseURL is normally empty, meaning the real Resend API. It is only set
// so an end-to-end test can run the whole stack against a stub provider, and
// config rejects it outright in production.
func NewProviderFactory(resendBaseURL string) ProviderFactory {
	return func(provider providers.Provider, apiKey string) (providers.MailProvider, error) {
		switch provider {
		case providers.ProviderResend:
			if resendBaseURL != "" {
				return resend.New(apiKey, resend.WithBaseURL(resendBaseURL)), nil
			}
			return resend.New(apiKey), nil
		default:
			return nil, fmt.Errorf("accounts: unsupported provider %q", provider)
		}
	}
}

// DefaultProviderFactory builds the production clients.
var DefaultProviderFactory = NewProviderFactory("")

type Service struct {
	repo        *Repository
	sealer      *crypto.Sealer
	newProvider ProviderFactory
	logger      *slog.Logger
}

func NewService(repo *Repository, sealer *crypto.Sealer, factory ProviderFactory, logger *slog.Logger) *Service {
	if factory == nil {
		factory = DefaultProviderFactory
	}
	return &Service{repo: repo, sealer: sealer, newProvider: factory, logger: logger}
}

type ConnectParams struct {
	Name     string
	Provider providers.Provider
	APIKey   string
	Color    string
}

// Connect verifies the credentials against the provider before storing them,
// so a typo in an API key is reported while the user is looking at the form
// rather than when their first message silently fails days later.
func (s *Service) Connect(ctx context.Context, userID uuid.UUID, params ConnectParams) (*MailAccount, error) {
	if !params.Provider.Valid() {
		return nil, fmt.Errorf("accounts: unsupported provider %q", params.Provider)
	}

	apiKey := strings.TrimSpace(params.APIKey)
	provider, err := s.newProvider(params.Provider, apiKey)
	if err != nil {
		return nil, err
	}
	if err := provider.VerifyCredentials(ctx); err != nil {
		if providers.IsKind(err, providers.KindAuth) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("accounts: could not reach %s: %w", params.Provider, err)
	}

	sealed, err := s.sealer.Seal(apiKey)
	if err != nil {
		return nil, err
	}

	account, err := s.repo.CreateAccount(ctx, CreateAccountParams{
		UserID:       userID,
		Name:         strings.TrimSpace(params.Name),
		Provider:     params.Provider,
		APIKeySealed: sealed,
		APIKeyHint:   crypto.Hint(apiKey),
		Color:        defaultColor(params.Color),
	})
	if err != nil {
		return nil, err
	}

	// Pulling the domains now means the account is immediately usable. A
	// failure here is not worth undoing a valid connection — the user can sync
	// again from the settings screen.
	if _, err := s.repo.SyncDomains(ctx, account.ID, mustDomains(ctx, provider, s.logger)); err != nil {
		s.logger.WarnContext(ctx, "initial domain sync failed", "error", err, "mail_account_id", account.ID)
	}
	return account, nil
}

func mustDomains(ctx context.Context, provider providers.MailProvider, logger *slog.Logger) []providers.Domain {
	domains, err := provider.ListDomains(ctx)
	if err != nil {
		logger.WarnContext(ctx, "could not list provider domains", "error", err)
		return nil
	}
	return domains
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]MailAccount, error) {
	return s.repo.ListAccounts(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, accountID uuid.UUID) (*MailAccount, error) {
	return s.repo.GetAccount(ctx, userID, accountID)
}

func (s *Service) Update(ctx context.Context, userID, accountID uuid.UUID, params UpdateAccountParams) (*MailAccount, error) {
	if params.Name != nil {
		trimmed := strings.TrimSpace(*params.Name)
		params.Name = &trimmed
	}
	return s.repo.UpdateAccount(ctx, userID, accountID, params)
}

func (s *Service) Delete(ctx context.Context, userID, accountID uuid.UUID) error {
	return s.repo.DeleteAccount(ctx, userID, accountID)
}

func (s *Service) SetDefault(ctx context.Context, userID, accountID uuid.UUID) error {
	return s.repo.SetDefault(ctx, userID, accountID)
}

// RotateAPIKey swaps in a new credential, again verifying it first.
func (s *Service) RotateAPIKey(ctx context.Context, userID, accountID uuid.UUID, apiKey string) (*MailAccount, error) {
	account, err := s.repo.GetAccount(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}

	apiKey = strings.TrimSpace(apiKey)
	provider, err := s.newProvider(account.Provider, apiKey)
	if err != nil {
		return nil, err
	}
	if err := provider.VerifyCredentials(ctx); err != nil {
		if providers.IsKind(err, providers.KindAuth) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("accounts: could not reach %s: %w", account.Provider, err)
	}

	sealed, err := s.sealer.Seal(apiKey)
	if err != nil {
		return nil, err
	}
	return s.repo.RotateAPIKey(ctx, userID, accountID, sealed, crypto.Hint(apiKey))
}

// SetWebhookSecret stores the signing secret used to verify this account's
// provider callbacks.
func (s *Service) SetWebhookSecret(ctx context.Context, userID, accountID uuid.UUID, secret string) (*MailAccount, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		// Clearing it falls the account back to the deployment-wide secret.
		return s.repo.SetWebhookSecret(ctx, userID, accountID, nil, "")
	}

	sealed, err := s.sealer.Seal(secret)
	if err != nil {
		return nil, err
	}
	return s.repo.SetWebhookSecret(ctx, userID, accountID, sealed, crypto.Hint(secret))
}

// CheckCredentials re-tests a stored credential and records the outcome, so the
// settings screen can show an account whose key has been revoked.
func (s *Service) CheckCredentials(ctx context.Context, userID, accountID uuid.UUID) (*MailAccount, error) {
	provider, err := s.ProviderFor(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}

	status, message := StatusActive, ""
	if err := provider.VerifyCredentials(ctx); err != nil {
		if providers.IsKind(err, providers.KindAuth) {
			status, message = StatusInvalidCredentials, "The provider rejected the stored API key."
		} else {
			status, message = StatusActive, "Could not reach the provider to check the key."
		}
	}
	if err := s.repo.MarkStatus(ctx, accountID, status, message); err != nil {
		return nil, err
	}
	return s.repo.GetAccount(ctx, userID, accountID)
}

// ProviderFor builds a transport client for an account the user owns. This is
// the only path from a request to a decrypted credential.
func (s *Service) ProviderFor(ctx context.Context, userID, accountID uuid.UUID) (providers.MailProvider, error) {
	account, err := s.repo.GetAccount(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	sealed, err := s.repo.SealedAPIKey(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	return s.buildProvider(account.Provider, sealed)
}

// ProviderForWorker is the background-job equivalent, for code that has already
// established ownership through the message it is processing.
func (s *Service) ProviderForWorker(ctx context.Context, accountID uuid.UUID) (providers.MailProvider, error) {
	provider, err := s.repo.AccountProvider(ctx, accountID)
	if err != nil {
		return nil, err
	}
	sealed, err := s.repo.SealedAPIKeyForWorker(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return s.buildProvider(provider, sealed)
}

func (s *Service) buildProvider(provider providers.Provider, sealed []byte) (providers.MailProvider, error) {
	apiKey, err := s.sealer.Open(sealed)
	if err != nil {
		// Usually means the master key changed; the credential must be re-entered.
		return nil, fmt.Errorf("accounts: stored credential could not be decrypted: %w", err)
	}
	return s.newProvider(provider, apiKey)
}

// ------------------------------------------------------------- domains ----

// SyncDomains refreshes an account's domains from the provider.
func (s *Service) SyncDomains(ctx context.Context, userID, accountID uuid.UUID) ([]Domain, error) {
	provider, err := s.ProviderFor(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}

	remote, err := provider.ListDomains(ctx)
	if err != nil {
		if providers.IsKind(err, providers.KindAuth) {
			_ = s.repo.MarkStatus(ctx, accountID, StatusInvalidCredentials,
				"The provider rejected the stored API key.")
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	return s.repo.SyncDomains(ctx, accountID, remote)
}

func (s *Service) ListDomains(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]Domain, error) {
	if accountID == nil {
		return s.repo.ListDomainsForUser(ctx, userID)
	}
	// Resolving the account first is what stops another user's id being passed
	// in as a filter.
	if _, err := s.repo.GetAccount(ctx, userID, *accountID); err != nil {
		return nil, err
	}
	return s.repo.ListDomains(ctx, *accountID)
}

// RefreshDomain re-reads one domain from the provider, which is how the
// verification screen picks up DNS that has just propagated.
func (s *Service) RefreshDomain(ctx context.Context, userID, domainID uuid.UUID) (*Domain, error) {
	domain, err := s.repo.GetDomain(ctx, userID, domainID)
	if err != nil {
		return nil, err
	}

	provider, err := s.ProviderFor(ctx, userID, domain.MailAccountID)
	if err != nil {
		return nil, err
	}
	remote, err := provider.GetDomain(ctx, domain.ProviderDomainID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.SyncDomains(ctx, domain.MailAccountID, []providers.Domain{*remote}); err != nil {
		return nil, err
	}
	return s.repo.GetDomain(ctx, userID, domainID)
}

func (s *Service) GetDomain(ctx context.Context, userID, domainID uuid.UUID) (*Domain, error) {
	return s.repo.GetDomain(ctx, userID, domainID)
}

// ----------------------------------------------------------- addresses ----

// CreateAddress adds a sender identity, rejecting an address whose domain part
// does not match the domain it is being filed under.
func (s *Service) CreateAddress(ctx context.Context, userID, domainID uuid.UUID, email, displayName string) (*MailAddress, error) {
	domain, err := s.repo.GetDomain(ctx, userID, domainID)
	if err != nil {
		return nil, err
	}

	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at < 0 || !strings.EqualFold(email[at+1:], domain.Domain) {
		return nil, ErrAddressNotOnDomain
	}

	return s.repo.CreateAddress(ctx, CreateAddressParams{
		DomainID:    domainID,
		Email:       email,
		DisplayName: strings.TrimSpace(displayName),
	})
}

func (s *Service) ListAddresses(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]MailAddress, error) {
	if accountID == nil {
		return s.repo.ListAddressesForUser(ctx, userID)
	}
	return s.repo.ListAddressesForAccount(ctx, userID, *accountID)
}

func (s *Service) GetAddress(ctx context.Context, userID, addressID uuid.UUID) (*MailAddress, error) {
	return s.repo.GetAddress(ctx, userID, addressID)
}

func (s *Service) UpdateAddress(ctx context.Context, userID, addressID uuid.UUID, params UpdateAddressParams) (*MailAddress, error) {
	if params.DisplayName != nil {
		trimmed := strings.TrimSpace(*params.DisplayName)
		params.DisplayName = &trimmed
	}
	return s.repo.UpdateAddress(ctx, userID, addressID, params)
}

func (s *Service) SetDefaultAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	return s.repo.SetDefaultAddress(ctx, userID, addressID)
}

func (s *Service) DeleteAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	return s.repo.DeleteAddress(ctx, userID, addressID)
}

// ResolveInbound maps an incoming recipient to the owning account.
func (s *Service) ResolveInbound(ctx context.Context, email string) (*Route, error) {
	return s.repo.ResolveInbound(ctx, strings.ToLower(strings.TrimSpace(email)))
}

// defaultColor keeps the switcher legible when the client does not pick one.
func defaultColor(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return "slate"
	}
	return color
}
