package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// doAs makes a request authenticated by API token and nothing else: no cookie
// is attached, so a pass proves the token alone carried it.
func (c *client) doAs(token, method, path string, body any) (int, map[string]any) {
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
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	var decoded map[string]any
	if raw := rec.Body.Bytes(); len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}
	return rec.Code, decoded
}

// createToken mints a token through the API and returns the secret and its id.
func (c *client) createToken(name, expiresIn string, accountID *string) (secret, id string) {
	c.t.Helper()
	body := map[string]any{"name": name, "expires_in": expiresIn}
	if accountID != nil {
		body["mail_account_id"] = *accountID
	}
	status, payload := c.do(http.MethodPost, "/api/v1/api-tokens", body)
	if status != http.StatusCreated {
		c.t.Fatalf("create token: status %d, body %v", status, payload)
	}
	secret, _ = payload["token"].(string)
	token, _ := payload["api_token"].(map[string]any)
	id, _ = token["id"].(string)
	if secret == "" || id == "" {
		c.t.Fatalf("create token: incomplete response %v", payload)
	}
	return secret, id
}

func TestAnAPITokenAuthenticatesTheSameEndpointsAsALogin(t *testing.T) {
	c := newTestServer(t)
	c.signUp("tokens@example.com")
	accountID := c.connectAccount("Work")

	secret, _ := c.createToken("CI", "30d", nil)

	// The point of the feature: the browser signs out, and the same endpoints
	// still answer for the token alone.
	c.signOut()

	for _, path := range []string{
		"/api/v1/me",
		"/api/v1/mail-accounts",
		"/api/v1/domains",
		"/api/v1/addresses",
		"/api/v1/threads",
		"/api/v1/labels",
		"/api/v1/drafts",
	} {
		status, body := c.doAs(secret, http.MethodGet, path, nil)
		if status != http.StatusOK {
			t.Errorf("GET %s with a token: status %d, body %v", path, status, body)
		}
	}

	// And writes, not only reads.
	status, body := c.doAs(secret, http.MethodPatch, "/api/v1/mail-accounts/"+accountID,
		map[string]string{"name": "Renamed by token"})
	if status != http.StatusOK {
		t.Fatalf("PATCH mail-account with a token: status %d, body %v", status, body)
	}
}

func TestWithoutAnyCredentialEndpointsStayClosed(t *testing.T) {
	c := newTestServer(t)
	c.signUp("closed@example.com")
	c.signOut()

	if status, _ := c.do(http.MethodGet, "/api/v1/mail-accounts", nil); status != http.StatusUnauthorized {
		t.Errorf("no credential: status %d, want 401", status)
	}
	if status, _ := c.doAs("xm_not_a_real_token", http.MethodGet, "/api/v1/mail-accounts", nil); status != http.StatusUnauthorized {
		t.Errorf("bogus token: status %d, want 401", status)
	}
	// A value without the prefix is refused without a database lookup.
	if status, _ := c.doAs("just-some-string", http.MethodGet, "/api/v1/mail-accounts", nil); status != http.StatusUnauthorized {
		t.Errorf("unprefixed token: status %d, want 401", status)
	}
}

