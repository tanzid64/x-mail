package mailbox

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dark/x-mail/api/internal/sanitize"
)

// NewMessage describes a message to store. It is the single entry point for
// putting mail into the mailbox, used by the sending path, the inbound webhook
// processor and the development seeder alike.
type NewMessage struct {
	MailAccountID uuid.UUID

	// ThreadID places the message in an existing conversation. When nil, the
	// message starts a new thread.
	ThreadID *uuid.UUID

	MessageID         *string
	InReplyTo         *string
	References        []string
	ProviderMessageID *string

	SenderName  string
	SenderEmail string

	Subject  string
	HTMLBody string
	TextBody string

	To  []Recipient
	Cc  []Recipient
	Bcc []Recipient

	Direction string
	Status    string

	IsRead      bool
	IsStarred   bool
	IsImportant bool

	// SystemLabels are the placement labels to apply, e.g. INBOX or SENT.
	SystemLabels []string

	SentAt     *time.Time
	ReceivedAt *time.Time
}

// snippetLength matches the preview a mailbox row can show.
const snippetLength = 160

// InsertMessage stores a message with its recipients and labels, creating or
// updating its thread, all in one transaction.
//
// The thread aggregates are refreshed here rather than by the caller, so a
// message can never be visible without the thread summary that describes it.
func (r *Repository) InsertMessage(ctx context.Context, msg NewMessage) (*Message, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	threadID, err := r.resolveThread(ctx, tx, msg)
	if err != nil {
		return nil, err
	}

	snippet := Snippet(msg.TextBody, msg.HTMLBody)

	// Sanitising here rather than at each call site means every stored message
	// has a render-safe body, whatever wrote it — the inbound webhook, the
	// seeder, or an importer added later. The original stays in html_body.
	sanitized := sanitize.HTML(msg.HTMLBody)
	size := len(msg.HTMLBody) + len(msg.TextBody)
	hasAttachments := false

	// A nil slice encodes as SQL NULL, which the NOT NULL column rejects.
	references := msg.References
	if references == nil {
		references = []string{}
	}

	var messageID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (
			mail_account_id, thread_id, message_id, in_reply_to, "references",
			provider_message_id, sender_name, sender_email, subject, snippet,
			html_body, html_sanitized, text_body, direction, status,
			is_read, is_starred, is_important, has_attachments, size_bytes,
			sent_at, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21, $22)
		RETURNING id`,
		msg.MailAccountID, threadID, msg.MessageID, msg.InReplyTo, references,
		msg.ProviderMessageID, msg.SenderName, msg.SenderEmail, msg.Subject, snippet,
		msg.HTMLBody, sanitized, msg.TextBody, msg.Direction, msg.Status,
		msg.IsRead, msg.IsStarred, msg.IsImportant, hasAttachments, size,
		msg.SentAt, msg.ReceivedAt,
	).Scan(&messageID)
	if err != nil {
		return nil, err
	}

	for _, group := range []struct {
		kind string
		list []Recipient
	}{{"to", msg.To}, {"cc", msg.Cc}, {"bcc", msg.Bcc}} {
		for _, recipient := range group.list {
			if _, err := tx.Exec(ctx,
				`INSERT INTO message_recipients (message_id, email, name, type) VALUES ($1, $2, $3, $4)`,
				messageID, strings.ToLower(strings.TrimSpace(recipient.Email)), recipient.Name, group.kind); err != nil {
				return nil, err
			}
		}
	}

	for _, key := range msg.SystemLabels {
		if _, err := tx.Exec(ctx, `
			INSERT INTO message_labels (message_id, label_id)
			SELECT $1, l.id FROM labels l
			 WHERE l.mail_account_id = $2 AND l.system_key = $3
			ON CONFLICT DO NOTHING`, messageID, msg.MailAccountID, key); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx, `SELECT refresh_thread($1)`, threadID); err != nil {
		return nil, err
	}

	stored, err := scanMessage(tx.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages m WHERE m.id = $1`, messageID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return stored, nil
}

// resolveThread finds the conversation a message belongs to, creating one when
// it starts a new exchange.
//
// Header matching comes first — In-Reply-To and References name the exact
// parent — because subject matching alone would merge unrelated mail that
// happens to share a subject line.
func (r *Repository) resolveThread(ctx context.Context, tx pgx.Tx, msg NewMessage) (uuid.UUID, error) {
	if msg.ThreadID != nil {
		return *msg.ThreadID, nil
	}

	// Collect every header that could name an ancestor, nearest first.
	candidates := make([]string, 0, len(msg.References)+1)
	if msg.InReplyTo != nil && *msg.InReplyTo != "" {
		candidates = append(candidates, *msg.InReplyTo)
	}
	for i := len(msg.References) - 1; i >= 0; i-- {
		if msg.References[i] != "" {
			candidates = append(candidates, msg.References[i])
		}
	}

	if len(candidates) > 0 {
		var threadID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT thread_id FROM messages
			 WHERE mail_account_id = $1 AND message_id = ANY($2)
			 ORDER BY created_at DESC LIMIT 1`, msg.MailAccountID, candidates).Scan(&threadID)
		if err == nil {
			return threadID, nil
		}
		if err != pgx.ErrNoRows {
			return uuid.Nil, err
		}
	}

	var threadID uuid.UUID
	err := tx.QueryRow(ctx,
		`INSERT INTO threads (mail_account_id, subject) VALUES ($1, $2) RETURNING id`,
		msg.MailAccountID, msg.Subject).Scan(&threadID)
	return threadID, err
}

// Snippet builds the one-line preview shown in a mailbox row, preferring the
// plain-text body and falling back to stripped HTML. Exported so outbound mail
// and inbound mail produce previews the same way.
func Snippet(text, html string) string {
	source := strings.TrimSpace(text)
	if source == "" {
		source = stripHTML(html)
	}

	// Collapse all whitespace so a preview never contains newlines.
	source = strings.Join(strings.FieldsFunc(source, unicode.IsSpace), " ")
	if len(source) <= snippetLength {
		return source
	}

	trimmed := source[:snippetLength]
	// Prefer cutting at a word boundary when one is close to the limit.
	if idx := strings.LastIndex(trimmed, " "); idx > snippetLength-24 {
		trimmed = trimmed[:idx]
	}
	return trimmed + "…"
}

// stripHTML removes tags for preview purposes only. It is not a sanitiser —
// inbound HTML is sanitised separately before it is ever rendered.
func stripHTML(html string) string {
	var out strings.Builder
	depth := 0
	for _, r := range html {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			out.WriteRune(r)
		}
	}
	return out.String()
}
