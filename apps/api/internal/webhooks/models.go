package webhooks

import (
	"encoding/json"
	"strings"
	"time"
)

// Event types Resend sends. Anything not listed is recorded and ignored, so a
// new provider event cannot break the endpoint.
const (
	EventReceived        = "email.received"
	EventSent            = "email.sent"
	EventDelivered       = "email.delivered"
	EventDeliveryDelayed = "email.delivery_delayed"
	EventBounced         = "email.bounced"
	EventComplained      = "email.complained"
	EventOpened          = "email.opened"
	EventClicked         = "email.clicked"
)

// Envelope is the outer shape every Resend webhook shares.
type Envelope struct {
	Type      string          `json:"type"`
	CreatedAt string          `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// Header is one RFC 5322 header as Resend reports it.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// InboundAttachment is a file on a received message.
type InboundAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	// Content is base64; Resend may instead give a URL for large files.
	Content    string `json:"content"`
	ContentID  string `json:"content_id"`
	ContentURL string `json:"content_url"`
	Size       int64  `json:"size"`
}

// EventData covers the union of fields across Resend's event payloads. A
// single struct is simpler than one per event type, since they overlap heavily
// and absent fields decode as zero values.
type EventData struct {
	EmailID string   `json:"email_id"`
	ID      string   `json:"id"`
	From    string   `json:"from"`
	To      []string `json:"to"`
	Cc      []string `json:"cc"`
	Bcc     []string `json:"bcc"`
	ReplyTo []string `json:"reply_to"`
	Subject string   `json:"subject"`

	HTML string `json:"html"`
	Text string `json:"text"`

	Headers     []Header            `json:"headers"`
	Attachments []InboundAttachment `json:"attachments"`

	CreatedAt string `json:"created_at"`

	// Present on bounce events.
	Bounce *struct {
		Type    string `json:"type"`
		SubType string `json:"subType"`
		Message string `json:"message"`
	} `json:"bounce"`

	// Present on click events.
	Click *struct {
		Link string `json:"link"`
	} `json:"click"`
}

// ProviderEmailID is the provider's id for the message, which is what
// correlates a delivery or bounce back to the message we sent.
func (d EventData) ProviderEmailID() string {
	if d.EmailID != "" {
		return d.EmailID
	}
	return d.ID
}

// Header returns a named header, case-insensitively.
func (d EventData) Header(name string) string {
	for _, header := range d.Headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

// References parses the References header into its individual message ids.
func (d EventData) References() []string {
	raw := d.Header("References")
	if raw == "" {
		return nil
	}
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// OccurredAt is when the provider says the event happened.
func (e Envelope) OccurredAt() time.Time {
	if parsed, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
		return parsed
	}
	return time.Now()
}
