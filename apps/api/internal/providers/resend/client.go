// Package resend implements providers.MailProvider on top of the Resend HTTP
// API.
//
// Everything Resend-specific — request shapes, status-code semantics, field
// names — is confined to this package. Callers see only the provider-agnostic
// types.
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/dark/x-mail/api/internal/providers"
)

const (
	defaultBaseURL = "https://api.resend.com"
	requestTimeout = 30 * time.Second
	// maxErrorBody caps how much of a failure response is read, so a provider
	// returning an HTML error page cannot blow up a log line.
	maxErrorBody = 8 << 10
)

// Client talks to Resend with one account's API key. It is safe for concurrent
// use.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// Option customises a Client, mainly so tests can point it at a stub server.
type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Name() providers.Provider { return providers.ProviderResend }

// VerifyCredentials makes the cheapest authenticated call available. Listing
// domains is read-only and works for keys of any permission level.
func (c *Client) VerifyCredentials(ctx context.Context) error {
	_, err := c.ListDomains(ctx)
	return err
}

// errorResponse is Resend's failure envelope.
type errorResponse struct {
	StatusCode int    `json:"statusCode"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

// do performs a request and decodes the response into out, translating any
// failure into a classified providers.Error.
func (c *Client) do(ctx context.Context, method, path string, body any, headers map[string]string, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return providers.NewError(providers.ProviderResend, providers.KindInvalidRequest, 0,
				"could not encode request", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return providers.NewError(providers.ProviderResend, providers.KindTransient, 0,
			"could not build request", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Network failures, timeouts and cancellations are all worth retrying.
		return providers.NewError(providers.ProviderResend, providers.KindTransient, 0,
			"request to resend failed", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return c.responseError(resp)
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return providers.NewError(providers.ProviderResend, providers.KindTransient, resp.StatusCode,
			"could not decode response from resend", err)
	}
	return nil
}

// responseError maps an HTTP failure onto the provider error kinds, which is
// what decides whether the worker retries the job or fails the message.
func (c *Client) responseError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

	message := string(bytes.TrimSpace(raw))
	var decoded errorResponse
	if err := json.Unmarshal(raw, &decoded); err == nil && decoded.Message != "" {
		message = decoded.Message
	}
	if message == "" {
		message = resp.Status
	}

	var kind providers.ErrorKind
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		kind = providers.KindAuth
	case resp.StatusCode == http.StatusNotFound:
		kind = providers.KindNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		kind = providers.KindRateLimited
	case resp.StatusCode >= 500:
		kind = providers.KindTransient
	default:
		// Everything else in the 4xx range is our fault and will fail again.
		kind = providers.KindInvalidRequest
	}

	return providers.NewError(providers.ProviderResend, kind, resp.StatusCode, message, nil)
}

// Compile-time guard that Client keeps satisfying the interface as it evolves.
var _ providers.MailProvider = (*Client)(nil)
