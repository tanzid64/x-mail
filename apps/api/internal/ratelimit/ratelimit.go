// Package ratelimit provides Redis-backed request limiting for the endpoints
// that are worth protecting: login, registration, password reset, sending and
// uploads.
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/dark/x-mail/api/internal/httpx"
)

// Limiter counts requests in fixed windows. Fixed windows allow a burst across
// a boundary, which is an acceptable trade for the simplicity and the single
// round trip; the goal here is to stop credential stuffing, not to shape
// traffic precisely.
type Limiter struct {
	rdb    *redis.Client
	logger *slog.Logger
}

func New(rdb *redis.Client, logger *slog.Logger) *Limiter {
	return &Limiter{rdb: rdb, logger: logger}
}

type Result struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

// Allow records a hit against key and reports whether it fits within limit.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	redisKey := "ratelimit:" + key

	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, redisKey)
	// Setting the TTL only when the key is new keeps the window fixed rather
	// than sliding forward on every request.
	pipe.ExpireNX(ctx, redisKey, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return Result{}, err
	}

	count := int(incr.Val())
	if count > limit {
		ttl, err := l.rdb.TTL(ctx, redisKey).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return Result{Allowed: false, RetryAfter: ttl}, nil
	}
	return Result{Allowed: true, Remaining: limit - count}, nil
}

// KeyFunc derives the bucket a request counts against.
type KeyFunc func(*http.Request) string

// ByIP buckets by client address, which is what protects anonymous endpoints.
func ByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "ip:" + host
}

// Middleware limits a route. A Redis failure fails open: an outage in the
// limiter should not be able to lock every user out of the product.
func (l *Limiter) Middleware(name string, limit int, window time.Duration, keyFn KeyFunc) func(http.Handler) http.Handler {
	if keyFn == nil {
		keyFn = ByIP
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := fmt.Sprintf("%s:%s", name, keyFn(r))

			result, err := l.Allow(r.Context(), key, limit, window)
			if err != nil {
				l.logger.WarnContext(r.Context(), "rate limiter unavailable, allowing request",
					"error", err, "limiter", name)
				next.ServeHTTP(w, r)
				return
			}

			if !result.Allowed {
				retryAfter := int(result.RetryAfter.Seconds()) + 1
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				httpx.Fail(w, r, httpx.TooManyRequests(
					fmt.Sprintf("Too many attempts. Try again in %d seconds.", retryAfter)))
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			next.ServeHTTP(w, r)
		})
	}
}
