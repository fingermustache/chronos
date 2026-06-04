package database

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Host != "localhost" {
		t.Fatalf("Host = %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 5432 {
		t.Fatalf("Port = %d, want %d", cfg.Port, 5432)
	}
	if cfg.User != "chronos" {
		t.Fatalf("User = %q, want %q", cfg.User, "chronos")
	}
	if cfg.Password != "chronos" {
		t.Fatalf("Password = %q, want %q", cfg.Password, "chronos")
	}
	if cfg.Database != "chronos" {
		t.Fatalf("Database = %q, want %q", cfg.Database, "chronos")
	}
	if cfg.SSLMode != "disable" {
		t.Fatalf("SSLMode = %q, want %q", cfg.SSLMode, "disable")
	}
	if cfg.MaxOpenConns != 25 {
		t.Fatalf("MaxOpenConns = %d, want %d", cfg.MaxOpenConns, 25)
	}
	if cfg.MaxIdleConns != 5 {
		t.Fatalf("MaxIdleConns = %d, want %d", cfg.MaxIdleConns, 5)
	}
	if cfg.ConnMaxLifetime != 5*time.Minute {
		t.Fatalf("ConnMaxLifetime = %v, want %v", cfg.ConnMaxLifetime, 5*time.Minute)
	}
	if cfg.ConnMaxIdleTime != 2*time.Minute {
		t.Fatalf("ConnMaxIdleTime = %v, want %v", cfg.ConnMaxIdleTime, 2*time.Minute)
	}
}
