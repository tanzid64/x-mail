// Package mailer sends the platform's own transactional email — verification
// links and password resets.
//
// This is deliberately separate from the per-mail-account providers in
// internal/providers: those send a *user's* mail using the user's own
// credentials, whereas this sends x-mail's mail using the operator's key.
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Message is a single outbound transactional email.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Mailer delivers transactional email.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// New picks a real sender when an API key is configured and falls back to
// logging otherwise, so a fresh developer checkout can complete the signup and
// password-reset flows without any third-party credentials.
func New(apiKey, from string, logger *slog.Logger) Mailer {
	if apiKey == "" {
		logger.Warn("SYSTEM_RESEND_API_KEY is not set; transactional email will be logged instead of sent")
		return &LogMailer{logger: logger}
	}
	return &ResendMailer{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// LogMailer writes messages to the log instead of delivering them.
type LogMailer struct{ logger *slog.Logger }

func (m *LogMailer) Send(_ context.Context, msg Message) error {
	m.logger.Info("transactional email (not sent — no API key configured)",
		"to", msg.To, "subject", msg.Subject, "text", msg.Text)
	return nil
}

// ResendMailer delivers through the Resend HTTP API.
type ResendMailer struct {
	apiKey string
	from   string
	client *http.Client
}

func (m *ResendMailer) Send(ctx context.Context, msg Message) error {
	payload, err := json.Marshal(map[string]any{
		"from":    m.from,
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"html":    msg.HTML,
		"text":    msg.Text,
	})
	if err != nil {
		return fmt.Errorf("mailer: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("mailer: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mailer: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("mailer: resend returned %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return nil
}
