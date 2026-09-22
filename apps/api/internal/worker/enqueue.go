package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// Enqueuer submits background jobs. The API holds one; the worker consumes
// what it produces.
type Enqueuer struct {
	client *asynq.Client
}

// NewEnqueuer builds a client from the same Redis connection details the rest
// of the application uses.
func NewEnqueuer(addr, password string, db int) *Enqueuer {
	return &Enqueuer{
		client: asynq.NewClient(asynq.RedisClientOpt{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

func (e *Enqueuer) Close() error { return e.client.Close() }

// EnqueueSendEmail queues a message for delivery.
//
// A task id that already exists means this message is queued or running
// already, which is success rather than an error: the caller asked for the
// message to be sent, and it will be.
func (e *Enqueuer) EnqueueSendEmail(ctx context.Context, messageID uuid.UUID) error {
	task, options, err := NewSendEmailTask(messageID)
	if err != nil {
		return err
	}

	if _, err := e.client.EnqueueContext(ctx, task, options...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("worker: enqueue send for %s: %w", messageID, err)
	}
	return nil
}

// EnqueueProcessEvent queues a recorded provider webhook for processing.
func (e *Enqueuer) EnqueueProcessEvent(ctx context.Context, eventID uuid.UUID) error {
	task, options, err := NewProcessEventTask(eventID)
	if err != nil {
		return err
	}

	if _, err := e.client.EnqueueContext(ctx, task, options...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("worker: enqueue event %s: %w", eventID, err)
	}
	return nil
}

// RedisOpt builds the Asynq connection options from the application's Redis
// settings, so the worker and the API cannot drift apart.
func RedisOpt(client *redis.Client) asynq.RedisClientOpt {
	options := client.Options()
	return asynq.RedisClientOpt{
		Addr:     options.Addr,
		Password: options.Password,
		DB:       options.DB,
	}
}
