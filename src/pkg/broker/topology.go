package broker

import amqp "github.com/rabbitmq/amqp091-go"

const (
	ExchangeTrigger = "task.trigger"
	ExchangeDLX     = "task.dlx"
	QueueTrigger    = "task.trigger.queue"
	QueueDLQ        = "task.dlq"
	RoutingKey      = "task.trigger"
)

// SetupTopology declares the exchange/queue topology on the broker.
// Safe to call on every startup — all declarations are idempotent.
//
// Topology:
//
//	task.trigger (direct exchange)
//	  └── task.trigger.queue   dead-letters → task.dlx
//	task.dlx (fanout exchange)
//	  └── task.dlq
func SetupTopology(conn *Connection) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	// Dead-letter exchange
	if err := ch.ExchangeDeclare(ExchangeDLX, amqp.ExchangeFanout, true, false, false, false, nil); err != nil {
		return err
	}

	// Dead-letter queue bound to the DLX
	dlq, err := ch.QueueDeclare(QueueDLQ, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(dlq.Name, "", ExchangeDLX, false, nil); err != nil {
		return err
	}

	// Main trigger exchange
	if err := ch.ExchangeDeclare(ExchangeTrigger, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return err
	}

	// Main trigger queue — dead-letters to the DLX
	triggerArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
	}
	q, err := ch.QueueDeclare(QueueTrigger, true, false, false, false, triggerArgs)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(q.Name, RoutingKey, ExchangeTrigger, false, nil); err != nil {
		return err
	}

	return nil
}
