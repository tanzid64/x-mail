package compose_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/compose"
	"github.com/dark/x-mail/api/internal/storage"
	"github.com/dark/x-mail/api/internal/testsupport"
)

// recordingQueue captures what would have been enqueued.
type recordingQueue struct {
	mu    sync.Mutex
	items []uuid.UUID
	err   error
}

func (q *recordingQueue) EnqueueSendEmail(_ context.Context, messageID uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.items = append(q.items, messageID)
	return nil
}

func (q *recordingQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

type fixture struct {
	pool    *pgxpool.Pool
	repo    *compose.Repository
	service *compose.Service
	queue   *recordingQueue
	store   storage.Storage
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testsupport.Pool(t)

	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	queue := &recordingQueue{}
	repo := compose.NewRepository(pool)

	return &fixture{
		pool:    pool,
		repo:    repo,
		service: compose.NewService(repo, store, queue, slog.New(slog.NewTextHandler(io.Discard, nil))),
		queue:   queue,
		store:   store,
	}
}

// account is a user with a mail account, a verified domain and a sender address.
type account struct {
	userID    uuid.UUID
	accountID uuid.UUID
	addressID uuid.UUID
}

func (f *fixture) newAccount(t *testing.T, email string, domainStatus string) account {
	t.Helper()
	ctx := context.Background()

	hash, err := auth.HashPassword("a-very-good-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user, err := auth.NewRepository(f.pool).CreateUser(ctx, email, hash, "Tester")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	var accountID, domainID, addressID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO mail_accounts (user_id, name, provider, api_key_encrypted)
		 VALUES ($1, 'Work', 'resend', '\x00'::bytea) RETURNING id`, user.ID).Scan(&accountID); err != nil {
		t.Fatalf("create account: %v", err)
	}

	local, domain, _ := strings.Cut(email, "@")
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO domains (mail_account_id, domain, provider_domain_id, status)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		accountID, domain, "prov_"+domain, domainStatus).Scan(&domainID); err != nil {
		t.Fatalf("create domain: %v", err)
	}

	if err := f.pool.QueryRow(ctx,
		`INSERT INTO mail_addresses (domain_id, email, display_name, is_default)
		 VALUES ($1, $2, 'Tester', true) RETURNING id`,
		// Addresses are globally unique, so each account gets a distinct one.
		domainID, local+"@"+domain).Scan(&addressID); err != nil {
		t.Fatalf("create address: %v", err)
	}

	return account{userID: user.ID, accountID: accountID, addressID: addressID}
}

func (f *fixture) newDraft(t *testing.T, a account, to string) *compose.Draft {
	t.Helper()
	draft, err := f.service.CreateDraft(context.Background(), compose.CreateDraftParams{
		UserID:        a.userID,
		MailAccountID: a.accountID,
		FromAddressID: &a.addressID,
		To:            []compose.Address{{Email: to, Name: "Recipient"}},
		Subject:       "Hello",
		TextBody:      "Hello there",
		HTMLBody:      "<p>Hello there</p>",
	})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	return draft
}

func TestDraftLifecycle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.newAccount(t, "owner@company.test", "verified")

	draft := f.newDraft(t, a, "john@example.com")
	if draft.Status != compose.DraftStatusDraft {
		t.Errorf("status = %q, want draft", draft.Status)
	}

	// Autosave.
	subject := "Hello again"
	updated, err := f.service.UpdateDraft(ctx, a.userID, draft.ID, compose.UpdateDraftParams{Subject: &subject})
	if err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	if updated.Subject != "Hello again" {
		t.Errorf("subject = %q", updated.Subject)
	}

	list, err := f.service.ListDrafts(ctx, a.userID, nil)
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d drafts, want 1", len(list))
	}

	if err := f.service.DeleteDraft(ctx, a.userID, draft.ID); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if _, err := f.service.GetDraft(ctx, a.userID, draft.ID); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("GetDraft after delete = %v, want ErrNotFound", err)
	}
}

func TestDraftsAreInvisibleToOtherUsers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	alice := f.newAccount(t, "alice@alice.test", "verified")
	bob := f.newAccount(t, "bob@bob.test", "verified")

	draft := f.newDraft(t, alice, "john@example.com")

	if _, err := f.service.GetDraft(ctx, bob.userID, draft.ID); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("GetDraft = %v, want ErrNotFound", err)
	}
	subject := "Hijacked"
	if _, err := f.service.UpdateDraft(ctx, bob.userID, draft.ID, compose.UpdateDraftParams{Subject: &subject}); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("UpdateDraft = %v, want ErrNotFound", err)
	}
	if err := f.service.DeleteDraft(ctx, bob.userID, draft.ID); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("DeleteDraft = %v, want ErrNotFound", err)
	}
	if _, err := f.service.Send(ctx, bob.userID, draft.ID); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("Send = %v, want ErrNotFound", err)
	}

	// Bob cannot create a draft on Alice's account either.
	if _, err := f.service.CreateDraft(ctx, compose.CreateDraftParams{
		UserID: bob.userID, MailAccountID: alice.accountID,
	}); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("CreateDraft on another user's account = %v, want ErrNotFound", err)
	}

	if _, err := f.service.GetDraft(ctx, alice.userID, draft.ID); err != nil {
		t.Errorf("alice lost her own draft: %v", err)
	}
}

func TestSendTurnsADraftIntoAQueuedMessage(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.newAccount(t, "owner@company.test", "verified")
	draft := f.newDraft(t, a, "john@example.com")

	queued, err := f.service.Send(ctx, a.userID, draft.ID)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if f.queue.count() != 1 {
		t.Errorf("enqueued %d jobs, want 1", f.queue.count())
	}

	// The draft is gone; a message stands in its place.
	if _, err := f.service.GetDraft(ctx, a.userID, draft.ID); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("the draft should be consumed by sending, got %v", err)
	}

	outbound, err := f.repo.LoadOutbound(ctx, queued.MessageID)
	if err != nil {
		t.Fatalf("LoadOutbound: %v", err)
	}
	if outbound.Status != "queued" {
		t.Errorf("status = %q, want queued", outbound.Status)
	}
	if outbound.IdempotencyKey == "" {
		t.Error("no idempotency key was generated")
	}
	if !strings.HasPrefix(outbound.MessageID, "<") || !strings.Contains(outbound.MessageID, "@company.test") {
		t.Errorf("Message-ID = %q, want a proper id on the sending domain", outbound.MessageID)
	}
	if len(outbound.To) != 1 || outbound.To[0].Email != "john@example.com" {
		t.Errorf("recipients = %+v", outbound.To)
	}
	if outbound.SenderEmail != "owner@company.test" {
		t.Errorf("sender = %q", outbound.SenderEmail)
	}

	// It lands in Sent.
	var inSent bool
	if err := f.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM message_labels ml
			  JOIN labels l ON l.id = ml.label_id
			 WHERE ml.message_id = $1 AND l.system_key = 'SENT')`,
		queued.MessageID).Scan(&inSent); err != nil {
		t.Fatalf("check sent label: %v", err)
	}
	if !inSent {
		t.Error("the sent message is not in the Sent folder")
	}
}

// A double-click, or a retried request, must not send twice.
func TestSendingTheSameDraftTwiceIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.newAccount(t, "owner@company.test", "verified")
	draft := f.newDraft(t, a, "john@example.com")

	if _, err := f.service.Send(ctx, a.userID, draft.ID); err != nil {
		t.Fatalf("first Send: %v", err)
	}

	// The draft no longer exists, so the second attempt cannot queue anything.
	_, err := f.service.Send(ctx, a.userID, draft.ID)
	if !errors.Is(err, compose.ErrNotFound) && !errors.Is(err, compose.ErrDraftSending) {
		t.Errorf("second Send = %v, want not-found or already-sending", err)
	}
	if f.queue.count() != 1 {
		t.Errorf("enqueued %d jobs, want exactly 1", f.queue.count())
	}
}

