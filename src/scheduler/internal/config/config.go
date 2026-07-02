package config

import (
	"os"
	"strconv"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
)

type Config struct {
	PollIntervalSeconds int
	ClaimBatchSize      int
	Database            database.Config
	Broker              broker.Config
}

func Load() Config {
	return Config{
		PollIntervalSeconds: getEnvInt("POLL_INTERVAL_SECONDS", 5),
		ClaimBatchSize:      getEnvInt("CLAIM_BATCH_SIZE", 10),
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "chronos"),
			Password: getEnv("DB_PASSWORD", "chronos"),
			Database: getEnv("DB_NAME", "chronos"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Broker: broker.DefaultConfig(),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
