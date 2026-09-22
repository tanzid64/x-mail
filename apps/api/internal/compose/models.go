package compose

import (
	"time"

	"github.com/google/uuid"
)

// Draft status values, mirrored from the CHECK constraint.
const (
	DraftStatusDraft   = "draft"
	DraftStatusSending = "sending"
)

// Address is a recipient as the compose form holds it.
type Address struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// Draft is an unsent message being composed.
type Draft struct {
	ID            uuid.UUID  `json:"id"`
	MailAccountID uuid.UUID  `json:"mail_account_id"`
	ThreadID      *uuid.UUID `json:"thread_id"`
	InReplyToID   *uuid.UUID `json:"in_reply_to_id"`
	FromAddressID *uuid.UUID `json:"from_address_id"`

	To  []Address `json:"to"`
	Cc  []Address `json:"cc"`
	Bcc []Address `json:"bcc"`

	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`

	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Attachments []Attachment `json:"attachments"`
}

// Attachment is a file carried by a draft or a sent message.
type Attachment struct {
	ID        uuid.UUID  `json:"id"`
	MessageID *uuid.UUID `json:"message_id"`
	DraftID   *uuid.UUID `json:"draft_id"`
	Filename  string     `json:"filename"`
	MimeType  string     `json:"mime_type"`
	SizeBytes int64      `json:"size_bytes"`
	ContentID *string    `json:"content_id"`
	IsInline  bool       `json:"is_inline"`
	CreatedAt time.Time  `json:"created_at"`

	// StorageKey stays server-side: it is the only thing needed to fetch the
	// bytes, so handing it to the browser would bypass authorization.
	StorageKey string `json:"-"`
}
