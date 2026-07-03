package execution_test

import (
	"context"
	"errors"
	"fmt"
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
	// Mimic a real DB client: a call made with an already-cancelled context fails.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &models.ExecutionHistory{ID: uuid.New(), TaskID: params.TaskID, Status: params.Status}, nil
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, params repository.UpdateExecutionParams) error {
	m.updates = append(m.updates, params)
	if m.updateFn != nil {
		return m.updateFn(ctx, id, params)
	}
	return ctx.Err()
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
	exec := execution.NewWithRunners(nil, repo, logger, map[string]runners.TaskRunner{
		taskType: runner,
	})
	// Tests don't want to wait out real backoff delays unless they opt in.
	execution.SetSleep(exec, func(context.Context, time.Duration) {})
	return exec
}

// sequenceRunner returns one entry from results per call, in order, and
// panics if called more times than there are results — used to script
// multi-attempt retry scenarios.
type sequenceRunner struct {
	results []struct {
		result runners.Result
		err    error
	}
	calls int
}

func (s *sequenceRunner) Run(_ context.Context, _ map[string]any) (runners.Result, error) {
	r := s.results[s.calls]
	s.calls++
	return r.result, r.err
}

func newSequenceRunner(entries ...struct {
	result runners.Result
	err    error
}) *sequenceRunner {
	return &sequenceRunner{results: entries}
}

func ok(output string) struct {
	result runners.Result
	err    error
} {
	return struct {
		result runners.Result
		err    error
	}{result: runners.Result{StatusCode: 200, Output: output}}
}

func fail(msg string) struct {
	result runners.Result
	err    error
} {
	return struct {
		result runners.Result
		err    error
	}{err: errors.New(msg)}
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
	if len(repo.updates) != 0 {
		t.Errorf("expected no update calls for unsupported task type, got %d", len(repo.updates))
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
			return runners.Result{StatusCode: 500, Output: `{"error":"boom"}`}, errors.New("server error")
		},
	}, repo)

	evt := triggerEvent("http")
	evt.MaxRetries = 0
	if err := execution.Handle(exec, evt); err == nil {
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
	if update.Output == nil || *update.Output != `{"error":"boom"}` {
		t.Errorf("expected the runner's output to be recorded on failure too, got %v", update.Output)
	}
}

