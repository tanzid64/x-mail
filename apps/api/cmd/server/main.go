// Command server runs the x-mail HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/database"
	"github.com/dark/x-mail/api/internal/logging"
	"github.com/dark/x-mail/api/internal/redisx"
	"github.com/dark/x-mail/api/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.Env, os.Getenv("LOG_LEVEL"))

	// Shutdown begins as soon as the first signal arrives; a second signal
	// bypasses the graceful path entirely.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("connected to postgres")

	rdb, err := redisx.Connect(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	logger.Info("connected to redis")

	// Applying migrations at startup keeps a freshly started container in sync
	// with the code it is running. The advisory lock makes this safe when the
	// API and worker start together.
	applied, err := database.MigrateUp(ctx, pool)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if len(applied) > 0 {
		for _, m := range applied {
			logger.Info("applied migration", "version", m.Version, "name", m.Name)
		}
	}

	apiServer, err := server.New(server.Deps{Config: cfg, DB: pool, Redis: rdb, Logger: logger})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// The hub fans Redis events out to the sockets this process holds.
	go func() {
		if err := apiServer.Hub().Run(ctx); err != nil {
			logger.Error("realtime hub stopped", "error", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}
