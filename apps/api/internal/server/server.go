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

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/compose"
	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/crypto"
	"github.com/dark/x-mail/api/internal/httpx"
	"github.com/dark/x-mail/api/internal/mailbox"
	"github.com/dark/x-mail/api/internal/mailer"
	"github.com/dark/x-mail/api/internal/ratelimit"
	"github.com/dark/x-mail/api/internal/realtime"
	"github.com/dark/x-mail/api/internal/server/middleware"
	"github.com/dark/x-mail/api/internal/storage"
	"github.com/dark/x-mail/api/internal/webhooks"
	"github.com/dark/x-mail/api/internal/worker"
)

type Deps struct {
	Config *config.Config
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Logger *slog.Logger

	// Storage holds attachment bytes. Built from config when nil.
	Storage storage.Storage
	// Enqueuer submits background jobs. Built from config when nil.
	Enqueuer compose.Enqueuer

	// ProviderFactory overrides how transport clients are built. Tests supply a
	// fake here; production leaves it nil and gets the real implementations.
	ProviderFactory accounts.ProviderFactory
}

type Server struct {
	deps     Deps
	router   chi.Router
	limiter  *ratelimit.Limiter
	auth     *auth.Handler
	accounts *accounts.Handler
	mailbox  *mailbox.Handler
	compose  *compose.Handler
	webhooks *webhooks.Handler
	hub      *realtime.Hub
}

// New wires the dependency graph and returns a ready router.
//
// It fails rather than returns an error only for configuration that Load has
// already validated — the master key length in particular.
func New(deps Deps) (*Server, error) {
	cfg := deps.Config

	limiter := ratelimit.New(deps.Redis, deps.Logger, cfg.RateLimitScale)

	sealer, err := crypto.NewSealer(cfg.EncryptionMasterKey)
	if err != nil {
		return nil, err
	}

	authHandler := auth.NewHandler(
		auth.NewService(
			auth.NewRepository(deps.DB),
			mailer.New(cfg.SystemResendAPIKey, cfg.SystemMailFrom, deps.Logger),
			auth.Options{SessionTTL: cfg.SessionTTL, AppBaseURL: cfg.AppBaseURL},
			deps.Logger,
		),
		auth.Cookies{
			Name:   cfg.SessionCookieName,
			Secure: cfg.SessionCookieSecure,
			TTL:    cfg.SessionTTL,
		},
		limiter,
		deps.Logger,
	)

	providerFactory := deps.ProviderFactory
	if providerFactory == nil {
		providerFactory = accounts.NewProviderFactory(cfg.ResendBaseURL)
	}

	accountsRepo := accounts.NewRepository(deps.DB)

	accountsHandler := accounts.NewHandler(
		accounts.NewService(accountsRepo, sealer, providerFactory, deps.Logger),
		deps.Logger,
	)

	publisher := realtime.NewPublisher(deps.Redis, deps.Logger)

	mailboxService := mailbox.NewService(mailbox.NewRepository(deps.DB), accountsRepo, deps.Logger)
	mailboxService.SetPublisher(publisher)

	mailboxHandler := mailbox.NewHandler(mailboxService, deps.Logger)

	store := deps.Storage
	if store == nil {
		store, err = storage.New(storage.Options{
			Driver:    cfg.StorageDriver,
			LocalPath: cfg.StorageLocalPath,
			S3: storage.S3Config{
				Endpoint:  cfg.S3Endpoint,
				Region:    cfg.S3Region,
				Bucket:    cfg.S3Bucket,
				AccessKey: cfg.S3AccessKeyID,
				SecretKey: cfg.S3SecretKey,
			},
		})
		if err != nil {
			return nil, err
		}
	}

	queue := deps.Enqueuer
	if queue == nil {
		queue = worker.NewEnqueuer(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	}

	composeHandler := compose.NewHandler(
		compose.NewService(compose.NewRepository(deps.DB), store, queue, deps.Logger),
		deps.Logger,
	)

	// The webhook enqueuer is the same client the compose service uses, but
	// typed for the event task.
	eventQueue, ok := queue.(webhooks.Queue)
	if !ok {
		eventQueue = worker.NewEnqueuer(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	}

	webhooksHandler := webhooks.NewHandler(
		webhooks.NewRepository(deps.DB),
		sealer,
		cfg.ResendWebhookSecret,
		eventQueue,
		deps.Logger,
	)

	hub := realtime.NewHub(deps.Redis, deps.Logger, cfg.CORSOrigins)

	s := &Server{
		deps:     deps,
		router:   chi.NewRouter(),
		limiter:  limiter,
		auth:     authHandler,
		accounts: accountsHandler,
		mailbox:  mailboxHandler,
		compose:  composeHandler,
		webhooks: webhooksHandler,
		hub:      hub,
	}
	s.setupMiddleware()
	s.setupRoutes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.router }

// Hub exposes the realtime hub so the process can run its subscription loop.
func (s *Server) Hub() *realtime.Hub { return s.hub }

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
	// HEAD as well as GET: container healthchecks and uptime monitors commonly
	// probe with HEAD, and chi — unlike net/http's ServeMux — does not imply it.
	s.router.Get("/healthz", s.handleLiveness)
	s.router.Head("/healthz", s.handleLiveness)
	s.router.Get("/readyz", s.handleReadiness)
	s.router.Head("/readyz", s.handleReadiness)

	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
			httpx.JSON(w, http.StatusOK, map[string]string{"message": "pong"})
		})

		r.Route("/auth", s.auth.RoutesPublic)

		// Provider callbacks carry no session; the request signature is what
		// authenticates them, so they sit outside the auth middleware.
		r.Route("/webhooks", s.webhooks.Routes)

		r.Route("/me", func(r chi.Router) {
			r.Use(s.auth.RequireAuth)
			s.auth.RoutesMe(r)
		})

		// Everything below this point is scoped to the signed-in user, and each
		// handler resolves ownership through mail_accounts before touching a row.
		r.Group(func(r chi.Router) {
			r.Use(s.auth.RequireAuth)

			// The socket is authenticated by the session cookie like any other
			// request, and then only ever carries that user's events.
			r.Get("/ws", s.hub.Handler)

			// Token management is behind RequireSession inside RoutesAPITokens:
			// reachable with a session, never with a token.
			r.Route("/api-tokens", s.auth.RoutesAPITokens)

			r.Route("/mail-accounts", s.accounts.RoutesAccounts)
			r.Route("/domains", s.accounts.RoutesDomains)
			r.Route("/addresses", s.accounts.RoutesAddresses)

			r.Route("/threads", s.mailbox.RoutesThreads)
			r.Route("/labels", s.mailbox.RoutesLabels)
			r.Route("/drafts", s.compose.RoutesDrafts)
			r.Route("/attachments", s.compose.RoutesAttachments)
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
