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

func seedTask(t *testing.T, name string, scheduleType string, enabled bool, nextExecution *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testDB.Exec(`
		INSERT INTO tasks (
			id, name, schedule_type, schedule_config,
			task_type, task_config, enabled, next_execution_time
		) VALUES ($1, $2, $3, '{}'::jsonb, 'http', '{}'::jsonb, $4, $5)`,
		id, name, scheduleType, enabled, nextExecution,
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

	id := seedTask(t, "my-task", "cron", true, nil)

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

	id := seedTask(t, "deleted-task", "cron", true, nil)
	_, _ = testDB.Exec("UPDATE tasks SET deleted_at = NOW() WHERE id = $1", id)

	_, err := testRepo.GetByID(ctx, id)
	if err == nil {
		t.Fatal("expected soft-deleted task to be invisible, got nil error")
	}
}

func TestClaimDueTasks(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	future := time.Now().Add(10 * time.Minute)

	seedTask(t, "due-task-1", "cron", true, &past)
	seedTask(t, "due-task-2", "cron", true, &past)
	seedTask(t, "future-task", "cron", true, &future)

	var count int
	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		claimed, err := testRepo.ClaimDueTasks(ctx, q, 10)
		count = len(claimed)
		return err
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("got %d due tasks, want 2", count)
	}
}

func TestClaimDueTasks_OrderedOldestFirst(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	oldest := time.Now().Add(-30 * time.Minute)
	newer := time.Now().Add(-5 * time.Minute)

	seedTask(t, "newer-task", "cron", true, &newer)
	seedTask(t, "oldest-task", "cron", true, &oldest)

	var firstName string
	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		tasks, err := testRepo.ClaimDueTasks(ctx, q, 10)
		if err != nil {
			return err
		}
		if len(tasks) > 0 {
			firstName = tasks[0].Name
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if firstName != "oldest-task" {
		t.Errorf("expected oldest task first, got %q", firstName)
	}
}

func TestClaimDueTasks_RespectsLimit(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	for i := range 5 {
		seedTask(t, fmt.Sprintf("task-%d", i), "cron", true, &past)
	}

	var count int
	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		tasks, err := testRepo.ClaimDueTasks(ctx, q, 3)
		count = len(tasks)
		return err
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("got %d tasks, want 3", count)
	}
}

func TestClaimDueTasks_ExcludesDisabled(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	seedTask(t, "enabled-task", "cron", true, &past)
	seedTask(t, "disabled-task", "cron", false, &past)

	var count int
	var name string
	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		tasks, err := testRepo.ClaimDueTasks(ctx, q, 10)
		count = len(tasks)
		if count > 0 {
			name = tasks[0].Name
		}
		return err
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d tasks, want 1", count)
	}
	if name != "enabled-task" {
		t.Errorf("got %q, want enabled-task", name)
	}
}

func TestClaimDueTasks_ExcludesSoftDeleted(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	id := seedTask(t, "deleted-task", "cron", true, &past)
	_, _ = testDB.Exec("UPDATE tasks SET deleted_at = NOW() WHERE id = $1", id)

	var count int
	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		tasks, err := testRepo.ClaimDueTasks(ctx, q, 10)
		count = len(tasks)
		return err
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tasks, got %d", count)
	}
}

func TestClaimDueTasks_NullNextExecutionTimeExcluded(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	seedTask(t, "unscheduled-task", "cron", true, nil)

	var count int
	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		tasks, err := testRepo.ClaimDueTasks(ctx, q, 10)
		count = len(tasks)
		return err
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tasks, got %d", count)
	}
}

func TestClaimDueTasks_SkipsLockedRows(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	past := time.Now().Add(-5 * time.Minute)
	for i := range 4 {
		seedTask(t, fmt.Sprintf("task-%d", i), "cron", true, &past)
	}

	// First transaction claims 2 tasks and holds the lock.
	tx1, err := testDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	defer tx1.Rollback()

	claimed1, err := testRepo.ClaimDueTasks(ctx, tx1, 2)
	if err != nil {
		t.Fatalf("claim tx1: %v", err)
	}
	if len(claimed1) != 2 {
		t.Fatalf("tx1 claimed %d tasks, want 2", len(claimed1))
	}

	// Second transaction claims the remaining 2 — the locked rows are skipped.
	var count2 int
	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		tasks, err := testRepo.ClaimDueTasks(ctx, q, 10)
		count2 = len(tasks)
		return err
	}); err != nil {
		t.Fatalf("claim tx2: %v", err)
	}
	if count2 != 2 {
		t.Errorf("tx2 claimed %d tasks, want 2", count2)
	}
}

func TestUpdateNextExecutionTime(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	id := seedTask(t, "my-task", "cron", true, nil)
	next := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Millisecond)

	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		return testRepo.UpdateNextExecutionTime(ctx, q, id, next)
	}); err != nil {
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

	err := testDB.WithTx(ctx, func(q database.Querier) error {
		return testRepo.UpdateNextExecutionTime(ctx, q, uuid.New(), time.Now().Add(time.Hour))
	})
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}

func TestDisableTask(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	id := seedTask(t, "once-task", "once", true, ptr(time.Now().Add(-time.Minute)))

	if err := testDB.WithTx(ctx, func(q database.Querier) error {
		return testRepo.DisableTask(ctx, q, id)
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task, _ := testRepo.GetByID(ctx, id)
	if task.Enabled {
		t.Error("expected task to be disabled, but it is still enabled")
	}
}

func TestDisableTask_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	err := testDB.WithTx(ctx, func(q database.Querier) error {
		return testRepo.DisableTask(ctx, q, uuid.New())
	})
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}
