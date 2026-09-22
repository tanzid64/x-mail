package worker_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/mailbox"
	"github.com/dark/x-mail/api/internal/storage"
	"github.com/dark/x-mail/api/internal/testsupport"
	"github.com/dark/x-mail/api/internal/webhooks"
	"github.com/dark/x-mail/api/internal/worker"
)

type inboundFixture struct {
	pool      *pgxpool.Pool
	events    *webhooks.Repository
	mailbox   *mailbox.Repository
	handler   *worker.ProcessEventHandler
	userID    uuid.UUID
	accountID uuid.UUID
	address   string
}

func newInboundFixture(t *testing.T) *inboundFixture {
	t.Helper()
	pool := testsupport.Pool(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	hash, err := auth.HashPassword("a-very-good-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user, err := auth.NewRepository(pool).CreateUser(ctx, "owner@company.test", hash, "Owner")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	var accountID, domainID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO mail_accounts (user_id, name, provider, api_key_encrypted)
		 VALUES ($1, 'Support', 'resend', '\x00'::bytea) RETURNING id`, user.ID).Scan(&accountID); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO domains (mail_account_id, domain, provider_domain_id, status)
		 VALUES ($1, 'company.test', 'prov_d', 'verified') RETURNING id`,
		accountID).Scan(&domainID); err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO mail_addresses (domain_id, email, display_name, is_default)
		 VALUES ($1, 'support@company.test', 'Support', true)`, domainID); err != nil {
		t.Fatalf("create address: %v", err)
	}

	eventsRepo := webhooks.NewRepository(pool)
	mailboxRepo := mailbox.NewRepository(pool)

	return &inboundFixture{
		pool:    pool,
		events:  eventsRepo,
		mailbox: mailboxRepo,
		handler: worker.NewProcessEventHandler(
			eventsRepo, mailboxRepo, accounts.NewRepository(pool), store, logger,
		),
		userID:    user.ID,
		accountID: accountID,
		address:   "support@company.test",
	}
}

// deliver records a webhook and runs the job for it, the way the endpoint and
// worker do together.
func (f *inboundFixture) deliver(t *testing.T, providerEventID string, payload map[string]any) error {
	t.Helper()
	ctx := context.Background()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	stored, err := f.events.Record(ctx, f.accountID, "resend", providerEventID,
		payload["type"].(string), body)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if stored.Duplicate {
		// The endpoint would acknowledge and queue nothing.
		return nil
	}

	task, err := json.Marshal(worker.ProcessEventPayload{EventID: stored.ID})
	if err != nil {
		t.Fatalf("encode task: %v", err)
	}
	return f.handler.ProcessTask(ctx, asynq.NewTask(worker.TaskProcessEvent, task))
}

func receivedPayload(to, subject, messageID string) map[string]any {
	return map[string]any{
		"type":       "email.received",
		"created_at": time.Now().Format(time.RFC3339),
		"data": map[string]any{
			"email_id": "prov_" + uuid.NewString(),
			"from":     "John Smith <john@example.com>",
			"to":       []string{to},
			"subject":  subject,
			"html":     `<p>Hello</p><script>alert(1)</script>`,
			"text":     "Hello",
			"headers": []map[string]string{
				{"name": "Message-Id", "value": messageID},
			},
		},
	}
}

func (f *inboundFixture) inboxCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM messages WHERE mail_account_id = $1 AND direction = 'inbound'`,
		f.accountID).Scan(&count); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	return count
}

