//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// seedExecution inserts an execution_history row directly, offsetting
// started_at by startedAtOffset so ordering across seeded rows is
// deterministic without relying on real-time gaps between inserts.
func seedExecution(t *testing.T, taskID uuid.UUID, status models.ExecutionStatus, startedAtOffset time.Duration) uuid.UUID {
	t.Helper()
	id := uuid.New()
	startedAt := time.Now().UTC().Add(startedAtOffset)
	_, err := testDB.Exec(`
		INSERT INTO execution_history (id, task_id, started_at, status, retry_count)
		VALUES ($1, $2, $3, $4, 0)`,
		id, taskID, startedAt, status,
	)
	if err != nil {
		t.Fatalf("seedExecution: %v", err)
	}
	return id
}

func TestExecutionRepository_GetByTaskID_OrderedNewestFirst(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	task, err := testRepo.Create(ctx, createParams())
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	execRepo := repository.NewExecutionHistoryRepository(testDB)

	older := seedExecution(t, task.ID, models.StatusSuccess, -2*time.Minute)
	newer := seedExecution(t, task.ID, models.StatusFailed, -1*time.Minute)

	records, err := execRepo.GetByTaskID(ctx, task.ID, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].ID != newer {
		t.Errorf("expected newest record first, got %s", records[0].ID)
	}
	if records[1].ID != older {
		t.Errorf("expected oldest record second, got %s", records[1].ID)
	}
}

func TestExecutionRepository_GetByTaskID_EmptyForUnknownTask(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	execRepo := repository.NewExecutionHistoryRepository(testDB)

	records, err := execRepo.GetByTaskID(ctx, uuid.New(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestExecutionRepository_CountByTaskID_ResolvesCursor(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	task, err := testRepo.Create(ctx, createParams())
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	execRepo := repository.NewExecutionHistoryRepository(testDB)

	seedExecution(t, task.ID, models.StatusSuccess, -3*time.Minute) // oldest
	cursor := seedExecution(t, task.ID, models.StatusSuccess, -2*time.Minute)
	seedExecution(t, task.ID, models.StatusSuccess, -1*time.Minute) // newest

	count, err := execRepo.CountByTaskID(ctx, task.ID, cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The cursor row itself counts toward the offset (newest + the cursor
	// row = 2), so the next page resumes right after it without repeating it.
	if count != 2 {
		t.Errorf("expected offset 2 (newest row + the cursor row itself), got %d", count)
	}
}

func TestExecutionRepository_CountByTaskID_InvalidCursorReturnsNotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	task, err := testRepo.Create(ctx, createParams())
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	execRepo := repository.NewExecutionHistoryRepository(testDB)

	_, err = execRepo.CountByTaskID(ctx, task.ID, uuid.New())
	if err != repository.ErrExecutionCursorNotFound {
		t.Errorf("expected ErrExecutionCursorNotFound, got %v", err)
	}
}

func TestExecutionRepository_CountByTaskID_CursorFromOtherTaskReturnsNotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	taskA, err := testRepo.Create(ctx, createParams())
	if err != nil {
		t.Fatalf("seed task A: %v", err)
	}
	taskB, err := testRepo.Create(ctx, createParams(func(p *repository.CreateTaskParams) { p.Name = "task-b" }))
	if err != nil {
		t.Fatalf("seed task B: %v", err)
	}

	execRepo := repository.NewExecutionHistoryRepository(testDB)

	cursorFromB := seedExecution(t, taskB.ID, models.StatusSuccess, -1*time.Minute)

	_, err = execRepo.CountByTaskID(ctx, taskA.ID, cursorFromB)
	if err != repository.ErrExecutionCursorNotFound {
		t.Errorf("expected ErrExecutionCursorNotFound for a cursor scoped to a different task, got %v", err)
	}
}
