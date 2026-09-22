package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dark/x-mail/api/internal/mailbox"
)

var (
	// ErrNoRecipients means the draft has nobody to send to.
	ErrNoRecipients = errors.New("compose: add at least one recipient")
	// ErrNoSender means no verified sender address was chosen.
	ErrNoSender = errors.New("compose: choose an address to send from")
	// ErrSenderNotUsable means the chosen address is disabled or its domain is
	// not verified, so the provider would reject the message.
	ErrSenderNotUsable = errors.New("compose: that sender address is not ready to send from")
)

// QueuedMessage is the result of turning a draft into an outbound message.
type QueuedMessage struct {
	MessageID      uuid.UUID
	ThreadID       uuid.UUID
	MailAccountID  uuid.UUID
	IdempotencyKey string
}

// sender is the resolved From identity for a send.
type sender struct {
	addressID   uuid.UUID
	email       string
	displayName string
	domain      string
	verified    bool
	active      bool
}

// resolveSender loads the chosen From address and checks it can actually send.
func (r *Repository) resolveSender(ctx context.Context, tx pgx.Tx, userID uuid.UUID, addressID *uuid.UUID, accountID uuid.UUID) (*sender, error) {
	if addressID == nil {
		return nil, ErrNoSender
	}

	var s sender
	var status string
	err := tx.QueryRow(ctx, `
		SELECT m.id, m.email, m.display_name, d.domain, d.status, (m.is_active AND a.is_active)
		  FROM mail_addresses m
		  JOIN domains d ON d.id = m.domain_id
		  JOIN mail_accounts a ON a.id = d.mail_account_id
		 WHERE m.id = $1 AND a.user_id = $2 AND a.id = $3`,
		*addressID, userID, accountID,
	).Scan(&s.addressID, &s.email, &s.displayName, &s.domain, &status, &s.active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Not theirs, or not on this account.
			return nil, ErrNoSender
		}
		return nil, err
	}

	s.verified = status == "verified"
	if !s.active || !s.verified {
		return nil, ErrSenderNotUsable
	}
	return &s, nil
}

