package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/crypto"
	"github.com/dark/x-mail/api/internal/httpx"
)

// maxWebhookBody caps an inbound payload. A received email with attachments is
// the largest thing that arrives here.
const maxWebhookBody = 30 << 20 // 30 MiB

// Queue hands recorded events to the worker.
type Queue interface {
	EnqueueProcessEvent(ctx context.Context, eventID uuid.UUID) error
}

type Handler struct {
	repo *Repository
	// sealer opens the per-account webhook secret.
	sealer *crypto.Sealer
	// fallbackSecret is used when an account has no secret of its own, which
	// covers a single-tenant deployment configured entirely from the environment.
	fallbackSecret string
	queue          Queue
	logger         *slog.Logger
}

func NewHandler(repo *Repository, sealer *crypto.Sealer, fallbackSecret string, queue Queue, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, sealer: sealer, fallbackSecret: fallbackSecret, queue: queue, logger: logger}
}

// Routes mounts the provider callbacks. These are deliberately outside the
// session middleware: the provider has no cookie, and the signature is what
// authenticates the request.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/resend/{mailAccountID}", h.handleResend)
}

// handleResend verifies, records and queues. It does no parsing or threading:
// a webhook endpoint that does real work will eventually time out and be
// retried, turning one email into several.
func (h *Handler) handleResend(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "mailAccountID"))
	if err != nil {
		// Do not confirm or deny which accounts exist to an unauthenticated caller.
		httpx.Fail(w, r, httpx.NotFound("Unknown webhook endpoint."))
		return
	}

	// The signature covers the exact bytes received, so the raw body has to be
	// read before anything decodes it.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil {
		httpx.Fail(w, r, httpx.BadRequest("Could not read the request body."))
		return
	}
	if len(body) > maxWebhookBody {
		httpx.Fail(w, r, httpx.BadRequest("Webhook payload is too large."))
		return
	}

	secret, err := h.secretFor(r.Context(), accountID)
	if err != nil {
		h.logger.WarnContext(r.Context(), "webhook for an account with no usable secret",
			"error", err, "mail_account_id", accountID)
		httpx.Fail(w, r, httpx.NotFound("Unknown webhook endpoint."))
		return
	}

	verifier, err := NewSvixVerifier(secret)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "webhook secret is malformed", "mail_account_id", accountID)
		httpx.Fail(w, r, httpx.NotFound("Unknown webhook endpoint."))
		return
	}
	if err := verifier.Verify(r.Header, body); err != nil {
		// Anything unsigned is treated the same as anything else unauthorised.
		h.logger.WarnContext(r.Context(), "rejected an unverified webhook",
			"error", err, "mail_account_id", accountID)
		httpx.Fail(w, r, httpx.Unauthorized("Signature verification failed."))
		return
	}

	var envelope Envelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Type == "" {
		httpx.Fail(w, r, httpx.BadRequest("Malformed webhook payload."))
		return
	}

	eventID := r.Header.Get("svix-id")
	if eventID == "" {
		eventID = r.Header.Get("webhook-id")
	}

	stored, err := h.repo.Record(r.Context(), accountID, "resend", eventID, envelope.Type, body)
	if err != nil {
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}

	if stored.Duplicate {
		// A redelivery. Acknowledging without queuing is what makes the
		// endpoint idempotent.
		h.logger.InfoContext(r.Context(), "ignored a duplicate webhook",
			"provider_event_id", eventID, "type", envelope.Type)
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}

	if err := h.queue.EnqueueProcessEvent(r.Context(), stored.ID); err != nil {
		// The event is recorded; a sweeper can still pick it up. Returning 500
		// asks the provider to redeliver, which the unique constraint makes safe.
		h.logger.ErrorContext(r.Context(), "could not queue webhook processing",
			"error", err, "event_id", stored.ID)
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// secretFor prefers the account's own secret and falls back to the deployment
// one.
func (h *Handler) secretFor(ctx context.Context, accountID uuid.UUID) (string, error) {
	sealed, err := h.repo.AccountWebhookSecret(ctx, accountID)
	if err != nil {
		return "", err
	}

	if len(sealed) > 0 {
		secret, err := h.sealer.Open(sealed)
		if err != nil {
			return "", err
		}
		return secret, nil
	}

	if h.fallbackSecret == "" {
		return "", errors.New("webhooks: no signing secret configured for this account")
	}
	return h.fallbackSecret, nil
}
