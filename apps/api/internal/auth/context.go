package auth

import (
	"context"
	"net/http"

	"github.com/dark/x-mail/api/internal/httpx"
)

type contextKey string

const (
	userContextKey    contextKey = "auth.user"
	sessionContextKey contextKey = "auth.session"
)

// WithUser stores the authenticated user on the request context.
func WithUser(ctx context.Context, user *User, session *Session) context.Context {
	ctx = context.WithValue(ctx, userContextKey, user)
	return context.WithValue(ctx, sessionContextKey, session)
}

// UserFrom returns the authenticated user, if any.
func UserFrom(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(userContextKey).(*User)
	return user, ok
}

// SessionFrom returns the current session, if any.
func SessionFrom(ctx context.Context) (*Session, bool) {
	session, ok := ctx.Value(sessionContextKey).(*Session)
	return session, ok
}

// MustUser returns the authenticated user for handlers mounted behind
// RequireAuth. The error path exists so a routing mistake surfaces as a 401
// rather than a nil dereference.
func MustUser(ctx context.Context) (*User, error) {
	user, ok := UserFrom(ctx)
	if !ok || user == nil {
		return nil, httpx.Unauthorized("You must be signed in to do that.")
	}
	return user, nil
}

// RequireAuth rejects requests without a valid session cookie.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := h.cookies.Read(r)
		user, session, err := h.service.Authenticate(r.Context(), token)
		if err != nil {
			// A cookie that no longer resolves is stale; clearing it stops the
			// browser sending it on every subsequent request.
			if token != "" {
				h.cookies.Clear(w)
			}
			httpx.Fail(w, r, httpx.Unauthorized("You must be signed in to do that."))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user, session)))
	})
}

// RequireVerifiedEmail gates actions that should not be available until the
// user has confirmed their address.
func (h *Handler) RequireVerifiedEmail(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := MustUser(r.Context())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if user.EmailVerifiedAt == nil {
			httpx.Fail(w, r, httpx.Forbidden("Confirm your email address to continue."))
			return
		}
		next.ServeHTTP(w, r)
	})
}
