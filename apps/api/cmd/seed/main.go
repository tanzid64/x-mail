// Command seed fills a user's mailbox with sample conversations.
//
// Inbound mail does not arrive until the receiving phase, so without this there
// is no way to exercise the mailbox UI or write end-to-end tests against it.
// It only ever writes to an existing account, and refuses to run against a
// production configuration.
//
//	go run ./cmd/seed -email dark@example.com
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/database"
	"github.com/dark/x-mail/api/internal/mailbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "email address of the user to seed (required)")
	accountName := flag.String("account", "", "mail account name; defaults to the user's first account")
	flag.Parse()

	if *email == "" {
		return errors.New("-email is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.IsProduction() {
		return errors.New("refusing to seed a production database")
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	user, err := auth.NewRepository(pool).GetUserByEmail(ctx, *email)
	if err != nil {
		return fmt.Errorf("find user %s: %w", *email, err)
	}

	accountRepo := accounts.NewRepository(pool)
	list, err := accountRepo.ListAccounts(ctx, user.ID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("user %s has no mail accounts; connect one first", *email)
	}

	account := list[0]
	if *accountName != "" {
		found := false
		for _, candidate := range list {
			if candidate.Name == *accountName {
				account, found = candidate, true
				break
			}
		}
		if !found {
			return fmt.Errorf("no mail account named %q for %s", *accountName, *email)
		}
	}

	repo := mailbox.NewRepository(pool)
	created, err := seedMailbox(ctx, repo, account.ID, user.Email)
	if err != nil {
		return err
	}

	fmt.Printf("seeded %d messages into %q (%s)\n", created, account.Name, user.Email)
	return nil
}

// conversation is a sample exchange: the opening message plus any replies.
type conversation struct {
	subject   string
	from      string
	fromName  string
	body      string
	labels    []string
	unread    bool
	starred   bool
	important bool
	ageHours  int
	replies   []reply
}

type reply struct {
	fromSelf bool
	from     string
	fromName string
	body     string
	ageHours int
}

func sampleConversations(self string) []conversation {
	return []conversation{
		{
			subject: "Project update — week 12", from: "john@example.com", fromName: "John Smith",
			body:   "Hey,\n\nHere's where we landed this week. The migration is done and the new indexes cut the slow query from 4s to about 90ms.\n\nI'd like to walk through the rollout plan tomorrow if you have half an hour.\n\nJohn",
			labels: []string{mailbox.LabelInbox}, unread: true, important: true, ageHours: 2,
			replies: []reply{
				{fromSelf: true, from: self, fromName: "", body: "That's a big improvement. Tomorrow at 10 works for me.", ageHours: 1},
				{from: "john@example.com", fromName: "John Smith", body: "Booked. I'll send the doc beforehand.", ageHours: 1},
			},
		},
		{
			subject: "Your invoice from AWS is ready", from: "billing@amazon.com", fromName: "AWS Billing",
			body:   "Your invoice for the period ending last month is now available. Total due: $412.86.",
			labels: []string{mailbox.LabelInbox}, unread: true, ageHours: 5,
		},
		{
			subject: "Security alert: new sign-in", from: "noreply@github.com", fromName: "GitHub",
			body:   "A new sign-in to your account was detected from a device we don't recognise. If this was you, no action is needed.",
			labels: []string{mailbox.LabelInbox}, unread: true, ageHours: 9,
		},
		{
			subject: "Weekend plans?", from: "sara@example.com", fromName: "Sara Ahmed",
			body:   "Are you around on Saturday? A few of us are thinking of going up the coast if the weather holds.",
			labels: []string{mailbox.LabelInbox}, starred: true, ageHours: 26,
			replies: []reply{
				{fromSelf: true, from: self, body: "I'm in. What time were you thinking?", ageHours: 25},
			},
		},
		{
			subject: "Contract for review", from: "legal@client.com", fromName: "Client Legal",
			body:   "Attached is the revised agreement. The only substantive change is in section 4, which now caps liability at the fees paid in the preceding twelve months.",
			labels: []string{mailbox.LabelInbox}, important: true, ageHours: 48,
		},
		{
			subject: "Re: Payment issue on order 8812", from: "customer@example.net", fromName: "Priya Nair",
			body:   "Thanks for sorting that out so quickly — the refund came through this morning.",
			labels: []string{mailbox.LabelArchive}, ageHours: 72,
		},
		{
			subject: "Congratulations! You have won", from: "prize@suspicious.example", fromName: "Prize Team",
			body:   "You have been selected to receive a substantial cash prize. Reply with your bank details to claim.",
			labels: []string{mailbox.LabelSpam}, ageHours: 30,
		},
		{
			subject: "Old thread we finished with", from: "someone@example.org", fromName: "Someone",
			body:   "Closing this one out, nothing further needed.",
			labels: []string{mailbox.LabelTrash}, ageHours: 200,
		},
	}
}

func seedMailbox(ctx context.Context, repo *mailbox.Repository, accountID uuid.UUID, self string) (int, error) {
	now := time.Now()
	created := 0

	for _, conv := range sampleConversations(self) {
		receivedAt := now.Add(-time.Duration(conv.ageHours) * time.Hour)
		messageID := fmt.Sprintf("<%s@seed.x-mail>", uuid.NewString())

		root, err := repo.InsertMessage(ctx, mailbox.NewMessage{
			MailAccountID: accountID,
			MessageID:     &messageID,
			SenderName:    conv.fromName,
			SenderEmail:   conv.from,
			Subject:       conv.subject,
			TextBody:      conv.body,
			HTMLBody:      "<p>" + conv.body + "</p>",
			To:            []mailbox.Recipient{{Email: self, Type: "to"}},
			Direction:     mailbox.DirectionInbound,
			Status:        mailbox.StatusReceived,
			IsRead:        !conv.unread,
			IsStarred:     conv.starred,
			IsImportant:   conv.important,
			SystemLabels:  conv.labels,
			ReceivedAt:    &receivedAt,
		})
		if err != nil {
			return created, fmt.Errorf("seed %q: %w", conv.subject, err)
		}
		created++

		parentID := messageID
		for _, rep := range conv.replies {
			replyAt := now.Add(-time.Duration(rep.ageHours) * time.Hour)
			replyMessageID := fmt.Sprintf("<%s@seed.x-mail>", uuid.NewString())
			inReplyTo := parentID

			message := mailbox.NewMessage{
				MailAccountID: accountID,
				ThreadID:      &root.ThreadID,
				MessageID:     &replyMessageID,
				InReplyTo:     &inReplyTo,
				References:    []string{parentID},
				SenderName:    rep.fromName,
				SenderEmail:   rep.from,
				Subject:       "Re: " + conv.subject,
				TextBody:      rep.body,
				HTMLBody:      "<p>" + rep.body + "</p>",
				To:            []mailbox.Recipient{{Email: conv.from, Type: "to"}},
				IsRead:        true,
				SystemLabels:  conv.labels,
			}

			if rep.fromSelf {
				// A reply the user sent is outbound and files under Sent as
				// well as wherever the conversation lives.
				message.Direction = mailbox.DirectionOutbound
				message.Status = mailbox.StatusSent
				message.SentAt = &replyAt
				message.SystemLabels = append(append([]string{}, conv.labels...), mailbox.LabelSent)
			} else {
				message.Direction = mailbox.DirectionInbound
				message.Status = mailbox.StatusReceived
				message.ReceivedAt = &replyAt
				message.To = []mailbox.Recipient{{Email: self, Type: "to"}}
			}

			if _, err := repo.InsertMessage(ctx, message); err != nil {
				return created, fmt.Errorf("seed reply to %q: %w", conv.subject, err)
			}
			created++
			parentID = replyMessageID
		}
	}
	return created, nil
}
