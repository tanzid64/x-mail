package accounts

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/httpx"
	"github.com/dark/x-mail/api/internal/providers"
	"github.com/dark/x-mail/api/internal/validate"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// RoutesAccounts mounts /mail-accounts. The caller wraps these with RequireAuth.
func (h *Handler) RoutesAccounts(r chi.Router) {
	r.Get("/", h.handleList)
	r.Post("/", h.handleConnect)
	r.Get("/{id}", h.handleGet)
	r.Patch("/{id}", h.handleUpdate)
	r.Delete("/{id}", h.handleDelete)
	r.Post("/{id}/default", h.handleSetDefault)
	r.Post("/{id}/api-key", h.handleRotateAPIKey)
	r.Post("/{id}/check", h.handleCheckCredentials)
	r.Post("/{id}/sync-domains", h.handleSyncDomains)
	r.Post("/{id}/webhook-secret", h.handleSetWebhookSecret)
}

// RoutesDomains mounts /domains.
func (h *Handler) RoutesDomains(r chi.Router) {
	r.Get("/", h.handleListDomains)
	r.Get("/{id}", h.handleGetDomain)
	r.Post("/{id}/refresh", h.handleRefreshDomain)
	r.Post("/{id}/addresses", h.handleCreateAddress)
}

// RoutesAddresses mounts /addresses.
func (h *Handler) RoutesAddresses(r chi.Router) {
	r.Get("/", h.handleListAddresses)
	r.Get("/{id}", h.handleGetAddress)
	r.Patch("/{id}", h.handleUpdateAddress)
	r.Delete("/{id}", h.handleDeleteAddress)
	r.Post("/{id}/default", h.handleSetDefaultAddress)
}

// --------------------------------------------------------------- accounts --

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	list, err := h.service.List(r.Context(), user.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mail_accounts": list})
}

type connectRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Color    string `json:"color"`
}

func (h *Handler) handleConnect(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req connectRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("name", req.Name)
	v.MaxLen("name", req.Name, 80)
	v.Required("provider", req.Provider)
	v.In("provider", req.Provider, string(providers.ProviderResend))
	v.Required("api_key", req.APIKey)
	v.MaxLen("api_key", req.APIKey, 512)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	account, err := h.service.Connect(r.Context(), user.ID, ConnectParams{
		Name:     req.Name,
		Provider: providers.Provider(req.Provider),
		APIKey:   req.APIKey,
		Color:    req.Color,
	})
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, "api_key"))
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"mail_account": account})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	account, err := h.service.Get(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mail_account": account})
}

type updateAccountRequest struct {
	Name     *string `json:"name"`
	Color    *string `json:"color"`
	IsActive *bool   `json:"is_active"`
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req updateAccountRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if req.Name != nil {
		v := validate.New()
		v.Required("name", *req.Name)
		v.MaxLen("name", *req.Name, 80)
		if err := v.Err(); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}

	account, err := h.service.Update(r.Context(), user.ID, accountID, UpdateAccountParams{
		Name: req.Name, Color: req.Color, IsActive: req.IsActive,
	})
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, "name"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mail_account": account})
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.Delete(r.Context(), user.ID, accountID); err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) handleSetDefault(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.SetDefault(r.Context(), user.ID, accountID); err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}

	list, err := h.service.List(r.Context(), user.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mail_accounts": list})
}

type rotateKeyRequest struct {
	APIKey string `json:"api_key"`
}

func (h *Handler) handleRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req rotateKeyRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("api_key", req.APIKey)
	v.MaxLen("api_key", req.APIKey, 512)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	account, err := h.service.RotateAPIKey(r.Context(), user.ID, accountID, req.APIKey)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, "api_key"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mail_account": account})
}

type webhookSecretRequest struct {
	Secret string `json:"secret"`
}

// handleSetWebhookSecret stores the provider's signing secret for this account.
func (h *Handler) handleSetWebhookSecret(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req webhookSecretRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.MaxLen("secret", req.Secret, 512)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	account, err := h.service.SetWebhookSecret(r.Context(), user.ID, accountID, req.Secret)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, "secret"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mail_account": account})
}

