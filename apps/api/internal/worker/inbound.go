package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/mailbox"
	"github.com/dark/x-mail/api/internal/storage"
	"github.com/dark/x-mail/api/internal/webhooks"
)

// maxInboundAttachmentBytes caps a single received attachment.
const maxInboundAttachmentBytes = 25 << 20

// ProcessEventHandler acts on a recorded provider webhook: either storing a
// received message, or updating the delivery state of one we sent.
type ProcessEventHandler struct {
	events   *webhooks.Repository
	mailbox  *mailbox.Repository
	accounts *accounts.Repository
	storage  storage.Storage
	// publish notifies connected clients. Optional until realtime lands.
	publish func(ctx context.Context, accountID uuid.UUID, event string, data map[string]any)
	logger  *slog.Logger
}

func NewProcessEventHandler(
	events *webhooks.Repository,
	mailboxRepo *mailbox.Repository,
	accountsRepo *accounts.Repository,
	store storage.Storage,
	logger *slog.Logger,
) *ProcessEventHandler {
	return &ProcessEventHandler{
		events:   events,
		mailbox:  mailboxRepo,
		accounts: accountsRepo,
		storage:  store,
		logger:   logger,
	}
}

// SetPublisher installs the realtime publisher.
func (h *ProcessEventHandler) SetPublisher(
	publish func(ctx context.Context, accountID uuid.UUID, event string, data map[string]any),
) {
	h.publish = publish
}

func (h *ProcessEventHandler) notify(ctx context.Context, accountID uuid.UUID, event string, data map[string]any) {
	if h.publish != nil {
		h.publish(ctx, accountID, event, data)
	}
}

// ProcessTask handles one recorded event.
func (h *ProcessEventHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload ProcessEventPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		h.logger.ErrorContext(ctx, "undecodable event payload", "error", err)
		return nil
	}

	logger := h.logger.With("event_id", payload.EventID)

	event, owner, err := h.events.Load(ctx, payload.EventID)
	if err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("load event: %w", err)
	}

	// A job that runs twice must not produce a second inbox entry.
	if owner.AlreadyProcessed {
		logger.InfoContext(ctx, "event already processed, skipping", "type", event.EventType)
		return nil
	}

	var envelope webhooks.Envelope
	if err := json.Unmarshal(event.Payload, &envelope); err != nil {
		return h.finish(ctx, payload.EventID, "payload could not be decoded")
	}

	var data webhooks.EventData
	if len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return h.finish(ctx, payload.EventID, "event data could not be decoded")
		}
	}

	// Handlers report two different things: `skipped` is a reason the event
	// cannot be acted on, which is final and worth recording; `err` is a
	// failure worth retrying, which leaves the event unprocessed so the retry
	// does the work.
	var skipped string

	switch envelope.Type {
	case webhooks.EventReceived:
		skipped, err = h.handleReceived(ctx, payload.EventID, owner.MailAccountID, envelope, data)
	case webhooks.EventSent, webhooks.EventDelivered, webhooks.EventDeliveryDelayed,
		webhooks.EventBounced, webhooks.EventComplained, webhooks.EventOpened, webhooks.EventClicked:
		skipped, err = h.handleDeliveryEvent(ctx, payload.EventID, owner.MailAccountID, envelope, data)
	default:
		// Recorded but not acted on, so a new provider event cannot fail a job.
		logger.InfoContext(ctx, "ignoring an unhandled event type", "type", envelope.Type)
		skipped = "unhandled event type " + envelope.Type
	}

	if err != nil {
		return err
	}
	return h.finish(ctx, payload.EventID, skipped)
}

