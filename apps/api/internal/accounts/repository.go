package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dark/x-mail/api/internal/providers"
)

var (
	ErrNotFound     = errors.New("accounts: not found")
	ErrNameTaken    = errors.New("accounts: an account with that name already exists")
	ErrAddressTaken = errors.New("accounts: that email address is already in use")
	ErrDomainExists = errors.New("accounts: that domain is already connected")
)

const uniqueViolation = "23505"

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const accountColumns = `id, user_id, name, provider, provider_account_id, api_key_hint,
	webhook_secret_hint, status, status_message, last_checked_at, color, is_default, is_active,
	created_at, updated_at`

func scanAccount(row pgx.Row) (*MailAccount, error) {
	var a MailAccount
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &a.Provider, &a.ProviderAccountID, &a.APIKeyHint,
		&a.WebhookSecretHint, &a.Status, &a.StatusMessage, &a.LastCheckedAt, &a.Color,
		&a.IsDefault, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// ------------------------------------------------------------- accounts ----

type CreateAccountParams struct {
	UserID       uuid.UUID
	Name         string
	Provider     providers.Provider
	APIKeySealed []byte
	APIKeyHint   string
	Color        string
}

// CreateAccount inserts an account, making it the default when it is the
// user's first. The check and the insert share a transaction so two concurrent
// first-account requests cannot both claim the default.
func (r *Repository) CreateAccount(ctx context.Context, params CreateAccountParams) (*MailAccount, error) {
	var account *MailAccount

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var existing int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM mail_accounts WHERE user_id = $1`, params.UserID).Scan(&existing); err != nil {
			return err
		}

		var err error
		account, err = scanAccount(tx.QueryRow(ctx,
			`INSERT INTO mail_accounts (user_id, name, provider, api_key_encrypted, api_key_hint, color, is_default)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING `+accountColumns,
			params.UserID, params.Name, params.Provider, params.APIKeySealed, params.APIKeyHint,
			params.Color, existing == 0))
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrNameTaken
		}
		return nil, fmt.Errorf("create mail account: %w", err)
	}
	return account, nil
}

// ListAccounts returns every account owned by the user, defaults first.
func (r *Repository) ListAccounts(ctx context.Context, userID uuid.UUID) ([]MailAccount, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+accountColumns+` FROM mail_accounts
		  WHERE user_id = $1
		  ORDER BY is_default DESC, lower(name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]MailAccount, 0)
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *account)
	}
	return accounts, rows.Err()
}

// AccountIDs returns every account the user owns. It is the basis of mailbox
// scoping: a unified inbox reads this set, and a single-account view is one
// element of it. Callers never take account ids from the client.
func (r *Repository) AccountIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id FROM mail_accounts WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetAccount loads one account **owned by userID**.
//
// The ownership predicate lives in the SQL rather than in a separate check, so
// there is no way to call this and forget to authorize: a request for another
// user's account is indistinguishable from one that does not exist.
func (r *Repository) GetAccount(ctx context.Context, userID, accountID uuid.UUID) (*MailAccount, error) {
	return scanAccount(r.db.QueryRow(ctx,
		`SELECT `+accountColumns+` FROM mail_accounts WHERE id = $1 AND user_id = $2`,
		accountID, userID))
}

