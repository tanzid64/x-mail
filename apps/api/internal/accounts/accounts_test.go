package accounts_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/crypto"
	"github.com/dark/x-mail/api/internal/providers"
	"github.com/dark/x-mail/api/internal/testsupport"
)

// fakeProvider stands in for Resend so tests never touch the network. It
// accepts one known API key and rejects everything else, which is what lets the
// credential-verification paths be exercised.
type fakeProvider struct {
	apiKey  string
	domains []providers.Domain
}

const goodAPIKey = "re_good_key_1234567890"

func (f *fakeProvider) Name() providers.Provider { return providers.ProviderResend }

func (f *fakeProvider) VerifyCredentials(context.Context) error {
	if f.apiKey != goodAPIKey {
		return providers.NewError(providers.ProviderResend, providers.KindAuth, 401, "API key is invalid", nil)
	}
	return nil
}

func (f *fakeProvider) Send(context.Context, providers.Email) (*providers.SendResult, error) {
	return &providers.SendResult{ProviderMessageID: "fake"}, nil
}

func (f *fakeProvider) ListDomains(context.Context) ([]providers.Domain, error) {
	if err := f.VerifyCredentials(context.Background()); err != nil {
		return nil, err
	}
	return f.domains, nil
}

func (f *fakeProvider) GetDomain(_ context.Context, id string) (*providers.Domain, error) {
	for _, d := range f.domains {
		if d.ID == id {
			return &d, nil
		}
	}
	return nil, providers.NewError(providers.ProviderResend, providers.KindNotFound, 404, "no such domain", nil)
}

var stubDomains = []providers.Domain{
	{ID: "prov_dom_1", Name: "company.com", Status: providers.DomainVerified, Region: "us-east-1"},
	{ID: "prov_dom_2", Name: "darkmail.com", Status: providers.DomainPending, Region: "us-east-1"},
}

func newService(t *testing.T) (*accounts.Service, *pgxpool.Pool) {
	t.Helper()

	pool := testsupport.Pool(t)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	sealer, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	factory := func(_ providers.Provider, apiKey string) (providers.MailProvider, error) {
		return &fakeProvider{apiKey: apiKey, domains: stubDomains}, nil
	}

	svc := accounts.NewService(
		accounts.NewRepository(pool), sealer, factory,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return svc, pool
}

// newUser creates a user directly, since account tests are not about signup.
func newUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	hash, err := auth.HashPassword("a-very-good-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user, err := auth.NewRepository(pool).CreateUser(context.Background(), email, hash, "Test")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return user.ID
}

func connect(t *testing.T, svc *accounts.Service, userID uuid.UUID, name string) *accounts.MailAccount {
	t.Helper()
	account, err := svc.Connect(context.Background(), userID, accounts.ConnectParams{
		Name: name, Provider: providers.ProviderResend, APIKey: goodAPIKey,
	})
	if err != nil {
		t.Fatalf("Connect(%s): %v", name, err)
	}
	return account
}

func TestConnectStoresCredentialEncryptedAndReturnsOnlyAHint(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	userID := newUser(t, pool, "owner@example.com")

	account := connect(t, svc, userID, "Work")

	if account.APIKeyHint == goodAPIKey {
		t.Fatal("the raw API key was returned to the caller")
	}
	if account.APIKeyHint != crypto.Hint(goodAPIKey) {
		t.Errorf("hint = %q, want %q", account.APIKeyHint, crypto.Hint(goodAPIKey))
	}

	// The stored bytes must not contain the plaintext key.
	var stored []byte
	if err := pool.QueryRow(ctx,
		`SELECT api_key_encrypted FROM mail_accounts WHERE id = $1`, account.ID).Scan(&stored); err != nil {
		t.Fatalf("read stored key: %v", err)
	}
	if string(stored) == goodAPIKey {
		t.Fatal("the API key is stored in plaintext")
	}
	if len(stored) == 0 {
		t.Fatal("no credential was stored")
	}
}

func TestConnectRejectsCredentialsTheProviderRefuses(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	userID := newUser(t, pool, "owner@example.com")

	_, err := svc.Connect(ctx, userID, accounts.ConnectParams{
		Name: "Work", Provider: providers.ProviderResend, APIKey: "re_wrong_key",
	})
	if !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}

	// Nothing may be stored for a key that was never accepted.
	list, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("stored %d accounts after a rejected key, want 0", len(list))
	}
}

