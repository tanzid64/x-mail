package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/providers"
	"github.com/dark/x-mail/api/internal/server"
	"github.com/dark/x-mail/api/internal/testsupport"
)

const goodAPIKey = "re_good_key_1234567890"

// fakeProvider accepts one API key, so the credential-verification paths run
// without reaching the network.
type fakeProvider struct{ apiKey string }

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
	return []providers.Domain{
		{ID: "prov_dom_1", Name: "company.com", Status: providers.DomainVerified},
	}, nil
}

func (f *fakeProvider) GetDomain(context.Context, string) (*providers.Domain, error) {
	return &providers.Domain{ID: "prov_dom_1", Name: "company.com", Status: providers.DomainVerified}, nil
}

// client drives the API the way a browser does, carrying cookies between calls.
type client struct {
	t       *testing.T
	handler http.Handler
	cookies []*http.Cookie
}

func newTestServer(t *testing.T) *client {
	t.Helper()

	pool := testsupport.Pool(t)
	rdb := testsupport.Redis(t)

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	srv, err := server.New(server.Deps{
		Config: &config.Config{
			Env:                 "test",
			CORSOrigins:         []string{"http://localhost:5180"},
			AppBaseURL:          "http://localhost:5180",
			SessionCookieName:   "xmail_session",
			SessionTTL:          time.Hour,
			EncryptionMasterKey: key,
			SystemMailFrom:      "x-mail <noreply@example.com>",
		},
		DB:     pool,
		Redis:  rdb,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		ProviderFactory: func(_ providers.Provider, apiKey string) (providers.MailProvider, error) {
			return &fakeProvider{apiKey: apiKey}, nil
		},
	})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	return &client{t: t, handler: srv.Handler()}
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("encode body: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, path, payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	if set := rec.Result().Cookies(); len(set) > 0 {
		c.cookies = set
	}

	var decoded map[string]any
	if raw := rec.Body.Bytes(); len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}
	return rec.Code, decoded
}

// signUp registers a user and leaves the client authenticated as them.
func (c *client) signUp(email string) {
	c.t.Helper()
	status, body := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": email, "password": "a-very-good-password", "name": "Tester",
	})
	if status != http.StatusCreated {
		c.t.Fatalf("register: status %d, body %v", status, body)
	}
}

func (c *client) signOut() {
	c.t.Helper()
	c.do(http.MethodPost, "/api/v1/auth/logout", nil)
	c.cookies = nil
}

// connectAccount adds a mail account and returns its id.
func (c *client) connectAccount(name string) string {
	c.t.Helper()
	status, body := c.do(http.MethodPost, "/api/v1/mail-accounts", map[string]string{
		"name": name, "provider": "resend", "api_key": goodAPIKey,
	})
	if status != http.StatusCreated {
		c.t.Fatalf("connect account: status %d, body %v", status, body)
	}
	account, _ := body["mail_account"].(map[string]any)
	id, _ := account["id"].(string)
	if id == "" {
		c.t.Fatalf("no account id in response: %v", body)
	}
	return id
}

func TestMailAccountEndpointsRequireASession(t *testing.T) {
	c := newTestServer(t)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/mail-accounts"},
		{http.MethodPost, "/api/v1/mail-accounts"},
		{http.MethodGet, "/api/v1/domains"},
		{http.MethodGet, "/api/v1/addresses"},
	} {
		status, _ := c.do(route.method, route.path, nil)
		if status != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", route.method, route.path, status)
		}
	}
}

func TestConnectAccountNeverReturnsTheAPIKey(t *testing.T) {
	c := newTestServer(t)
	c.signUp("owner@example.com")

	status, body := c.do(http.MethodPost, "/api/v1/mail-accounts", map[string]string{
		"name": "Work", "provider": "resend", "api_key": goodAPIKey,
	})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, body %v", status, body)
	}

	// The whole response is searched, not just the field we expect.
	raw, _ := json.Marshal(body)
	if bytes.Contains(raw, []byte(goodAPIKey)) {
		t.Fatalf("the API key was echoed back to the client: %s", raw)
	}

	account := body["mail_account"].(map[string]any)
	if account["api_key_hint"] == "" {
		t.Error("expected a masked hint")
	}
	if !account["is_default"].(bool) {
		t.Error("the first account should be the default")
	}
}

