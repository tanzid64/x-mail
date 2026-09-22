package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// Deps is everything the job handlers need.
type Deps struct {
	Redis     asynq.RedisClientOpt
	Logger    *slog.Logger
	SendEmail *SendEmailHandler
	// Concurrency caps how many jobs run at once. Sending is bounded by the
	// provider's rate limits rather than by local CPU, so this is deliberately
	// modest.
	Concurrency int
}

// Server runs the background jobs.
type Server struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	logger *slog.Logger
}

func New(deps Deps) *Server {
	concurrency := deps.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	server := asynq.NewServer(deps.Redis, asynq.Config{
		Concurrency: concurrency,
		// Outbound mail is what a user is waiting on, so it outranks
		// housekeeping work.
		Queues: map[string]int{
			QueueCritical: 6,
			QueueDefault:  3,
		},
		Logger:   &asynqLogger{logger: deps.Logger},
		LogLevel: asynq.InfoLevel,
		ErrorHandler: asynq.ErrorHandlerFunc(func(_ context.Context, task *asynq.Task, err error) {
			deps.Logger.Error("job failed", "type", task.Type(), "error", err)
		}),
	})

	mux := asynq.NewServeMux()
	mux.Handle(TaskSendEmail, deps.SendEmail)

	return &Server{server: server, mux: mux, logger: deps.Logger}
}

// Run blocks until Shutdown is called.
func (s *Server) Run() error {
	s.logger.Info("worker started", "jobs", []string{TaskSendEmail})
	if err := s.server.Run(s.mux); err != nil {
		return fmt.Errorf("worker: %w", err)
	}
	return nil
}

// Shutdown stops accepting jobs and waits for the running ones to finish, so a
// deploy cannot cut a send in half.
func (s *Server) Shutdown() {
	s.server.Shutdown()
}

// asynqLogger adapts slog to the interface Asynq expects.
type asynqLogger struct{ logger *slog.Logger }

func (l *asynqLogger) Debug(args ...any) { l.logger.Debug(fmt.Sprint(args...)) }
func (l *asynqLogger) Info(args ...any)  { l.logger.Info(fmt.Sprint(args...)) }
func (l *asynqLogger) Warn(args ...any)  { l.logger.Warn(fmt.Sprint(args...)) }
func (l *asynqLogger) Error(args ...any) { l.logger.Error(fmt.Sprint(args...)) }
func (l *asynqLogger) Fatal(args ...any) { l.logger.Error(fmt.Sprint(args...)) }