// QueueDraft turns a draft into an outbound message ready for the worker.
//
// Everything happens in one transaction: the draft is claimed, the message and
// its recipients are written, attachments are re-pointed from the draft to the
// message, and the draft is removed. A crash anywhere leaves either a draft the
// user still has, or a queued message the worker will send — never both and
// never neither.
func (r *Repository) QueueDraft(ctx context.Context, userID, draftID uuid.UUID) (*QueuedMessage, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	// Claiming inside the transaction means two concurrent sends cannot both
	// proceed: the second finds the draft already claimed.
	var draft *Draft
	{
		var id uuid.UUID
		err := tx.QueryRow(ctx, `
			UPDATE drafts d SET status = 'sending'
			 WHERE d.id = $2 AND d.status = 'draft'
			   AND EXISTS (SELECT 1 FROM mail_accounts a WHERE a.id = d.mail_account_id AND a.user_id = $1)
			RETURNING d.id`, userID, draftID).Scan(&id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if _, getErr := r.GetDraft(ctx, userID, draftID); getErr == nil {
					return nil, ErrDraftSending
				}
				return nil, ErrNotFound
			}
			return nil, err
		}
		draft, err = scanDraft(tx.QueryRow(ctx,
			`SELECT `+draftColumns+` FROM drafts d WHERE d.id = $1`, draftID))
		if err != nil {
			return nil, err
		}
	}

	if len(draft.To)+len(draft.Cc)+len(draft.Bcc) == 0 {
		return nil, ErrNoRecipients
	}

	from, err := r.resolveSender(ctx, tx, userID, draft.FromAddressID, draft.MailAccountID)
	if err != nil {
		return nil, err
	}

	// Threading headers: a reply must name its parent, and carry the parent's
	// own References so the recipient's client can rebuild the chain.
	var inReplyTo *string
	var references []string
	threadID := draft.ThreadID

	if draft.InReplyToID != nil {
		var parentMessageID *string
		var parentReferences []string
		var parentThread uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT m.message_id, m."references", m.thread_id FROM messages m
			   JOIN mail_accounts a ON a.id = m.mail_account_id
			  WHERE m.id = $1 AND a.user_id = $2`,
			*draft.InReplyToID, userID).Scan(&parentMessageID, &parentReferences, &parentThread)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			if parentMessageID != nil {
				inReplyTo = parentMessageID
				references = append(append([]string{}, parentReferences...), *parentMessageID)
			}
			if threadID == nil {
				threadID = &parentThread
			}
		}
	}

	if threadID == nil {
		var newThread uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO threads (mail_account_id, subject) VALUES ($1, $2) RETURNING id`,
			draft.MailAccountID, draft.Subject).Scan(&newThread); err != nil {
			return nil, err
		}
		threadID = &newThread
	}

	// The Message-ID is ours to mint and must be globally unique; using the
	// sending domain keeps it plausible to receiving servers.
	messageID := fmt.Sprintf("<%s@%s>", uuid.NewString(), from.domain)
	idempotencyKey := uuid.NewString()
	if references == nil {
		references = []string{}
	}

	snippet := makeSnippet(draft.TextBody, draft.HTMLBody)

	var newMessageID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (
			mail_account_id, thread_id, message_id, in_reply_to, "references",
			sender_name, sender_email, subject, snippet, html_body, text_body,
			direction, status, is_read, idempotency_key, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
		        'outbound', 'queued', true, $12, $13)
		RETURNING id`,
		draft.MailAccountID, *threadID, messageID, inReplyTo, references,
		from.displayName, from.email, draft.Subject, snippet, draft.HTMLBody, draft.TextBody,
		idempotencyKey, len(draft.HTMLBody)+len(draft.TextBody)).Scan(&newMessageID)
	if err != nil {
		return nil, err
	}

	for kind, list := range map[string][]Address{"to": draft.To, "cc": draft.Cc, "bcc": draft.Bcc} {
		for _, address := range list {
			if _, err := tx.Exec(ctx,
				`INSERT INTO message_recipients (message_id, email, name, type) VALUES ($1, $2, $3, $4)`,
				newMessageID, strings.ToLower(strings.TrimSpace(address.Email)), address.Name, kind); err != nil {
				return nil, err
			}
		}
	}

	// A sent message belongs in Sent, and in the conversation it continues.
	for _, key := range []string{mailbox.LabelSent} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO message_labels (message_id, label_id)
			SELECT $1, l.id FROM labels l
			 WHERE l.mail_account_id = $2 AND l.system_key = $3
			ON CONFLICT DO NOTHING`, newMessageID, draft.MailAccountID, key); err != nil {
			return nil, err
		}
	}

	// Attachments move from the draft to the message rather than being copied,
	// so the bytes in object storage are never duplicated.
	if _, err := tx.Exec(ctx, `
		UPDATE attachments SET message_id = $1, draft_id = NULL WHERE draft_id = $2`,
		newMessageID, draftID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE messages SET has_attachments = EXISTS (SELECT 1 FROM attachments WHERE message_id = $1)
		  WHERE id = $1`, newMessageID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM drafts WHERE id = $1`, draftID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT refresh_thread($1)`, *threadID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &QueuedMessage{
		MessageID:      newMessageID,
		ThreadID:       *threadID,
		MailAccountID:  draft.MailAccountID,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// OutboundMessage is everything the worker needs to hand a message to a
// provider.
type OutboundMessage struct {
	ID             uuid.UUID
	MailAccountID  uuid.UUID
	ThreadID       uuid.UUID
	Status         string
	IdempotencyKey string
	MessageID      string
	InReplyTo      *string
	References     []string

	SenderName  string
	SenderEmail string
	Subject     string
	HTMLBody    string
	TextBody    string

	To, Cc, Bcc  []Address
	Attachments  []Attachment
	AttemptCount int
}

// LoadOutbound fetches a queued message for sending. It takes no user, because
// the worker has already established ownership through the queued message.
func (r *Repository) LoadOutbound(ctx context.Context, messageID uuid.UUID) (*OutboundMessage, error) {
	var m OutboundMessage
	var idempotencyKey, rawMessageID *string

	err := r.db.QueryRow(ctx, `
		SELECT id, mail_account_id, thread_id, status, idempotency_key, message_id,
		       in_reply_to, "references", sender_name, sender_email, subject,
		       html_body, text_body, attempt_count
		  FROM messages WHERE id = $1`, messageID,
	).Scan(&m.ID, &m.MailAccountID, &m.ThreadID, &m.Status, &idempotencyKey, &rawMessageID,
		&m.InReplyTo, &m.References, &m.SenderName, &m.SenderEmail, &m.Subject,
		&m.HTMLBody, &m.TextBody, &m.AttemptCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if idempotencyKey != nil {
		m.IdempotencyKey = *idempotencyKey
	}
	if rawMessageID != nil {
		m.MessageID = *rawMessageID
	}

	rows, err := r.db.Query(ctx,
		`SELECT email, name, type FROM message_recipients WHERE message_id = $1`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var address Address
		var kind string
		if err := rows.Scan(&address.Email, &address.Name, &kind); err != nil {
			return nil, err
		}
		switch kind {
		case "to":
			m.To = append(m.To, address)
		case "cc":
			m.Cc = append(m.Cc, address)
		case "bcc":
			m.Bcc = append(m.Bcc, address)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	attachments, err := r.queryAttachments(ctx,
		`SELECT `+attachmentColumns+` FROM attachments at WHERE at.message_id = $1 ORDER BY at.created_at`,
		messageID)
	if err != nil {
		return nil, err
	}
	m.Attachments = attachments
	return &m, nil
}

// MarkSending records an attempt before the provider is called, so a message
// that disappears mid-flight can still be found by its attempt count.
func (r *Repository) MarkSending(ctx context.Context, messageID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE messages
		   SET status = 'sending', attempt_count = attempt_count + 1, last_attempt_at = now()
		 WHERE id = $1`, messageID)
	return err
}

// MarkSent records a successful hand-off to the provider.
func (r *Repository) MarkSent(ctx context.Context, messageID uuid.UUID, providerMessageID string, sentAt time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var threadID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE messages
		   SET status = 'sent', provider_message_id = $2, sent_at = $3, failure_reason = ''
		 WHERE id = $1
		RETURNING thread_id`, messageID, providerMessageID, sentAt).Scan(&threadID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT refresh_thread($1)`, threadID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkFailed records a permanent failure, so the message stops being retried
// and the user is told why.
func (r *Repository) MarkFailed(ctx context.Context, messageID uuid.UUID, reason string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE messages SET status = 'failed', failure_reason = $2 WHERE id = $1`,
		messageID, truncate(reason, 500))
	return err
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// makeSnippet mirrors the mailbox preview rule for outbound mail.
func makeSnippet(text, html string) string {
	return mailbox.Snippet(text, html)
}