// SealedAPIKey returns the encrypted credential for an account the user owns.
// It is separate from GetAccount so that the ciphertext is only ever loaded by
// the code that is about to call a provider.
func (r *Repository) SealedAPIKey(ctx context.Context, userID, accountID uuid.UUID) ([]byte, error) {
	var sealed []byte
	err := r.db.QueryRow(ctx,
		`SELECT api_key_encrypted FROM mail_accounts WHERE id = $1 AND user_id = $2`,
		accountID, userID).Scan(&sealed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return sealed, nil
}

// AccountProvider returns which transport an account uses, without loading the
// rest of the row. Used by background jobs that already hold an account id.
func (r *Repository) AccountProvider(ctx context.Context, accountID uuid.UUID) (providers.Provider, error) {
	var provider providers.Provider
	err := r.db.QueryRow(ctx, `SELECT provider FROM mail_accounts WHERE id = $1`, accountID).Scan(&provider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return provider, nil
}

// SealedAPIKeyForWorker loads a credential without a user in hand. Only
// background jobs may use it: they have already resolved ownership through the
// message they are processing.
func (r *Repository) SealedAPIKeyForWorker(ctx context.Context, accountID uuid.UUID) ([]byte, error) {
	var sealed []byte
	err := r.db.QueryRow(ctx,
		`SELECT api_key_encrypted FROM mail_accounts WHERE id = $1`, accountID).Scan(&sealed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return sealed, nil
}

type UpdateAccountParams struct {
	Name     *string
	Color    *string
	IsActive *bool
}

func (r *Repository) UpdateAccount(ctx context.Context, userID, accountID uuid.UUID, params UpdateAccountParams) (*MailAccount, error) {
	account, err := scanAccount(r.db.QueryRow(ctx,
		`UPDATE mail_accounts SET
		     name      = COALESCE($3, name),
		     color     = COALESCE($4, color),
		     is_active = COALESCE($5, is_active)
		  WHERE id = $1 AND user_id = $2
		  RETURNING `+accountColumns,
		accountID, userID, params.Name, params.Color, params.IsActive))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	return account, nil
}

// RotateAPIKey replaces the stored credential and clears any previous
// credential failure.
func (r *Repository) RotateAPIKey(ctx context.Context, userID, accountID uuid.UUID, sealed []byte, hint string) (*MailAccount, error) {
	return scanAccount(r.db.QueryRow(ctx,
		`UPDATE mail_accounts
		    SET api_key_encrypted = $3, api_key_hint = $4,
		        status = '`+StatusActive+`', status_message = '', last_checked_at = now()
		  WHERE id = $1 AND user_id = $2
		  RETURNING `+accountColumns,
		accountID, userID, sealed, hint))
}

// SetWebhookSecret stores the signing secret for an account's provider
// callbacks, encrypted with the same master key as the API key — a webhook
// secret is equally capable of letting an attacker inject mail.
func (r *Repository) SetWebhookSecret(ctx context.Context, userID, accountID uuid.UUID, sealed []byte, hint string) (*MailAccount, error) {
	return scanAccount(r.db.QueryRow(ctx,
		`UPDATE mail_accounts SET webhook_secret_encrypted = $3, webhook_secret_hint = $4
		  WHERE id = $1 AND user_id = $2
		  RETURNING `+accountColumns,
		accountID, userID, sealed, hint))
}

// MarkStatus records the outcome of a credential check. It takes no user id
// because background health checks run without a request context.
func (r *Repository) MarkStatus(ctx context.Context, accountID uuid.UUID, status, message string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE mail_accounts SET status = $2, status_message = $3, last_checked_at = now() WHERE id = $1`,
		accountID, status, message)
	return err
}

// SetDefault moves the default flag to one account, clearing the previous one
// in the same transaction so the partial unique index is never violated.
func (r *Repository) SetDefault(ctx context.Context, userID, accountID uuid.UUID) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		var owned bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM mail_accounts WHERE id = $1 AND user_id = $2`,
			accountID, userID).Scan(&owned); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE mail_accounts SET is_default = false WHERE user_id = $1 AND is_default`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE mail_accounts SET is_default = true WHERE id = $1 AND user_id = $2`, accountID, userID)
		return err
	})
}

// DeleteAccount removes an account and, through cascades, its domains,
// addresses and mail. If it was the default, the oldest remaining account
// takes over so the user is never left without one.
func (r *Repository) DeleteAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		var wasDefault bool
		err := tx.QueryRow(ctx,
			`DELETE FROM mail_accounts WHERE id = $1 AND user_id = $2 RETURNING is_default`,
			accountID, userID).Scan(&wasDefault)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if !wasDefault {
			return nil
		}

		_, err = tx.Exec(ctx,
			`UPDATE mail_accounts SET is_default = true
			  WHERE id = (SELECT id FROM mail_accounts WHERE user_id = $1 ORDER BY created_at LIMIT 1)`,
			userID)
		return err
	})
}

func (r *Repository) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// -------------------------------------------------------------- domains ----

const domainColumns = `id, mail_account_id, domain, provider_domain_id, status, region,
	records, last_synced_at, created_at, updated_at`

