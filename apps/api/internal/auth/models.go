package auth

import (
	"time"

	"github.com/google/uuid"
)

// User is the account that owns mail accounts. It is deliberately not an email
// mailbox — see the mail account model in docs/PLAN.md §4.
type User struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	AvatarURL       *string    `json:"avatar_url"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	IsAdmin         bool       `json:"is_admin"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// PasswordHash never leaves the server.
	PasswordHash string `json:"-"`
}

// Session is a logged-in browser. The cookie value itself is never stored.
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	UserAgent  string
	IPAddress  *string
	ExpiresAt  time.Time
	LastSeenAt time.Time
	CreatedAt  time.Time
}
