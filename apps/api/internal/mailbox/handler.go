package mailbox

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

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

// RoutesThreads mounts /threads. The caller wraps these with RequireAuth.
func (h *Handler) RoutesThreads(r chi.Router) {
	r.Get("/", h.handleList)
	r.Get("/counts", h.handleCounts)
	r.Post("/actions", h.handleBulkAction)
	r.Get("/{id}", h.handleGet)
}

// RoutesLabels mounts /labels.
func (h *Handler) RoutesLabels(r chi.Router) {
	r.Get("/", h.handleListLabels)
	r.Post("/", h.handleCreateLabel)
	r.Patch("/{id}", h.handleUpdateLabel)
	r.Delete("/{id}", h.handleDeleteLabel)
}

// ---------------------------------------------------------------- threads --

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	folder := FolderInbox
	if raw := r.URL.Query().Get("folder"); raw != "" {
		parsed, err := ParseFolder(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.BadRequest("Unknown folder."))
			return
		}
		folder = parsed
	}

	accountID, err := optionalUUID(r, "mail_account_id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	labelID, err := optionalUUID(r, "label_id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, convErr := strconv.Atoi(raw)
		if convErr != nil || parsed <= 0 {
			httpx.Fail(w, r, httpx.BadRequest("limit must be a positive number."))
			return
		}
		limit = parsed
	}

	page, err := h.service.ListThreads(r.Context(), user.ID, ListRequest{
		AccountID:  accountID,
		Folder:     folder,
		LabelID:    labelID,
		Limit:      limit,
		Cursor:     r.URL.Query().Get("cursor"),
		UnreadOnly: r.URL.Query().Get("unread") == "true",
	})
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	user, threadID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Opening a thread marks it read, unless the client is only peeking.
	markRead := r.URL.Query().Get("mark_read") != "false"

	detail, err := h.service.GetThread(r.Context(), user.ID, threadID, markRead)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handler) handleCounts(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	accountID, err := optionalUUID(r, "mail_account_id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	counts, err := h.service.FolderCounts(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"counts": counts})
}

type bulkActionRequest struct {
	Action    string   `json:"action"`
	ThreadIDs []string `json:"thread_ids"`
	// LabelID is required for the label/unlabel actions.
	LabelID *string `json:"label_id"`
}

// handleBulkAction applies one action to many threads, which is what the
// toolbar does for a multi-row selection.
func (h *Handler) handleBulkAction(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req bulkActionRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("action", req.Action)
	v.Check(len(req.ThreadIDs) > 0, "thread_ids", "Select at least one conversation.")
	v.Check(len(req.ThreadIDs) <= 500, "thread_ids", "Too many conversations in one request.")
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	threadIDs := make([]uuid.UUID, 0, len(req.ThreadIDs))
	for _, raw := range req.ThreadIDs {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			httpx.Fail(w, r, httpx.BadRequest("thread_ids must all be valid ids."))
			return
		}
		threadIDs = append(threadIDs, id)
	}

	// Labelling is a separate call shape because it needs a label id.
	if req.Action == "label" || req.Action == "unlabel" {
		if req.LabelID == nil {
			httpx.Fail(w, r, httpx.BadRequest("label_id is required for this action."))
			return
		}
		labelID, parseErr := uuid.Parse(*req.LabelID)
		if parseErr != nil {
			httpx.Fail(w, r, httpx.BadRequest("label_id must be a valid id."))
			return
		}
		affected, applyErr := h.service.ApplyLabel(r.Context(), user.ID, threadIDs, labelID, req.Action == "label")
		if applyErr != nil {
			httpx.Fail(w, r, h.mapError(applyErr))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"affected": affected})
		return
	}

	action, ok := ParseAction(req.Action)
	if !ok {
		httpx.Fail(w, r, httpx.BadRequest("Unknown action."))
		return
	}

	affected, err := h.service.Apply(r.Context(), user.ID, action, threadIDs, nil)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"affected": affected})
}

// ----------------------------------------------------------------- labels --

func (h *Handler) handleListLabels(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	accountID, err := optionalUUID(r, "mail_account_id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	labels, err := h.service.ListLabels(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"labels": labels})
}

type createLabelRequest struct {
	MailAccountID string `json:"mail_account_id"`
	Name          string `json:"name"`
	Color         string `json:"color"`
}

func (h *Handler) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req createLabelRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("name", req.Name)
	v.MaxLen("name", req.Name, 60)
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

	label, err := h.service.CreateLabel(r.Context(), user.ID, accountID, req.Name, req.Color)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"label": label})
}

type updateLabelRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (h *Handler) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	user, labelID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req updateLabelRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if req.Name != nil {
		v := validate.New()
		v.Required("name", *req.Name)
		v.MaxLen("name", *req.Name, 60)
		if err := v.Err(); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}

	label, err := h.service.UpdateLabel(r.Context(), user.ID, labelID, req.Name, req.Color)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"label": label})
}

func (h *Handler) handleDeleteLabel(w http.ResponseWriter, r *http.Request) {
	user, labelID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.DeleteLabel(r.Context(), user.ID, labelID); err != nil {
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

func optionalUUID(r *http.Request, name string) (*uuid.UUID, error) {
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
		return httpx.NotFound("That conversation does not exist.")
	case errors.Is(err, ErrInvalidPage):
		return httpx.BadRequest("That pagination cursor is not valid.")
	case errors.Is(err, ErrLabelExists):
		return httpx.Conflict("You already have a label with that name.")
	case errors.Is(err, ErrSystemLabel):
		return httpx.BadRequest("Built-in labels cannot be renamed or removed.")
	default:
		return httpx.Internal(err)
	}
}
