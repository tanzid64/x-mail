// Package server wires configuration, dependencies and routes into one
// http.Handler.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/httpx"
	"github.com/dark/x-mail/api/internal/server/middleware"
)

type Deps struct {
	Config *config.Config
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Logger *slog.Logger
}

type Server struct {
	deps   Deps
	router chi.Router
}

func New(deps Deps) *Server {
	s := &Server{deps: deps, router: chi.NewRouter()}
	s.setupMiddleware()
	s.setupRoutes()
	return s
}

func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) setupMiddleware() {
	s.router.Use(chimw.RequestID)
	s.router.Use(chimw.RealIP)
	s.router.Use(middleware.RequestLogger(s.deps.Logger))
	s.router.Use(middleware.Recoverer(s.deps.Logger))
	s.router.Use(chimw.Timeout(30 * time.Second))

	// Credentials are carried in a cookie, so the allowed origin list must be
	// explicit — "*" is not permitted with AllowCredentials.
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.deps.Config.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
}

func (s *Server) setupRoutes() {
	s.router.Get("/healthz", s.handleLiveness)
	s.router.Get("/readyz", s.handleReadiness)

	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
			httpx.JSON(w, http.StatusOK, map[string]string{"message": "pong"})
		})
	})

	s.router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.Fail(w, r, httpx.NotFound("The requested endpoint does not exist."))
	})
	s.router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.Fail(w, r, &httpx.Error{
			Status:  http.StatusMethodNotAllowed,
			Code:    "method_not_allowed",
			Message: "That method is not allowed on this endpoint.",
		})
	})
}

// handleLiveness answers as long as the process is running.
func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadiness verifies the dependencies a request actually needs, so a
// load balancer stops sending traffic when Postgres or Redis is unreachable.
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := map[string]string{"postgres": "ok", "redis": "ok"}
	status := http.StatusOK

	if err := s.deps.DB.Ping(ctx); err != nil {
		checks["postgres"] = "unavailable"
		status = http.StatusServiceUnavailable
	}
	if err := s.deps.Redis.Ping(ctx).Err(); err != nil {
		checks["redis"] = "unavailable"
		status = http.StatusServiceUnavailable
	}

	httpx.JSON(w, status, map[string]any{"status": statusWord(status), "checks": checks})
}

func statusWord(status int) string {
	if status == http.StatusOK {
		return "ok"
	}
	return "degraded"
}
