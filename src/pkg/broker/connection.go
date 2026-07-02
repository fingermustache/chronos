package broker

import (
	"errors"
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

// ErrPermanentlyDisconnected is returned via the error channel when all
// reconnect attempts are exhausted and the connection cannot be recovered.
var ErrPermanentlyDisconnected = errors.New("broker: permanently disconnected after max retries")

// Connection wraps an amqp.Connection and transparently reconnects on failure.
type Connection struct {
	cfg    Config
	mu     sync.RWMutex
	conn   *amqp.Connection
	logger *slog.Logger
	errCh  chan error
}

// NewConnection dials RabbitMQ and returns a managed Connection.
// The returned error channel receives ErrPermanentlyDisconnected if all
// reconnect attempts are exhausted. Callers should select on it alongside
// their own shutdown logic.
func NewConnection(cfg Config) (*Connection, error) {
	c := &Connection{
		cfg:    cfg,
		logger: slog.Default(),
		errCh:  make(chan error, 1),
	}
	if err := c.dial(); err != nil {
		return nil, err
	}
	go c.watchClose()
	return c, nil
}

// Err returns a channel that receives a fatal error when the connection
// cannot be recovered. It is closed after the error is sent.
func (c *Connection) Err() <-chan error {
	return c.errCh
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
	url := c.cfg.AMQPURL()
	conn, err := amqp.Dial(url)
	if err != nil {
		return fmt.Errorf("broker: dial %s: %w", url, err)
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
			return // connection was closed cleanly via Close()
		}

		c.logger.Warn("broker: connection closed, reconnecting", "error", err)

		reconnected := false
		for i := range maxRetries {
			time.Sleep(reconnectDelay)
			if dialErr := c.dial(); dialErr == nil {
				c.logger.Info("broker: reconnected")
				reconnected = true
				break
			}
			c.logger.Warn("broker: reconnect attempt failed", "attempt", i+1)
		}

		if !reconnected {
			c.logger.Error("broker: max reconnect attempts exhausted, giving up")
			c.errCh <- ErrPermanentlyDisconnected
			return
		}
	}
}