func TestSendRefusesDraftsThatCannotGoOut(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	t.Run("no recipients", func(t *testing.T) {
		a := f.newAccount(t, "norecipients@company.test", "verified")
		draft, err := f.service.CreateDraft(ctx, compose.CreateDraftParams{
			UserID: a.userID, MailAccountID: a.accountID, FromAddressID: &a.addressID,
			Subject: "Nobody",
		})
		if err != nil {
			t.Fatalf("CreateDraft: %v", err)
		}
		if _, err := f.service.Send(ctx, a.userID, draft.ID); !errors.Is(err, compose.ErrNoRecipients) {
			t.Errorf("err = %v, want ErrNoRecipients", err)
		}
	})

	t.Run("no sender chosen", func(t *testing.T) {
		a := f.newAccount(t, "nosender@company.test", "verified")
		draft, err := f.service.CreateDraft(ctx, compose.CreateDraftParams{
			UserID: a.userID, MailAccountID: a.accountID,
			To: []compose.Address{{Email: "john@example.com"}},
		})
		if err != nil {
			t.Fatalf("CreateDraft: %v", err)
		}
		if _, err := f.service.Send(ctx, a.userID, draft.ID); !errors.Is(err, compose.ErrNoSender) {
			t.Errorf("err = %v, want ErrNoSender", err)
		}
	})

	// The provider would reject mail from an unverified domain, so the send is
	// stopped before it is queued rather than failing in the worker.
	t.Run("unverified sending domain", func(t *testing.T) {
		a := f.newAccount(t, "unverified@pending.test", "pending")
		draft := f.newDraft(t, a, "john@example.com")
		if _, err := f.service.Send(ctx, a.userID, draft.ID); !errors.Is(err, compose.ErrSenderNotUsable) {
			t.Errorf("err = %v, want ErrSenderNotUsable", err)
		}
	})
}