func TestConnectAccountRejectsABadKeyAsAFieldError(t *testing.T) {
	c := newTestServer(t)
	c.signUp("owner@example.com")

	status, body := c.do(http.MethodPost, "/api/v1/mail-accounts", map[string]string{
		"name": "Work", "provider": "resend", "api_key": "re_not_a_real_key",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %v", status, body)
	}

	apiErr := body["error"].(map[string]any)
	fields, _ := apiErr["fields"].(map[string]any)
	if fields["api_key"] == nil {
		t.Errorf("expected an api_key field error, got %v", apiErr)
	}
}

func TestConnectAccountValidatesInput(t *testing.T) {
	c := newTestServer(t)
	c.signUp("owner@example.com")

	status, body := c.do(http.MethodPost, "/api/v1/mail-accounts", map[string]string{
		"name": "", "provider": "sendgrid", "api_key": "",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", status)
	}

	fields := body["error"].(map[string]any)["fields"].(map[string]any)
	for _, field := range []string{"name", "provider", "api_key"} {
		if fields[field] == nil {
			t.Errorf("expected a %q field error, got %v", field, fields)
		}
	}
}

// Another user's records must read as missing. A 403 would confirm the record
// exists, which is itself a disclosure.
func TestOneUsersAccountIsInvisibleToAnother(t *testing.T) {
	alice := newTestServer(t)
	alice.signUp("alice@example.com")
	accountID := alice.connectAccount("Alice Work")

	status, body := alice.do(http.MethodGet, "/api/v1/domains?mail_account_id="+accountID, nil)
	if status != http.StatusOK {
		t.Fatalf("alice listing her domains: %d %v", status, body)
	}
	domains := body["domains"].([]any)
	if len(domains) != 1 {
		t.Fatalf("expected one synced domain, got %v", domains)
	}
	domainID := domains[0].(map[string]any)["id"].(string)

	// Same server, different user.
	bob := &client{t: t, handler: alice.handler}
	bob.signUp("bob@example.com")

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/mail-accounts/" + accountID},
		{http.MethodPatch, "/api/v1/mail-accounts/" + accountID},
		{http.MethodDelete, "/api/v1/mail-accounts/" + accountID},
		{http.MethodPost, "/api/v1/mail-accounts/" + accountID + "/default"},
		{http.MethodPost, "/api/v1/mail-accounts/" + accountID + "/sync-domains"},
		{http.MethodGet, "/api/v1/domains/" + domainID},
		{http.MethodPost, "/api/v1/domains/" + domainID + "/refresh"},
	} {
		var body any
		if route.method == http.MethodPatch {
			body = map[string]string{"name": "Hijacked"}
		}
		status, _ := bob.do(route.method, route.path, body)
		if status != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", route.method, route.path, status)
		}
	}

	// Rotating a credential on someone else's account must also fail.
	status, _ = bob.do(http.MethodPost, "/api/v1/mail-accounts/"+accountID+"/api-key",
		map[string]string{"api_key": goodAPIKey})
	if status != http.StatusNotFound {
		t.Errorf("rotate key on another user's account = %d, want 404", status)
	}

	// Bob's own listing is empty, and Alice still has her account.
	_, bobList := bob.do(http.MethodGet, "/api/v1/mail-accounts", nil)
	if len(bobList["mail_accounts"].([]any)) != 0 {
		t.Errorf("bob sees alice's accounts: %v", bobList)
	}

	status, aliceList := alice.do(http.MethodGet, "/api/v1/mail-accounts", nil)
	if status != http.StatusOK || len(aliceList["mail_accounts"].([]any)) != 1 {
		t.Errorf("alice's account did not survive: %d %v", status, aliceList)
	}
}

func TestAddressLifecycleThroughTheAPI(t *testing.T) {
	c := newTestServer(t)
	c.signUp("owner@example.com")
	accountID := c.connectAccount("Work")

	_, body := c.do(http.MethodGet, "/api/v1/domains?mail_account_id="+accountID, nil)
	domainID := body["domains"].([]any)[0].(map[string]any)["id"].(string)

	status, created := c.do(http.MethodPost, "/api/v1/domains/"+domainID+"/addresses",
		map[string]string{"email": "support@company.com", "display_name": "Support"})
	if status != http.StatusCreated {
		t.Fatalf("create address: %d %v", status, created)
	}
	addressID := created["address"].(map[string]any)["id"].(string)

	// An address on a domain the account does not own is rejected as a field error.
	status, mismatch := c.do(http.MethodPost, "/api/v1/domains/"+domainID+"/addresses",
		map[string]string{"email": "someone@elsewhere.com"})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("mismatched domain = %d, want 422; %v", status, mismatch)
	}

	status, listed := c.do(http.MethodGet, "/api/v1/addresses", nil)
	if status != http.StatusOK || len(listed["addresses"].([]any)) != 1 {
		t.Fatalf("list addresses: %d %v", status, listed)
	}

	status, _ = c.do(http.MethodPatch, "/api/v1/addresses/"+addressID,
		map[string]string{"display_name": "Customer Support"})
	if status != http.StatusOK {
		t.Errorf("update address = %d", status)
	}

	status, _ = c.do(http.MethodDelete, "/api/v1/addresses/"+addressID, nil)
	if status != http.StatusNoContent {
		t.Errorf("delete address = %d, want 204", status)
	}

	// Signing out revokes access entirely.
	c.signOut()
	status, _ = c.do(http.MethodGet, "/api/v1/addresses", nil)
	if status != http.StatusUnauthorized {
		t.Errorf("after sign out = %d, want 401", status)
	}
}

func TestMalformedIdReadsAsNotFound(t *testing.T) {
	c := newTestServer(t)
	c.signUp("owner@example.com")

	status, _ := c.do(http.MethodGet, "/api/v1/mail-accounts/not-a-uuid", nil)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}
