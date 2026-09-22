package database

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dark/x-mail/api/db"
)

// advisoryLockID guards migrations so that the API and the worker starting at
// the same moment cannot apply the same migration twice.
const advisoryLockID int64 = 8531104771

type Migration struct {
	Version int64
	Name    string
	Up      string
	Down    string
}

// LoadMigrations reads the embedded db/migrations directory. Files are named
// `<version>_<name>.up.sql` and `<version>_<name>.down.sql`.
func LoadMigrations() ([]Migration, error) {
	migrationFS := db.Migrations()
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	byVersion := map[int64]*Migration{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, name, direction, err := parseMigrationName(e.Name())
		if err != nil {
			return nil, err
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		m, ok := byVersion[version]
		if !ok {
			m = &Migration{Version: version, Name: name}
			byVersion[version] = m
		}
		if direction == "up" {
			m.Up = string(body)
		} else {
			m.Down = string(body)
		}
	}

	out := make([]Migration, 0, len(byVersion))
	for _, m := range byVersion {
		if strings.TrimSpace(m.Up) == "" {
			return nil, fmt.Errorf("migration %d (%s) has no up file", m.Version, m.Name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func parseMigrationName(filename string) (version int64, name, direction string, err error) {
	base := strings.TrimSuffix(filename, ".sql")

	switch {
	case strings.HasSuffix(base, ".up"):
		direction = "up"
		base = strings.TrimSuffix(base, ".up")
	case strings.HasSuffix(base, ".down"):
		direction = "down"
		base = strings.TrimSuffix(base, ".down")
	default:
		return 0, "", "", fmt.Errorf("migration %q must end in .up.sql or .down.sql", filename)
	}

	prefix, rest, found := strings.Cut(base, "_")
	if !found {
		return 0, "", "", fmt.Errorf("migration %q must be named <version>_<name>.<up|down>.sql", filename)
	}
	version, err = strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, "", "", fmt.Errorf("migration %q has a non-numeric version: %w", filename, err)
	}
	return version, rest, direction, nil
}

func ensureMigrationTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     bigint      PRIMARY KEY,
			name        text        NOT NULL,
			applied_at  timestamptz NOT NULL DEFAULT now()
		)`)
	return err
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int64]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[int64]bool{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// MigrateUp applies every pending migration in version order. Each migration
// runs inside its own transaction together with its schema_migrations insert,
// so a failure can never leave a half-applied version recorded.
func MigrateUp(ctx context.Context, pool *pgxpool.Pool) ([]Migration, error) {
	migrations, err := LoadMigrations()
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, advisoryLockID) }()

	if err := ensureMigrationTable(ctx, pool); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}
	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}

	var ran []Migration
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := runInTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.Up); err != nil {
				return err
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name)
			return err
		}); err != nil {
			return ran, fmt.Errorf("migration %d_%s: %w", m.Version, m.Name, err)
		}
		ran = append(ran, m)
	}
	return ran, nil
}

// MigrateDown rolls back the most recently applied migration.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool) (*Migration, error) {
	migrations, err := LoadMigrations()
	if err != nil {
		return nil, err
	}
	if err := ensureMigrationTable(ctx, pool); err != nil {
		return nil, err
	}

	var version int64
	err = pool.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&version)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	for _, m := range migrations {
		if m.Version != version {
			continue
		}
		if strings.TrimSpace(m.Down) == "" {
			return nil, fmt.Errorf("migration %d_%s has no down file", m.Version, m.Name)
		}
		if err := runInTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.Down); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.Version)
			return err
		}); err != nil {
			return nil, fmt.Errorf("rollback %d_%s: %w", m.Version, m.Name, err)
		}
		return &m, nil
	}
	return nil, fmt.Errorf("applied migration %d has no file on disk", version)
}

// MigrationStatus reports every known migration and whether it is applied.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) ([]Migration, map[int64]bool, error) {
	migrations, err := LoadMigrations()
	if err != nil {
		return nil, nil, err
	}
	if err := ensureMigrationTable(ctx, pool); err != nil {
		return nil, nil, err
	}
	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, nil, err
	}
	return migrations, applied, nil
}

func runInTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