func (h *Handler) handleCheckCredentials(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	account, err := h.service.CheckCredentials(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mail_account": account})
}

func (h *Handler) handleSyncDomains(w http.ResponseWriter, r *http.Request) {
	user, accountID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	domains, err := h.service.SyncDomains(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"domains": domains})
}

// ---------------------------------------------------------------- domains --

func (h *Handler) handleListDomains(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	accountID, err := optionalUUIDParam(r, "mail_account_id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	domains, err := h.service.ListDomains(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"domains": domains})
}

func (h *Handler) handleGetDomain(w http.ResponseWriter, r *http.Request) {
	user, domainID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	domain, err := h.service.GetDomain(r.Context(), user.ID, domainID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"domain": domain})
}

func (h *Handler) handleRefreshDomain(w http.ResponseWriter, r *http.Request) {
	user, domainID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	domain, err := h.service.RefreshDomain(r.Context(), user.ID, domainID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"domain": domain})
}

// -------------------------------------------------------------- addresses --

type createAddressRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func (h *Handler) handleCreateAddress(w http.ResponseWriter, r *http.Request) {
	user, domainID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req createAddressRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("email", req.Email)
	v.Email("email", req.Email)
	v.MaxLen("display_name", req.DisplayName, 120)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	address, err := h.service.CreateAddress(r.Context(), user.ID, domainID, req.Email, req.DisplayName)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, "email"))
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"address": address})
}

func (h *Handler) handleListAddresses(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	accountID, err := optionalUUIDParam(r, "mail_account_id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	addresses, err := h.service.ListAddresses(r.Context(), user.ID, accountID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"addresses": addresses})
}

func (h *Handler) handleGetAddress(w http.ResponseWriter, r *http.Request) {
	user, addressID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	address, err := h.service.GetAddress(r.Context(), user.ID, addressID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"address": address})
}

type updateAddressRequest struct {
	DisplayName *string `json:"display_name"`
	IsActive    *bool   `json:"is_active"`
}

func (h *Handler) handleUpdateAddress(w http.ResponseWriter, r *http.Request) {
	user, addressID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req updateAddressRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	address, err := h.service.UpdateAddress(r.Context(), user.ID, addressID, UpdateAddressParams{
		DisplayName: req.DisplayName, IsActive: req.IsActive,
	})
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"address": address})
}

func (h *Handler) handleSetDefaultAddress(w http.ResponseWriter, r *http.Request) {
	user, addressID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.SetDefaultAddress(r.Context(), user.ID, addressID); err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}

	address, err := h.service.GetAddress(r.Context(), user.ID, addressID)
	if err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"address": address})
}

func (h *Handler) handleDeleteAddress(w http.ResponseWriter, r *http.Request) {
	user, addressID, err := h.userAndID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.DeleteAddress(r.Context(), user.ID, addressID); err != nil {
		httpx.Fail(w, r, h.mapError(err, ""))
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
		// A malformed id could never name a real row, so this is a 404 rather
		// than a 400 — and it reads the same as someone else's id would.
		return nil, uuid.Nil, httpx.NotFound("That record does not exist.")
	}
	return user, id, nil
}

func optionalUUIDParam(r *http.Request, name string) (*uuid.UUID, error) {
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

// mapError translates domain errors into API responses.
//
// Anything the user does not own surfaces as 404, never 403: telling someone
// that a record exists but belongs to another user is itself a disclosure.
func (h *Handler) mapError(err error, field string) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("That record does not exist.")

	case errors.Is(err, ErrInvalidCredentials):
		return httpx.UnprocessableEntity("Please correct the highlighted fields.").
			WithFields(map[string]string{field: "The provider rejected this API key."})

	case errors.Is(err, ErrTokenRestricted):
		return httpx.Forbidden("This API token is limited to one mail account.")

	case errors.Is(err, ErrNameTaken):
		return httpx.Conflict("You already have a mail account with that name.")

	case errors.Is(err, ErrAddressTaken):
		return httpx.Conflict("That email address is already in use.")

	case errors.Is(err, ErrAddressNotOnDomain):
		return httpx.UnprocessableEntity("Please correct the highlighted fields.").
			WithFields(map[string]string{"email": "This address must be on the selected domain."})

	default:
		return httpx.Internal(err)
	}
}
