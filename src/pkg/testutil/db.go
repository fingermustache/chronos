package testutil

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/fingermustache/chronos/pkg/database"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewTestDB spins up a throwaway PostgreSQL container, runs all migrations,
// and returns a ready *database.DB. The container is terminated when the
// test ends via t.Cleanup. Use this for individual tests that need
// an isolated database.
func NewTestDB(t *testing.T) *database.DB {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("chronos_test"),
		postgres.WithUsername("chronos"),
		postgres.WithPassword("chronos"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("get container port: %v", err)
	}
	portStr := port.Port()
	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	cfg := database.Config{
		Host:     host,
		Port:     portInt,
		User:     "chronos",
		Password: "chronos",
		Database: "chronos_test",
		SSLMode:  "disable",
	}

	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runMigrations(t.Fatalf, host, portInt)

	return db
}

// NewTestDBWithTeardown spins up a throwaway PostgreSQL container, runs all
// migrations, and returns a ready *database.DB along with a teardown function.
// This is intended for use in TestMain where no *testing.T is available at
// setup time. Call teardown() after m.Run() returns.
func NewTestDBWithTeardown(ctx context.Context) (*database.DB, func()) {
	container, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("chronos_test"),
		postgres.WithUsername("chronos"),
		postgres.WithPassword("chronos"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		log.Fatalf("get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		log.Fatalf("get container port: %v", err)
	}
	portStr := port.Port()
	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("parse port: %v", err)
	}

	cfg := database.Config{
		Host:     host,
		Port:     portInt,
		User:     "chronos",
		Password: "chronos",
		Database: "chronos_test",
		SSLMode:  "disable",
	}

	db, err := database.New(cfg)
	if err != nil {
		log.Fatalf("connect to test database: %v", err)
	}

	runMigrations(log.Fatalf, host, portInt)

	teardown := func() {
		_ = db.Close()
		_ = container.Terminate(ctx)
	}

	return db, teardown
}

var validTables = map[string]bool{
	"tasks":             true,
	"execution_history": true,
}

func Truncate(t *testing.T, db *database.DB, tables ...string) {
	t.Helper()
	for _, table := range tables {
		if !validTables[table] {
			t.Fatalf("truncate: unknown table %q", table)
		}
		if _, err := db.Exec("TRUNCATE TABLE " + table + " CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// runMigrations applies all up migrations against the given host/port.
// fatalf is either t.Fatalf or log.Fatalf depending on the caller context.
func runMigrations(fatalf func(string, ...any), host string, port int) {
	dsn := fmt.Sprintf("pgx5://chronos:chronos@%s:%d/chronos_test?sslmode=disable",
		host, port)

	m, err := migrate.New(fmt.Sprintf("file://%s", migrationsDir()), dsn)
	if err != nil {
		fatalf("create migrator: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		fatalf("run migrations: %v", err)
	}
}

// migrationsDir resolves the path to database/migrations relative to
// this file's location in the repo (pkg/testutil/ → ../../database/migrations).
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "database", "migrations")
}
