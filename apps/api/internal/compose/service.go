package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/mail"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/storage"
)

// Limits on what a single attachment may be.
const (
	MaxAttachmentBytes = 25 << 20 // 25 MiB
	maxAttachmentCount = 20
)

// ErrTooManyAttachments means the draft has hit the per-message limit.
var ErrTooManyAttachments = errors.New("compose: too many attachments on this message")

// ErrAttachmentTooLarge means one file exceeds the size limit.
var ErrAttachmentTooLarge = errors.New("compose: that file is too large")

// Enqueuer hands a queued message to the background worker. It is an interface
// so the service can be tested without Redis.
type Enqueuer interface {
	EnqueueSendEmail(ctx context.Context, messageID uuid.UUID) error
}

type Service struct {
	repo    *Repository
	storage storage.Storage
	queue   Enqueuer
	logger  *slog.Logger
}

func NewService(repo *Repository, store storage.Storage, queue Enqueuer, logger *slog.Logger) *Service {
	return &Service{repo: repo, storage: store, queue: queue, logger: logger}
}

func (s *Service) CreateDraft(ctx context.Context, params CreateDraftParams) (*Draft, error) {
	params.To = normalizeAddresses(params.To)
	params.Cc = normalizeAddresses(params.Cc)
	params.Bcc = normalizeAddresses(params.Bcc)
	return s.repo.CreateDraft(ctx, params)
}

func (s *Service) GetDraft(ctx context.Context, userID, draftID uuid.UUID) (*Draft, error) {
	return s.repo.GetDraft(ctx, userID, draftID)
}

func (s *Service) ListDrafts(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]Draft, error) {
	return s.repo.ListDrafts(ctx, userID, accountID)
}

func (s *Service) UpdateDraft(ctx context.Context, userID, draftID uuid.UUID, params UpdateDraftParams) (*Draft, error) {
	if params.To != nil {
		normalized := normalizeAddresses(*params.To)
		params.To = &normalized
	}
	if params.Cc != nil {
		normalized := normalizeAddresses(*params.Cc)
		params.Cc = &normalized
	}
	if params.Bcc != nil {
		normalized := normalizeAddresses(*params.Bcc)
		params.Bcc = &normalized
	}
	return s.repo.UpdateDraft(ctx, userID, draftID, params)
}

// DeleteDraft removes a draft and the attachment bytes it owned.
func (s *Service) DeleteDraft(ctx context.Context, userID, draftID uuid.UUID) error {
	attachments, err := s.repo.ListDraftAttachments(ctx, userID, draftID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteDraft(ctx, userID, draftID); err != nil {
		return err
	}

	// Orphaned bytes are cleaned up after the row is gone: a failure here wastes
	// storage but must not fail the user's delete.
	for _, attachment := range attachments {
		if err := s.storage.Delete(ctx, attachment.StorageKey); err != nil {
			s.logger.WarnContext(ctx, "could not remove attachment bytes",
				"error", err, "storage_key", attachment.StorageKey)
		}
	}
	return nil
}

// Send turns a draft into a queued message and hands it to the worker.
//
// If queuing fails the draft is released, so the user is never left staring at
// a draft they can neither edit nor send.
func (s *Service) Send(ctx context.Context, userID, draftID uuid.UUID) (*QueuedMessage, error) {
	queued, err := s.repo.QueueDraft(ctx, userID, draftID)
	if err != nil {
		return nil, err
	}

	if err := s.queue.EnqueueSendEmail(ctx, queued.MessageID); err != nil {
		// The message row exists and is queued; a sweeper could still pick it
		// up, but the immediate failure is worth reporting.
		s.logger.ErrorContext(ctx, "could not queue send",
			"error", err, "message_id", queued.MessageID)
		return nil, fmt.Errorf("compose: could not queue the message for sending: %w", err)
	}
	return queued, nil
}

// ------------------------------------------------------------ attachments --

type UploadParams struct {
	DraftID  uuid.UUID
	Filename string
	MimeType string
	Size     int64
	Body     io.Reader
}

// Upload stores an attachment's bytes and records it against the draft.
func (s *Service) Upload(ctx context.Context, userID uuid.UUID, params UploadParams) (*Attachment, error) {
	if params.Size > MaxAttachmentBytes {
		return nil, ErrAttachmentTooLarge
	}

	existing, err := s.repo.ListDraftAttachments(ctx, userID, params.DraftID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= maxAttachmentCount {
		return nil, ErrTooManyAttachments
	}

	filename := storage.SanitizeFilename(params.Filename)
	mimeType := resolveMimeType(params.MimeType, filename)
	key := storage.BuildKey(time.Now(), filename)

	// Cap the copy as well as trusting the declared size: a client can lie
	// about Content-Length.
	object, err := s.storage.Put(ctx, key, mimeType, io.LimitReader(params.Body, MaxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if object.Size > MaxAttachmentBytes {
		_ = s.storage.Delete(ctx, key)
		return nil, ErrAttachmentTooLarge
	}

	attachment, err := s.repo.CreateAttachment(ctx, userID, CreateAttachmentParams{
		DraftID:    params.DraftID,
		Filename:   filename,
		MimeType:   mimeType,
		SizeBytes:  object.Size,
		StorageKey: key,
	})
	if err != nil {
		// Do not leave bytes behind for a row that was never created.
		_ = s.storage.Delete(ctx, key)
		return nil, err
	}
	return attachment, nil
}

// Download opens an attachment the user is allowed to read.
func (s *Service) Download(ctx context.Context, userID, attachmentID uuid.UUID) (io.ReadCloser, *Attachment, error) {
	attachment, err := s.repo.GetAttachment(ctx, userID, attachmentID)
	if err != nil {
		return nil, nil, err
	}

	reader, _, err := s.storage.Get(ctx, attachment.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return reader, attachment, nil
}

func (s *Service) DeleteAttachment(ctx context.Context, userID, attachmentID uuid.UUID) error {
	attachment, err := s.repo.DeleteAttachment(ctx, userID, attachmentID)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, attachment.StorageKey); err != nil {
		s.logger.WarnContext(ctx, "could not remove attachment bytes",
			"error", err, "storage_key", attachment.StorageKey)
	}
	return nil
}

func (s *Service) ListMessageAttachments(ctx context.Context, userID, messageID uuid.UUID) ([]Attachment, error) {
	return s.repo.ListMessageAttachments(ctx, userID, messageID)
}

// ---------------------------------------------------------------- helpers --

// normalizeAddresses lowercases addresses and drops entries that are not
// parseable, so an unusable recipient never reaches the provider.
func normalizeAddresses(list []Address) []Address {
	out := make([]Address, 0, len(list))
	seen := map[string]bool{}

	for _, address := range list {
		email := strings.ToLower(strings.TrimSpace(address.Email))
		if email == "" || seen[email] {
			continue
		}
		if _, err := mail.ParseAddress(email); err != nil {
			continue
		}
		seen[email] = true
		out = append(out, Address{Email: email, Name: strings.TrimSpace(address.Name)})
	}
	return out
}

// resolveMimeType prefers the extension over the client's claim, since the
// browser's Content-Type is attacker-controlled.
func resolveMimeType(declared, filename string) string {
	if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); byExt != "" {
		return byExt
	}
	declared = strings.TrimSpace(declared)
	if declared == "" || strings.ContainsAny(declared, "\r\n") {
		return "application/octet-stream"
	}
	return declared
}
