//go:build integration

package execution_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fingermustache/chronos/executor/internal/execution"
	"github.com/fingermustache/chronos/executor/internal/repository"
	"github.com/fingermustache/chronos/executor/internal/runners"
	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/fingermustache/chronos/pkg/testutil"
	"github.com/google/uuid"
)

// seedTask inserts a minimal task row to satisfy the execution_history FK.
func seedTask(t *testing.T, db *database.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.Exec(`
		INSERT INTO tasks (
			id, name, schedule_type, schedule_config,
			task_type, task_config, enabled
		) VALUES ($1, $2, 'cron', '{}'::jsonb, 'http', '{}'::jsonb, true)`,
		id, "task-"+id.String(),
	)
	if err != nil {
		t.Fatalf("seedTask: %v", err)
	}
	return id
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestIntegration_Handle_HTTPTask_Success_WritesExecutionRow(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	repo := repository.NewExecutionRepository(db)
	taskID := seedTask(t, db)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	exec := execution.NewWithRunners(nil, repo, newTestLogger(), map[string]runners.TaskRunner{
		"http": runners.NewHTTPRunner(),
	})

	evt := broker.TaskTriggerEvent{
		TaskID:         taskID,
		TaskType:       "http",
		ScheduleType:   "cron",
		TaskConfig:     map[string]any{"url": srv.URL},
		TimeoutSeconds: 5,
		TriggeredAt:    time.Now().UTC(),
	}

	if err := execution.Handle(exec, evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records, err := repo.GetByTaskID(ctx, taskID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 execution record, got %d", len(records))
	}

	record := records[0]
	if record.Status != models.StatusSuccess {
		t.Errorf("got status %q, want %q", record.Status, models.StatusSuccess)
	}
	if record.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
	if record.DurationMs == nil {
		t.Error("expected duration_ms to be set")
	}
	if record.Output == nil || *record.Output != `{"ok":true}` {
		t.Errorf("got output %v, want '{\"ok\":true}'", record.Output)
	}
	if record.ErrorMessage != nil {
		t.Errorf("expected no error_message, got %v", *record.ErrorMessage)
	}
}

func TestIntegration_Handle_HTTPTask_Failure_WritesExecutionRow(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	repo := repository.NewExecutionRepository(db)
	taskID := seedTask(t, db)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	exec := execution.NewWithRunners(nil, repo, newTestLogger(), map[string]runners.TaskRunner{
		"http": runners.NewHTTPRunner(),
	})

	evt := broker.TaskTriggerEvent{
		TaskID:         taskID,
		TaskType:       "http",
		ScheduleType:   "cron",
		TaskConfig:     map[string]any{"url": srv.URL},
		TimeoutSeconds: 5,
		TriggeredAt:    time.Now().UTC(),
	}

	if err := execution.Handle(exec, evt); err == nil {
		t.Fatal("expected error for non-2xx response, got nil")
	}

	records, err := repo.GetByTaskID(ctx, taskID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 execution record, got %d", len(records))
	}

	record := records[0]
	if record.Status != models.StatusFailed {
		t.Errorf("got status %q, want %q", record.Status, models.StatusFailed)
	}
	if record.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
	if record.DurationMs == nil {
		t.Error("expected duration_ms to be set")
	}
	if record.ErrorMessage == nil {
		t.Error("expected error_message to be set")
	}
}

func TestIntegration_Handle_RetriesThenSucceeds_WritesOneRowPerAttempt(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	repo := repository.NewExecutionRepository(db)
	taskID := seedTask(t, db)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	exec := execution.NewWithRunners(nil, repo, newTestLogger(), map[string]runners.TaskRunner{
		"http": runners.NewHTTPRunner(),
	})
	execution.SetSleep(exec, func(time.Duration) {}) // skip real backoff delay

	evt := broker.TaskTriggerEvent{
		TaskID:         taskID,
		TaskType:       "http",
		ScheduleType:   "cron",
		TaskConfig:     map[string]any{"url": srv.URL},
		MaxRetries:     1,
		TimeoutSeconds: 5,
		TriggeredAt:    time.Now().UTC(),
	}

	if err := execution.Handle(exec, evt); err != nil {
		t.Fatalf("expected eventual success on retry, got: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected the server to be hit twice (1 failure + 1 retry), got %d", requests)
	}

	records, err := repo.GetByTaskID(ctx, taskID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 execution records (one per attempt), got %d", len(records))
	}

	// GetByTaskID orders newest first, so index 0 is the successful retry.
	retry, first := records[0], records[1]

	if first.RetryCount != 0 || first.Status != models.StatusFailed {
		t.Errorf("expected the first attempt to be retry_count=0/failed, got retry_count=%d status=%q", first.RetryCount, first.Status)
	}
	if retry.RetryCount != 1 || retry.Status != models.StatusSuccess {
		t.Errorf("expected the retry to be retry_count=1/success, got retry_count=%d status=%q", retry.RetryCount, retry.Status)
	}
	if retry.Output == nil || *retry.Output != `{"ok":true}` {
		t.Errorf("expected the retry's output to be recorded, got %v", retry.Output)
	}
}

func TestIntegration_Handle_UnsupportedTaskType_WritesNoExecutionRow(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	repo := repository.NewExecutionRepository(db)
	taskID := seedTask(t, db)

	exec := execution.NewWithRunners(nil, repo, newTestLogger(), map[string]runners.TaskRunner{})

	evt := broker.TaskTriggerEvent{
		TaskID:         taskID,
		TaskType:       "grpc",
		ScheduleType:   "cron",
		TaskConfig:     map[string]any{},
		TimeoutSeconds: 5,
		TriggeredAt:    time.Now().UTC(),
	}

	if err := execution.Handle(exec, evt); err == nil {
		t.Fatal("expected error for unsupported task type, got nil")
	}

	records, err := repo.GetByTaskID(ctx, taskID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 execution records for unsupported task type, got %d", len(records))
	}
}