func TestFirstAccountBecomesDefaultAndDefaultCanMove(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	userID := newUser(t, pool, "owner@example.com")

	first := connect(t, svc, userID, "Personal")
	if !first.IsDefault {
		t.Error("the first account should become the default")
	}

	second := connect(t, svc, userID, "Work")
	if second.IsDefault {
		t.Error("a later account should not take the default automatically")
	}

	if err := svc.SetDefault(ctx, userID, second.ID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	list, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	defaults := 0
	for _, a := range list {
		if a.IsDefault {
			defaults++
			if a.ID != second.ID {
				t.Error("the default did not move to the requested account")
			}
		}
	}
	if defaults != 1 {
		t.Errorf("found %d default accounts, want exactly 1", defaults)
	}
}

func TestDeletingTheDefaultPromotesAnother(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	userID := newUser(t, pool, "owner@example.com")

	first := connect(t, svc, userID, "Personal")
	connect(t, svc, userID, "Work")

	if err := svc.Delete(ctx, userID, first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d accounts, want 1", len(list))
	}
	if !list[0].IsDefault {
		t.Error("the remaining account should have been promoted to default")
	}
}

// The central guarantee of the whole system: one user's mail account is
// invisible to another, and reads as missing rather than forbidden.
func TestAccountsAreIsolatedBetweenUsers(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	alice := newUser(t, pool, "alice@example.com")
	bob := newUser(t, pool, "bob@example.com")

	aliceAccount := connect(t, svc, alice, "Alice Work")

	t.Run("list does not leak", func(t *testing.T) {
		list, err := svc.List(ctx, bob)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("bob sees %d of alice's accounts", len(list))
		}
	})

	t.Run("get is not found", func(t *testing.T) {
		if _, err := svc.Get(ctx, bob, aliceAccount.ID); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("Get = %v, want ErrNotFound", err)
		}
	})

	t.Run("update is not found", func(t *testing.T) {
		name := "Hijacked"
		if _, err := svc.Update(ctx, bob, aliceAccount.ID, accounts.UpdateAccountParams{Name: &name}); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("Update = %v, want ErrNotFound", err)
		}
	})

	t.Run("delete is not found", func(t *testing.T) {
		if err := svc.Delete(ctx, bob, aliceAccount.ID); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("Delete = %v, want ErrNotFound", err)
		}
	})

	t.Run("set default is not found", func(t *testing.T) {
		if err := svc.SetDefault(ctx, bob, aliceAccount.ID); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("SetDefault = %v, want ErrNotFound", err)
		}
	})

	t.Run("credentials cannot be reached", func(t *testing.T) {
		if _, err := svc.ProviderFor(ctx, bob, aliceAccount.ID); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("ProviderFor = %v, want ErrNotFound", err)
		}
	})

	// Alice's account must have survived every one of Bob's attempts.
	if _, err := svc.Get(ctx, alice, aliceAccount.ID); err != nil {
		t.Errorf("alice lost access to her own account: %v", err)
	}
}

func TestAccountNamesAreUniquePerUserNotGlobally(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	alice := newUser(t, pool, "alice@example.com")
	bob := newUser(t, pool, "bob@example.com")

	connect(t, svc, alice, "Work")

	_, err := svc.Connect(ctx, alice, accounts.ConnectParams{
		Name: "  work  ", Provider: providers.ProviderResend, APIKey: goodAPIKey,
	})
	if !errors.Is(err, accounts.ErrNameTaken) {
		t.Errorf("duplicate name for the same user = %v, want ErrNameTaken", err)
	}

	// Two different users may each have an account called "Work".
	connect(t, svc, bob, "Work")
}

