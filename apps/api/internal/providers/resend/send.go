package resend

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/dark/x-mail/api/internal/providers"
)

// sendRequest is Resend's POST /emails body.
type sendRequest struct {
	From        string              `json:"from"`
	To          []string            `json:"to"`
	Cc          []string            `json:"cc,omitempty"`
	Bcc         []string            `json:"bcc,omitempty"`
	ReplyTo     []string            `json:"reply_to,omitempty"`
	Subject     string              `json:"subject"`
	HTML        string              `json:"html,omitempty"`
	Text        string              `json:"text,omitempty"`
	Headers     map[string]string   `json:"headers,omitempty"`
	Attachments []requestAttachment `json:"attachments,omitempty"`
	ScheduledAt string              `json:"scheduled_at,omitempty"`
}

type requestAttachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

type sendResponse struct {
	ID string `json:"id"`
}

// Send submits a message to Resend.
//
// When the caller supplies an idempotency key it is passed through, so a job
// that is retried after the provider already accepted the message does not
// deliver it a second time.
func (c *Client) Send(ctx context.Context, email providers.Email) (*providers.SendResult, error) {
	body := sendRequest{
		From:        email.From.String(),
		To:          formatAddresses(email.To),
		Cc:          formatAddresses(email.Cc),
		Bcc:         formatAddresses(email.Bcc),
		ReplyTo:     formatAddresses(email.ReplyTo),
		Subject:     email.Subject,
		HTML:        email.HTML,
		Text:        email.Text,
		Headers:     email.Headers,
		Attachments: formatAttachments(email.Attachments),
	}
	if email.ScheduledAt != nil {
		body.ScheduledAt = email.ScheduledAt.UTC().Format(time.RFC3339)
	}

	headers := map[string]string{}
	if email.IdempotencyKey != "" {
		headers["Idempotency-Key"] = email.IdempotencyKey
	}

	var resp sendResponse
	if err := c.do(ctx, "POST", "/emails", body, headers, &resp); err != nil {
		return nil, err
	}
	if resp.ID == "" {
		return nil, providers.NewError(providers.ProviderResend, providers.KindTransient, 0,
			"resend accepted the message but returned no id", nil)
	}

	return &providers.SendResult{
		ProviderMessageID: resp.ID,
		ScheduledAt:       email.ScheduledAt,
	}, nil
}

func formatAddresses(addresses []providers.Address) []string {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(addresses))
	for _, a := range addresses {
		out = append(out, a.String())
	}
	return out
}

func formatAttachments(attachments []providers.Attachment) []requestAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]requestAttachment, 0, len(attachments))
	for _, a := range attachments {
		out = append(out, requestAttachment{
			Filename:    a.Filename,
			Content:     base64.StdEncoding.EncodeToString(a.Content),
			ContentType: a.ContentType,
			ContentID:   a.ContentID,
		})
	}
	return out
}
