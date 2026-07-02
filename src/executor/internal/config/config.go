package config

import (
	"os"
	"strconv"

	"github.com/fingermustache/chronos/pkg/broker"
)

type Config struct {
	Broker broker.Config
}

func Load() Config {
	return Config{
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
