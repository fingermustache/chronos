package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fingermustache/chronos/executor/internal/repository"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/fingermustache/chronos/pkg/testutil"
	"github.com/google/uuid"
)

var (
	testDB   *database.DB
	testRepo repository.ExecutionRepository
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	db, teardown := testutil.NewTestDBWithTeardown(ctx)
	testDB = db
	testRepo = repository.NewExecutionRepository(testDB) // whatever the constructor is

	code := m.Run()
	teardown()
	os.Exit(code)
}

// helpers

func truncate(t *testing.T) {
	t.Helper()
	testutil.Truncate(t, testDB, "execution_history", "tasks")
}

// seedTask inserts a minimal task row to satisfy the FK constraint.
func seedTask(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testDB.Exec(`
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

func createParams(taskID uuid.UUID) repository.CreateExecutionParams {
	return repository.CreateExecutionParams{
		TaskID:     taskID,
		Status:     models.StatusPending,
		RetryCount: 0,
		Metadata:   models.JSONB{"source": "test"},
	}
}

func ptr[T any](v T) *T { return &v }

// tests

func TestCreate(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	record, err := testRepo.Create(ctx, createParams(taskID))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.ID == uuid.Nil {
		t.Error("expected non-nil UUID")
	}
	if record.TaskID != taskID {
		t.Errorf("got task_id %s, want %s", record.TaskID, taskID)
	}
	if record.Status != models.StatusPending {
		t.Errorf("got status %q, want %q", record.Status, models.StatusPending)
	}
	if record.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set by DB")
	}
}

func TestCreate_InvalidTaskIDRejected(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	// No matching task — FK constraint should reject this.
	_, err := testRepo.Create(ctx, createParams(uuid.New()))
	if err == nil {
		t.Fatal("expected FK violation, got nil")
	}
}

func TestGetByID(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	created, _ := testRepo.Create(ctx, createParams(taskID))

	got, err := testRepo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("got ID %s, want %s", got.ID, created.ID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	_, err := testRepo.GetByID(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}

func TestUpdateStatus_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	created, _ := testRepo.Create(ctx, createParams(taskID))

	completedAt := time.Now().UTC().Truncate(time.Millisecond)
	err := testRepo.UpdateStatus(ctx, created.ID, repository.UpdateExecutionParams{
		Status:      models.StatusSuccess,
		CompletedAt: &completedAt,
		DurationMs:  ptr(120),
		Output:      ptr(`{"result":"ok"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := testRepo.GetByID(ctx, created.ID)
	if got.Status != models.StatusSuccess {
		t.Errorf("got status %q, want %q", got.Status, models.StatusSuccess)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Errorf("got completed_at %v, want %v", got.CompletedAt, completedAt)
	}
	if got.DurationMs == nil || *got.DurationMs != 120 {
		t.Errorf("got duration_ms %v, want 120", got.DurationMs)
	}
}

func TestUpdateStatus_Failed(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	created, _ := testRepo.Create(ctx, createParams(taskID))

	completedAt := time.Now().UTC().Truncate(time.Millisecond)
	err := testRepo.UpdateStatus(ctx, created.ID, repository.UpdateExecutionParams{
		Status:       models.StatusFailed,
		CompletedAt:  &completedAt,
		ErrorMessage: ptr("connection refused"),
		DurationMs:   ptr(50),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := testRepo.GetByID(ctx, created.ID)
	if got.Status != models.StatusFailed {
		t.Errorf("got status %q, want %q", got.Status, models.StatusFailed)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != "connection refused" {
		t.Errorf("got error_message %v, want 'connection refused'", got.ErrorMessage)
	}
}

func TestUpdateStatus_Timeout(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	created, _ := testRepo.Create(ctx, createParams(taskID))

	completedAt := time.Now().UTC().Truncate(time.Millisecond)
	err := testRepo.UpdateStatus(ctx, created.ID, repository.UpdateExecutionParams{
		Status:       models.StatusTimeout,
		CompletedAt:  &completedAt,
		ErrorMessage: ptr("exceeded 30s timeout"),
		DurationMs:   ptr(30000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := testRepo.GetByID(ctx, created.ID)
	if got.Status != models.StatusTimeout {
		t.Errorf("got status %q, want %q", got.Status, models.StatusTimeout)
	}
}

func TestUpdateStatus_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	err := testRepo.UpdateStatus(ctx, uuid.New(), repository.UpdateExecutionParams{
		Status: models.StatusSuccess,
	})
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}

func TestGetByTaskID(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	for range 3 {
		_, err := testRepo.Create(ctx, createParams(taskID))
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	records, err := testRepo.GetByTaskID(ctx, taskID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("got %d records, want 3", len(records))
	}
}

func TestGetByTaskID_OrderedNewestFirst(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	first, _ := testRepo.Create(ctx, createParams(taskID))
	second, _ := testRepo.Create(ctx, createParams(taskID))

	records, err := testRepo.GetByTaskID(ctx, taskID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if records[0].ID != second.ID {
		t.Errorf("expected newest record first, got %s", records[0].ID)
	}
	if records[1].ID != first.ID {
		t.Errorf("expected oldest record second, got %s", records[1].ID)
	}
}

func TestGetByTaskID_PaginationWorks(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskID := seedTask(t)
	for range 5 {
		_, _ = testRepo.Create(ctx, createParams(taskID))
	}

	page, err := testRepo.GetByTaskID(ctx, taskID, 2, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page) != 2 {
		t.Errorf("got %d records, want 2", len(page))
	}
}

func TestGetByTaskID_IsolatedToTaskID(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskA := seedTask(t)
	taskB := seedTask(t)

	_, _ = testRepo.Create(ctx, createParams(taskA))
	_, _ = testRepo.Create(ctx, createParams(taskA))
	_, _ = testRepo.Create(ctx, createParams(taskB))

	records, err := testRepo.GetByTaskID(ctx, taskA, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records for taskA, want 2", len(records))
	}
	for _, r := range records {
		if r.TaskID != taskA {
			t.Errorf("record %s belongs to wrong task %s", r.ID, r.TaskID)
		}
	}
}

func TestGetByTaskID_EmptyForUnknownTask(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	records, err := testRepo.GetByTaskID(ctx, uuid.New(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}
