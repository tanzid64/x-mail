package mailbox_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/mailbox"
	"github.com/dark/x-mail/api/internal/testsupport"
)

type fixture struct {
	pool    *pgxpool.Pool
	repo    *mailbox.Repository
	service *mailbox.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testsupport.Pool(t)
	repo := mailbox.NewRepository(pool)
	return &fixture{
		pool:    pool,
		repo:    repo,
		service: mailbox.NewService(repo, accounts.NewRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil))),
	}
}

// newUserWithAccount creates a user and a mail account directly. The account
// insert fires the trigger that creates the system labels.
func (f *fixture) newUserWithAccount(t *testing.T, email, accountName string) (uuid.UUID, uuid.UUID) {
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

	var accountID uuid.UUID
	err = f.pool.QueryRow(ctx,
		`INSERT INTO mail_accounts (user_id, name, provider, api_key_encrypted)
		 VALUES ($1, $2, 'resend', '\x00'::bytea) RETURNING id`,
		user.ID, accountName).Scan(&accountID)
	if err != nil {
		t.Fatalf("create mail account: %v", err)
	}
	return user.ID, accountID
}

// deliver puts an inbound message in a folder.
func (f *fixture) deliver(t *testing.T, accountID uuid.UUID, subject, from string, label string, age time.Duration) *mailbox.Message {
	t.Helper()
	receivedAt := time.Now().Add(-age)
	messageID := fmt.Sprintf("<%s@test>", uuid.NewString())

	message, err := f.repo.InsertMessage(context.Background(), mailbox.NewMessage{
		MailAccountID: accountID,
		MessageID:     &messageID,
		SenderName:    "Sender",
		SenderEmail:   from,
		Subject:       subject,
		TextBody:      "Body of " + subject,
		To:            []mailbox.Recipient{{Email: "me@company.com", Type: "to"}},
		Direction:     mailbox.DirectionInbound,
		Status:        mailbox.StatusReceived,
		SystemLabels:  []string{label},
		ReceivedAt:    &receivedAt,
	})
	if err != nil {
		t.Fatalf("InsertMessage(%s): %v", subject, err)
	}
	return message
}

func TestSystemLabelsExistForEveryNewAccount(t *testing.T) {
	f := newFixture(t)
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")

	labels, err := f.service.ListLabels(context.Background(), userID, &accountID)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}

	found := map[string]bool{}
	for _, label := range labels {
		if label.SystemKey != nil {
			found[*label.SystemKey] = true
		}
	}
	for _, key := range []string{"INBOX", "SENT", "DRAFT", "ARCHIVE", "SPAM", "TRASH"} {
		if !found[key] {
			t.Errorf("system label %s was not created", key)
		}
	}
}

func TestThreadAggregatesTrackTheirMessages(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")

	root := f.deliver(t, accountID, "Project update", "john@example.com", mailbox.LabelInbox, time.Hour)

	// A reply that names its parent joins the same conversation.
	replyID := "<reply@test>"
	inReplyTo := *root.MessageID
	if _, err := f.repo.InsertMessage(ctx, mailbox.NewMessage{
		MailAccountID: accountID,
		MessageID:     &replyID,
		InReplyTo:     &inReplyTo,
		References:    []string{*root.MessageID},
		SenderName:    "John",
		SenderEmail:   "john@example.com",
		Subject:       "Re: Project update",
		TextBody:      "One more thing",
		Direction:     mailbox.DirectionInbound,
		Status:        mailbox.StatusReceived,
		SystemLabels:  []string{mailbox.LabelInbox},
	}); err != nil {
		t.Fatalf("InsertMessage(reply): %v", err)
	}

	page, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: mailbox.FolderInbox})
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(page.Threads) != 1 {
		t.Fatalf("got %d threads, want 1 — the reply did not join its parent", len(page.Threads))
	}

	thread := page.Threads[0]
	if thread.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2", thread.MessageCount)
	}
	if thread.UnreadCount != 2 {
		t.Errorf("unread_count = %d, want 2", thread.UnreadCount)
	}
	if thread.Subject != "Project update" {
		t.Errorf("subject = %q, want the first message's subject", thread.Subject)
	}
	if thread.Snippet != "One more thing" {
		t.Errorf("snippet = %q, want the newest message's preview", thread.Snippet)
	}
	if len(thread.Participants) != 1 {
		t.Errorf("participants = %v, want one distinct sender", thread.Participants)
	}
}

