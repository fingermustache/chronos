package broker

import (
	"fmt"
	"os"
)

// Config holds the AMQP connection parameters.
// The URL takes precedence over the individual host/port/user/pass fields.
type Config struct {
	URL      string
	Host     string
	Port     string
	User     string
	Password string
	Vhost    string
}

// DefaultConfig loads broker configuration from environment variables.
// RABBITMQ_URL takes precedence. Falls back to RABBITMQ_HOST, RABBITMQ_PORT,
// RABBITMQ_USER, RABBITMQ_PASSWORD, and RABBITMQ_VHOST.
func DefaultConfig() Config {
	if url := os.Getenv("RABBITMQ_URL"); url != "" {
		return Config{URL: url}
	}
	return Config{
		Host:     getEnv("RABBITMQ_HOST", "localhost"),
		Port:     getEnv("RABBITMQ_PORT", "5672"),
		User:     getEnv("RABBITMQ_USER", "guest"),
		Password: getEnv("RABBITMQ_PASSWORD", "guest"),
		Vhost:    getEnv("RABBITMQ_VHOST", "/"),
	}
}

// AMQPURL returns the fully-qualified AMQP URL for the config.
func (c Config) AMQPURL() string {
	if c.URL != "" {
		return c.URL
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%s%s", c.User, c.Password, c.Host, c.Port, c.Vhost)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
