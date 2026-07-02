package config

import (
	"os"
	"strconv"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
)

type Config struct {
	Database database.Config
	Broker   broker.Config
}

func Load() Config {
	dbCfg := database.DefaultConfig()
	dbCfg.Host = getEnv("DB_HOST", dbCfg.Host)
	dbCfg.Port = getEnvInt("DB_PORT", dbCfg.Port)
	dbCfg.User = getEnv("DB_USER", dbCfg.User)
	dbCfg.Password = getEnv("DB_PASSWORD", dbCfg.Password)
	dbCfg.Database = getEnv("DB_NAME", dbCfg.Database)
	dbCfg.SSLMode = getEnv("DB_SSLMODE", dbCfg.SSLMode)

	return Config{
		Database: dbCfg,
		Broker:   broker.DefaultConfig(),
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
