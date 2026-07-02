//go:build !integration

package broker_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/google/uuid"
)

// --- TaskTriggerEvent schema tests ---

func TestTaskTriggerEvent_MarshalRoundtrip(t *testing.T) {
	taskID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	orig := broker.TaskTriggerEvent{
		TaskID:         taskID,
		ScheduleType:   "cron",
		TaskType:       "http",
		TaskConfig:     map[string]any{"url": "https://example.com", "method": "POST"},
		MaxRetries:     3,
		TimeoutSeconds: 60,
		TriggeredAt:    now,
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got broker.TaskTriggerEvent
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.TaskID != orig.TaskID {
		t.Errorf("TaskID: got %v, want %v", got.TaskID, orig.TaskID)
	}
	if got.ScheduleType != orig.ScheduleType {
		t.Errorf("ScheduleType: got %v, want %v", got.ScheduleType, orig.ScheduleType)
	}
	if got.TaskType != orig.TaskType {
		t.Errorf("TaskType: got %v, want %v", got.TaskType, orig.TaskType)
	}
	if got.MaxRetries != orig.MaxRetries {
		t.Errorf("MaxRetries: got %v, want %v", got.MaxRetries, orig.MaxRetries)
	}
	if got.TimeoutSeconds != orig.TimeoutSeconds {
		t.Errorf("TimeoutSeconds: got %v, want %v", got.TimeoutSeconds, orig.TimeoutSeconds)
	}
	if !got.TriggeredAt.Equal(orig.TriggeredAt) {
		t.Errorf("TriggeredAt: got %v, want %v", got.TriggeredAt, orig.TriggeredAt)
	}
}

func TestTaskTriggerEvent_RequiredFields(t *testing.T) {
	raw := `{"task_id":"00000000-0000-0000-0000-000000000000","schedule_type":"interval","task_type":"http","task_config":{},"max_retries":0,"timeout_seconds":0,"triggered_at":"2026-01-01T00:00:00Z"}`
	var evt broker.TaskTriggerEvent
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if evt.ScheduleType != "interval" {
		t.Errorf("expected schedule_type 'interval', got %q", evt.ScheduleType)
	}
}

// --- Config tests ---

func TestConfig_Defaults(t *testing.T) {
	cfg := broker.DefaultConfig()
	if cfg.AMQPURL() == "" {
		t.Error("expected non-empty default AMQP URL")
	}
}

func TestConfig_URLFromEnv(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "amqp://user:pass@myhost:5672/")
	cfg := broker.DefaultConfig()
	if cfg.AMQPURL() != "amqp://user:pass@myhost:5672/" {
		t.Errorf("expected URL from env, got %q", cfg.AMQPURL())
	}
}

// --- Interface compliance (compile-time) ---
// These assignments verify that the mock types below satisfy the interfaces.
// If Publisher or Consumer interfaces change incompatibly, these lines fail to compile.

var _ broker.Publisher = (*mockPublisher)(nil)
var _ broker.Consumer = (*mockConsumer)(nil)

type mockPublisher struct{}

func (m *mockPublisher) Publish(_ broker.TaskTriggerEvent) error { return nil }
func (m *mockPublisher) Close() error                            { return nil }

type mockConsumer struct{}

func (m *mockConsumer) Consume(_ func(broker.TaskTriggerEvent) error) error { return nil }
func (m *mockConsumer) Close() error                                         { return nil }
