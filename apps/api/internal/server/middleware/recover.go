package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/dark/x-mail/api/internal/httpx"
)

// Recoverer turns a panic into a 500 instead of a dropped connection, logging
// the stack so the fault is still diagnosable.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// A client disconnecting mid-write is not a server fault.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", RequestIDFrom(r.Context()),
					"stack", string(debug.Stack()),
				)
				httpx.JSON(w, http.StatusInternalServerError, map[string]any{
					"error": map[string]string{
						"code":    "internal_error",
						"message": "Something went wrong. Please try again.",
					},
				})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
