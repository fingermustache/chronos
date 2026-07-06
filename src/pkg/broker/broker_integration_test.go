//go:build integration

package broker_test

import (
	"context"
	"testing"
	"time"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

func newTestBroker(t *testing.T) (broker.Publisher, broker.Consumer, func()) {
	t.Helper()

	ctx := context.Background()
	container, err := rabbitmq.Run(ctx, "rabbitmq:3.13-management-alpine")
	if err != nil {
		t.Fatalf("start rabbitmq container: %v", err)
	}

	amqpURL, err := container.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("get amqp url: %v", err)
	}

	cfg := broker.Config{URL: amqpURL}

	conn, err := broker.NewConnection(cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	if err := broker.SetupTopology(conn); err != nil {
		t.Fatalf("setup topology: %v", err)
	}

	pub, err := broker.NewPublisher(conn)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	sub, err := broker.NewConsumer(conn)
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}

	cleanup := func() {
		pub.Close()
		sub.Close()
		conn.Close()
		container.Terminate(ctx)
	}

	return pub, sub, cleanup
}

func TestIntegration_PublishAndConsume(t *testing.T) {
	pub, sub, cleanup := newTestBroker(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sent := broker.TaskTriggerEvent{
		TaskID:         uuid.New(),
		ScheduleType:   "cron",
		TaskType:       "http",
		TaskConfig:     map[string]any{"url": "https://example.com"},
		MaxRetries:     3,
		TimeoutSeconds: 30,
		TriggeredAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := pub.Publish(ctx, sent); err != nil {
		t.Fatalf("publish: %v", err)
	}

	received := make(chan broker.TaskTriggerEvent, 1)

	go func() {
		sub.Consume(func(evt broker.TaskTriggerEvent) error {
			received <- evt
			return nil
		})
	}()

	select {
	case got := <-received:
		if got.TaskID != sent.TaskID {
			t.Errorf("TaskID: got %v, want %v", got.TaskID, sent.TaskID)
		}
		if got.ScheduleType != sent.ScheduleType {
			t.Errorf("ScheduleType: got %v, want %v", got.ScheduleType, sent.ScheduleType)
		}
		if got.TaskType != sent.TaskType {
			t.Errorf("TaskType: got %v, want %v", got.TaskType, sent.TaskType)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

func TestIntegration_DeadLetterOnNack(t *testing.T) {
	pub, sub, cleanup := newTestBroker(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	evt := broker.TaskTriggerEvent{
		TaskID:         uuid.New(),
		ScheduleType:   "once",
		TaskType:       "http",
		TaskConfig:     map[string]any{},
		MaxRetries:     0,
		TimeoutSeconds: 10,
		TriggeredAt:    time.Now().UTC(),
	}

	if err := pub.Publish(ctx, evt); err != nil {
		t.Fatalf("publish: %v", err)
	}

	done := make(chan struct{})
	go func() {
		sub.Consume(func(_ broker.TaskTriggerEvent) error {
			close(done)
			return broker.ErrNack
		})
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for consumer to process message")
	}

	// Poll the DLQ — dead-letter routing is async so give RabbitMQ a moment.
	deadline := time.Now().Add(5 * time.Second)
	var dlqMsg *broker.TaskTriggerEvent
	for time.Now().Before(deadline) {
		var peekErr error
		dlqMsg, peekErr = broker.PeekDLQ(sub)
		if peekErr != nil {
			t.Fatalf("peek DLQ: %v", peekErr)
		}
		if dlqMsg != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if dlqMsg == nil {
		t.Error("expected message in DLQ after nack, got none after 5s")
	}
}

// TestIntegration_DefaultConfigMatchesDockerComposeCredentials reproduces the
// documented local-dev flow: a RabbitMQ instance provisioned exactly like
// docker-compose.yaml (RABBITMQ_DEFAULT_USER/PASS=chronos), reached via
// broker.DefaultConfig()'s real environment-variable fallback with only
// RABBITMQ_HOST/PORT set — RABBITMQ_USER/PASSWORD deliberately left unset, so
// this exercises the actual default credentials a fresh clone would use.
func TestIntegration_DefaultConfigMatchesDockerComposeCredentials(t *testing.T) {
	ctx := context.Background()

	container, err := rabbitmq.Run(ctx, "rabbitmq:3.13-management-alpine",
		rabbitmq.WithAdminUsername("chronos"),
		rabbitmq.WithAdminPassword("chronos"),
	)
	if err != nil {
		t.Fatalf("start rabbitmq container: %v", err)
	}
	defer container.Terminate(ctx)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5672")
	if err != nil {
		t.Fatalf("get container port: %v", err)
	}

	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_HOST", host)
	t.Setenv("RABBITMQ_PORT", port.Port())
	t.Setenv("RABBITMQ_USER", "")
	t.Setenv("RABBITMQ_PASSWORD", "")

	conn, err := broker.NewConnection(broker.DefaultConfig())
	if err != nil {
		t.Fatalf("expected broker.DefaultConfig() to connect using the same credentials docker-compose.yaml provisions the broker with, got: %v", err)
	}
	conn.Close()
}