func TestDomainsSyncFromTheProviderAndAreIsolated(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	alice := newUser(t, pool, "alice@example.com")
	bob := newUser(t, pool, "bob@example.com")
	account := connect(t, svc, alice, "Work")

	// Connect already synced; syncing again must be idempotent.
	domains, err := svc.SyncDomains(ctx, alice, account.ID)
	if err != nil {
		t.Fatalf("SyncDomains: %v", err)
	}
	if len(domains) != len(stubDomains) {
		t.Fatalf("got %d domains, want %d", len(domains), len(stubDomains))
	}

	all, err := svc.ListDomains(ctx, alice, nil)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(all) != len(stubDomains) {
		t.Errorf("re-syncing duplicated domains: got %d", len(all))
	}

	bobsView, err := svc.ListDomains(ctx, bob, nil)
	if err != nil {
		t.Fatalf("ListDomains(bob): %v", err)
	}
	if len(bobsView) != 0 {
		t.Errorf("bob sees %d of alice's domains", len(bobsView))
	}

	// Passing Alice's account id as a filter must not work for Bob either.
	if _, err := svc.ListDomains(ctx, bob, &account.ID); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("ListDomains with another user's account filter = %v, want ErrNotFound", err)
	}
	if _, err := svc.GetDomain(ctx, bob, all[0].ID); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("GetDomain = %v, want ErrNotFound", err)
	}
}

