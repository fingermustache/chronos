package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/fingermustache/chronos/pkg/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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

func createParams(overrides ...func(*repository.CreateTaskParams)) repository.CreateTaskParams {
	p := repository.CreateTaskParams{
		Name:           "test-task",
		Description:    ptr("a test task"),
		ScheduleType:   models.ScheduleTypeCron,
		ScheduleConfig: models.JSONB{"cron_expr": "0 * * * *"},
		TaskType:       models.TaskTypeHTTP,
		TaskConfig:     models.JSONB{"url": "https://example.com", "method": "GET"},
		MaxRetries:     3,
		TimeoutSeconds: 30,
	}
	for _, o := range overrides {
		o(&p)
	}
	return p
}

func ptr[T any](v T) *T { return &v }

// tests

func TestCreate(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	task, err := testRepo.Create(ctx, createParams())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID == uuid.Nil {
		t.Error("expected non-nil UUID")
	}
	if task.Name != "test-task" {
		t.Errorf("got name %q, want %q", task.Name, "test-task")
	}
	if !task.Enabled {
		t.Error("expected task to be enabled by default")
	}
	if task.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set by DB")
	}
}

func TestCreate_DuplicateNameRejected(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	_, err := testRepo.Create(ctx, createParams())
	if err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	_, err = testRepo.Create(ctx, createParams()) // same name, not deleted
	if err == nil {
		t.Fatal("expected unique constraint violation, got nil")
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			// unique_violation
			return
		}
	}

	t.Fatalf("expected unique constraint violation, got: %v", err)
}

func TestGetByID(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	created, _ := testRepo.Create(ctx, createParams())

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

func TestGetByID_SoftDeletedNotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	created, _ := testRepo.Create(ctx, createParams())
	_ = testRepo.Delete(ctx, created.ID)

	_, err := testRepo.GetByID(ctx, created.ID)
	if err == nil {
		t.Fatal("expected soft-deleted task to be invisible, got nil error")
	}
}

func TestList(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := testRepo.Create(ctx, createParams(func(p *repository.CreateTaskParams) {
			p.Name = fmt.Sprintf("task-%d", i)
		}))
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	tasks, err := testRepo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}
}

func TestList_PaginationWorks(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	for i := range 5 {
		_, _ = testRepo.Create(ctx, createParams(func(p *repository.CreateTaskParams) {
			p.Name = fmt.Sprintf("task-%d", i)
		}))
	}

	page, err := testRepo.List(ctx, 2, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page) != 2 {
		t.Errorf("got %d tasks, want 2", len(page))
	}
}

func TestList_ExcludesSoftDeleted(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	created, _ := testRepo.Create(ctx, createParams())
	_ = testRepo.Delete(ctx, created.ID)

	tasks, err := testRepo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestUpdate(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	created, _ := testRepo.Create(ctx, createParams())

	newName := "updated-task"
	enabled := false
	updated, err := testRepo.Update(ctx, created.ID, repository.UpdateTaskParams{
		Name:    &newName,
		Enabled: &enabled,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("got name %q, want %q", updated.Name, newName)
	}
	if updated.Enabled != false {
		t.Error("expected task to be disabled")
	}
	// Fields not in params should be unchanged
	if updated.TaskType != models.TaskTypeHTTP {
		t.Errorf("task_type changed unexpectedly: %s", updated.TaskType)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	newName := "ghost"
	_, err := testRepo.Update(ctx, uuid.New(), repository.UpdateTaskParams{
		Name: &newName,
	})
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}

func TestDelete(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	created, _ := testRepo.Create(ctx, createParams())

	if err := testRepo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be invisible to normal queries
	_, err := testRepo.GetByID(ctx, created.ID)
	if err == nil {
		t.Fatal("expected deleted task to be invisible")
	}

	// But should still exist in the DB with deleted_at set
	var count int
	_ = testDB.Get(&count, "SELECT COUNT(*) FROM tasks WHERE id = $1 AND deleted_at IS NOT NULL", created.ID)
	if count != 1 {
		t.Error("expected soft-deleted row to remain in DB")
	}
}

func TestDelete_NotFound(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	err := testRepo.Delete(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}

func TestCount(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	for i := range 3 {
		_, _ = testRepo.Create(ctx, createParams(func(p *repository.CreateTaskParams) {
			p.Name = fmt.Sprintf("task-%d", i)
		}))
	}

	// Soft-delete one — should not appear in count
	tasks, _ := testRepo.List(ctx, 1, 0)
	_ = testRepo.Delete(ctx, tasks[0].ID)

	count, err := testRepo.Count(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("got count %d, want 2", count)
	}
}