func TestHandle_Timeout_RecordsStatusTimeout(t *testing.T) {
	repo := &mockRepo{}
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			return runners.Result{}, fmt.Errorf("http runner: execute request: %w", context.DeadlineExceeded)
		},
	}, repo)

	evt := triggerEvent("http")
	evt.MaxRetries = 0
	if err := execution.Handle(exec, evt); err == nil {
		t.Fatal("expected error on timeout, got nil")
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(repo.updates))
	}
	if update := repo.updates[0]; update.Status != models.StatusTimeout {
		t.Errorf("expected timeout status on update, got %q", update.Status)
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

func TestHandle_RecordWritesSurviveCancelledShutdownContext(t *testing.T) {
	// Simulates a SIGTERM arriving mid-task: the shutdown context is already
	// done by the time the runner finishes, but the execution history writes
	// must still succeed since they run on their own context.
	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := &mockRepo{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	exec := execution.NewWithRunners(nil, repo, logger, map[string]runners.TaskRunner{
		"http": &mockRunner{
			runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
				return runners.Result{StatusCode: 200, Output: "ok"}, nil
			},
		},
	})

	if err := execution.HandleWithContext(exec, shutdownCtx, triggerEvent("http")); err != nil {
		t.Fatalf("expected success even with an already-cancelled shutdown context, got: %v", err)
	}
	if len(repo.creates) != 1 {
		t.Fatalf("expected the running record to be created despite the cancelled shutdown context, got %d creates", len(repo.creates))
	}
	if len(repo.updates) != 1 || repo.updates[0].Status != models.StatusSuccess {
		t.Fatalf("expected the completion record to be written despite the cancelled shutdown context, got %+v", repo.updates)
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

// --- retry loop ---

func TestHandle_Retry_SucceedsOnFirstAttempt_NoRetries(t *testing.T) {
	repo := &mockRepo{}
	var sleeps []time.Duration
	exec := newTestExecutor("http", newSequenceRunner(ok(`{"ok":true}`)), repo)
	execution.SetSleep(exec, func(_ context.Context, d time.Duration) { sleeps = append(sleeps, d) })

	evt := triggerEvent("http")
	evt.MaxRetries = 3
	if err := execution.Handle(exec, evt); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if len(repo.creates) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(repo.creates))
	}
	if repo.creates[0].RetryCount != 0 {
		t.Errorf("expected retry_count 0 on the first attempt, got %d", repo.creates[0].RetryCount)
	}
	if len(repo.updates) != 1 || repo.updates[0].Status != models.StatusSuccess {
		t.Fatalf("expected 1 successful update, got %+v", repo.updates)
	}
	if len(sleeps) != 0 {
		t.Errorf("expected no backoff sleep when the first attempt succeeds, got %v", sleeps)
	}
}

func TestHandle_Retry_SucceedsOnRetry2(t *testing.T) {
	repo := &mockRepo{}
	var sleeps []time.Duration
	exec := newTestExecutor("http", newSequenceRunner(
		fail("boom 1"),
		fail("boom 2"),
		ok(`{"ok":true}`),
	), repo)
	execution.SetSleep(exec, func(_ context.Context, d time.Duration) { sleeps = append(sleeps, d) })

	evt := triggerEvent("http")
	evt.MaxRetries = 3
	if err := execution.Handle(exec, evt); err != nil {
		t.Fatalf("expected eventual success, got: %v", err)
	}

	if len(repo.creates) != 3 {
		t.Fatalf("expected 3 create calls (2 failed attempts + 1 success), got %d", len(repo.creates))
	}
	for i, c := range repo.creates {
		if c.RetryCount != i {
			t.Errorf("create[%d]: expected retry_count %d, got %d", i, i, c.RetryCount)
		}
	}

	if len(repo.updates) != 3 {
		t.Fatalf("expected 3 update calls, got %d", len(repo.updates))
	}
	if repo.updates[0].Status != models.StatusFailed || repo.updates[1].Status != models.StatusFailed {
		t.Errorf("expected the first two attempts to be recorded as failed, got %+v", repo.updates)
	}
	if repo.updates[2].Status != models.StatusSuccess {
		t.Errorf("expected the final attempt to be recorded as success, got %q", repo.updates[2].Status)
	}

	wantSleeps := []time.Duration{1 * time.Second, 2 * time.Second}
	if len(sleeps) != len(wantSleeps) {
		t.Fatalf("expected backoff sleeps %v, got %v", wantSleeps, sleeps)
	}
	for i, want := range wantSleeps {
		if sleeps[i] != want {
			t.Errorf("sleep[%d]: expected %v, got %v", i, want, sleeps[i])
		}
	}
}

func TestHandle_Retry_ExhaustsAfterMaxRetries(t *testing.T) {
	repo := &mockRepo{}
	var sleeps []time.Duration
	exec := newTestExecutor("http", newSequenceRunner(
		fail("boom 1"),
		fail("boom 2"),
		fail("boom 3"),
	), repo)
	execution.SetSleep(exec, func(_ context.Context, d time.Duration) { sleeps = append(sleeps, d) })

	evt := triggerEvent("http")
	evt.MaxRetries = 2
	err := execution.Handle(exec, evt)
	if err == nil {
		t.Fatal("expected error after exhausting all retries, got nil")
	}
	if err.Error() != "boom 3" {
		t.Errorf("expected the last attempt's error to be returned, got %q", err.Error())
	}

	if len(repo.creates) != 3 {
		t.Fatalf("expected 3 attempts (1 initial + 2 retries), got %d", len(repo.creates))
	}
	for _, u := range repo.updates {
		if u.Status != models.StatusFailed {
			t.Errorf("expected every attempt to be recorded as failed, got %q", u.Status)
		}
	}

	wantSleeps := []time.Duration{1 * time.Second, 2 * time.Second}
	if len(sleeps) != len(wantSleeps) {
		t.Fatalf("expected backoff sleeps %v, got %v", wantSleeps, sleeps)
	}
}

func TestHandle_Retry_TimeoutCountsAsFailureAndConsumesRetrySlot(t *testing.T) {
	repo := &mockRepo{}
	exec := newTestExecutor("http", newSequenceRunner(
		struct {
			result runners.Result
			err    error
		}{err: fmt.Errorf("http runner: execute request: %w", context.DeadlineExceeded)},
		ok(`{"ok":true}`),
	), repo)

	evt := triggerEvent("http")
	evt.MaxRetries = 1
	if err := execution.Handle(exec, evt); err != nil {
		t.Fatalf("expected success on the retry after a timeout, got: %v", err)
	}

	if len(repo.updates) != 2 {
		t.Fatalf("expected 2 update calls, got %d", len(repo.updates))
	}
	if repo.updates[0].Status != models.StatusTimeout {
		t.Errorf("expected the first attempt to be recorded as timeout, got %q", repo.updates[0].Status)
	}
	if repo.updates[1].Status != models.StatusSuccess {
		t.Errorf("expected the retry to be recorded as success, got %q", repo.updates[1].Status)
	}
}

func TestHandle_Retry_MaxRetriesZero_OneAttemptOnly(t *testing.T) {
	repo := &mockRepo{}
	var sleeps []time.Duration
	exec := newTestExecutor("http", newSequenceRunner(fail("boom")), repo)
	execution.SetSleep(exec, func(_ context.Context, d time.Duration) { sleeps = append(sleeps, d) })

	evt := triggerEvent("http")
	evt.MaxRetries = 0
	if err := execution.Handle(exec, evt); err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(repo.creates) != 1 {
		t.Fatalf("expected exactly 1 attempt when max_retries=0, got %d", len(repo.creates))
	}
	if len(sleeps) != 0 {
		t.Errorf("expected no backoff sleep with max_retries=0, got %v", sleeps)
	}
}

func TestHandle_Retry_CreateFailsOnRetryPath_ContinuesWithoutNacking(t *testing.T) {
	repo := &mockRepo{
		createFn: func(_ context.Context, params repository.CreateExecutionParams) (*models.ExecutionHistory, error) {
			if params.RetryCount == 1 {
				return nil, errors.New("db unavailable")
			}
			return &models.ExecutionHistory{ID: uuid.New(), TaskID: params.TaskID, Status: params.Status}, nil
		},
	}
	runnerInvocations := 0
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			runnerInvocations++
			if runnerInvocations == 1 {
				return runners.Result{}, errors.New("first attempt fails")
			}
			return runners.Result{StatusCode: 200, Output: "ok"}, nil
		},
	}, repo)

	evt := triggerEvent("http")
	evt.MaxRetries = 2
	// Attempt 0: Create succeeds, runner fails -> recorded as failed.
	// Attempt 1 (retry_count=1): Create fails (simulated DB hiccup) but the
	// runner still runs (best-effort history on retry paths) and succeeds,
	// so the message is still acked even though that attempt goes unrecorded.
	if err := execution.Handle(exec, evt); err != nil {
		t.Fatalf("expected eventual success despite the retry's history write failing, got: %v", err)
	}

	if runnerInvocations != 2 {
		t.Fatalf("expected the runner to still be invoked on the retry despite the failed history write, got %d invocations", runnerInvocations)
	}
	if len(repo.creates) != 2 {
		t.Fatalf("expected 2 create attempts (retry_count 0 and 1), got %d", len(repo.creates))
	}
	if len(repo.updates) != 1 || repo.updates[0].Status != models.StatusFailed {
		t.Fatalf("expected only the first (failed) attempt to have a completion record — the successful retry's history write was skipped since Create failed for it, got %+v", repo.updates)
	}
}

