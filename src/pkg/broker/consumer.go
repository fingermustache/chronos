package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ErrNack signals that the handler rejected the message and it should be
// nacked without requeue, routing it to the dead-letter queue.
var ErrNack = errors.New("broker: message rejected")

// ErrConsumerClosed is returned by Consume when the broker closes the channel
// unexpectedly (as opposed to a clean shutdown via Close).
var ErrConsumerClosed = errors.New("broker: consumer channel closed unexpectedly")

// Consumer consumes TaskTriggerEvents from the broker.
type Consumer interface {
	// Consume blocks, calling handler for each message. Returns nil on clean
	// shutdown (via Close). Returns ErrConsumerClosed if the broker closes the
	// channel unexpectedly. Returning ErrNack from handler nacks without requeue.
	Consume(handler func(TaskTriggerEvent) error) error
	Close() error
}

type amqpConsumer struct {
	conn    *Connection
	ch      *amqp.Channel
	closing atomic.Bool
}

// NewConsumer creates a Consumer backed by a dedicated AMQP channel.
func NewConsumer(conn *Connection) (Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("broker: open consumer channel: %w", err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("broker: set QoS: %w", err)
	}
	return &amqpConsumer{conn: conn, ch: ch}, nil
}

func (c *amqpConsumer) Consume(handler func(TaskTriggerEvent) error) error {
	deliveries, err := c.ch.Consume(QueueTrigger, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("broker: start consume: %w", err)
	}

	for d := range deliveries {
		var evt TaskTriggerEvent
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			d.Nack(false, false)
			continue
		}
		if err := handler(evt); err != nil {
			d.Nack(false, false)
			continue
		}
		d.Ack(false)
	}

	// closing flag is set by Close() before ch.Close() — distinguishes
	// intentional shutdown from broker-initiated channel termination.
	if c.closing.Load() {
		return nil
	}
	return ErrConsumerClosed
}

func (c *amqpConsumer) Close() error {
	c.closing.Store(true)
	return c.ch.Close()
}
