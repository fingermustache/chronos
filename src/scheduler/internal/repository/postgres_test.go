//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/testutil"
	"github.com/fingermustache/chronos/scheduler/internal/repository"
	"github.com/google/uuid"
)

var (
	testDB   *database.DB
	testRepo repository.TaskRepository
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	db, teardown := testutil.NewTestDBWithTeardown(ctx)
	testDB = db
	testRepo = repository.NewTaskRepository(testDB)

	code := m.Run()
	teardown()
	os.Exit(code)
}

// helpers

func truncate(t *testing.T) {
	t.Helper()
	testutil.Truncate(t, testDB, "execution_history", "tasks")
}

// seedTask inserts directly into the DB — the scheduler repo is read-only
// so tests need a way to set up state without going through the scheduler.
func seedTask(t *testing.T, name string, enabled bool, nextExecution *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testDB.Exec(`
		INSERT INTO tasks (
			id, name, schedule_type, schedule_config,
			task_type, task_config, enabled, next_execution_time
		) VALUES ($1, $2, 'cron', '{}'::jsonb, 'http', '{}'::jsonb, $3, $4)`,
		id, name, enabled, nextExecution,
	)
	if err != nil {
		t.Fatalf("seedTask: %v", err)
	}
	return id
}

func ptr[T any](v T) *T { return &v }

// tests

func TestGetByID(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	id := seedTask(t, "my-task", true, nil)

	got, err := testRepo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != id {
		t.Errorf("got ID %s, want %s", got.ID, id)
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

func TestGetByID_SoftDeletedNotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	id := seedTask(t, "deleted-task", true, nil)
	_, _ = testDB.Exec("UPDATE tasks SET deleted_at = NOW() WHERE id = $1", id)

	_, err := testRepo.GetByID(ctx, id)
	if err == nil {
		t.Fatal("expected soft-deleted task to be invisible, got nil error")
	}
}

func TestGetDueTasks(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	future := time.Now().Add(10 * time.Minute)

	seedTask(t, "due-task-1", true, &past)
	seedTask(t, "due-task-2", true, &past)
	seedTask(t, "future-task", true, &future)

	tasks, err := testRepo.GetDueTasks(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d due tasks, want 2", len(tasks))
	}
}

func TestGetDueTasks_OrderedOldestFirst(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	oldest := time.Now().Add(-30 * time.Minute)
	newer := time.Now().Add(-5 * time.Minute)

	seedTask(t, "newer-task", true, &newer)
	seedTask(t, "oldest-task", true, &oldest)

	tasks, err := testRepo.GetDueTasks(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tasks[0].Name != "oldest-task" {
		t.Errorf("expected oldest task first, got %q", tasks[0].Name)
	}
}

func TestGetDueTasks_RespectsLimit(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	for i := range 5 {
		seedTask(t, fmt.Sprintf("task-%d", i), true, &past)
	}

	tasks, err := testRepo.GetDueTasks(ctx, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}
}

func TestGetDueTasks_ExcludesDisabled(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	seedTask(t, "enabled-task", true, &past)
	seedTask(t, "disabled-task", false, &past)

	tasks, err := testRepo.GetDueTasks(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].Name != "enabled-task" {
		t.Errorf("got %q, want enabled-task", tasks[0].Name)
	}
}

func TestGetDueTasks_ExcludesSoftDeleted(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	id := seedTask(t, "deleted-task", true, &past)
	_, _ = testDB.Exec("UPDATE tasks SET deleted_at = NOW() WHERE id = $1", id)

	tasks, err := testRepo.GetDueTasks(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestGetDueTasks_NullNextExecutionTimeExcluded(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	// A task with no next_execution_time set should never be returned as due.
	seedTask(t, "unscheduled-task", true, nil)

	tasks, err := testRepo.GetDueTasks(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestUpdateNextExecutionTime(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	id := seedTask(t, "my-task", true, nil)
	next := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Millisecond)

	if err := testRepo.UpdateNextExecutionTime(ctx, id, next); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task, _ := testRepo.GetByID(ctx, id)
	if task.NextExecutionTime == nil {
		t.Fatal("expected NextExecutionTime to be set, got nil")
	}
	if !task.NextExecutionTime.Equal(next) {
		t.Errorf("got %v, want %v", task.NextExecutionTime, next)
	}
}

func TestUpdateNextExecutionTime_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	err := testRepo.UpdateNextExecutionTime(ctx, uuid.New(), time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}
