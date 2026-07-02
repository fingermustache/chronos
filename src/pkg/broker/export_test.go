package broker

import (
	"encoding/json"
	"fmt"
)

// PeekDLQ reads one message from the dead-letter queue without acknowledging
// it. Opens a dedicated channel so it does not race with a concurrent Consume
// goroutine. Returns nil if the queue is empty. Test use only.
func PeekDLQ(c Consumer) (*TaskTriggerEvent, error) {
	ac, ok := c.(*amqpConsumer)
	if !ok {
		return nil, fmt.Errorf("PeekDLQ: not an amqpConsumer")
	}
	ch, err := ac.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("broker: open DLQ peek channel: %w", err)
	}
	defer ch.Close()

	msg, ok2, err := ch.Get(QueueDLQ, false)
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