func TestHandle_Retry_ClampsExcessiveMaxRetries(t *testing.T) {
	repo := &mockRepo{}
	exec := newTestExecutor("http", &mockRunner{
		runFn: func(_ context.Context, _ map[string]any) (runners.Result, error) {
			return runners.Result{}, errors.New("always fails")
		},
	}, repo)

	evt := triggerEvent("http")
	evt.MaxRetries = 10_000 // far beyond the API-validated ceiling of 10

	if err := execution.Handle(exec, evt); err == nil {
		t.Fatal("expected error after exhausting the clamped retry ceiling, got nil")
	}

	const wantAttempts = 11 // maxAllowedRetries(10) + 1
	if len(repo.creates) != wantAttempts {
		t.Fatalf("expected max_retries to be clamped to a bounded number of attempts (%d), got %d", wantAttempts, len(repo.creates))
	}
}

func TestHandle_Retry_BackoffRespectsShutdownCancellation(t *testing.T) {
	repo := &mockRepo{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	exec := execution.NewWithRunners(nil, repo, logger, map[string]runners.TaskRunner{
		"http": newSequenceRunner(fail("boom 1"), fail("boom 2")),
	})
	// Deliberately not overriding sleep here — this exercises the real,
	// context-aware default backoff implementation.

	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel() // shutdown is already in progress before the first backoff wait

	evt := triggerEvent("http")
	evt.MaxRetries = 1

	start := time.Now()
	execution.HandleWithContext(exec, shutdownCtx, evt)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("expected the 1s backoff to be cut short by an already-cancelled shutdown context, took %v", elapsed)
	}
}
