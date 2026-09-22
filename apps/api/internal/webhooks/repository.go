package webhooks

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("webhooks: not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// StoredEvent is a recorded provider callback.
type StoredEvent struct {
	ID            uuid.UUID
	MailAccountID *uuid.UUID
	EventType     string
	Payload       []byte
	// Duplicate is true when this event had already been recorded, which is how
	// a redelivered webhook is recognised.
	Duplicate bool
}

// Record stores an event exactly once.
//
// Providers redeliver webhooks whenever they are unsure the first attempt
// arrived, so the unique constraint on (provider, provider_event_id) — not a
// check-then-insert — is what guarantees one row and therefore one piece of
// work, even for two deliveries racing each other.
func (r *Repository) Record(
	ctx context.Context,
	accountID uuid.UUID,
	provider, providerEventID, eventType string,
	payload []byte,
) (*StoredEvent, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO email_events (mail_account_id, provider, provider_event_id, event_type, payload)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
		RETURNING id`,
		accountID, provider, providerEventID, eventType, payload).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		// Already recorded: find the original so the caller can report it.
		var existing uuid.UUID
		if err := r.db.QueryRow(ctx,
			`SELECT id FROM email_events WHERE provider = $1 AND provider_event_id = $2`,
			provider, providerEventID).Scan(&existing); err != nil {
			return nil, err
		}
		return &StoredEvent{ID: existing, MailAccountID: &accountID, EventType: eventType, Duplicate: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &StoredEvent{ID: id, MailAccountID: &accountID, EventType: eventType, Payload: payload}, nil
}

// Load reads a recorded event for processing.
func (r *Repository) Load(ctx context.Context, eventID uuid.UUID) (*StoredEvent, *EventOwner, error) {
	var event StoredEvent
	var owner EventOwner
	var processedAt *string

	err := r.db.QueryRow(ctx, `
		SELECT e.id, e.mail_account_id, e.event_type, e.payload, e.processed_at::text, a.user_id
		  FROM email_events e
		  LEFT JOIN mail_accounts a ON a.id = e.mail_account_id
		 WHERE e.id = $1`, eventID,
	).Scan(&event.ID, &event.MailAccountID, &event.EventType, &event.Payload, &processedAt, &owner.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	owner.AlreadyProcessed = processedAt != nil
	if event.MailAccountID != nil {
		owner.MailAccountID = *event.MailAccountID
	}
	return &event, &owner, nil
}

// EventOwner is who an event belongs to.
type EventOwner struct {
	MailAccountID    uuid.UUID
	UserID           *uuid.UUID
	AlreadyProcessed bool
}

// MarkProcessed records that an event has been acted on, so a replay is a
// no-op rather than a second inbox entry.
func (r *Repository) MarkProcessed(ctx context.Context, eventID uuid.UUID, processError string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE email_events SET processed_at = now(), process_error = $2 WHERE id = $1`,
		eventID, processError)
	return err
}

// LinkMessage associates an event with the message it concerns.
func (r *Repository) LinkMessage(ctx context.Context, eventID, messageID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE email_events SET message_id = $2 WHERE id = $1`, eventID, messageID)
	return err
}

// AccountWebhookSecret returns the encrypted signing secret for an account.
func (r *Repository) AccountWebhookSecret(ctx context.Context, accountID uuid.UUID) ([]byte, error) {
	var secret []byte
	err := r.db.QueryRow(ctx,
		`SELECT webhook_secret_encrypted FROM mail_accounts WHERE id = $1`, accountID).Scan(&secret)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return secret, nil
}
