package realtime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Publisher sends events to whichever process is holding the user's socket.
type Publisher struct {
	rdb    *redis.Client
	logger *slog.Logger
}

func NewPublisher(rdb *redis.Client, logger *slog.Logger) *Publisher {
	return &Publisher{rdb: rdb, logger: logger}
}

// Publish delivers an event to the user's channel.
//
// Failures are returned but are rarely worth failing a request over: realtime
// is an optimisation, and a client that misses an event still sees the change
// on its next refetch.
func (p *Publisher) Publish(ctx context.Context, event Event) error {
	if event.UserID == uuid.Nil {
		return fmt.Errorf("realtime: event has no user")
	}

	payload, err := event.encode()
	if err != nil {
		return err
	}
	return p.rdb.Publish(ctx, channelFor(event.UserID), payload).Err()
}

// PublishQuietly logs a failure instead of returning it, for call sites where
// the work has already succeeded and only the notification would fail.
func (p *Publisher) PublishQuietly(ctx context.Context, event Event) {
	if err := p.Publish(ctx, event); err != nil {
		p.logger.WarnContext(ctx, "could not publish realtime event",
			"error", err, "type", event.Type)
	}
}
