package broker

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	reconnectDelay = 2 * time.Second
	maxRetries     = 10
)

// Connection wraps an amqp.Connection and transparently reconnects on failure.
type Connection struct {
	cfg    Config
	mu     sync.RWMutex
	conn   *amqp.Connection
	logger *slog.Logger
}

// NewConnection dials RabbitMQ and returns a managed Connection.
func NewConnection(cfg Config) (*Connection, error) {
	c := &Connection{cfg: cfg, logger: slog.Default()}
	if err := c.dial(); err != nil {
		return nil, err
	}
	go c.watchClose()
	return c, nil
}

// Channel returns a new AMQP channel on the current connection.
func (c *Connection) Channel() (*amqp.Channel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn.Channel()
}

// Close shuts down the underlying connection.
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}

func (c *Connection) dial() error {
	conn, err := amqp.Dial(c.cfg.AMQPURL())
	if err != nil {
		return fmt.Errorf("broker: dial %s: %w", c.cfg.AMQPURL(), err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

// watchClose listens for connection-closed notifications and reconnects.
func (c *Connection) watchClose() {
	for {
		c.mu.RLock()
		notify := c.conn.NotifyClose(make(chan *amqp.Error, 1))
		c.mu.RUnlock()

		err, ok := <-notify
		if !ok {
			return // connection was closed cleanly
		}

		c.logger.Warn("broker: connection closed, reconnecting", "error", err)

		for i := range maxRetries {
			time.Sleep(reconnectDelay)
			if dialErr := c.dial(); dialErr == nil {
				c.logger.Info("broker: reconnected")
				break
			}
			c.logger.Warn("broker: reconnect attempt failed", "attempt", i+1)
		}
	}
}