func scanDomain(row pgx.Row) (*Domain, error) {
	var d Domain
	var records []byte
	err := row.Scan(&d.ID, &d.MailAccountID, &d.Domain, &d.ProviderDomainID, &d.Status, &d.Region,
		&records, &d.LastSyncedAt, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if len(records) > 0 {
		if err := json.Unmarshal(records, &d.Records); err != nil {
			return nil, fmt.Errorf("decode dns records: %w", err)
		}
	}
	if d.Records == nil {
		d.Records = []providers.DNSRecord{}
	}
	return &d, nil
}

// SyncDomains reconciles the stored domains for an account with what the
// provider reports. Domains the provider no longer knows about are removed, so
// the local view cannot drift into referencing a domain that is gone.
func (r *Repository) SyncDomains(ctx context.Context, accountID uuid.UUID, remote []providers.Domain) ([]Domain, error) {
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		seen := make([]string, 0, len(remote))

		for _, d := range remote {
			records, err := json.Marshal(d.Records)
			if err != nil {
				return fmt.Errorf("encode dns records: %w", err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO domains (mail_account_id, domain, provider_domain_id, status, region, records, last_synced_at)
				 VALUES ($1, $2, $3, $4, $5, $6, now())
				 ON CONFLICT (mail_account_id, provider_domain_id) DO UPDATE
				    SET domain = EXCLUDED.domain,
				        status = EXCLUDED.status,
				        region = EXCLUDED.region,
				        records = EXCLUDED.records,
				        last_synced_at = now()`,
				accountID, d.Name, d.ID, string(d.Status), d.Region, records); err != nil {
				return err
			}
			seen = append(seen, d.ID)
		}

		_, err := tx.Exec(ctx,
			`DELETE FROM domains WHERE mail_account_id = $1 AND NOT (provider_domain_id = ANY($2))`,
			accountID, seen)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.ListDomains(ctx, accountID)
}

func (r *Repository) ListDomains(ctx context.Context, accountID uuid.UUID) ([]Domain, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+domainColumns+` FROM domains WHERE mail_account_id = $1 ORDER BY domain`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make([]Domain, 0)
	for rows.Next() {
		domain, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, *domain)
	}
	return domains, rows.Err()
}

// ListDomainsForUser returns every domain across all of the user's accounts.
func (r *Repository) ListDomainsForUser(ctx context.Context, userID uuid.UUID) ([]Domain, error) {
	rows, err := r.db.Query(ctx,
		`SELECT d.id, d.mail_account_id, d.domain, d.provider_domain_id, d.status, d.region,
		        d.records, d.last_synced_at, d.created_at, d.updated_at
		   FROM domains d
		   JOIN mail_accounts a ON a.id = d.mail_account_id
		  WHERE a.user_id = $1
		  ORDER BY d.domain`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make([]Domain, 0)
	for rows.Next() {
		domain, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, *domain)
	}
	return domains, rows.Err()
}

// GetDomain resolves a domain through its account to the owning user, so a
// domain belonging to someone else reads as missing.
func (r *Repository) GetDomain(ctx context.Context, userID, domainID uuid.UUID) (*Domain, error) {
	return scanDomain(r.db.QueryRow(ctx,
		`SELECT d.id, d.mail_account_id, d.domain, d.provider_domain_id, d.status, d.region,
		        d.records, d.last_synced_at, d.created_at, d.updated_at
		   FROM domains d
		   JOIN mail_accounts a ON a.id = d.mail_account_id
		  WHERE d.id = $1 AND a.user_id = $2`, domainID, userID))
}

// ------------------------------------------------------------ addresses ----

const addressSelect = `SELECT m.id, m.domain_id, m.email, m.display_name, m.is_default, m.is_active,
	m.created_at, m.updated_at, d.domain, d.mail_account_id
	FROM mail_addresses m
	JOIN domains d ON d.id = m.domain_id`

func scanAddress(row pgx.Row) (*MailAddress, error) {
	var a MailAddress
	err := row.Scan(&a.ID, &a.DomainID, &a.Email, &a.DisplayName, &a.IsDefault, &a.IsActive,
		&a.CreatedAt, &a.UpdatedAt, &a.DomainName, &a.MailAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

type CreateAddressParams struct {
	DomainID    uuid.UUID
	Email       string
	DisplayName string
}

// CreateAddress adds a sender identity. The first address on a domain becomes
// its default.
func (r *Repository) CreateAddress(ctx context.Context, params CreateAddressParams) (*MailAddress, error) {
	var address *MailAddress

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var existing int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM mail_addresses WHERE domain_id = $1`, params.DomainID).Scan(&existing); err != nil {
			return err
		}

		var id uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO mail_addresses (domain_id, email, display_name, is_default)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			params.DomainID, params.Email, params.DisplayName, existing == 0).Scan(&id); err != nil {
			return err
		}

		var err error
		address, err = scanAddress(tx.QueryRow(ctx, addressSelect+` WHERE m.id = $1`, id))
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrAddressTaken
		}
		return nil, err
	}
	return address, nil
}

