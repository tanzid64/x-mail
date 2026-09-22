package mailbox

import (
	"time"

	"github.com/google/uuid"
)

// System label keys. These are the placement labels every mail account gets.
const (
	LabelInbox   = "INBOX"
	LabelSent    = "SENT"
	LabelDraft   = "DRAFT"
	LabelArchive = "ARCHIVE"
	LabelSpam    = "SPAM"
	LabelTrash   = "TRASH"
)

// Message direction and status values, mirrored from the CHECK constraints.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"

	StatusDraft     = "draft"
	StatusQueued    = "queued"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusBounced   = "bounced"
	StatusFailed    = "failed"
	StatusReceived  = "received"
)

// Participant is one person shown in a thread's summary line.
type Participant struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Thread is a conversation. Its counters are maintained by refresh_thread so
// the mailbox list never has to aggregate over messages.
type Thread struct {
	ID             uuid.UUID     `json:"id"`
	MailAccountID  uuid.UUID     `json:"mail_account_id"`
	Subject        string        `json:"subject"`
	Snippet        string        `json:"snippet"`
	MessageCount   int           `json:"message_count"`
	UnreadCount    int           `json:"unread_count"`
	HasAttachments bool          `json:"has_attachments"`
	IsStarred      bool          `json:"is_starred"`
	Participants   []Participant `json:"participants"`
	LastMessageAt  time.Time     `json:"last_message_at"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`

	// Labels the thread's messages carry, for the chips on a row.
	Labels []Label `json:"labels,omitempty"`

	// AccountName and AccountColor let a unified-inbox row show where it came
	// from without the client joining accounts itself.
	AccountName  string `json:"account_name,omitempty"`
	AccountColor string `json:"account_color,omitempty"`
}

// Recipient is one addressee of a message.
type Recipient struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
	// Type is "to", "cc" or "bcc".
	Type string `json:"type"`
}

// Message is a single email within a thread.
type Message struct {
	ID            uuid.UUID `json:"id"`
	MailAccountID uuid.UUID `json:"mail_account_id"`
	ThreadID      uuid.UUID `json:"thread_id"`

	MessageID         *string  `json:"message_id"`
	InReplyTo         *string  `json:"in_reply_to"`
	References        []string `json:"references"`
	ProviderMessageID *string  `json:"provider_message_id"`

	SenderName  string `json:"sender_name"`
	SenderEmail string `json:"sender_email"`

	Subject string `json:"subject"`
	Snippet string `json:"snippet"`
	// HTMLBody is the body exactly as received and is never rendered directly.
	HTMLBody string `json:"html_body"`
	// HTMLSanitized is the render-safe copy, produced once on arrival.
	HTMLSanitized string `json:"html_sanitized"`
	TextBody      string `json:"text_body"`

	Direction string `json:"direction"`
	Status    string `json:"status"`

	IsRead         bool `json:"is_read"`
	IsStarred      bool `json:"is_starred"`
	IsImportant    bool `json:"is_important"`
	HasAttachments bool `json:"has_attachments"`

	SizeBytes int `json:"size_bytes"`

	SentAt     *time.Time `json:"sent_at"`
	ReceivedAt *time.Time `json:"received_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`

	Recipients []Recipient `json:"recipients"`
	Labels     []Label     `json:"labels"`
}

// Label is a system or user-created label.
type Label struct {
	ID            uuid.UUID `json:"id"`
	MailAccountID uuid.UUID `json:"mail_account_id"`
	Name          string    `json:"name"`
	Color         string    `json:"color"`
	// SystemKey is set for built-in labels and nil for user labels.
	SystemKey *string   `json:"system_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsSystem reports whether the label is built in and therefore not renameable
// or deletable.
func (l Label) IsSystem() bool { return l.SystemKey != nil }

// ThreadDetail is a thread together with its messages.
type ThreadDetail struct {
	Thread   Thread    `json:"thread"`
	Messages []Message `json:"messages"`
}

// Page is a cursor-paginated slice of threads.
type Page struct {
	Threads []Thread `json:"threads"`
	// NextCursor is empty when there are no further pages.
	NextCursor string `json:"next_cursor"`
}
