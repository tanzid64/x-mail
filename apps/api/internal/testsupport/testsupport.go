// Package testsupport gives integration tests a real PostgreSQL and Redis to
// work against.
//
// Tests skip themselves when TEST_DATABASE_URL is unset, so `go test ./...`
// still passes on a machine with no infrastructure running, while `make infra`
// turns the same tests into genuine end-to-end coverage.
package testsupport

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/dark/x-mail/api/internal/database"
)

var (
	migrateOnce sync.Once
	migrateErr  error
)

// Pool returns a connection pool to the test database with the schema migrated
// and every table emptied, so each test starts from a known state.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; run `make infra` to enable integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	// Migrations are schema-wide, so running them once per package is enough.
	migrateOnce.Do(func() {
		_, migrateErr = database.MigrateUp(ctx, pool)
	})
	if migrateErr != nil {
		t.Fatalf("migrate test database: %v", migrateErr)
	}

	Truncate(t, pool)
	return pool
}

// Truncate empties every application table. schema_migrations is preserved so
// the schema does not have to be rebuilt between tests.
func Truncate(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT tablename FROM pg_tables
		 WHERE schemaname = 'public' AND tablename <> 'schema_migrations'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, `"`+name+`"`)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) == 0 {
		return
	}

	stmt := "TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := pool.Exec(ctx, stmt); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

// Redis returns a flushed Redis client on the dedicated test database.
func Redis(t testing.TB) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set; run `make infra` to enable integration tests")
	}
	db, _ := strconv.Atoi(os.Getenv("TEST_REDIS_DB"))

	client := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush test redis: %v", err)
	}
	return client
}
