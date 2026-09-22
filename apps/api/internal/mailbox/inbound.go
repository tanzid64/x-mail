package mailbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NewAttachment is a file arriving with an inbound message.
type NewAttachment struct {
	Filename   string
	MimeType   string
	SizeBytes  int64
	StorageKey string
	ContentID  string
	IsInline   bool
}

// SetSanitizedHTML stores the render-safe copy of a message body.
//
// Sanitising once on arrival rather than on every read means the cost is paid
// per message instead of per view, and the original stays untouched so a
// sanitiser fix can be applied by re-running it.
func (r *Repository) SetSanitizedHTML(ctx context.Context, messageID uuid.UUID, html string) error {
	_, err := r.db.Exec(ctx, `UPDATE messages SET html_sanitized = $2 WHERE id = $1`, messageID, html)
	return err
}

// SetDeliveredToAddress records which of our addresses a message arrived at,
// so a reply can default to the identity it was sent to.
func (r *Repository) SetDeliveredToAddress(ctx context.Context, messageID, addressID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE messages SET delivered_to_address_id = $2 WHERE id = $1`, messageID, addressID)
	return err
}

// AttachToMessage records an inbound attachment and marks its message as
// carrying one.
func (r *Repository) AttachToMessage(ctx context.Context, messageID uuid.UUID, attachment NewAttachment) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var contentID *string
	if attachment.ContentID != "" {
		contentID = &attachment.ContentID
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO attachments (message_id, filename, mime_type, size_bytes, storage_key, content_id, is_inline)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		messageID, attachment.Filename, attachment.MimeType, attachment.SizeBytes,
		attachment.StorageKey, contentID, attachment.IsInline); err != nil {
		return err
	}

	var threadID uuid.UUID
	if err := tx.QueryRow(ctx,
		`UPDATE messages SET has_attachments = true WHERE id = $1 RETURNING thread_id`,
		messageID).Scan(&threadID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT refresh_thread($1)`, threadID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MessageByProviderID finds a message by the provider's own id, which is how a
// delivery or bounce webhook is correlated back to what we sent.
func (r *Repository) MessageByProviderID(ctx context.Context, providerMessageID string) (*Message, error) {
	return scanMessage(r.db.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages m WHERE m.provider_message_id = $1`, providerMessageID))
}

// ApplyDeliveryEvent records what the provider reported about a message.
//
// The lifecycle only moves forward: a "sent" event arriving after "delivered",
// which providers do reorder, must not walk the status backwards.
func (r *Repository) ApplyDeliveryEvent(ctx context.Context, messageID uuid.UUID, eventType string, occurredAt time.Time, reason string) error {
	var column, status string

	switch eventType {
	case "email.sent":
		column, status = "sent_at", StatusSent
	case "email.delivered":
		column, status = "delivered_at", StatusDelivered
	case "email.bounced":
		column, status = "bounced_at", StatusBounced
	case "email.complained":
		column, status = "complained_at", ""
	case "email.opened":
		column, status = "opened_at", ""
	case "email.clicked":
		column, status = "clicked_at", ""
	case "email.delivery_delayed":
		// Nothing terminal has happened; the timestamp alone is not useful.
		return nil
	default:
		return nil
	}

	query := fmt.Sprintf(`UPDATE messages SET %s = COALESCE(%s, $2)`, column, column)
	args := []any{messageID, occurredAt}

	if status != "" {
		// rank orders the lifecycle so a late event cannot regress it.
		args = append(args, status)
		query += fmt.Sprintf(`, status = CASE
			WHEN %s > %s THEN $3 ELSE status END`,
			statusRank("$3"), statusRank("status"))
	}
	if reason != "" {
		args = append(args, truncate(reason, 500))
		query += fmt.Sprintf(`, failure_reason = $%d`, len(args))
	}
	query += ` WHERE id = $1`

	_, err := r.db.Exec(ctx, query, args...)
	return err
}

// statusRank renders a SQL expression ordering the delivery lifecycle.
func statusRank(expr string) string {
	return fmt.Sprintf(`CASE %s
		WHEN 'draft' THEN 0
		WHEN 'queued' THEN 1
		WHEN 'sending' THEN 2
		WHEN 'sent' THEN 3
		WHEN 'delivered' THEN 4
		WHEN 'bounced' THEN 5
		WHEN 'failed' THEN 5
		ELSE 0 END`, expr)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
