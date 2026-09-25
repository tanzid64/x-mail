package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rpc sends one JSON-RPC call to the MCP endpoint with a bearer token.
func (c *client) rpc(token, method string, params any) (int, map[string]any) {
	c.t.Helper()

	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		c.t.Fatalf("encode rpc: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	var decoded map[string]any
	if raw, _ := io.ReadAll(rec.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}
	return rec.Code, decoded
}

// callTool invokes a tool and returns its structured result.
func (c *client) callTool(token, name string, args map[string]any) map[string]any {
	c.t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	status, body := c.rpc(token, "tools/call", map[string]any{"name": name, "arguments": args})
	if status != http.StatusOK {
		c.t.Fatalf("%s: status %d, body %v", name, status, body)
	}
	result, _ := body["result"].(map[string]any)
	if result == nil {
		c.t.Fatalf("%s: no result in %v", name, body)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		c.t.Fatalf("%s: tool reported an error: %v", name, result["content"])
	}
	structured, _ := result["structuredContent"].(map[string]any)
	return structured
}

func toolNames(t *testing.T, body map[string]any) []string {
	t.Helper()
	result, _ := body["result"].(map[string]any)
	raw, _ := result["tools"].([]any)
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		tool, _ := entry.(map[string]any)
		if name, _ := tool["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func TestMCPNeedsAnAPIToken(t *testing.T) {
	c := newTestServer(t)
	c.signUp("mcp-auth@example.com")

	// A signed-in browser is not enough: MCP is a machine interface, and a
	// cookie reaching it is a confused client or a cross-site request.
	status, body := c.do(http.MethodPost, "/api/v1/mcp",
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if status != http.StatusUnauthorized {
		t.Errorf("session cookie on /mcp: status %d, want 401 (body %v)", status, body)
	}

	if status, _ := c.rpc("", "tools/list", nil); status != http.StatusUnauthorized {
		t.Errorf("no credential: status %d, want 401", status)
	}
	if status, _ := c.rpc("xm_not_real", "tools/list", nil); status != http.StatusUnauthorized {
		t.Errorf("bogus token: status %d, want 401", status)
	}
}

func TestMCPInitializesAndListsItsTools(t *testing.T) {
	c := newTestServer(t)
	c.signUp("mcp-init@example.com")
	secret, _ := c.createToken("Assistant", "30d", nil)

	status, body := c.rpc(secret, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
	})
	if status != http.StatusOK {
		t.Fatalf("initialize: status %d, body %v", status, body)
	}
	result, _ := body["result"].(map[string]any)
	info, _ := result["serverInfo"].(map[string]any)
	if name, _ := info["name"].(string); name != "x-mail" {
		t.Errorf("serverInfo.name = %q, want x-mail", name)
	}
	// The instructions are where an assistant is told it cannot send.
	if instructions, _ := result["instructions"].(string); !strings.Contains(instructions, "cannot send") {
		t.Errorf("instructions do not say the server cannot send: %q", instructions)
	}

	_, listed := c.rpc(secret, "tools/list", nil)
	names := toolNames(t, listed)

	want := []string{
		"list_accounts", "list_addresses", "list_labels",
		"list_threads", "get_thread",
		"list_drafts", "get_draft", "create_draft", "update_draft",
	}
	for _, expected := range want {
		if !contains(strings.Join(names, ","), expected) {
			t.Errorf("tool %q is missing; got %v", expected, names)
		}
	}
}

// The safety property the whole design rests on.
func TestMCPExposesNoWayToSendOrDestroy(t *testing.T) {
	c := newTestServer(t)
	c.signUp("mcp-safety@example.com")
	secret, _ := c.createToken("Assistant", "30d", nil)

	_, listed := c.rpc(secret, "tools/list", nil)
	names := strings.Join(toolNames(t, listed), ",")

	// An assistant that reads untrusted email and can also send it is one
	// convincing instruction away from sending mail as the reader.
	for _, forbidden := range []string{"send", "delete", "remove", "revoke", "api_key", "token"} {
		if contains(names, forbidden) {
			t.Errorf("a tool mentioning %q is exposed; tools are: %s", forbidden, names)
		}
	}

	// And calling one by name does not work either. An unknown tool is a
	// JSON-RPC protocol error rather than a tool that ran and failed, which is
	// the stronger of the two answers.
	for _, name := range []string{"send_draft", "delete_draft", "send_email"} {
		status, body := c.rpc(secret, "tools/call",
			map[string]any{"name": name, "arguments": map[string]any{}})
		if status != http.StatusOK {
			continue // a transport-level rejection is fine too
		}
		if !rpcFailed(body) {
			t.Errorf("calling %q did not fail: %v", name, body)
		}
	}
}

func TestMCPReadsTheMailboxItIsGiven(t *testing.T) {
	c := newTestServer(t)
	c.signUp("mcp-read@example.com")
	accountID := c.connectAccount("Work")
	secret, _ := c.createToken("Assistant", "30d", nil)

	accounts := c.callTool(secret, "list_accounts", nil)
	list, _ := accounts["accounts"].([]any)
	if len(list) != 1 {
		t.Fatalf("list_accounts returned %d accounts, want 1", len(list))
	}
	first, _ := list[0].(map[string]any)
	if got, _ := first["id"].(string); got != accountID {
		t.Errorf("list_accounts returned %s, want %s", got, accountID)
	}

	// An empty mailbox is a valid answer, not an error.
	threads := c.callTool(secret, "list_threads", map[string]any{"folder": "inbox"})
	if _, ok := threads["threads"]; !ok {
		t.Errorf("list_threads returned no threads key: %v", threads)
	}

	// An unknown folder is reported usefully rather than silently defaulting.
	status, body := c.rpc(secret, "tools/call", map[string]any{
		"name": "list_threads", "arguments": map[string]any{"folder": "nonsense"},
	})
	if status != http.StatusOK {
		t.Fatalf("list_threads(nonsense): status %d", status)
	}
	if !rpcFailed(body) {
		t.Errorf("an unknown folder was accepted: %v", body)
	}
}

func TestMCPDraftsButNeverSends(t *testing.T) {
	c := newTestServer(t)
	c.signUp("mcp-draft@example.com")
	accountID := c.connectAccount("Work")
	secret, _ := c.createToken("Assistant", "30d", nil)

	draft := c.callTool(secret, "create_draft", map[string]any{
		"mail_account_id": accountID,
		"to":              []map[string]any{{"email": "customer@example.com", "name": "Customer"}},
		"subject":         "Following up",
		"body":            "Hello,\n\nJust checking in.\n\nThanks",
	})

	if status, _ := draft["status"].(string); status != "draft" {
		t.Errorf("new draft has status %q, want draft", status)
	}
	// The model is told, in the payload, that nothing was sent — this is the
	// single most likely thing for it to get wrong when reporting back.
	if note, _ := draft["note"].(string); !contains(note, "NOT been sent") {
		t.Errorf("draft note does not say it was not sent: %q", note)
	}

	draftID, _ := draft["id"].(string)
	if draftID == "" {
		t.Fatalf("no draft id in %v", draft)
	}

	updated := c.callTool(secret, "update_draft", map[string]any{
		"draft_id": draftID,
		"subject":  "Following up on your order",
	})
	if subject, _ := updated["subject"].(string); subject != "Following up on your order" {
		t.Errorf("update_draft left the subject as %q", subject)
	}

	// It really is in the mailbox, and really is still a draft.
	_, body := c.do(http.MethodGet, "/api/v1/drafts/"+draftID, nil)
	stored, _ := body["draft"].(map[string]any)
	if status, _ := stored["status"].(string); status != "draft" {
		t.Errorf("stored draft has status %q, want draft", status)
	}
}

func TestMCPHonoursATokenLimitedToOneAccount(t *testing.T) {
	c := newTestServer(t)
	c.signUp("mcp-scope@example.com")
	support := c.connectAccount("Support")
	personal := c.connectAccount("Personal")

	secret, _ := c.createToken("Support assistant", "30d", &support)

	// The restriction is applied in SQL, so it reaches MCP without this
	// package knowing anything about it.
	accounts := c.callTool(secret, "list_accounts", nil)
	list, _ := accounts["accounts"].([]any)
	if len(list) != 1 {
		t.Fatalf("a restricted token saw %d accounts, want 1", len(list))
	}
	if got, _ := list[0].(map[string]any)["id"].(string); got != support {
		t.Errorf("restricted token saw account %s, want %s", got, support)
	}

	// Naming the other account explicitly does not get round it either.
	status, body := c.rpc(secret, "tools/call", map[string]any{
		"name":      "create_draft",
		"arguments": map[string]any{"mail_account_id": personal, "subject": "Reaching"},
	})
	if status != http.StatusOK {
		t.Fatalf("create_draft: status %d", status)
	}
	if !rpcFailed(body) {
		t.Errorf("a restricted token drafted in another account: %v", body)
	}
}

func TestMCPRefusesARevokedToken(t *testing.T) {
	c := newTestServer(t)
	c.signUp("mcp-revoke@example.com")
	secret, tokenID := c.createToken("Assistant", "never", nil)

	if status, _ := c.rpc(secret, "tools/list", nil); status != http.StatusOK {
		t.Fatalf("fresh token: status %d, want 200", status)
	}

	if status, _ := c.do(http.MethodDelete, "/api/v1/api-tokens/"+tokenID, nil); status != http.StatusNoContent {
		t.Fatalf("revoke: status %d", status)
	}

	// Revocation reaches MCP for free: it is the same middleware.
	if status, _ := c.rpc(secret, "tools/list", nil); status != http.StatusUnauthorized {
		t.Errorf("revoked token on /mcp: status %d, want 401", status)
	}
}

// rpcFailed reports whether a call was rejected, either as a JSON-RPC protocol
// error (an unknown tool, a malformed request) or as a tool that ran and
// reported failure. Both are refusals; which one is not the caller's concern.
func rpcFailed(body map[string]any) bool {
	if _, ok := body["error"]; ok {
		return true
	}
	result, _ := body["result"].(map[string]any)
	isErr, _ := result["isError"].(bool)
	return isErr
}
