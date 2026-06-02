package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultConfig returns sensible defaults for local development.
// Override all credentials via environment variables in production.
func DefaultConfig() Config {
	return Config{
		Host:            "localhost",
		Port:            5432,
		User:            "chronos",
		Password:        "chronos",
		Database:        "chronos",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}
}

// Querier is implemented by both *DB and *sqlx.Tx, allowing repositories
// to operate identically inside or outside a transaction.
type Querier interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)
	PrepareNamedContext(ctx context.Context, query string) (*sqlx.NamedStmt, error)
}

type DB struct {
	*sqlx.DB
}

func New(cfg Config) (*DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(cfg.User),
		url.QueryEscape(cfg.Password),
		cfg.Host, cfg.Port,
		cfg.Database,
		cfg.SSLMode,
	)

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	if err := pingWithRetry(db, 5, time.Second); err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) WithTx(ctx context.Context, fn func(Querier) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) Close() error {
	return db.DB.Close()
}

func (db *DB) HealthCheck(ctx context.Context) error {
	return db.PingContext(ctx)
}

func pingWithRetry(db *sqlx.DB, maxAttempts int, initialWait time.Duration) error {
	wait := initialWait
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := db.PingContext(ctx)
		cancel()

		if err == nil {
			return nil
		}

		if attempt == maxAttempts {
			return fmt.Errorf("database not reachable after %d attempts: %w", maxAttempts, err)
		}

		time.Sleep(wait)
		wait *= 2
	}
	return nil
}
