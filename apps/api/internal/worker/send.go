package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/compose"
	"github.com/dark/x-mail/api/internal/mailbox"
	"github.com/dark/x-mail/api/internal/providers"
	"github.com/dark/x-mail/api/internal/storage"
)

// maxAttachmentBytes caps what one message may carry to the provider.
const maxAttachmentBytes = 25 << 20 // 25 MiB

// SendEmailHandler delivers a queued message through its account's provider.
type SendEmailHandler struct {
	compose  *compose.Repository
	accounts *accounts.Service
	storage  storage.Storage
	logger   *slog.Logger
}

func NewSendEmailHandler(
	composeRepo *compose.Repository,
	accountsService *accounts.Service,
	store storage.Storage,
	logger *slog.Logger,
) *SendEmailHandler {
	return &SendEmailHandler{compose: composeRepo, accounts: accountsService, storage: store, logger: logger}
}

// ProcessTask sends one message.
//
// Returning an error tells Asynq to retry; returning nil means the job is done,
// whether the message went out or failed permanently. That distinction is the
// whole point of classifying provider errors: a rejected API key will be
// rejected again, so retrying it just delays telling the user.
func (h *SendEmailHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload SendEmailPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		// A payload we cannot read will never become readable.
		h.logger.ErrorContext(ctx, "undecodable send payload", "error", err)
		return nil
	}

	logger := h.logger.With("message_id", payload.MessageID)

	message, err := h.compose.LoadOutbound(ctx, payload.MessageID)
	if err != nil {
		if errors.Is(err, compose.ErrNotFound) {
			// The message was deleted before the job ran.
			logger.WarnContext(ctx, "queued message no longer exists")
			return nil
		}
		return fmt.Errorf("load outbound message: %w", err)
	}

	// A retry that arrives after the message already went out must not send it
	// a second time.
	switch message.Status {
	case mailbox.StatusSent, mailbox.StatusDelivered, mailbox.StatusBounced:
		logger.InfoContext(ctx, "message already sent, skipping", "status", message.Status)
		return nil
	case mailbox.StatusFailed:
		logger.InfoContext(ctx, "message already failed permanently, skipping")
		return nil
	}

	if err := h.compose.MarkSending(ctx, message.ID); err != nil {
		return fmt.Errorf("mark sending: %w", err)
	}

	provider, err := h.accounts.ProviderForWorker(ctx, message.MailAccountID)
	if err != nil {
		// The account or its credential is gone; no number of retries fixes it.
		logger.ErrorContext(ctx, "cannot build provider for account", "error", err)
		return h.fail(ctx, message, "This mail account can no longer send. Reconnect it in settings.")
	}

	email, err := h.buildEmail(ctx, message)
	if err != nil {
		logger.ErrorContext(ctx, "cannot assemble message", "error", err)
		return h.fail(ctx, message, err.Error())
	}

	result, err := provider.Send(ctx, email)
	if err != nil {
		if providers.IsRetryable(err) {
			logger.WarnContext(ctx, "send failed, will retry",
				"error", err, "attempt", message.AttemptCount+1)
			// Put it back to queued so the mailbox does not show it stuck at
			// "sending" between attempts.
			return fmt.Errorf("send: %w", err)
		}

		logger.ErrorContext(ctx, "send failed permanently", "error", err)
		return h.fail(ctx, message, permanentReason(err))
	}

	if err := h.compose.MarkSent(ctx, message.ID, result.ProviderMessageID, time.Now()); err != nil {
		// The provider has it. Failing here would retry the send, which the
		// idempotency key makes safe, so the error is worth surfacing.
		return fmt.Errorf("record sent message: %w", err)
	}

	logger.InfoContext(ctx, "message sent", "provider_message_id", result.ProviderMessageID)
	return nil
}

// fail records a permanent failure and completes the job, since retrying could
// not change the outcome.
func (h *SendEmailHandler) fail(ctx context.Context, message *compose.OutboundMessage, reason string) error {
	if err := h.compose.MarkFailed(ctx, message.ID, reason); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// buildEmail assembles the provider-agnostic message, pulling attachment bytes
// out of object storage.
func (h *SendEmailHandler) buildEmail(ctx context.Context, message *compose.OutboundMessage) (providers.Email, error) {
	email := providers.Email{
		From:    providers.Address{Email: message.SenderEmail, Name: message.SenderName},
		To:      toProviderAddresses(message.To),
		Cc:      toProviderAddresses(message.Cc),
		Bcc:     toProviderAddresses(message.Bcc),
		Subject: message.Subject,
		HTML:    message.HTMLBody,
		Text:    message.TextBody,
		// The same key on every attempt is what lets the provider collapse a
		// retried send into the original delivery.
		IdempotencyKey: message.IdempotencyKey,
		Headers:        map[string]string{},
	}

	if message.MessageID != "" {
		email.Headers["Message-ID"] = message.MessageID
	}
	if message.InReplyTo != nil && *message.InReplyTo != "" {
		email.Headers["In-Reply-To"] = *message.InReplyTo
	}
	if len(message.References) > 0 {
		email.Headers["References"] = joinReferences(message.References)
	}

	var total int64
	for _, attachment := range message.Attachments {
		total += attachment.SizeBytes
		if total > maxAttachmentBytes {
			return email, errors.New("The attachments on this message are too large to send.")
		}

		reader, _, err := h.storage.Get(ctx, attachment.StorageKey)
		if err != nil {
			return email, fmt.Errorf("attachment %q could not be read", attachment.Filename)
		}
		content, err := io.ReadAll(io.LimitReader(reader, maxAttachmentBytes+1))
		_ = reader.Close()
		if err != nil {
			return email, fmt.Errorf("attachment %q could not be read", attachment.Filename)
		}

		email.Attachments = append(email.Attachments, providers.Attachment{
			Filename:    attachment.Filename,
			ContentType: attachment.MimeType,
			Content:     content,
			ContentID:   contentIDOrEmpty(attachment.ContentID, attachment.IsInline),
		})
	}
	return email, nil
}

func toProviderAddresses(list []compose.Address) []providers.Address {
	if len(list) == 0 {
		return nil
	}
	out := make([]providers.Address, 0, len(list))
	for _, address := range list {
		out = append(out, providers.Address{Email: address.Email, Name: address.Name})
	}
	return out
}

func contentIDOrEmpty(contentID *string, inline bool) string {
	if inline && contentID != nil {
		return *contentID
	}
	return ""
}

// joinReferences renders the References header, which is a space-separated
// list of message ids oldest first.
func joinReferences(references []string) string {
	out := ""
	for i, reference := range references {
		if i > 0 {
			out += " "
		}
		out += reference
	}
	return out
}

// permanentReason turns a provider failure into something worth showing the
// user, without leaking internals.
func permanentReason(err error) string {
	switch {
	case providers.IsKind(err, providers.KindAuth):
		return "The provider rejected this account's API key. Reconnect it in settings."
	case providers.IsKind(err, providers.KindInvalidRequest):
		var provErr *providers.Error
		if errors.As(err, &provErr) && provErr.Message != "" {
			return "The provider rejected this message: " + provErr.Message
		}
		return "The provider rejected this message."
	default:
		return "This message could not be sent."
	}
}
