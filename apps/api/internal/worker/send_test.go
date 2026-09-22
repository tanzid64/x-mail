package worker_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/compose"
	"github.com/dark/x-mail/api/internal/crypto"
	"github.com/dark/x-mail/api/internal/providers"
	"github.com/dark/x-mail/api/internal/storage"
	"github.com/dark/x-mail/api/internal/testsupport"
	"github.com/dark/x-mail/api/internal/worker"
)

// countingProvider records every send so the test can prove how many times the
// provider was actually called, and with which idempotency keys.
type countingProvider struct {
	mu   sync.Mutex
	sent []providers.Email
	err  error
}

func (p *countingProvider) Name() providers.Provider                { return providers.ProviderResend }
func (p *countingProvider) VerifyCredentials(context.Context) error { return nil }

func (p *countingProvider) Send(_ context.Context, email providers.Email) (*providers.SendResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	p.sent = append(p.sent, email)
	return &providers.SendResult{ProviderMessageID: "prov_" + uuid.NewString()}, nil
}

func (p *countingProvider) ListDomains(context.Context) ([]providers.Domain, error) { return nil, nil }
func (p *countingProvider) GetDomain(context.Context, string) (*providers.Domain, error) {
	return nil, nil
}

func (p *countingProvider) calls() []providers.Email {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]providers.Email{}, p.sent...)
}

type sendFixture struct {
	pool      *pgxpool.Pool
	compose   *compose.Repository
	handler   *worker.SendEmailHandler
	provider  *countingProvider
	userID    uuid.UUID
	accountID uuid.UUID
	addressID uuid.UUID
}

func newSendFixture(t *testing.T) *sendFixture {
	t.Helper()
	pool := testsupport.Pool(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	key := make([]byte, 32)
	sealer, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	provider := &countingProvider{}
	accountsService := accounts.NewService(
		accounts.NewRepository(pool), sealer,
		func(providers.Provider, string) (providers.MailProvider, error) { return provider, nil },
		logger,
	)

	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	composeRepo := compose.NewRepository(pool)

	// A user with an account, a verified domain and a sender address.
	hash, err := auth.HashPassword("a-very-good-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user, err := auth.NewRepository(pool).CreateUser(ctx, "sender@company.test", hash, "Sender")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	sealed, err := sealer.Seal("re_key")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	var accountID, domainID, addressID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO mail_accounts (user_id, name, provider, api_key_encrypted)
		 VALUES ($1, 'Work', 'resend', $2) RETURNING id`, user.ID, sealed).Scan(&accountID); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO domains (mail_account_id, domain, provider_domain_id, status)
		 VALUES ($1, 'company.test', 'prov_d', 'verified') RETURNING id`,
		accountID).Scan(&domainID); err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO mail_addresses (domain_id, email, display_name, is_default)
		 VALUES ($1, 'sender@company.test', 'Sender', true) RETURNING id`,
		domainID).Scan(&addressID); err != nil {
		t.Fatalf("create address: %v", err)
	}

	f := &sendFixture{
		pool:     pool,
		compose:  composeRepo,
		provider: provider,
		handler:  worker.NewSendEmailHandler(composeRepo, accountsService, store, logger),
		userID:   user.ID,
	}
	f.accountID, f.addressID = accountID, addressID
	return f
}

// queueMessage creates a draft and turns it into a queued message.
func (f *sendFixture) queueMessage(t *testing.T) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	draft, err := compose.NewService(f.compose, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).
		CreateDraft(ctx, compose.CreateDraftParams{
			UserID:        f.userID,
			MailAccountID: f.accountID,
			FromAddressID: &f.addressID,
			To:            []compose.Address{{Email: "john@example.com", Name: "John"}},
			Subject:       "Hello",
			TextBody:      "Hello there",
		})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	queued, err := f.compose.QueueDraft(ctx, f.userID, draft.ID)
	if err != nil {
		t.Fatalf("QueueDraft: %v", err)
	}
	return queued.MessageID
}

func (f *sendFixture) task(t *testing.T, messageID uuid.UUID) *asynq.Task {
	t.Helper()
	payload, err := json.Marshal(worker.SendEmailPayload{MessageID: messageID})
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return asynq.NewTask(worker.TaskSendEmail, payload)
}

func (f *sendFixture) status(t *testing.T, messageID uuid.UUID) (string, string) {
	t.Helper()
	var status, reason string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, failure_reason FROM messages WHERE id = $1`, messageID).Scan(&status, &reason); err != nil {
		t.Fatalf("read status: %v", err)
	}
	return status, reason
}