func TestSyncRemovesDomainsTheProviderNoLongerHas(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	userID := newUser(t, pool, "owner@example.com")

	key := make([]byte, 32)
	sealer, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	// A factory whose domain list shrinks between calls.
	remote := stubDomains
	factory := func(_ providers.Provider, apiKey string) (providers.MailProvider, error) {
		return &fakeProvider{apiKey: apiKey, domains: remote}, nil
	}
	svc := accounts.NewService(accounts.NewRepository(pool), sealer, factory,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	account := connect(t, svc, userID, "Work")

	remote = stubDomains[:1]
	domains, err := svc.SyncDomains(ctx, userID, account.ID)
	if err != nil {
		t.Fatalf("SyncDomains: %v", err)
	}
	if len(domains) != 1 || domains[0].Domain != "company.com" {
		t.Fatalf("stale domain was not removed: %+v", domains)
	}
}

// Rechecking one domain's DNS reconciled the whole account against a
// single-element list, which silently deleted every other domain it owned —
// and the addresses hanging off them.
func TestRefreshingOneDomainLeavesTheOthersAlone(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	userID := newUser(t, pool, "owner@example.com")
	account := connect(t, svc, userID, "Work")

	before, err := svc.ListDomains(ctx, userID, &account.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(before) != len(stubDomains) {
		t.Fatalf("setup: got %d domains, want %d", len(before), len(stubDomains))
	}

	if _, err := svc.RefreshDomain(ctx, userID, before[0].ID); err != nil {
		t.Fatalf("RefreshDomain: %v", err)
	}

	after, err := svc.ListDomains(ctx, userID, &account.ID)
	if err != nil {
		t.Fatalf("ListDomains after refresh: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("refreshing one domain left %d of %d domains", len(after), len(before))
	}
}

func TestAddressesBelongToTheirDomainAndAreGloballyUnique(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()

	alice := newUser(t, pool, "alice@example.com")
	bob := newUser(t, pool, "bob@example.com")

	aliceAccount := connect(t, svc, alice, "Work")
	aliceDomains, err := svc.ListDomains(ctx, alice, &aliceAccount.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	companyDomain := aliceDomains[0]

	t.Run("address must sit on the domain", func(t *testing.T) {
		_, err := svc.CreateAddress(ctx, alice, companyDomain.ID, "someone@elsewhere.com", "Nope")
		if !errors.Is(err, accounts.ErrAddressNotOnDomain) {
			t.Errorf("err = %v, want ErrAddressNotOnDomain", err)
		}
	})

	address, err := svc.CreateAddress(ctx, alice, companyDomain.ID, "Support@Company.com", "Support")
	if err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}
	if address.Email != "support@company.com" {
		t.Errorf("email = %q, want it normalised to lowercase", address.Email)
	}
	if !address.IsDefault {
		t.Error("the first address on a domain should become its default")
	}

	t.Run("bob cannot add an address to alice's domain", func(t *testing.T) {
		_, err := svc.CreateAddress(ctx, bob, companyDomain.ID, "intruder@company.com", "")
		if !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("bob cannot read, edit or delete alice's address", func(t *testing.T) {
		if _, err := svc.GetAddress(ctx, bob, address.ID); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("GetAddress = %v, want ErrNotFound", err)
		}
		name := "Hijacked"
		if _, err := svc.UpdateAddress(ctx, bob, address.ID, accounts.UpdateAddressParams{DisplayName: &name}); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("UpdateAddress = %v, want ErrNotFound", err)
		}
		if err := svc.DeleteAddress(ctx, bob, address.ID); !errors.Is(err, accounts.ErrNotFound) {
			t.Errorf("DeleteAddress = %v, want ErrNotFound", err)
		}

		// Alice's address is untouched.
		still, err := svc.GetAddress(ctx, alice, address.ID)
		if err != nil {
			t.Fatalf("alice lost her address: %v", err)
		}
		if still.DisplayName != "Support" {
			t.Errorf("display name = %q, want it unchanged", still.DisplayName)
		}
	})

	// Two mailboxes claiming one address would make inbound routing ambiguous.
	t.Run("the same address cannot exist twice", func(t *testing.T) {
		bobAccount := connect(t, svc, bob, "Bob Work")
		bobDomains, err := svc.ListDomains(ctx, bob, &bobAccount.ID)
		if err != nil {
			t.Fatalf("ListDomains: %v", err)
		}
		if _, err := svc.CreateAddress(ctx, bob, bobDomains[0].ID, "support@company.com", ""); !errors.Is(err, accounts.ErrAddressTaken) {
			t.Errorf("err = %v, want ErrAddressTaken", err)
		}
	})
}

func TestResolveInboundFindsTheOwningAccount(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	userID := newUser(t, pool, "owner@example.com")

	account := connect(t, svc, userID, "Support")
	domains, err := svc.ListDomains(ctx, userID, &account.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if _, err := svc.CreateAddress(ctx, userID, domains[0].ID, "support@company.com", "Support"); err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}

	// Inbound recipients arrive with arbitrary casing and whitespace.
	route, err := svc.ResolveInbound(ctx, "  Support@Company.COM ")
	if err != nil {
		t.Fatalf("ResolveInbound: %v", err)
	}
	if route.MailAccountID != account.ID {
		t.Error("resolved to the wrong mail account")
	}
	if route.UserID != userID {
		t.Error("resolved to the wrong user")
	}
	if !route.AccountActive {
		t.Error("expected the route to be active")
	}

	if _, err := svc.ResolveInbound(ctx, "nobody@company.com"); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("unknown recipient = %v, want ErrNotFound", err)
	}
}

func TestCheckCredentialsMarksARevokedKey(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()
	userID := newUser(t, pool, "owner@example.com")

	key := make([]byte, 32)
	sealer, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	// The provider starts accepting the key, then stops — as if it was revoked.
	accepted := goodAPIKey
	factory := func(_ providers.Provider, apiKey string) (providers.MailProvider, error) {
		return &fakeProvider{apiKey: apiKey + accepted[:0], domains: stubDomains}, nil
	}
	svc := accounts.NewService(accounts.NewRepository(pool), sealer, factory,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	account := connect(t, svc, userID, "Work")
	if account.Status != accounts.StatusActive {
		t.Fatalf("status = %q, want active", account.Status)
	}

	// Rebuild the service so the provider now rejects the stored key.
	revoking := func(_ providers.Provider, _ string) (providers.MailProvider, error) {
		return &fakeProvider{apiKey: "revoked", domains: stubDomains}, nil
	}
	svc = accounts.NewService(accounts.NewRepository(pool), sealer, revoking,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	checked, err := svc.CheckCredentials(ctx, userID, account.ID)
	if err != nil {
		t.Fatalf("CheckCredentials: %v", err)
	}
	if checked.Status != accounts.StatusInvalidCredentials {
		t.Errorf("status = %q, want invalid_credentials", checked.Status)
	}
	if checked.LastCheckedAt == nil {
		t.Error("last_checked_at was not recorded")
	}
}
