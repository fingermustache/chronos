package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const publishConfirmTimeout = 10 * time.Second

// Publisher publishes TaskTriggerEvents to the broker.
type Publisher interface {
	Publish(ctx context.Context, evt TaskTriggerEvent) error
	Close() error
}

type amqpPublisher struct {
	ch *amqp.Channel
}

// NewPublisher creates a Publisher backed by a dedicated AMQP channel.
func NewPublisher(conn *Connection) (Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("broker: open publisher channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("broker: enable publisher confirms: %w", err)
	}
	return &amqpPublisher{ch: ch}, nil
}

func (p *amqpPublisher) Publish(ctx context.Context, evt TaskTriggerEvent) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("broker: marshal event: %w", err)
	}

	msg := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}

	ctx, cancel := context.WithTimeout(ctx, publishConfirmTimeout)
	defer cancel()

	confirms, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, ExchangeTrigger, RoutingKey, true, false, msg)
	if err != nil {
		return fmt.Errorf("broker: publish: %w", err)
	}
	ok, err := confirms.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("broker: publish confirm context error: %w", err)
	}
	if !ok {
		return fmt.Errorf("broker: publish not confirmed by broker")
	}
	return nil
}

func (p *amqpPublisher) Close() error {
	return p.ch.Close()
}