func TestSendDeliversAMessageAndRecordsIt(t *testing.T) {
	f := newSendFixture(t)
	ctx := context.Background()
	messageID := f.queueMessage(t)

	if err := f.handler.ProcessTask(ctx, f.task(t, messageID)); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	calls := f.provider.calls()
	if len(calls) != 1 {
		t.Fatalf("provider called %d times, want 1", len(calls))
	}
	if calls[0].From.Email != "sender@company.test" {
		t.Errorf("From = %q", calls[0].From.Email)
	}
	if calls[0].IdempotencyKey == "" {
		t.Error("the send carried no idempotency key")
	}
	if calls[0].Headers["Message-ID"] == "" {
		t.Error("the send carried no Message-ID header")
	}

	status, _ := f.status(t, messageID)
	if status != "sent" {
		t.Errorf("status = %q, want sent", status)
	}
}

// The plan's worker-retry scenario: a job that runs again after the message has
// already gone out must not send it a second time.
func TestRetryingACompletedSendDoesNothing(t *testing.T) {
	f := newSendFixture(t)
	ctx := context.Background()
	messageID := f.queueMessage(t)

	if err := f.handler.ProcessTask(ctx, f.task(t, messageID)); err != nil {
		t.Fatalf("first ProcessTask: %v", err)
	}
	if err := f.handler.ProcessTask(ctx, f.task(t, messageID)); err != nil {
		t.Fatalf("retry ProcessTask: %v", err)
	}

	if calls := f.provider.calls(); len(calls) != 1 {
		t.Fatalf("provider called %d times across a retry, want 1", len(calls))
	}
}

// If the worker dies after the provider accepted the message but before the
// result was recorded, the retry does call the provider again — carrying the
// same idempotency key, which is what lets the provider collapse the two into
// one delivery.
func TestRetryAfterACrashReusesTheIdempotencyKey(t *testing.T) {
	f := newSendFixture(t)
	ctx := context.Background()
	messageID := f.queueMessage(t)

	if err := f.handler.ProcessTask(ctx, f.task(t, messageID)); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	// Simulate the crash: the provider took it, but the success was never
	// written back.
	if _, err := f.pool.Exec(ctx,
		`UPDATE messages SET status = 'sending', provider_message_id = NULL, sent_at = NULL
		  WHERE id = $1`, messageID); err != nil {
		t.Fatalf("simulate crash: %v", err)
	}

	if err := f.handler.ProcessTask(ctx, f.task(t, messageID)); err != nil {
		t.Fatalf("retry ProcessTask: %v", err)
	}

	calls := f.provider.calls()
	if len(calls) != 2 {
		t.Fatalf("provider called %d times, want 2 (the crash is invisible to it)", len(calls))
	}
	if calls[0].IdempotencyKey != calls[1].IdempotencyKey {
		t.Errorf("idempotency keys differ across the retry: %q vs %q — the provider would send twice",
			calls[0].IdempotencyKey, calls[1].IdempotencyKey)
	}
	if calls[0].IdempotencyKey == "" {
		t.Error("no idempotency key was carried")
	}
}

func TestRetryableFailureIsReturnedSoAsynqRetries(t *testing.T) {
	f := newSendFixture(t)
	ctx := context.Background()
	messageID := f.queueMessage(t)

	f.provider.err = providers.NewError(providers.ProviderResend, providers.KindRateLimited, 429, "slow down", nil)

	err := f.handler.ProcessTask(ctx, f.task(t, messageID))
	if err == nil {
		t.Fatal("a retryable failure should be returned so the job is retried")
	}

	status, _ := f.status(t, messageID)
	if status == "failed" {
		t.Error("a throttled send should not be marked permanently failed")
	}
}

// A rejected API key will be rejected again, so retrying only delays telling
// the user.
func TestPermanentFailureStopsRetryingAndExplainsWhy(t *testing.T) {
	f := newSendFixture(t)
	ctx := context.Background()
	messageID := f.queueMessage(t)

	f.provider.err = providers.NewError(providers.ProviderResend, providers.KindInvalidRequest, 400,
		"domain is not verified", nil)

	if err := f.handler.ProcessTask(ctx, f.task(t, messageID)); err != nil {
		t.Fatalf("a permanent failure should complete the job, got %v", err)
	}

	status, reason := f.status(t, messageID)
	if status != "failed" {
		t.Errorf("status = %q, want failed", status)
	}
	if !strings.Contains(reason, "domain is not verified") {
		t.Errorf("failure_reason = %q, want it to explain the rejection", reason)
	}
}

func TestMissingMessageCompletesWithoutRetrying(t *testing.T) {
	f := newSendFixture(t)

	// A message deleted before its job ran.
	if err := f.handler.ProcessTask(context.Background(), f.task(t, uuid.New())); err != nil {
		t.Errorf("err = %v, want the job to complete quietly", err)
	}
	if calls := f.provider.calls(); len(calls) != 0 {
		t.Errorf("provider called %d times for a missing message", len(calls))
	}
}