func TestFoldersShowOnlyTheirOwnThreads(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")

	f.deliver(t, accountID, "In the inbox", "a@example.com", mailbox.LabelInbox, time.Hour)
	f.deliver(t, accountID, "Archived", "b@example.com", mailbox.LabelArchive, 2*time.Hour)
	f.deliver(t, accountID, "Junk", "c@example.com", mailbox.LabelSpam, 3*time.Hour)
	f.deliver(t, accountID, "Discarded", "d@example.com", mailbox.LabelTrash, 4*time.Hour)

	for folder, want := range map[mailbox.Folder]string{
		mailbox.FolderInbox:   "In the inbox",
		mailbox.FolderArchive: "Archived",
		mailbox.FolderSpam:    "Junk",
		mailbox.FolderTrash:   "Discarded",
	} {
		page, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: folder})
		if err != nil {
			t.Fatalf("ListThreads(%s): %v", folder, err)
		}
		if len(page.Threads) != 1 {
			t.Errorf("%s has %d threads, want 1", folder, len(page.Threads))
			continue
		}
		if page.Threads[0].Subject != want {
			t.Errorf("%s shows %q, want %q", folder, page.Threads[0].Subject, want)
		}
	}
}

func TestActionsMoveThreadsBetweenFolders(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")
	message := f.deliver(t, accountID, "Moves around", "a@example.com", mailbox.LabelInbox, time.Hour)
	threadID := message.ThreadID

	inFolder := func(folder mailbox.Folder) bool {
		t.Helper()
		page, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: folder})
		if err != nil {
			t.Fatalf("ListThreads(%s): %v", folder, err)
		}
		return len(page.Threads) == 1
	}

	steps := []struct {
		action mailbox.Action
		lands  mailbox.Folder
		leaves mailbox.Folder
	}{
		{mailbox.ActionArchive, mailbox.FolderArchive, mailbox.FolderInbox},
		{mailbox.ActionInbox, mailbox.FolderInbox, mailbox.FolderArchive},
		{mailbox.ActionSpam, mailbox.FolderSpam, mailbox.FolderInbox},
		{mailbox.ActionNotSpam, mailbox.FolderInbox, mailbox.FolderSpam},
		{mailbox.ActionTrash, mailbox.FolderTrash, mailbox.FolderInbox},
		{mailbox.ActionRestore, mailbox.FolderInbox, mailbox.FolderTrash},
	}

	for _, step := range steps {
		affected, err := f.service.Apply(ctx, userID, step.action, []uuid.UUID{threadID}, nil)
		if err != nil {
			t.Fatalf("Apply(%s): %v", step.action, err)
		}
		if affected != 1 {
			t.Fatalf("Apply(%s) affected %d threads, want 1", step.action, affected)
		}
		if !inFolder(step.lands) {
			t.Errorf("after %s the thread is not in %s", step.action, step.lands)
		}
		// Placement is exclusive: it must have left the previous folder.
		if inFolder(step.leaves) {
			t.Errorf("after %s the thread is still in %s", step.action, step.leaves)
		}
	}
}

func TestReadStarAndDeleteActions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")
	message := f.deliver(t, accountID, "Flags", "a@example.com", mailbox.LabelInbox, time.Hour)
	threadID := message.ThreadID

	thread := func() mailbox.Thread {
		t.Helper()
		page, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: mailbox.FolderInbox})
		if err != nil {
			t.Fatalf("ListThreads: %v", err)
		}
		if len(page.Threads) == 0 {
			t.Fatal("thread disappeared")
		}
		return page.Threads[0]
	}

	if thread().UnreadCount != 1 {
		t.Error("a delivered message should start unread")
	}

	if _, err := f.service.Apply(ctx, userID, mailbox.ActionRead, []uuid.UUID{threadID}, nil); err != nil {
		t.Fatalf("Apply(read): %v", err)
	}
	if thread().UnreadCount != 0 {
		t.Error("marking read did not clear the unread count")
	}

	if _, err := f.service.Apply(ctx, userID, mailbox.ActionStar, []uuid.UUID{threadID}, nil); err != nil {
		t.Fatalf("Apply(star): %v", err)
	}
	if !thread().IsStarred {
		t.Error("starring did not reach the thread summary")
	}

	starred, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: mailbox.FolderStarred})
	if err != nil {
		t.Fatalf("ListThreads(starred): %v", err)
	}
	if len(starred.Threads) != 1 {
		t.Error("the starred folder should show the starred thread")
	}

	// A starred thread in the trash must not keep appearing under Starred.
	if _, err := f.service.Apply(ctx, userID, mailbox.ActionTrash, []uuid.UUID{threadID}, nil); err != nil {
		t.Fatalf("Apply(trash): %v", err)
	}
	starred, err = f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: mailbox.FolderStarred})
	if err != nil {
		t.Fatalf("ListThreads(starred): %v", err)
	}
	if len(starred.Threads) != 0 {
		t.Error("a trashed thread should not appear under Starred")
	}

	if _, err := f.service.Apply(ctx, userID, mailbox.ActionDelete, []uuid.UUID{threadID}, nil); err != nil {
		t.Fatalf("Apply(delete): %v", err)
	}
	trash, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: mailbox.FolderTrash})
	if err != nil {
		t.Fatalf("ListThreads(trash): %v", err)
	}
	if len(trash.Threads) != 0 {
		t.Error("the thread should be gone after a permanent delete")
	}
}

