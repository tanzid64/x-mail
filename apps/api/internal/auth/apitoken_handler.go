package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/httpx"
	"github.com/dark/x-mail/api/internal/ratelimit"
)

// RoutesAPITokens mounts token management.
//
// Every route here is behind RequireSession as well as RequireAuth: a token
// must not be able to issue or revoke tokens, or its expiry would be a
// suggestion rather than a limit.
func (h *Handler) RoutesAPITokens(r chi.Router) {
	r.Use(h.RequireSession)
	r.Get("/", h.handleListAPITokens)
	r.Group(func(r chi.Router) {
		r.Use(h.limiter.Middleware("auth.token_create", 20, time.Hour, ratelimit.ByIP))
		r.Post("/", h.handleCreateAPIToken)
	})
	r.Delete("/{tokenID}", h.handleRevokeAPIToken)
}

type apiTokenResponse struct {
	APIToken
	// ExpiresIn echoes which of the four lifetimes produced this token's
	// expiry, so the UI does not have to infer it from a date.
	ExpiresIn Lifetime `json:"expires_in"`
}

type createAPITokenRequest struct {
	Name          string     `json:"name"`
	ExpiresIn     Lifetime   `json:"expires_in"`
	MailAccountID *uuid.UUID `json:"mail_account_id"`
}

type createAPITokenResponse struct {
	APIToken apiTokenResponse `json:"api_token"`
	// Token is the secret, returned here and nowhere else, ever again.
	Token string `json:"token"`
}

func (h *Handler) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	user, err := MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	tokens, err := h.service.ListAPITokens(r.Context(), user.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	out := make([]apiTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, apiTokenResponse{APIToken: token, ExpiresIn: lifetimeOf(&token)})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"api_tokens": out})
}

func (h *Handler) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	user, err := MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req createAPITokenRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	created, err := h.service.CreateAPIToken(r.Context(), user.ID, CreateTokenRequest{
		Name:          req.Name,
		Lifetime:      req.ExpiresIn,
		MailAccountID: req.MailAccountID,
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, createAPITokenResponse{
		APIToken: apiTokenResponse{APIToken: *created.Token, ExpiresIn: req.ExpiresIn},
		Token:    created.Plaintext,
	})
}

func (h *Handler) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	user, err := MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	tokenID, err := uuid.Parse(chi.URLParam(r, "tokenID"))
	if err != nil {
		httpx.Fail(w, r, httpx.NotFound("That API token does not exist."))
		return
	}

	if err := h.service.RevokeAPIToken(r.Context(), user.ID, tokenID); err != nil {
		if errors.Is(err, ErrAPITokenNotFound) {
			httpx.Fail(w, r, httpx.NotFound("That API token does not exist."))
			return
		}
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// lifetimeOf recovers which option a stored token was created with.
//
// The expiry is the authority; this is only so the list can say "30 days"
// rather than making the reader subtract two dates. A token whose span matches
// none of the four (one issued before an option changed, say) reports the date
// it actually has by falling back to the nearest longer option.
func lifetimeOf(token *APIToken) Lifetime {
	if token.ExpiresAt == nil {
		return LifetimeNever
	}
	span := token.ExpiresAt.Sub(token.CreatedAt)
	switch {
	case span <= 15*24*time.Hour+time.Hour:
		return Lifetime15Days
	case span <= 30*24*time.Hour+time.Hour:
		return Lifetime30Days
	default:
		return Lifetime1Year
	}
}
