package worker

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// HealthServer exposes liveness and readiness for the worker process.
//
// A worker with no endpoint cannot be probed by an orchestrator, so a process
// that is alive but unable to reach Postgres or Redis would keep receiving
// traffic it cannot serve.
type HealthServer struct {
	server *http.Server
	logger *slog.Logger
}

func NewHealthServer(addr string, db *pgxpool.Pool, rdb *redis.Client, logger *slog.Logger) *HealthServer {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := http.StatusOK
		if err := db.Ping(ctx); err != nil {
			status = http.StatusServiceUnavailable
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"degraded"}`))
	})

	return &HealthServer{
		server: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Start listens in the background. A failure here is logged rather than fatal:
// losing the health endpoint should not stop the worker doing its job.
func (h *HealthServer) Start() {
	go func() {
		h.logger.Info("worker health endpoint listening", "addr", h.server.Addr)
		if err := h.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.logger.Error("worker health endpoint stopped", "error", err)
		}
	}()
}

func (h *HealthServer) Shutdown(ctx context.Context) {
	_ = h.server.Shutdown(ctx)
}