func TestInboundMailIsRoutedStoredAndSanitized(t *testing.T) {
	f := newInboundFixture(t)
	ctx := context.Background()

	if err := f.deliver(t, "evt_1", receivedPayload(f.address, "Payment issue", "<a@example.com>")); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	if got := f.inboxCount(t); got != 1 {
		t.Fatalf("stored %d inbound messages, want 1", got)
	}

	var subject, html, sanitized, sender string
	if err := f.pool.QueryRow(ctx,
		`SELECT subject, html_body, html_sanitized, sender_email FROM messages
		  WHERE mail_account_id = $1`, f.accountID).Scan(&subject, &html, &sanitized, &sender); err != nil {
		t.Fatalf("read message: %v", err)
	}

	if subject != "Payment issue" {
		t.Errorf("subject = %q", subject)
	}
	if sender != "john@example.com" {
		t.Errorf("sender = %q, want the address parsed out of the display name", sender)
	}
	// The original is kept as received, so a sanitiser fix can be re-applied.
	if !strings.Contains(html, "<script>") {
		t.Error("the original body should be stored untouched")
	}
	// What renders must not be.
	if strings.Contains(strings.ToLower(sanitized), "<script") {
		t.Errorf("the sanitised body still contains a script: %s", sanitized)
	}
	if !strings.Contains(sanitized, "Hello") {
		t.Errorf("sanitising removed the message text: %s", sanitized)
	}

	// It lands in the inbox.
	var inInbox bool
	if err := f.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM messages m
			  JOIN message_labels ml ON ml.message_id = m.id
			  JOIN labels l ON l.id = ml.label_id
			 WHERE m.mail_account_id = $1 AND l.system_key = 'INBOX')`,
		f.accountID).Scan(&inInbox); err != nil {
		t.Fatalf("check inbox label: %v", err)
	}
	if !inInbox {
		t.Error("the received message is not in the inbox")
	}
}

// The plan's duplicate-webhook scenario: providers redeliver, and the same
// webhook arriving twice must create exactly one message.
func TestTheSameWebhookDeliveredTwiceCreatesOneMessage(t *testing.T) {
	f := newInboundFixture(t)
	payload := receivedPayload(f.address, "Only once", "<dup@example.com>")

	for range 3 {
		if err := f.deliver(t, "evt_duplicate", payload); err != nil {
			t.Fatalf("deliver: %v", err)
		}
	}

	if got := f.inboxCount(t); got != 1 {
		t.Fatalf("three deliveries produced %d messages, want 1", got)
	}
}

// Re-running the job for an already-processed event must also be a no-op, which
// is what covers an Asynq retry rather than a provider redelivery.
func TestReprocessingAnEventDoesNothing(t *testing.T) {
	f := newInboundFixture(t)
	ctx := context.Background()

	body, _ := json.Marshal(receivedPayload(f.address, "Once only", "<retry@example.com>"))
	stored, err := f.events.Record(ctx, f.accountID, "resend", "evt_retry", "email.received", body)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	task, _ := json.Marshal(worker.ProcessEventPayload{EventID: stored.ID})
	asynqTask := asynq.NewTask(worker.TaskProcessEvent, task)

	for range 3 {
		if err := f.handler.ProcessTask(ctx, asynqTask); err != nil {
			t.Fatalf("ProcessTask: %v", err)
		}
	}

	if got := f.inboxCount(t); got != 1 {
		t.Fatalf("three job runs produced %d messages, want 1", got)
	}
}

func TestInboundRepliesJoinTheirThread(t *testing.T) {
	f := newInboundFixture(t)
	ctx := context.Background()

	if err := f.deliver(t, "evt_root", receivedPayload(f.address, "Original", "<root@example.com>")); err != nil {
		t.Fatalf("deliver root: %v", err)
	}

	reply := receivedPayload(f.address, "Re: Original", "<reply@example.com>")
	data := reply["data"].(map[string]any)
	data["headers"] = []map[string]string{
		{"name": "Message-Id", "value": "<reply@example.com>"},
		{"name": "In-Reply-To", "value": "<root@example.com>"},
		{"name": "References", "value": "<root@example.com>"},
	}

	if err := f.deliver(t, "evt_reply", reply); err != nil {
		t.Fatalf("deliver reply: %v", err)
	}

	var threads int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM threads WHERE mail_account_id = $1`, f.accountID).Scan(&threads); err != nil {
		t.Fatalf("count threads: %v", err)
	}
	if threads != 1 {
		t.Fatalf("got %d threads, want the reply to join the original", threads)
	}

	var messageCount, unread int
	if err := f.pool.QueryRow(ctx,
		`SELECT message_count, unread_count FROM threads WHERE mail_account_id = $1`,
		f.accountID).Scan(&messageCount, &unread); err != nil {
		t.Fatalf("read thread: %v", err)
	}
	if messageCount != 2 || unread != 2 {
		t.Errorf("thread has %d messages / %d unread, want 2 / 2", messageCount, unread)
	}
}

