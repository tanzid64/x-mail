// Package middleware holds cross-cutting HTTP concerns: request correlation,
// structured access logging and panic recovery.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestIDFrom returns the correlation id chi assigned to this request.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// RequestLogger emits one structured line per request and puts the request id
// on the context so downstream logs can be correlated.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := middleware.GetReqID(r.Context())
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			r = r.WithContext(ctx)

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			started := time.Now()

			defer func() {
				attrs := []any{
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(started).Milliseconds(),
					"remote_ip", realIP(r),
					"request_id", requestID,
				}
				switch {
				case ww.Status() >= http.StatusInternalServerError:
					logger.ErrorContext(ctx, "http request", attrs...)
				case ww.Status() >= http.StatusBadRequest:
					logger.WarnContext(ctx, "http request", attrs...)
				default:
					logger.InfoContext(ctx, "http request", attrs...)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
