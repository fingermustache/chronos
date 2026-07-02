package execution_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fingermustache/chronos/executor/internal/execution"
	"github.com/fingermustache/chronos/executor/internal/runners"
	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/google/uuid"
)

// mockRunner is a TaskRunner whose behaviour is set per test.
type mockRunner struct {
	runFn func(ctx context.Context, config map[string]any) (runners.Result, error)
}

func (m *mockRunner) Run(ctx context.Context, config map[string]any) (runners.Result, error) {
	return m.runFn(ctx, config)
}

func triggerEvent(taskType string) broker.TaskTriggerEvent {
	return broker.TaskTriggerEvent{
		TaskID:         uuid.New(),
		TaskType:       taskType,
		ScheduleType:   "cron",
		TaskConfig:     map[string]any{"url": "http://example.com"},
		MaxRetries:     3,
		TimeoutSeconds: 30,
		TriggeredAt:    time.Now().UTC(),
	}
}

func newTestExecutor(taskType string, runner runners.TaskRunner) *execution.Executor {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return execution.NewWithRunners(nil, logger, map[string]runners.TaskRunner{
		taskType: runner,
	})
}

func TestHandle_UnsupportedTaskType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	exec := execution.NewWithRunners(nil, logger, map[string]runners.TaskRunner{})

	err := execution.Handle(exec, triggerEvent("grpc"))
	if err == nil {
		t.Fatal("expected error for unsupported task type, got nil")
	}
	if !errors.Is(err, runners.ErrUnsupportedTaskType) {
		t.Errorf("expected ErrUnsupportedTaskType, got %v", err)
	}
}

func TestHandle_HTTPTask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			return runners.Result{StatusCode: 200}, nil
		},
	})

	if err := execution.Handle(exec, triggerEvent("http")); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestHandle_HTTPTask_Failure(t *testing.T) {
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			return runners.Result{StatusCode: 500}, errors.New("server error")
		},
	})

	if err := execution.Handle(exec, triggerEvent("http")); err == nil {
		t.Fatal("expected error on runner failure, got nil")
	}
}

func TestHandle_EnforcesTimeout(t *testing.T) {
	var ctxDeadlineSet bool
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(ctx context.Context, _ map[string]any) (runners.Result, error) {
			_, ctxDeadlineSet = ctx.Deadline()
			return runners.Result{StatusCode: 200}, nil
		},
	})

	evt := triggerEvent("http")
	evt.TimeoutSeconds = 5
	execution.Handle(exec, evt)

	if !ctxDeadlineSet {
		t.Error("expected context to have a deadline from timeout_seconds, got none")
	}
}
