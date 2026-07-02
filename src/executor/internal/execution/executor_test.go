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
	"github.com/fingermustache/chronos/executor/internal/repository"
	"github.com/fingermustache/chronos/executor/internal/runners"
	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// mockRunner is a TaskRunner whose behaviour is set per test.
type mockRunner struct {
	runFn func(ctx context.Context, config map[string]any) (runners.Result, error)
}

func (m *mockRunner) Run(ctx context.Context, config map[string]any) (runners.Result, error) {
	return m.runFn(ctx, config)
}

// mockRepo is an ExecutionRepository whose behaviour is set per test. It
// records every call so tests can assert on status transitions.
type mockRepo struct {
	createFn func(ctx context.Context, params repository.CreateExecutionParams) (*models.ExecutionHistory, error)
	updateFn func(ctx context.Context, id uuid.UUID, params repository.UpdateExecutionParams) error

	creates []repository.CreateExecutionParams
	updates []repository.UpdateExecutionParams
}

func (m *mockRepo) Create(ctx context.Context, params repository.CreateExecutionParams) (*models.ExecutionHistory, error) {
	m.creates = append(m.creates, params)
	if m.createFn != nil {
		return m.createFn(ctx, params)
	}
	return &models.ExecutionHistory{ID: uuid.New(), TaskID: params.TaskID, Status: params.Status}, nil
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, params repository.UpdateExecutionParams) error {
	m.updates = append(m.updates, params)
	if m.updateFn != nil {
		return m.updateFn(ctx, id, params)
	}
	return nil
}

func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.ExecutionHistory, error) {
	return nil, errors.New("mockRepo: GetByID not implemented")
}

func (m *mockRepo) GetByTaskID(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error) {
	return nil, errors.New("mockRepo: GetByTaskID not implemented")
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

func newTestExecutor(taskType string, runner runners.TaskRunner, repo repository.ExecutionRepository) *execution.Executor {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return execution.NewWithRunners(nil, repo, logger, map[string]runners.TaskRunner{
		taskType: runner,
	})
}

func TestHandle_UnsupportedTaskType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	repo := &mockRepo{}
	exec := execution.NewWithRunners(nil, repo, logger, map[string]runners.TaskRunner{})

	err := execution.Handle(exec, triggerEvent("grpc"))
	if err == nil {
		t.Fatal("expected error for unsupported task type, got nil")
	}
	if !errors.Is(err, runners.ErrUnsupportedTaskType) {
		t.Errorf("expected ErrUnsupportedTaskType, got %v", err)
	}
	if len(repo.creates) != 0 {
		t.Errorf("expected no execution record for unsupported task type, got %d", len(repo.creates))
	}
}

func TestHandle_HTTPTask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &mockRepo{}
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			return runners.Result{StatusCode: 200, Output: `{"ok":true}`}, nil
		},
	}, repo)

	if err := execution.Handle(exec, triggerEvent("http")); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if len(repo.creates) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(repo.creates))
	}
	if repo.creates[0].Status != models.StatusRunning {
		t.Errorf("expected running status on create, got %q", repo.creates[0].Status)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(repo.updates))
	}
	update := repo.updates[0]
	if update.Status != models.StatusSuccess {
		t.Errorf("expected success status on update, got %q", update.Status)
	}
	if update.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	if update.DurationMs == nil {
		t.Error("expected DurationMs to be set")
	}
	if update.Output == nil || *update.Output != `{"ok":true}` {
		t.Errorf("expected output to be recorded, got %v", update.Output)
	}
	if update.ErrorMessage != nil {
		t.Errorf("expected no error message on success, got %v", *update.ErrorMessage)
	}
}

func TestHandle_HTTPTask_Failure(t *testing.T) {
	repo := &mockRepo{}
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			return runners.Result{StatusCode: 500}, errors.New("server error")
		},
	}, repo)

	if err := execution.Handle(exec, triggerEvent("http")); err == nil {
		t.Fatal("expected error on runner failure, got nil")
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(repo.updates))
	}
	update := repo.updates[0]
	if update.Status != models.StatusFailed {
		t.Errorf("expected failed status on update, got %q", update.Status)
	}
	if update.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	if update.DurationMs == nil {
		t.Error("expected DurationMs to be set")
	}
	if update.ErrorMessage == nil || *update.ErrorMessage != "server error" {
		t.Errorf("expected error message 'server error', got %v", update.ErrorMessage)
	}
}

func TestHandle_EnforcesTimeout(t *testing.T) {
	var ctxDeadlineSet bool
	repo := &mockRepo{}
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(ctx context.Context, _ map[string]any) (runners.Result, error) {
			_, ctxDeadlineSet = ctx.Deadline()
			return runners.Result{StatusCode: 200}, nil
		},
	}, repo)

	evt := triggerEvent("http")
	evt.TimeoutSeconds = 5
	execution.Handle(exec, evt)

	if !ctxDeadlineSet {
		t.Error("expected context to have a deadline from timeout_seconds, got none")
	}
}

func TestHandle_CreateFails_NacksWithoutInvokingRunner(t *testing.T) {
	runnerInvoked := false
	repo := &mockRepo{
		createFn: func(_ context.Context, _ repository.CreateExecutionParams) (*models.ExecutionHistory, error) {
			return nil, errors.New("db unavailable")
		},
	}
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			runnerInvoked = true
			return runners.Result{StatusCode: 200}, nil
		},
	}, repo)

	err := execution.Handle(exec, triggerEvent("http"))
	if err == nil {
		t.Fatal("expected error when Create fails, got nil")
	}
	if runnerInvoked {
		t.Error("expected runner not to be invoked when the initial record write fails")
	}
	if len(repo.updates) != 0 {
		t.Errorf("expected no update calls when Create fails, got %d", len(repo.updates))
	}
}

func TestHandle_UpdateStatusFails_StillAcksSuccessfulRun(t *testing.T) {
	repo := &mockRepo{
		updateFn: func(_ context.Context, _ uuid.UUID, _ repository.UpdateExecutionParams) error {
			return errors.New("db unavailable")
		},
	}
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			return runners.Result{StatusCode: 200}, nil
		},
	}, repo)

	if err := execution.Handle(exec, triggerEvent("http")); err != nil {
		t.Fatalf("expected success to still ack even if the status update fails, got: %v", err)
	}
}
