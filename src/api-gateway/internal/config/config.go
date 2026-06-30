package config

import (
	"os"
	"strconv"

	"github.com/fingermustache/chronos/pkg/database"
)

type Config struct {
	Port         string
	RateLimitRPM int
	Database     database.Config
}

func Load() Config {
	return Config{
		Port:         getEnv("PORT", "8080"),
		RateLimitRPM: getEnvInt("RATE_LIMIT_RPM", 60),
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "chronos"),
			Password: getEnv("DB_PASSWORD", "chronos"),
			Database: getEnv("DB_NAME", "chronos"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
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
