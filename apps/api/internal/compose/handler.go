package compose

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/httpx"
	"github.com/dark/x-mail/api/internal/validate"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// RoutesDrafts mounts /drafts. The caller wraps these with RequireAuth.
func (h *Handler) RoutesDrafts(r chi.Router) {
	r.Get("/", h.handleList)
	r.Post("/", h.handleCreate)
	r.Get("/{id}", h.handleGet)
	r.Patch("/{id}", h.handleUpdate)
	r.Delete("/{id}", h.handleDelete)
	r.Post("/{id}/send", h.handleSend)
	r.Post("/{id}/attachments", h.handleUpload)
}

// RoutesAttachments mounts /attachments.
func (h *Handler) RoutesAttachments(r chi.Router) {
	r.Get("/{id}", h.handleDownload)
	r.Delete("/{id}", h.handleDeleteAttachment)
}

// ----------------------------------------------------------------- drafts --

type addressInput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func toAddresses(list []addressInput) []Address {
	out := make([]Address, 0, len(list))
	for _, item := range list {
		out = append(out, Address{Email: item.Email, Name: item.Name})
	}
	return out
}

type createDraftRequest struct {
	MailAccountID string         `json:"mail_account_id"`
	FromAddressID *string        `json:"from_address_id"`
	ThreadID      *string        `json:"thread_id"`
	InReplyToID   *string        `json:"in_reply_to_id"`
	To            []addressInput `json:"to"`
	Cc            []addressInput `json:"cc"`
	Bcc           []addressInput `json:"bcc"`
	Subject       string         `json:"subject"`
	HTMLBody      string         `json:"html_body"`
	TextBody      string         `json:"text_body"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req createDraftRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("mail_account_id", req.MailAccountID)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	accountID, err := uuid.Parse(req.MailAccountID)
	if err != nil {
		httpx.Fail(w, r, httpx.BadRequest("mail_account_id must be a valid id."))
		return
	}

	params := CreateDraftParams{
		UserID:        user.ID,
		MailAccountID: accountID,
		To:            toAddresses(req.To),
		Cc:            toAddresses(req.Cc),
		Bcc:           toAddresses(req.Bcc),
		Subject:       req.Subject,
		HTMLBody:      req.HTMLBody,
		TextBody:      req.TextBody,
	}
	if params.FromAddressID, err = optionalID(req.FromAddressID, "from_address_id"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if params.ThreadID, err = optionalID(req.ThreadID, "thread_id"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if params.InReplyToID, err = optionalID(req.InReplyToID, "in_reply_to_id"); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	draft, err := h.service.CreateDraft(r.Context(), params)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"draft": draft})
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	accountID, err := optionalQueryID(r, "mail_account_id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	drafts, err := h.service.ListDrafts(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"drafts": drafts})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	user, draftID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	draft, err := h.service.GetDraft(r.Context(), user.ID, draftID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"draft": draft})
}

type updateDraftRequest struct {
	FromAddressID *string         `json:"from_address_id"`
	To            *[]addressInput `json:"to"`
	Cc            *[]addressInput `json:"cc"`
	Bcc           *[]addressInput `json:"bcc"`
	Subject       *string         `json:"subject"`
	HTMLBody      *string         `json:"html_body"`
	TextBody      *string         `json:"text_body"`
}

// handleUpdate is the autosave endpoint. It is deliberately incapable of
// sending: a draft only goes out through the explicit send route.
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	user, draftID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req updateDraftRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	params := UpdateDraftParams{
		Subject:  req.Subject,
		HTMLBody: req.HTMLBody,
		TextBody: req.TextBody,
	}
	if params.FromAddressID, err = optionalID(req.FromAddressID, "from_address_id"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.To != nil {
		converted := toAddresses(*req.To)
		params.To = &converted
	}
	if req.Cc != nil {
		converted := toAddresses(*req.Cc)
		params.Cc = &converted
	}
	if req.Bcc != nil {
		converted := toAddresses(*req.Bcc)
		params.Bcc = &converted
	}

	draft, err := h.service.UpdateDraft(r.Context(), user.ID, draftID, params)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"draft": draft})
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	user, draftID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.DeleteDraft(r.Context(), user.ID, draftID); err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) handleSend(w http.ResponseWriter, r *http.Request) {
	user, draftID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	queued, err := h.service.Send(r.Context(), user.ID, draftID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}

	// 202: the provider has not been called yet, the worker will do that.
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"message_id": queued.MessageID,
		"thread_id":  queued.ThreadID,
	})
}

// ------------------------------------------------------------ attachments --

// handleUpload accepts one file as multipart form data.
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	user, draftID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Cap the request body before parsing, so an oversized upload is rejected
	// without being buffered.
	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.Fail(w, r, httpx.BadRequest("That upload is too large or not valid form data."))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, r, httpx.BadRequest("Attach a file under the \"file\" field."))
		return
	}
	defer func() { _ = file.Close() }()

	attachment, err := h.service.Upload(r.Context(), user.ID, UploadParams{
		DraftID:  draftID,
		Filename: header.Filename,
		MimeType: header.Header.Get("Content-Type"),
		Size:     header.Size,
		Body:     file,
	})
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"attachment": attachment})
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	user, attachmentID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	reader, attachment, err := h.service.Download(r.Context(), user.ID, attachmentID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	defer func() { _ = reader.Close() }()

	// Attachments are always downloaded, never rendered: an HTML or SVG
	// attachment displayed inline would run as a page on our origin.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename*=UTF-8''%s", escapeFilename(attachment.Filename)))
	w.Header().Set("Content-Length", fmt.Sprint(attachment.SizeBytes))
	w.Header().Set("Cache-Control", "private, no-store")

	if _, err := copyTo(w, reader); err != nil {
		h.logger.WarnContext(r.Context(), "attachment download interrupted", "error", err)
	}
}

func (h *Handler) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	user, attachmentID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.DeleteAttachment(r.Context(), user.ID, attachmentID); err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------- helpers --

func (h *Handler) userAndID(r *http.Request) (*auth.User, uuid.UUID, error) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		return nil, uuid.Nil, err
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return nil, uuid.Nil, httpx.NotFound("That record does not exist.")
	}
	return user, id, nil
}

func optionalID(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil, httpx.BadRequest(field + " must be a valid id.")
	}
	return &id, nil
}

func optionalQueryID(r *http.Request, name string) (*uuid.UUID, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, httpx.BadRequest("The " + name + " filter must be a valid id.")
	}
	return &id, nil
}

func (h *Handler) mapError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("That record does not exist.")
	case errors.Is(err, ErrDraftSending):
		return httpx.Conflict("This message is already being sent.")
	case errors.Is(err, ErrNoRecipients):
		return httpx.UnprocessableEntity("Please correct the highlighted fields.").
			WithFields(map[string]string{"to": "Add at least one recipient."})
	case errors.Is(err, ErrNoSender):
		return httpx.UnprocessableEntity("Please correct the highlighted fields.").
			WithFields(map[string]string{"from_address_id": "Choose an address to send from."})
	case errors.Is(err, ErrSenderNotUsable):
		return httpx.UnprocessableEntity("Please correct the highlighted fields.").
			WithFields(map[string]string{
				"from_address_id": "This address cannot send yet — its domain is not verified.",
			})
	case errors.Is(err, ErrAttachmentTooLarge):
		return httpx.BadRequest("That file is larger than the 25 MB limit.")
	case errors.Is(err, ErrTooManyAttachments):
		return httpx.BadRequest("A message can carry at most 20 attachments.")
	default:
		return httpx.Internal(err)
	}
}