// ListAddressesForUser returns every sender identity the user can send from.
func (r *Repository) ListAddressesForUser(ctx context.Context, userID uuid.UUID) ([]MailAddress, error) {
	return r.queryAddresses(ctx,
		addressSelect+`
		   JOIN mail_accounts a ON a.id = d.mail_account_id
		  WHERE a.user_id = $1
		  ORDER BY d.domain, m.email`, userID)
}

func (r *Repository) ListAddressesForAccount(ctx context.Context, userID, accountID uuid.UUID) ([]MailAddress, error) {
	return r.queryAddresses(ctx,
		addressSelect+`
		   JOIN mail_accounts a ON a.id = d.mail_account_id
		  WHERE a.user_id = $1 AND a.id = $2
		  ORDER BY d.domain, m.email`, userID, accountID)
}

func (r *Repository) queryAddresses(ctx context.Context, query string, args ...any) ([]MailAddress, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	addresses := make([]MailAddress, 0)
	for rows.Next() {
		address, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, *address)
	}
	return addresses, rows.Err()
}

func (r *Repository) GetAddress(ctx context.Context, userID, addressID uuid.UUID) (*MailAddress, error) {
	return scanAddress(r.db.QueryRow(ctx,
		addressSelect+`
		   JOIN mail_accounts a ON a.id = d.mail_account_id
		  WHERE m.id = $1 AND a.user_id = $2`, addressID, userID))
}

type UpdateAddressParams struct {
	DisplayName *string
	IsActive    *bool
}

func (r *Repository) UpdateAddress(ctx context.Context, userID, addressID uuid.UUID, params UpdateAddressParams) (*MailAddress, error) {
	// The ownership check and the update are separate statements, so the update
	// repeats the predicate through a subquery rather than trusting the check.
	return scanAddress(r.db.QueryRow(ctx,
		`WITH updated AS (
		     UPDATE mail_addresses m SET
		         display_name = COALESCE($3, m.display_name),
		         is_active    = COALESCE($4, m.is_active)
		     WHERE m.id = $1
		       AND EXISTS (
		           SELECT 1 FROM domains d
		             JOIN mail_accounts a ON a.id = d.mail_account_id
		            WHERE d.id = m.domain_id AND a.user_id = $2)
		     RETURNING m.id
		 )
		 `+addressSelect+` WHERE m.id IN (SELECT id FROM updated)`,
		addressID, userID, params.DisplayName, params.IsActive))
}

// SetDefaultAddress makes one address the default sender for its domain.
func (r *Repository) SetDefaultAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		var domainID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT m.domain_id FROM mail_addresses m
			   JOIN domains d ON d.id = m.domain_id
			   JOIN mail_accounts a ON a.id = d.mail_account_id
			  WHERE m.id = $1 AND a.user_id = $2`, addressID, userID).Scan(&domainID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE mail_addresses SET is_default = false WHERE domain_id = $1 AND is_default`, domainID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE mail_addresses SET is_default = true WHERE id = $1`, addressID)
		return err
	})
}

func (r *Repository) DeleteAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM mail_addresses m
		  WHERE m.id = $1
		    AND EXISTS (
		        SELECT 1 FROM domains d
		          JOIN mail_accounts a ON a.id = d.mail_account_id
		         WHERE d.id = m.domain_id AND a.user_id = $2)`, addressID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveInbound maps an incoming recipient address to the account that owns
// it. This is the primary lookup for inbound routing (docs/PLAN.md §22).
func (r *Repository) ResolveInbound(ctx context.Context, email string) (*Route, error) {
	var route Route
	err := r.db.QueryRow(ctx,
		`SELECT m.id, m.email, d.id, a.id, a.user_id, (a.is_active AND m.is_active)
		   FROM mail_addresses m
		   JOIN domains d ON d.id = m.domain_id
		   JOIN mail_accounts a ON a.id = d.mail_account_id
		  WHERE m.email = $1`, email,
	).Scan(&route.AddressID, &route.Email, &route.DomainID, &route.MailAccountID,
		&route.UserID, &route.AccountActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &route, nil
}
