package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port         string
	RateLimitRPM int
}

func Load() Config {
	return Config{
		Port:         getEnv("PORT", "8080"),
		RateLimitRPM: getEnvInt("RATE_LIMIT_RPM", 60),
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
