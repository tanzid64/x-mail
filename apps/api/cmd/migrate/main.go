// Command migrate applies, rolls back, inspects and scaffolds database migrations.
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down
//	go run ./cmd/migrate status
//	go run ./cmd/migrate create add_mail_accounts
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/dark/x-mail/api/internal/config"
	"github.com/dark/x-mail/api/internal/database"
)

const migrationsDir = "db/migrations"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: migrate <up|down|status|create <name>>")
	}
	command := os.Args[1]

	// `create` only touches the filesystem, so it must work without a database.
	if command == "create" {
		if len(os.Args) < 3 {
			return fmt.Errorf("usage: migrate create <name>")
		}
		return createMigration(strings.Join(os.Args[2:], "_"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch command {
	case "up":
		ran, err := database.MigrateUp(ctx, pool)
		if err != nil {
			return err
		}
		if len(ran) == 0 {
			fmt.Println("no pending migrations")
			return nil
		}
		for _, m := range ran {
			fmt.Printf("applied  %04d_%s\n", m.Version, m.Name)
		}
		return nil

	case "down":
		m, err := database.MigrateDown(ctx, pool)
		if err != nil {
			return err
		}
		if m == nil {
			fmt.Println("nothing to roll back")
			return nil
		}
		fmt.Printf("rolled back  %04d_%s\n", m.Version, m.Name)
		return nil

	case "status":
		migrations, applied, err := database.MigrationStatus(ctx, pool)
		if err != nil {
			return err
		}
		for _, m := range migrations {
			state := "pending"
			if applied[m.Version] {
				state = "applied"
			}
			fmt.Printf("%-8s %04d_%s\n", state, m.Version, m.Name)
		}
		return nil

	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

var nameSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

func createMigration(name string) error {
	name = nameSanitizer.ReplaceAllString(strings.ToLower(name), "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return fmt.Errorf("migration name must contain letters or digits")
	}

	migrations, err := database.LoadMigrations()
	if err != nil {
		return err
	}
	next := int64(1)
	if len(migrations) > 0 {
		next = migrations[len(migrations)-1].Version + 1
	}

	for suffix, body := range map[string]string{
		"up":   fmt.Sprintf("-- %04d_%s (up)\n", next, name),
		"down": fmt.Sprintf("-- %04d_%s (down)\n", next, name),
	} {
		path := filepath.Join(migrationsDir, fmt.Sprintf("%04d_%s.%s.sql", next, name, suffix))
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Println("created", path)
	}
	return nil
}