func (h *ProcessEventHandler) finish(ctx context.Context, eventID uuid.UUID, processError string) error {
	if err := h.events.MarkProcessed(ctx, eventID, processError); err != nil {
		return fmt.Errorf("mark event processed: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- inbound --

// handleReceived stores a message that arrived for one of our addresses.
func (h *ProcessEventHandler) handleReceived(
	ctx context.Context,
	eventID, accountID uuid.UUID,
	envelope webhooks.Envelope,
	data webhooks.EventData,
) (string, error) {
	// Route by recipient. The address is the primary key of inbound routing:
	// the account on the URL claims a destination, but the address is what
	// actually decides whose mailbox this belongs in.
	route, deliveredTo := h.route(ctx, data)
	if route == nil {
		h.logger.WarnContext(ctx, "received mail for an unknown address",
			"to", data.To, "event_id", eventID)
		return "no mail address matched any recipient", nil
	}
	if route.MailAccountID != accountID {
		// The signature proved who sent the webhook; the address decides where
		// the mail goes. A mismatch means the provider account is configured
		// for a domain another account owns.
		h.logger.WarnContext(ctx, "inbound recipient belongs to a different account",
			"webhook_account", accountID, "address_account", route.MailAccountID)
	}
	if !route.AccountActive {
		return "the destination account is disabled", nil
	}

	sender, senderName := parseAddress(data.From)
	messageID := data.Header("Message-Id")
	if messageID == "" {
		messageID = data.Header("Message-ID")
	}
	inReplyTo := data.Header("In-Reply-To")

	receivedAt := envelope.OccurredAt()

	message := mailbox.NewMessage{
		MailAccountID: route.MailAccountID,
		SenderName:    senderName,
		SenderEmail:   sender,
		Subject:       data.Subject,
		// The original is kept as received; the sanitised copy is what renders.
		HTMLBody:     data.HTML,
		TextBody:     data.Text,
		To:           toRecipients(data.To, "to"),
		Cc:           toRecipients(data.Cc, "cc"),
		Direction:    mailbox.DirectionInbound,
		Status:       mailbox.StatusReceived,
		SystemLabels: []string{mailbox.LabelInbox},
		ReceivedAt:   &receivedAt,
		References:   data.References(),
	}
	if messageID != "" {
		message.MessageID = &messageID
	}
	if inReplyTo != "" {
		message.InReplyTo = &inReplyTo
	}
	if providerID := data.ProviderEmailID(); providerID != "" {
		message.ProviderMessageID = &providerID
	}

	stored, err := h.mailbox.InsertMessage(ctx, message)
	if err != nil {
		return "", fmt.Errorf("store inbound message: %w", err)
	}

	if deliveredTo != uuid.Nil {
		if err := h.mailbox.SetDeliveredToAddress(ctx, stored.ID, deliveredTo); err != nil {
			h.logger.WarnContext(ctx, "could not record delivery address", "error", err)
		}
	}

	if err := h.storeAttachments(ctx, stored.ID, data.Attachments); err != nil {
		h.logger.WarnContext(ctx, "could not store all inbound attachments",
			"error", err, "message_id", stored.ID)
	}

	if err := h.events.LinkMessage(ctx, eventID, stored.ID); err != nil {
		h.logger.WarnContext(ctx, "could not link event to message", "error", err)
	}

	h.logger.InfoContext(ctx, "stored inbound message",
		"message_id", stored.ID, "thread_id", stored.ThreadID, "from", sender)

	h.notify(ctx, route.MailAccountID, "email.received", map[string]any{
		"mail_account_id": route.MailAccountID,
		"thread_id":       stored.ThreadID,
		"message_id":      stored.ID,
	})
	return "", nil
}

// route finds the account a received message belongs to, trying each recipient
// in turn since we may be one of several.
func (h *ProcessEventHandler) route(ctx context.Context, data webhooks.EventData) (*accounts.Route, uuid.UUID) {
	candidates := append(append([]string{}, data.To...), data.Cc...)
	candidates = append(candidates, data.Bcc...)

	for _, candidate := range candidates {
		email, _ := parseAddress(candidate)
		if email == "" {
			continue
		}
		route, err := h.accounts.ResolveInbound(ctx, email)
		if err == nil {
			return route, route.AddressID
		}
	}
	return nil, uuid.Nil
}

// storeAttachments writes inbound files to object storage.
func (h *ProcessEventHandler) storeAttachments(ctx context.Context, messageID uuid.UUID, attachments []webhooks.InboundAttachment) error {
	if len(attachments) == 0 {
		return nil
	}

	for _, attachment := range attachments {
		if attachment.Content == "" {
			// Large attachments are offered as a URL; fetching them is a
			// separate job rather than something to do inline here.
			h.logger.InfoContext(ctx, "skipping an attachment delivered by reference",
				"filename", attachment.Filename)
			continue
		}

		content, err := base64.StdEncoding.DecodeString(attachment.Content)
		if err != nil {
			h.logger.WarnContext(ctx, "undecodable attachment", "filename", attachment.Filename)
			continue
		}
		if len(content) > maxInboundAttachmentBytes {
			h.logger.WarnContext(ctx, "attachment exceeds the size limit", "filename", attachment.Filename)
			continue
		}

		filename := storage.SanitizeFilename(attachment.Filename)
		key := storage.BuildKey(time.Now(), filename)

		object, err := h.storage.Put(ctx, key, attachment.ContentType, strings.NewReader(string(content)))
		if err != nil {
			return fmt.Errorf("store attachment %q: %w", filename, err)
		}

		if err := h.mailbox.AttachToMessage(ctx, messageID, mailbox.NewAttachment{
			Filename:   filename,
			MimeType:   attachment.ContentType,
			SizeBytes:  object.Size,
			StorageKey: key,
			ContentID:  attachment.ContentID,
			IsInline:   attachment.ContentID != "",
		}); err != nil {
			_ = h.storage.Delete(ctx, key)
			return fmt.Errorf("record attachment %q: %w", filename, err)
		}
	}
	return nil
}

// --------------------------------------------------------- delivery events --

// handleDeliveryEvent updates the state of a message we sent.
func (h *ProcessEventHandler) handleDeliveryEvent(
	ctx context.Context,
	eventID, accountID uuid.UUID,
	envelope webhooks.Envelope,
	data webhooks.EventData,
) (string, error) {
	providerID := data.ProviderEmailID()
	if providerID == "" {
		return "event carried no provider message id", nil
	}

	message, err := h.mailbox.MessageByProviderID(ctx, providerID)
	if err != nil {
		if errors.Is(err, mailbox.ErrNotFound) {
			// A delivery event can arrive before the send result was recorded.
			// Returning an error lets the job retry rather than losing it.
			return "", fmt.Errorf("no message yet for provider id %s: %w", providerID, err)
		}
		return "", err
	}

	occurredAt := envelope.OccurredAt()
	reason := ""
	if data.Bounce != nil {
		reason = strings.TrimSpace(data.Bounce.Message)
	}

	if err := h.mailbox.ApplyDeliveryEvent(ctx, message.ID, envelope.Type, occurredAt, reason); err != nil {
		return "", fmt.Errorf("apply delivery event: %w", err)
	}
	if err := h.events.LinkMessage(ctx, eventID, message.ID); err != nil {
		h.logger.WarnContext(ctx, "could not link event to message", "error", err)
	}

	h.logger.InfoContext(ctx, "applied delivery event",
		"type", envelope.Type, "message_id", message.ID)

	h.notify(ctx, accountID, "email.updated", map[string]any{
		"mail_account_id": accountID,
		"thread_id":       message.ThreadID,
		"message_id":      message.ID,
		"status":          envelope.Type,
	})
	return "", nil
}

// ---------------------------------------------------------------- helpers --

// parseAddress splits `Name <email>` into its parts.
func parseAddress(raw string) (email, name string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	if open := strings.LastIndex(raw, "<"); open >= 0 {
		if close := strings.Index(raw[open:], ">"); close > 0 {
			email = strings.TrimSpace(raw[open+1 : open+close])
			name = strings.Trim(strings.TrimSpace(raw[:open]), `"`)
			return strings.ToLower(email), name
		}
	}
	return strings.ToLower(raw), ""
}

func toRecipients(list []string, kind string) []mailbox.Recipient {
	out := make([]mailbox.Recipient, 0, len(list))
	for _, raw := range list {
		email, name := parseAddress(raw)
		if email == "" {
			continue
		}
		out = append(out, mailbox.Recipient{Email: email, Name: name, Type: kind})
	}
	return out
}