func TestTheSecretIsShownOnceAndStoredOnlyAsADigest(t *testing.T) {
	c := newTestServer(t)
	c.signUp("once@example.com")

	secret, tokenID := c.createToken("Zapier", "1y", nil)

	// Listing gives a hint, never the secret.
	status, body := c.do(http.MethodGet, "/api/v1/api-tokens", nil)
	if status != http.StatusOK {
		t.Fatalf("list tokens: status %d, body %v", status, body)
	}
	raw, _ := json.Marshal(body)
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("the token list contains the secret")
	}
	list, _ := body["api_tokens"].([]any)
	if len(list) != 1 {
		t.Fatalf("got %d tokens, want 1", len(list))
	}
	first, _ := list[0].(map[string]any)
	if hint, _ := first["hint"].(string); hint == "" || hint == secret {
		t.Errorf("hint %q is missing or is the secret itself", hint)
	}
	if got, _ := first["expires_in"].(string); got != "1y" {
		t.Errorf("expires_in = %q, want \"1y\"", got)
	}

	// Nor is the plaintext anywhere in the table.
	var stored int
	err := c.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM api_tokens WHERE id = $1 AND encode(token_hash, 'escape') LIKE '%' || $2 || '%'`,
		tokenID, secret).Scan(&stored)
	if err != nil {
		t.Fatalf("query stored token: %v", err)
	}
	if stored != 0 {
		t.Fatal("the plaintext token was stored in the database")
	}
}

func TestEachOfTheFourLifetimesIsStored(t *testing.T) {
	c := newTestServer(t)
	c.signUp("lifetimes@example.com")

	cases := []struct {
		option string
		want   *time.Duration
	}{
		{"15d", durPtr(15 * 24 * time.Hour)},
		{"30d", durPtr(30 * 24 * time.Hour)},
		{"1y", durPtr(365 * 24 * time.Hour)},
		{"never", nil},
	}

	for _, tc := range cases {
		_, id := c.createToken("token-"+tc.option, tc.option, nil)

		var expiresAt *time.Time
		var createdAt time.Time
		err := c.pool.QueryRow(context.Background(),
			`SELECT expires_at, created_at FROM api_tokens WHERE id = $1`, id).Scan(&expiresAt, &createdAt)
		if err != nil {
			t.Fatalf("read %s token: %v", tc.option, err)
		}

		if tc.want == nil {
			if expiresAt != nil {
				t.Errorf("%q stored an expiry of %v, want never", tc.option, *expiresAt)
			}
			continue
		}
		if expiresAt == nil {
			t.Errorf("%q never expires, want about %v", tc.option, *tc.want)
			continue
		}
		// A day of slack absorbs leap years and daylight saving; the point is
		// that the option chosen is the span stored.
		if diff := expiresAt.Sub(createdAt) - *tc.want; diff > 24*time.Hour || diff < -24*time.Hour {
			t.Errorf("%q lasts %v, want about %v", tc.option, expiresAt.Sub(createdAt), *tc.want)
		}
	}
}

func TestAnUnsupportedLifetimeIsRefused(t *testing.T) {
	c := newTestServer(t)
	c.signUp("badlifetime@example.com")

	for _, bad := range []string{"", "7d", "90d", "forever", "10y"} {
		status, body := c.do(http.MethodPost, "/api/v1/api-tokens",
			map[string]string{"name": "Nope", "expires_in": bad})
		if status != http.StatusUnprocessableEntity {
			t.Errorf("expires_in=%q: status %d, want 422 (body %v)", bad, status, body)
		}
	}
}

func TestAnExpiredTokenStopsWorking(t *testing.T) {
	c := newTestServer(t)
	c.signUp("expiring@example.com")
	secret, tokenID := c.createToken("Short lived", "15d", nil)

	// It works right now.
	if status, _ := c.doAs(secret, http.MethodGet, "/api/v1/me", nil); status != http.StatusOK {
		t.Fatalf("fresh token: status %d, want 200", status)
	}

	// Move its expiry into the past rather than waiting 15 days.
	if _, err := c.pool.Exec(context.Background(),
		`UPDATE api_tokens SET expires_at = now() - interval '1 second' WHERE id = $1`, tokenID); err != nil {
		t.Fatalf("age the token: %v", err)
	}

	status, body := c.doAs(secret, http.MethodGet, "/api/v1/me", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expired token: status %d, want 401", status)
	}
	// The integrator is told which of the three it is, so they do not go
	// hunting for a typo in a key that is simply past its date.
	if msg := errorMessage(body); !contains(msg, "expired") {
		t.Errorf("expired token said %q, which does not mention expiry", msg)
	}
}

func TestARevokedTokenStopsWorkingImmediately(t *testing.T) {
	c := newTestServer(t)
	c.signUp("revoking@example.com")
	secret, tokenID := c.createToken("Leaked", "never", nil)

	if status, _ := c.doAs(secret, http.MethodGet, "/api/v1/me", nil); status != http.StatusOK {
		t.Fatalf("fresh token: status %d, want 200", status)
	}

	if status, body := c.do(http.MethodDelete, "/api/v1/api-tokens/"+tokenID, nil); status != http.StatusNoContent {
		t.Fatalf("revoke: status %d, body %v", status, body)
	}

	status, body := c.doAs(secret, http.MethodGet, "/api/v1/me", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked token: status %d, want 401", status)
	}
	if msg := errorMessage(body); !contains(msg, "revoked") {
		t.Errorf("revoked token said %q, which does not mention revocation", msg)
	}

	// And it is gone from the list.
	_, list := c.do(http.MethodGet, "/api/v1/api-tokens", nil)
	if tokens, _ := list["api_tokens"].([]any); len(tokens) != 0 {
		t.Errorf("revoked token still listed: %v", tokens)
	}
}

func TestATokenCannotMintOrRevokeTokens(t *testing.T) {
	c := newTestServer(t)
	c.signUp("escalation@example.com")
	secret, tokenID := c.createToken("Limited", "15d", nil)

	// The escalation this blocks: a 15-day key issuing itself a permanent one.
	status, body := c.doAs(secret, http.MethodPost, "/api/v1/api-tokens",
		map[string]string{"name": "Forever", "expires_in": "never"})
	if status != http.StatusForbidden {
		t.Errorf("token minting a token: status %d, want 403 (body %v)", status, body)
	}

	if status, _ := c.doAs(secret, http.MethodGet, "/api/v1/api-tokens", nil); status != http.StatusForbidden {
		t.Errorf("token listing tokens: status %d, want 403", status)
	}
	if status, _ := c.doAs(secret, http.MethodDelete, "/api/v1/api-tokens/"+tokenID, nil); status != http.StatusForbidden {
		t.Errorf("token revoking a token: status %d, want 403", status)
	}

	// A stolen key must not be able to lock the owner out of revoking it.
	status, _ = c.doAs(secret, http.MethodPost, "/api/v1/me/password",
		map[string]string{"current_password": "a-very-good-password", "new_password": "an-even-better-password"})
	if status != http.StatusForbidden {
		t.Errorf("token changing the password: status %d, want 403", status)
	}

	// The session it was created from still can, of course.
	if status, body := c.do(http.MethodGet, "/api/v1/api-tokens", nil); status != http.StatusOK {
		t.Errorf("session listing tokens: status %d, body %v", status, body)
	}
}

func TestATokenLimitedToOneAccountSeesOnlyThatAccount(t *testing.T) {
	c := newTestServer(t)
	c.signUp("restricted@example.com")

	support := c.connectAccount("Support")
	personal := c.connectAccount("Personal")

	secret, _ := c.createToken("Support desk", "30d", &support)
	c.signOut()

	// The list is narrowed to the one account, not merely filtered in the UI.
	status, body := c.doAs(secret, http.MethodGet, "/api/v1/mail-accounts", nil)
	if status != http.StatusOK {
		t.Fatalf("list accounts: status %d, body %v", status, body)
	}
	accounts, _ := body["mail_accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("restricted token sees %d accounts, want 1", len(accounts))
	}
	if got := accounts[0].(map[string]any)["id"].(string); got != support {
		t.Errorf("restricted token sees account %s, want %s", got, support)
	}

	// Naming the other account directly reads as missing, not as refused —
	// a 403 would confirm it exists.
	if status, _ := c.doAs(secret, http.MethodGet, "/api/v1/mail-accounts/"+personal, nil); status != http.StatusNotFound {
		t.Errorf("other account by id: status %d, want 404", status)
	}
	if status, _ := c.doAs(secret, http.MethodPatch, "/api/v1/mail-accounts/"+personal,
		map[string]string{"name": "Hijacked"}); status != http.StatusNotFound {
		t.Errorf("patching the other account: status %d, want 404", status)
	}
	if status, _ := c.doAs(secret, http.MethodDelete, "/api/v1/mail-accounts/"+personal, nil); status != http.StatusNotFound {
		t.Errorf("deleting the other account: status %d, want 404", status)
	}

	// Its own account is fully reachable.
	if status, _ := c.doAs(secret, http.MethodGet, "/api/v1/mail-accounts/"+support, nil); status != http.StatusOK {
		t.Errorf("own account: status %d, want 200", status)
	}

	// And it cannot widen its own scope by connecting another account.
	status, _ = c.doAs(secret, http.MethodPost, "/api/v1/mail-accounts",
		map[string]string{"name": "Sneaky", "provider": "resend", "api_key": goodAPIKey})
	if status != http.StatusForbidden {
		t.Errorf("restricted token connecting an account: status %d, want 403", status)
	}
}

func TestATokenCannotBePointedAtSomeoneElsesAccount(t *testing.T) {
	alice := newTestServer(t)
	alice.signUp("alice-tokens@example.com")
	alicesAccount := alice.connectAccount("Alice Work")

	bob := &client{t: t, handler: alice.handler, pool: alice.pool}
	bob.signUp("bob-tokens@example.com")

	status, body := bob.do(http.MethodPost, "/api/v1/api-tokens", map[string]any{
		"name": "Reaching", "expires_in": "30d", "mail_account_id": alicesAccount,
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("token against another user's account: status %d, want 422 (body %v)", status, body)
	}
}

func TestTokensAreInvisibleBetweenUsers(t *testing.T) {
	alice := newTestServer(t)
	alice.signUp("alice-list@example.com")
	_, aliceToken := alice.createToken("Alice CI", "30d", nil)

	bob := &client{t: t, handler: alice.handler, pool: alice.pool}
	bob.signUp("bob-list@example.com")

	_, body := bob.do(http.MethodGet, "/api/v1/api-tokens", nil)
	if tokens, _ := body["api_tokens"].([]any); len(tokens) != 0 {
		t.Errorf("bob sees %d of alice's tokens", len(tokens))
	}
	if status, _ := bob.do(http.MethodDelete, "/api/v1/api-tokens/"+aliceToken, nil); status != http.StatusNotFound {
		t.Errorf("bob revoking alice's token: status %d, want 404", status)
	}

	// Alice's token is untouched.
	_, body = alice.do(http.MethodGet, "/api/v1/api-tokens", nil)
	if tokens, _ := body["api_tokens"].([]any); len(tokens) != 1 {
		t.Errorf("alice has %d tokens after bob's attempt, want 1", len(tokens))
	}
}

func TestUsingATokenRecordsThatItWasUsed(t *testing.T) {
	c := newTestServer(t)
	c.signUp("lastused@example.com")
	secret, tokenID := c.createToken("Watched", "30d", nil)

	var before *time.Time
	if err := c.pool.QueryRow(context.Background(),
		`SELECT last_used_at FROM api_tokens WHERE id = $1`, tokenID).Scan(&before); err != nil {
		t.Fatalf("read last_used_at: %v", err)
	}
	if before != nil {
		t.Fatalf("a token that has never been used reports last_used_at = %v", *before)
	}

	if status, _ := c.doAs(secret, http.MethodGet, "/api/v1/me", nil); status != http.StatusOK {
		t.Fatalf("use token: status %d", status)
	}

	var after *time.Time
	if err := c.pool.QueryRow(context.Background(),
		`SELECT last_used_at FROM api_tokens WHERE id = $1`, tokenID).Scan(&after); err != nil {
		t.Fatalf("read last_used_at: %v", err)
	}
	if after == nil {
		t.Error("last_used_at was not recorded")
	}
}

func durPtr(d time.Duration) *time.Duration { return &d }

func errorMessage(body map[string]any) string {
	envelope, _ := body["error"].(map[string]any)
	msg, _ := envelope["message"].(string)
	return msg
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && bytes.Contains([]byte(haystack), []byte(needle))
}
