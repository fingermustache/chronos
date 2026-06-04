package database

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 5432 {
		t.Errorf("Port = %d, want %d", cfg.Port, 5432)
	}
	if cfg.User != "chronos" {
		t.Errorf("User = %q, want %q", cfg.User, "chronos")
	}
	// "chronos" is an intentionally insecure dev-only default — never use in production.
	if cfg.Password != "chronos" {
		t.Errorf("Password = %q, want %q", cfg.Password, "chronos")
	}
	if cfg.Database != "chronos" {
		t.Errorf("Database = %q, want %q", cfg.Database, "chronos")
	}
	if cfg.SSLMode != "disable" {
		t.Errorf("SSLMode = %q, want %q", cfg.SSLMode, "disable")
	}
	if cfg.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns = %d, want %d", cfg.MaxOpenConns, 25)
	}
	if cfg.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want %d", cfg.MaxIdleConns, 5)
	}
	if cfg.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("ConnMaxLifetime = %v, want %v", cfg.ConnMaxLifetime, 5*time.Minute)
	}
	if cfg.ConnMaxIdleTime != 2*time.Minute {
		t.Errorf("ConnMaxIdleTime = %v, want %v", cfg.ConnMaxIdleTime, 2*time.Minute)
	}
}
