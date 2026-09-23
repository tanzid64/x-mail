package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/dark/x-mail/api/internal/httpx"
)

type contextKey string

const (
	userContextKey     contextKey = "auth.user"
	sessionContextKey  contextKey = "auth.session"
	apiTokenContextKey contextKey = "auth.apitoken"
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

// WithAPIToken stores a request authenticated by API token rather than cookie.
func WithAPIToken(ctx context.Context, user *User, token *APIToken) context.Context {
	ctx = context.WithValue(ctx, userContextKey, user)
	return context.WithValue(ctx, apiTokenContextKey, token)
}

// APITokenFrom returns the API token this request was authenticated with, if it
// was authenticated that way rather than with a session cookie.
func APITokenFrom(ctx context.Context) (*APIToken, bool) {
	token, ok := ctx.Value(apiTokenContextKey).(*APIToken)
	return token, ok && token != nil
}

// AccountScopeArg returns the token's account restriction as a SQL argument:
// the account id, or nil when the request is a session or an unrestricted
// token. Repositories bind it beside the user id, so the predicate
// `($n::uuid IS NULL OR <account> = $n)` is a no-op unless a restriction
// applies — one expression that is correct in both cases.
func AccountScopeArg(ctx context.Context) any {
	if id, ok := RestrictedAccount(ctx); ok {
		return id
	}
	return nil
}

// RestrictedAccount returns the one mail account this request may touch.
//
// It is set only for a token created against a single account. Everything that
// resolves ownership reads it from the context and applies it in SQL beside the
// user predicate, so a query cannot honour one and quietly forget the other.
func RestrictedAccount(ctx context.Context) (uuid.UUID, bool) {
	token, ok := APITokenFrom(ctx)
	if !ok || token.MailAccountID == nil {
		return uuid.Nil, false
	}
	return *token.MailAccountID, true
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

// RequireAuth accepts either a session cookie or an API token.
//
// Both are first-class: the browser signs in and gets a cookie, a third-party
// integration presents `Authorization: Bearer xm_…`, and every route behind
// this middleware works either way.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A presented bearer token is the caller's stated intent, so its
		// failure is reported as itself rather than falling through to the
		// cookie and answering "you must be signed in" to a machine.
		if bearer, ok := BearerToken(r); ok {
			user, token, err := h.service.AuthenticateAPIToken(r.Context(), bearer)
			if err != nil {
				httpx.Fail(w, r, apiTokenError(err))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithAPIToken(r.Context(), user, token)))
			return
		}

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

// apiTokenError says which of the three things went wrong.
//
// An expired or revoked token is a fact about the caller's own credential, not
// a disclosure about anyone else's, and "invalid" would send an integrator
// hunting for a typo in a key that is simply past its date.
func apiTokenError(err error) error {
	switch {
	case errors.Is(err, ErrAPITokenExpired):
		return httpx.Unauthorized("That API token has expired. Create a new one in Settings → API tokens.")
	case errors.Is(err, ErrAPITokenRevoked):
		return httpx.Unauthorized("That API token has been revoked.")
	default:
		return httpx.Unauthorized("That API token is not valid.")
	}
}

// RequireSession rejects API tokens, leaving an endpoint reachable only from a
// signed-in browser.
//
// A token that could mint another token would make every expiry meaningless —
// a 15-day key could issue itself a permanent one — and a token that could
// change the password could lock the owner out of revoking it. Those two
// powers stay with the human.
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := APITokenFrom(r.Context()); ok {
			httpx.Fail(w, r, httpx.Forbidden("This action needs a signed-in browser; an API token cannot do it."))
			return
		}
		next.ServeHTTP(w, r)
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
