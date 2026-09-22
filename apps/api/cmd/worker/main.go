// Command worker runs background jobs: sending mail, and — as later phases land
// — processing inbound webhooks, waking snoozed threads and dispatching
// scheduled sends.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dark/x-mail/api/internal/accounts"
	"github.com/dark/x-mail/api/internal/compose"
	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/crypto"
	"github.com/dark/x-mail/api/internal/database"
	"github.com/dark/x-mail/api/internal/logging"
	"github.com/dark/x-mail/api/internal/redisx"
	"github.com/dark/x-mail/api/internal/storage"
	"github.com/dark/x-mail/api/internal/worker"
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

	sealer, err := crypto.NewSealer(cfg.EncryptionMasterKey)
	if err != nil {
		return err
	}

	store, err := storage.New(storage.Options{
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
		return err
	}

	accountsService := accounts.NewService(
		accounts.NewRepository(pool),
		sealer,
		accounts.NewProviderFactory(cfg.ResendBaseURL),
		logger,
	)

	// Liveness and readiness, so the worker can be probed like any other process.
	var health *worker.HealthServer
	if cfg.WorkerHTTPAddr != "" {
		health = worker.NewHealthServer(cfg.WorkerHTTPAddr, pool, rdb, logger)
		health.Start()
	}

	server := worker.New(worker.Deps{
		Redis:  worker.RedisOpt(rdb),
		Logger: logger,
		SendEmail: worker.NewSendEmailHandler(
			compose.NewRepository(pool), accountsService, store, logger,
		),
	})

	// Run blocks, so shutdown is driven from the signal context.
	done := make(chan error, 1)
	go func() { done <- server.Run() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, finishing in-flight jobs")
		// Shutdown waits for running jobs, so a deploy cannot cut a send in half.
		server.Shutdown()
		if health != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			health.Shutdown(shutdownCtx)
		}
		return nil
	}
}
