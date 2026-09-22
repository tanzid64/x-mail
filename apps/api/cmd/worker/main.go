// Command worker runs background jobs: sending mail, processing inbound
// webhooks, waking snoozed threads and dispatching scheduled sends.
//
// Job handlers are registered in internal/worker as each phase lands; until
// then this process exists so the deployment topology, configuration and
// connection handling are the same ones the jobs will run under.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/database"
	"github.com/dark/x-mail/api/internal/logging"
	"github.com/dark/x-mail/api/internal/redisx"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.Env, os.Getenv("LOG_LEVEL"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb, err := redisx.Connect(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()

	logger.Info("worker started", "env", cfg.Env, "registered_jobs", 0)
	logger.Warn("no job handlers registered yet; queue processing arrives with the sending phase")

	<-ctx.Done()
	logger.Info("worker shutdown complete")
	return nil
}
