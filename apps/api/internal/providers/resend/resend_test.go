package resend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dark/x-mail/api/internal/providers"
)

// stub spins up a fake Resend that records the last request it received.
type stub struct {
	server  *httptest.Server
	path    string
	method  string
	headers http.Header
	body    map[string]any
}

func newStub(t *testing.T, status int, response string) *stub {
	t.Helper()
	s := &stub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.path = r.URL.Path
		s.method = r.Method
		s.headers = r.Header.Clone()
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &s.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *stub) client() *Client {
	return New("re_test_key", WithBaseURL(s.server.URL))
}

func TestSendFormatsMessageAndReturnsProviderID(t *testing.T) {
	s := newStub(t, http.StatusOK, `{"id":"prov_abc123"}`)

	result, err := s.client().Send(context.Background(), providers.Email{
		From:           providers.Address{Email: "dark@company.com", Name: "Dark"},
		To:             []providers.Address{{Email: "john@example.com", Name: "John"}},
		Cc:             []providers.Address{{Email: "cc@example.com"}},
		Subject:        "Project update",
		HTML:           "<p>Hello</p>",
		Text:           "Hello",
		Headers:        map[string]string{"In-Reply-To": "<prev@example.com>"},
		IdempotencyKey: "job-42",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.ProviderMessageID != "prov_abc123" {
		t.Errorf("ProviderMessageID = %q, want prov_abc123", result.ProviderMessageID)
	}

	if s.method != http.MethodPost || s.path != "/emails" {
		t.Errorf("called %s %s, want POST /emails", s.method, s.path)
	}
	if got := s.headers.Get("Authorization"); got != "Bearer re_test_key" {
		t.Errorf("Authorization = %q", got)
	}
	// The idempotency key is what stops a retried job sending twice.
	if got := s.headers.Get("Idempotency-Key"); got != "job-42" {
		t.Errorf("Idempotency-Key = %q, want job-42", got)
	}
	if got := s.body["from"]; got != "Dark <dark@company.com>" {
		t.Errorf("from = %v, want RFC 5322 form", got)
	}
	if to, ok := s.body["to"].([]any); !ok || len(to) != 1 || to[0] != "John <john@example.com>" {
		t.Errorf("to = %v", s.body["to"])
	}
	if _, present := s.body["bcc"]; present {
		t.Error("empty bcc should be omitted from the request")
	}
}

// The worker decides between retrying and permanently failing a message purely
// from these classifications, so each one matters.
func TestSendClassifiesFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantKind  providers.ErrorKind
		wantRetry bool
	}{
		{"bad api key", http.StatusUnauthorized, `{"message":"API key is invalid"}`, providers.KindAuth, false},
		{"forbidden", http.StatusForbidden, `{"message":"not allowed"}`, providers.KindAuth, false},
		{"unverified sender", http.StatusBadRequest, `{"message":"domain is not verified"}`, providers.KindInvalidRequest, false},
		{"throttled", http.StatusTooManyRequests, `{"message":"slow down"}`, providers.KindRateLimited, true},
		{"provider outage", http.StatusBadGateway, `{"message":"upstream"}`, providers.KindTransient, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t, tc.status, tc.body)

			_, err := s.client().Send(context.Background(), providers.Email{
				From:    providers.Address{Email: "dark@company.com"},
				To:      []providers.Address{{Email: "john@example.com"}},
				Subject: "Hi",
			})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !providers.IsKind(err, tc.wantKind) {
				t.Errorf("kind = %v, want %s", err, tc.wantKind)
			}
			if providers.IsRetryable(err) != tc.wantRetry {
				t.Errorf("Retryable = %v, want %v", providers.IsRetryable(err), tc.wantRetry)
			}
		})
	}
}

func TestListDomainsMapsStatuses(t *testing.T) {
	s := newStub(t, http.StatusOK, `{"data":[
		{"id":"d1","name":"company.com","status":"verified","region":"us-east-1"},
		{"id":"d2","name":"pending.com","status":"not_started","region":"us-east-1"},
		{"id":"d3","name":"broken.com","status":"failed","region":"us-east-1"}
	]}`)

	domains, err := s.client().ListDomains(context.Background())
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(domains) != 3 {
		t.Fatalf("got %d domains, want 3", len(domains))
	}

	want := []providers.DomainStatus{providers.DomainVerified, providers.DomainPending, providers.DomainFailed}
	for i, status := range want {
		if domains[i].Status != status {
			t.Errorf("domain %d status = %q, want %q", i, domains[i].Status, status)
		}
	}
}

func TestGetDomainReturnsDNSRecords(t *testing.T) {
	s := newStub(t, http.StatusOK, `{
		"id":"d1","name":"company.com","status":"pending","region":"us-east-1",
		"records":[{"record":"SPF","name":"send","type":"TXT","ttl":"Auto","status":"pending","value":"v=spf1 include:amazonses.com ~all"}]
	}`)

	domain, err := s.client().GetDomain(context.Background(), "d1")
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if s.path != "/domains/d1" {
		t.Errorf("path = %q, want /domains/d1", s.path)
	}
	if len(domain.Records) != 1 || domain.Records[0].Type != "TXT" {
		t.Fatalf("records = %+v", domain.Records)
	}
}

func TestVerifyCredentialsReportsBadKey(t *testing.T) {
	ok := newStub(t, http.StatusOK, `{"data":[]}`)
	if err := ok.client().VerifyCredentials(context.Background()); err != nil {
		t.Errorf("valid credentials reported an error: %v", err)
	}

	bad := newStub(t, http.StatusUnauthorized, `{"message":"API key is invalid"}`)
	err := bad.client().VerifyCredentials(context.Background())
	if !providers.IsKind(err, providers.KindAuth) {
		t.Errorf("err = %v, want an auth error", err)
	}
}