func TestMailForAnUnknownAddressIsRecordedNotStored(t *testing.T) {
	f := newInboundFixture(t)
	ctx := context.Background()

	if err := f.deliver(t, "evt_unknown",
		receivedPayload("nobody@company.test", "Lost", "<lost@example.com>")); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	if got := f.inboxCount(t); got != 0 {
		t.Errorf("stored %d messages for an unknown recipient, want 0", got)
	}

	// The event is still recorded and marked processed, so it is diagnosable
	// and never retried forever.
	var processed bool
	var reason string
	if err := f.pool.QueryRow(ctx,
		`SELECT processed_at IS NOT NULL, process_error FROM email_events WHERE provider_event_id = 'evt_unknown'`,
	).Scan(&processed, &reason); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if !processed {
		t.Error("the event was left unprocessed and would be retried forever")
	}
	if reason == "" {
		t.Error("no reason was recorded for the undeliverable message")
	}
}

func TestInboundAttachmentsAreStored(t *testing.T) {
	f := newInboundFixture(t)
	ctx := context.Background()

	payload := receivedPayload(f.address, "With a file", "<file@example.com>")
	payload["data"].(map[string]any)["attachments"] = []map[string]any{
		{
			"filename":     "../../etc/report.pdf",
			"content_type": "application/pdf",
			"content":      base64.StdEncoding.EncodeToString([]byte("pdf bytes")),
		},
	}

	if err := f.deliver(t, "evt_attach", payload); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	var filename, storageKey string
	var size int64
	var hasAttachments bool
	if err := f.pool.QueryRow(ctx, `
		SELECT a.filename, a.storage_key, a.size_bytes, m.has_attachments
		  FROM attachments a JOIN messages m ON m.id = a.message_id
		 WHERE m.mail_account_id = $1`, f.accountID).Scan(&filename, &storageKey, &size, &hasAttachments); err != nil {
		t.Fatalf("read attachment: %v", err)
	}

	if filename != "report.pdf" {
		t.Errorf("filename = %q, want the path stripped", filename)
	}
	if strings.Contains(storageKey, "..") {
		t.Errorf("storage key contains traversal: %q", storageKey)
	}
	if size != int64(len("pdf bytes")) {
		t.Errorf("size = %d", size)
	}
	if !hasAttachments {
		t.Error("the message was not flagged as carrying an attachment")
	}
}

func TestDeliveryEventsUpdateASentMessage(t *testing.T) {
	f := newInboundFixture(t)
	ctx := context.Background()

	// A message we sent, as the worker would have recorded it.
	var threadID, messageID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO threads (mail_account_id, subject) VALUES ($1, 'Outbound') RETURNING id`,
		f.accountID).Scan(&threadID); err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO messages (mail_account_id, thread_id, sender_email, subject,
		                      direction, status, provider_message_id)
		VALUES ($1, $2, 'support@company.test', 'Outbound', 'outbound', 'sent', 'prov_sent_1')
		RETURNING id`, f.accountID, threadID).Scan(&messageID); err != nil {
		t.Fatalf("create message: %v", err)
	}

	event := func(kind string) map[string]any {
		return map[string]any{
			"type":       kind,
			"created_at": time.Now().Format(time.RFC3339),
			"data":       map[string]any{"email_id": "prov_sent_1"},
		}
	}

	if err := f.deliver(t, "evt_delivered", event("email.delivered")); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	var status string
	var deliveredAt *time.Time
	if err := f.pool.QueryRow(ctx,
		`SELECT status, delivered_at FROM messages WHERE id = $1`, messageID).Scan(&status, &deliveredAt); err != nil {
		t.Fatalf("read message: %v", err)
	}
	if status != "delivered" {
		t.Errorf("status = %q, want delivered", status)
	}
	if deliveredAt == nil {
		t.Error("delivered_at was not recorded")
	}

	// Providers reorder events; a late "sent" must not walk the status back.
	if err := f.deliver(t, "evt_sent_late", event("email.sent")); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if err := f.pool.QueryRow(ctx,
		`SELECT status FROM messages WHERE id = $1`, messageID).Scan(&status); err != nil {
		t.Fatalf("read message: %v", err)
	}
	if status != "delivered" {
		t.Errorf("a late 'sent' event regressed the status to %q", status)
	}

	// A bounce is terminal and does move it on.
	if err := f.deliver(t, "evt_bounced", event("email.bounced")); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if err := f.pool.QueryRow(ctx,
		`SELECT status FROM messages WHERE id = $1`, messageID).Scan(&status); err != nil {
		t.Fatalf("read message: %v", err)
	}
	if status != "bounced" {
		t.Errorf("status = %q, want bounced", status)
	}
}
