package broker

import (
	"encoding/json"
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ErrNack signals that the handler rejected the message and it should be
// nacked without requeue, routing it to the dead-letter queue.
var ErrNack = errors.New("broker: message rejected")

// Consumer consumes TaskTriggerEvents from the broker.
type Consumer interface {
	// Consume blocks, calling handler for each message. Returns when the
	// channel is closed or a fatal error occurs. Returning ErrNack from
	// handler nacks without requeue; any other error also nacks without requeue.
	Consume(handler func(TaskTriggerEvent) error) error
	Close() error
}

// PeekDLQ reads one message from the dead-letter queue without acknowledging
// it. Returns nil if the queue is empty. Intended for testing only.
func PeekDLQ(c Consumer) (*TaskTriggerEvent, error) {
	ac, ok := c.(*amqpConsumer)
	if !ok {
		return nil, fmt.Errorf("PeekDLQ: not an amqpConsumer")
	}
	msg, ok2, err := ac.ch.Get(QueueDLQ, false)
	if err != nil {
		return nil, fmt.Errorf("broker: peek DLQ: %w", err)
	}
	if !ok2 {
		return nil, nil
	}
	var evt TaskTriggerEvent
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		return nil, fmt.Errorf("broker: unmarshal DLQ message: %w", err)
	}
	return &evt, nil
}

type amqpConsumer struct {
	ch *amqp.Channel
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
	return &amqpConsumer{ch: ch}, nil
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
	return nil
}

func (c *amqpConsumer) Close() error {
	return c.ch.Close()
}
