package auth

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dark/x-mail/api/internal/httpx"
	"github.com/dark/x-mail/api/internal/ratelimit"
	"github.com/dark/x-mail/api/internal/validate"
)

type Handler struct {
	service *Service
	cookies Cookies
	limiter *ratelimit.Limiter
	logger  *slog.Logger
}

func NewHandler(service *Service, cookies Cookies, limiter *ratelimit.Limiter, logger *slog.Logger) *Handler {
	return &Handler{service: service, cookies: cookies, limiter: limiter, logger: logger}
}

// RoutesPublic mounts the endpoints usable without a session. Each is rate
// limited by IP: these are the endpoints an attacker can reach anonymously.
func (h *Handler) RoutesPublic(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.limiter.Middleware("auth.login", 10, time.Minute, ratelimit.ByIP))
		r.Post("/login", h.handleLogin)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.limiter.Middleware("auth.register", 5, time.Hour, ratelimit.ByIP))
		r.Post("/register", h.handleRegister)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.limiter.Middleware("auth.reset", 5, time.Hour, ratelimit.ByIP))
		r.Post("/forgot-password", h.handleForgotPassword)
		r.Post("/reset-password", h.handleResetPassword)
	})
	r.Post("/verify-email", h.handleVerifyEmail)
	r.Post("/logout", h.handleLogout)
}

// RoutesMe mounts the endpoints that describe and modify the signed-in user.
// The caller is expected to wrap these with RequireAuth.
func (h *Handler) RoutesMe(r chi.Router) {
	r.Get("/", h.handleMe)
	r.Patch("/", h.handleUpdateMe)
	// A token must not be able to change the password: that would let a stolen
	// key lock the owner out of the settings page where it is revoked.
	r.With(h.RequireSession).Post("/password", h.handleChangePassword)
	r.Post("/resend-verification", h.handleResendVerification)
}

// ------------------------------------------------------------- registration --

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("email", req.Email)
	v.Email("email", req.Email)
	v.Required("password", req.Password)
	v.MinLen("password", req.Password, minPasswordLength)
	v.MaxLen("password", req.Password, 256)
	v.MaxLen("name", req.Name, 120)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	user, token, err := h.service.Register(r.Context(), req.Email, req.Password, req.Name, h.requestMeta(r))
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			httpx.Fail(w, r, httpx.Conflict("An account with that email already exists."))
			return
		}
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}

	h.cookies.Set(w, token)
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": user})
}

// -------------------------------------------------------------------- login --

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("email", req.Email)
	v.Required("password", req.Password)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	user, token, err := h.service.Login(r.Context(), req.Email, req.Password, h.requestMeta(r))
	if err != nil {
		// Both "no such account" and "wrong password" land here deliberately.
		httpx.Fail(w, r, httpx.Unauthorized("Email or password is incorrect."))
		return
	}

	h.cookies.Set(w, token)
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Logout(r.Context(), h.cookies.Read(r)); err != nil {
		h.logger.WarnContext(r.Context(), "delete session on logout", "error", err)
	}
	h.cookies.Clear(w)
	httpx.NoContent(w)
}

// ----------------------------------------------------------- password reset --

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

func (h *Handler) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("email", req.Email)
	v.Email("email", req.Email)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if err := h.service.RequestPasswordReset(r.Context(), req.Email); err != nil {
		h.logger.ErrorContext(r.Context(), "request password reset", "error", err)
	}

	// Always the same answer, whether or not the address exists.
	httpx.JSON(w, http.StatusOK, map[string]string{
		"message": "If that email is registered, a reset link is on its way.",
	})
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (h *Handler) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("token", req.Token)
	v.Required("password", req.Password)
	v.MinLen("password", req.Password, minPasswordLength)
	v.MaxLen("password", req.Password, 256)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if err := h.service.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			httpx.Fail(w, r, httpx.BadRequest("That reset link is invalid or has expired."))
			return
		}
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}

	// The reset invalidated every session, including any this browser held.
	h.cookies.Clear(w)
	httpx.JSON(w, http.StatusOK, map[string]string{
		"message": "Your password has been changed. Please sign in.",
	})
}

// ------------------------------------------------------ email verification --

type verifyEmailRequest struct {
	Token string `json:"token"`
}

func (h *Handler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Token == "" {
		httpx.Fail(w, r, httpx.BadRequest("A verification token is required."))
		return
	}

	user, err := h.service.VerifyEmail(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			httpx.Fail(w, r, httpx.BadRequest("That confirmation link is invalid or has expired."))
			return
		}
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	user, err := MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.ResendVerification(r.Context(), user); err != nil {
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Confirmation email sent."})
}

// --------------------------------------------------------------------- me --

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user})
}

type updateMeRequest struct {
	Name      *string `json:"name"`
	AvatarURL *string `json:"avatar_url"`
}

func (h *Handler) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	user, err := MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req updateMeRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	name := user.Name
	if req.Name != nil {
		name = *req.Name
	}
	avatar := user.AvatarURL
	if req.AvatarURL != nil {
		avatar = req.AvatarURL
	}

	v := validate.New()
	v.MaxLen("name", name, 120)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	updated, err := h.service.UpdateProfile(r.Context(), user.ID, name, avatar)
	if err != nil {
		httpx.Fail(w, r, httpx.Internal(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": updated})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, err := MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req changePasswordRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	v := validate.New()
	v.Required("current_password", req.CurrentPassword)
	v.Required("new_password", req.NewPassword)
	v.MinLen("new_password", req.NewPassword, minPasswordLength)
	v.MaxLen("new_password", req.NewPassword, 256)
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Keeping the current cookie alive means the user is signed out everywhere
	// else, but not in the tab they just used.
	err = h.service.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword, h.cookies.Read(r))
	if err != nil {
		httpx.Fail(w, r, httpx.BadRequest("Your current password is incorrect."))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Password updated."})
}

// requestMeta captures the browser fingerprint stored against a new session.
func (h *Handler) requestMeta(r *http.Request) RequestMeta {
	meta := RequestMeta{UserAgent: r.UserAgent()}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		meta.IP = &addr
	}
	return meta
}