func TestReplyCarriesThreadingHeaders(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.newAccount(t, "owner@company.test", "verified")

	// An existing inbound message to reply to.
	var threadID, parentID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO threads (mail_account_id, subject) VALUES ($1, 'Original') RETURNING id`,
		a.accountID).Scan(&threadID); err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO messages (mail_account_id, thread_id, message_id, "references",
		                      sender_email, subject, direction, status)
		VALUES ($1, $2, '<parent@example.com>', ARRAY['<grandparent@example.com>'],
		        'john@example.com', 'Original', 'inbound', 'received')
		RETURNING id`, a.accountID, threadID).Scan(&parentID); err != nil {
		t.Fatalf("create parent message: %v", err)
	}

	draft, err := f.service.CreateDraft(ctx, compose.CreateDraftParams{
		UserID:        a.userID,
		MailAccountID: a.accountID,
		FromAddressID: &a.addressID,
		ThreadID:      &threadID,
		InReplyToID:   &parentID,
		To:            []compose.Address{{Email: "john@example.com"}},
		Subject:       "Re: Original",
		TextBody:      "Replying",
	})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	queued, err := f.service.Send(ctx, a.userID, draft.ID)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if queued.ThreadID != threadID {
		t.Error("the reply did not join its parent's thread")
	}

	outbound, err := f.repo.LoadOutbound(ctx, queued.MessageID)
	if err != nil {
		t.Fatalf("LoadOutbound: %v", err)
	}
	if outbound.InReplyTo == nil || *outbound.InReplyTo != "<parent@example.com>" {
		t.Errorf("In-Reply-To = %v, want the parent's Message-ID", outbound.InReplyTo)
	}
	// References must carry the parent's chain plus the parent itself, so the
	// recipient's client can rebuild the conversation.
	want := []string{"<grandparent@example.com>", "<parent@example.com>"}
	if len(outbound.References) != len(want) {
		t.Fatalf("References = %v, want %v", outbound.References, want)
	}
	for i, reference := range want {
		if outbound.References[i] != reference {
			t.Errorf("References[%d] = %q, want %q", i, outbound.References[i], reference)
		}
	}
}

func TestAttachmentsMoveFromDraftToMessageAndAreAuthorized(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	alice := f.newAccount(t, "alice@company.test", "verified")
	bob := f.newAccount(t, "bob@other.test", "verified")

	draft := f.newDraft(t, alice, "john@example.com")

	attachment, err := f.service.Upload(ctx, alice.userID, compose.UploadParams{
		DraftID:  draft.ID,
		Filename: "../../etc/report.pdf",
		MimeType: "text/html",
		Size:     11,
		Body:     strings.NewReader("attachment"),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// The path is stripped and the type derived from the extension, not from
	// what the client claimed.
	if attachment.Filename != "report.pdf" {
		t.Errorf("filename = %q, want it sanitised", attachment.Filename)
	}
	if attachment.MimeType != "application/pdf" {
		t.Errorf("mime type = %q, want it resolved from the extension", attachment.MimeType)
	}

	t.Run("bob cannot download or delete it", func(t *testing.T) {
		if _, _, err := f.service.Download(ctx, bob.userID, attachment.ID); !errors.Is(err, compose.ErrNotFound) {
			t.Errorf("Download = %v, want ErrNotFound", err)
		}
		if err := f.service.DeleteAttachment(ctx, bob.userID, attachment.ID); !errors.Is(err, compose.ErrNotFound) {
			t.Errorf("DeleteAttachment = %v, want ErrNotFound", err)
		}
	})

	reader, got, err := f.service.Download(ctx, alice.userID, attachment.ID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	body, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(body) != "attachment" {
		t.Errorf("body = %q", body)
	}
	if got.SizeBytes != int64(len("attachment")) {
		t.Errorf("size = %d", got.SizeBytes)
	}

	queued, err := f.service.Send(ctx, alice.userID, draft.ID)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The attachment now belongs to the message, and the bytes were not copied.
	outbound, err := f.repo.LoadOutbound(ctx, queued.MessageID)
	if err != nil {
		t.Fatalf("LoadOutbound: %v", err)
	}
	if len(outbound.Attachments) != 1 {
		t.Fatalf("message has %d attachments, want 1", len(outbound.Attachments))
	}
	if outbound.Attachments[0].StorageKey != attachment.StorageKey {
		t.Error("the attachment was re-uploaded rather than re-pointed")
	}

	// It is still downloadable, now through the message.
	if _, _, err := f.service.Download(ctx, alice.userID, attachment.ID); err != nil {
		t.Errorf("Download after send: %v", err)
	}
	if _, _, err := f.service.Download(ctx, bob.userID, attachment.ID); !errors.Is(err, compose.ErrNotFound) {
		t.Errorf("bob can download a sent attachment: %v", err)
	}
}

func TestUploadRejectsOversizedFiles(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.newAccount(t, "owner@company.test", "verified")
	draft := f.newDraft(t, a, "john@example.com")

	// Lying about the size must not get past the copy limit either.
	oversized := strings.NewReader(strings.Repeat("x", compose.MaxAttachmentBytes+1024))
	_, err := f.service.Upload(ctx, a.userID, compose.UploadParams{
		DraftID:  draft.ID,
		Filename: "big.bin",
		Size:     1,
		Body:     oversized,
	})
	if !errors.Is(err, compose.ErrAttachmentTooLarge) {
		t.Errorf("err = %v, want ErrAttachmentTooLarge", err)
	}
}
