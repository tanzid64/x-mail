// Package realtime pushes mailbox changes to connected browsers.
//
// The API and the worker are separate processes, so a message stored by the
// worker has to reach a browser held open by the API. Redis pub/sub carries the
// event between them, which also means this works unchanged with several API
// instances behind a load balancer.
package realtime

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event names. These are part of the contract with the frontend.
const (
	EventEmailReceived = "email.received"
	EventEmailUpdated  = "email.updated"
	EventThreadUpdated = "thread.updated"
	EventDraftUpdated  = "draft.updated"
)

// Event is one change worth telling a browser about.
//
// It carries ids rather than content: the client refetches through its normal
// authorized endpoints, so a bug here can never leak mail the recipient is not
// allowed to read.
type Event struct {
	Type          string         `json:"type"`
	UserID        uuid.UUID      `json:"-"`
	MailAccountID uuid.UUID      `json:"mail_account_id,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
	At            time.Time      `json:"at"`
}

// channelFor is the Redis channel a user's events travel on. Scoping by user
// means a subscriber only ever receives its own events.
func channelFor(userID uuid.UUID) string {
	return "realtime:user:" + userID.String()
}

func (e Event) encode() ([]byte, error) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	return json.Marshal(struct {
		Type          string         `json:"type"`
		MailAccountID string         `json:"mail_account_id,omitempty"`
		Data          map[string]any `json:"data,omitempty"`
		At            time.Time      `json:"at"`
	}{
		Type:          e.Type,
		MailAccountID: nonZeroUUID(e.MailAccountID),
		Data:          e.Data,
		At:            e.At,
	})
}

func nonZeroUUID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
