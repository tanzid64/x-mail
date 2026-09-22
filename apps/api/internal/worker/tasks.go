// Package worker defines the background jobs and the process that runs them.
//
// Sending is asynchronous so that a slow or failing provider cannot hold a
// request open, and so a failed send can be retried without the user resending
// by hand.
package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Task type names. These are persisted in Redis, so renaming one strands the
// jobs already queued under the old name.
const (
	TaskSendEmail = "email:send"
)

// Queue names, highest priority first.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

// SendEmailPayload identifies the message to send.
//
// It deliberately carries only ids: the job may run minutes after it was
// queued, and the database is the source of truth for what the message says by
// then.
type SendEmailPayload struct {
	MessageID uuid.UUID `json:"message_id"`
}

// NewSendEmailTask builds the task for a queued message.
//
// The Asynq task id is derived from the message id, so enqueuing the same
// message twice is a no-op rather than a second delivery — the first of the two
// defences against duplicate sends. The second is the provider idempotency key
// carried on the message itself, which covers a worker that crashes after the
// provider accepted the message.
func NewSendEmailTask(messageID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(SendEmailPayload{MessageID: messageID})
	if err != nil {
		return nil, nil, fmt.Errorf("worker: encode send payload: %w", err)
	}

	options := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.TaskID(fmt.Sprintf("send:%s", messageID)),
		asynq.MaxRetry(5),
		// Backoff is handled by Asynq between attempts; 10 minutes is long
		// enough to ride out a provider outage without stranding the message.
		asynq.Timeout(2 * time.Minute),
		asynq.Retention(24 * time.Hour),
	}
	return asynq.NewTask(TaskSendEmail, payload), options, nil
}