func TestBulkActionAppliesToEverySelectedThread(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")

	ids := make([]uuid.UUID, 0, 3)
	for i := range 3 {
		message := f.deliver(t, accountID, fmt.Sprintf("Bulk %d", i), "a@example.com", mailbox.LabelInbox,
			time.Duration(i+1)*time.Hour)
		ids = append(ids, message.ThreadID)
	}

	affected, err := f.service.Apply(ctx, userID, mailbox.ActionArchive, ids, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if affected != 3 {
		t.Errorf("affected = %d, want 3", affected)
	}

	inbox, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{Folder: mailbox.FolderInbox})
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(inbox.Threads) != 0 {
		t.Errorf("%d threads left in the inbox, want 0", len(inbox.Threads))
	}
}

// The plan's central mailbox guarantee: a message in one account must never
// surface in another, even for the same user.
func TestMailIsScopedToItsAccountAndOwner(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	alice, aliceWork := f.newUserWithAccount(t, "alice@example.com", "Work")
	_, alicePersonal := f.newUserWithAccount(t, "alice-second@example.com", "Personal")
	// Move the second account under Alice so she owns two.
	if _, err := f.pool.Exec(ctx,
		`UPDATE mail_accounts SET user_id = $1 WHERE id = $2`, alice, alicePersonal); err != nil {
		t.Fatalf("reassign account: %v", err)
	}

	bob, bobWork := f.newUserWithAccount(t, "bob@example.com", "Bob Work")

	workMessage := f.deliver(t, aliceWork, "Work mail", "client@example.com", mailbox.LabelInbox, time.Hour)
	f.deliver(t, alicePersonal, "Personal mail", "friend@example.com", mailbox.LabelInbox, 2*time.Hour)
	f.deliver(t, bobWork, "Bob's mail", "someone@example.com", mailbox.LabelInbox, time.Hour)

	t.Run("unified inbox spans only the owner's accounts", func(t *testing.T) {
		page, err := f.service.ListThreads(ctx, alice, mailbox.ListRequest{Folder: mailbox.FolderInbox})
		if err != nil {
			t.Fatalf("ListThreads: %v", err)
		}
		if len(page.Threads) != 2 {
			t.Fatalf("alice sees %d threads, want her 2", len(page.Threads))
		}
		for _, thread := range page.Threads {
			if thread.Subject == "Bob's mail" {
				t.Error("bob's mail leaked into alice's unified inbox")
			}
		}
	})

	t.Run("a single-account view excludes the other account", func(t *testing.T) {
		page, err := f.service.ListThreads(ctx, alice, mailbox.ListRequest{
			Folder: mailbox.FolderInbox, AccountID: &aliceWork,
		})
		if err != nil {
			t.Fatalf("ListThreads: %v", err)
		}
		if len(page.Threads) != 1 || page.Threads[0].Subject != "Work mail" {
			t.Errorf("work view = %+v, want only the work thread", page.Threads)
		}
	})

	t.Run("bob cannot scope to alice's account", func(t *testing.T) {
		if _, err := f.service.ListThreads(ctx, bob, mailbox.ListRequest{
			Folder: mailbox.FolderInbox, AccountID: &aliceWork,
		}); !errors.Is(err, mailbox.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("bob cannot read or change alice's thread", func(t *testing.T) {
		if _, err := f.service.GetThread(ctx, bob, workMessage.ThreadID, false); !errors.Is(err, mailbox.ErrNotFound) {
			t.Errorf("GetThread = %v, want ErrNotFound", err)
		}

		affected, err := f.service.Apply(ctx, bob, mailbox.ActionTrash, []uuid.UUID{workMessage.ThreadID}, nil)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if affected != 0 {
			t.Errorf("bob affected %d of alice's threads, want 0", affected)
		}

		// Alice's thread is untouched.
		page, err := f.service.ListThreads(ctx, alice, mailbox.ListRequest{
			Folder: mailbox.FolderInbox, AccountID: &aliceWork,
		})
		if err != nil {
			t.Fatalf("ListThreads: %v", err)
		}
		if len(page.Threads) != 1 {
			t.Error("alice's thread was moved by another user")
		}
	})
}

func TestOpeningAThreadMarksItRead(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")
	message := f.deliver(t, accountID, "Unread", "a@example.com", mailbox.LabelInbox, time.Hour)

	detail, err := f.service.GetThread(ctx, userID, message.ThreadID, true)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if detail.Thread.UnreadCount != 0 {
		t.Errorf("unread_count = %d, want 0 after opening", detail.Thread.UnreadCount)
	}
	if len(detail.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(detail.Messages))
	}
	if len(detail.Messages[0].Recipients) != 1 {
		t.Error("recipients were not loaded with the message")
	}

	// Peeking must not change state.
	if _, err := f.service.Apply(ctx, userID, mailbox.ActionUnread, []uuid.UUID{message.ThreadID}, nil); err != nil {
		t.Fatalf("Apply(unread): %v", err)
	}
	peeked, err := f.service.GetThread(ctx, userID, message.ThreadID, false)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if peeked.Thread.UnreadCount != 1 {
		t.Error("reading with mark_read=false should leave the thread unread")
	}
}

func TestPaginationWalksTheMailboxWithoutRepeating(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")

	const total = 7
	for i := range total {
		f.deliver(t, accountID, fmt.Sprintf("Message %d", i), "a@example.com",
			mailbox.LabelInbox, time.Duration(i+1)*time.Hour)
	}

	seen := map[uuid.UUID]bool{}
	cursor := ""
	pages := 0

	for {
		page, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{
			Folder: mailbox.FolderInbox, Limit: 3, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("ListThreads: %v", err)
		}
		pages++
		for _, thread := range page.Threads {
			if seen[thread.ID] {
				t.Fatalf("thread %s appeared on two pages", thread.ID)
			}
			seen[thread.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
	}

	if len(seen) != total {
		t.Errorf("walked %d threads, want %d", len(seen), total)
	}
}

func TestUserLabelsCanBeManagedButSystemLabelsCannot(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	userID, accountID := f.newUserWithAccount(t, "owner@example.com", "Work")
	message := f.deliver(t, accountID, "Labelled", "a@example.com", mailbox.LabelInbox, time.Hour)

	label, err := f.service.CreateLabel(ctx, userID, accountID, "Finance", "emerald")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}

	if _, err := f.service.CreateLabel(ctx, userID, accountID, "finance", "blue"); !errors.Is(err, mailbox.ErrLabelExists) {
		t.Errorf("duplicate label = %v, want ErrLabelExists", err)
	}

	if _, err := f.service.ApplyLabel(ctx, userID, []uuid.UUID{message.ThreadID}, label.ID, true); err != nil {
		t.Fatalf("ApplyLabel: %v", err)
	}

	page, err := f.service.ListThreads(ctx, userID, mailbox.ListRequest{
		Folder: mailbox.FolderInbox, LabelID: &label.ID,
	})
	if err != nil {
		t.Fatalf("ListThreads by label: %v", err)
	}
	if len(page.Threads) != 1 {
		t.Fatalf("label filter returned %d threads, want 1", len(page.Threads))
	}
	if len(page.Threads[0].Labels) != 1 || page.Threads[0].Labels[0].Name != "Finance" {
		t.Errorf("thread labels = %+v, want the Finance label", page.Threads[0].Labels)
	}

	// The built-in labels must not be renameable or removable.
	labels, err := f.service.ListLabels(ctx, userID, &accountID)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	var inbox mailbox.Label
	for _, candidate := range labels {
		if candidate.SystemKey != nil && *candidate.SystemKey == mailbox.LabelInbox {
			inbox = candidate
		}
	}
	newName := "Not the inbox"
	if _, err := f.service.UpdateLabel(ctx, userID, inbox.ID, &newName, nil); !errors.Is(err, mailbox.ErrSystemLabel) {
		t.Errorf("renaming a system label = %v, want ErrSystemLabel", err)
	}
	if err := f.service.DeleteLabel(ctx, userID, inbox.ID); !errors.Is(err, mailbox.ErrSystemLabel) {
		t.Errorf("deleting a system label = %v, want ErrSystemLabel", err)
	}

	if err := f.service.DeleteLabel(ctx, userID, label.ID); err != nil {
		t.Errorf("deleting a user label: %v", err)
	}
}
