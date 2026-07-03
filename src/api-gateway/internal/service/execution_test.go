//go:build !integration

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/api-gateway/internal/service"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// --- mock execution repository ---

type mockExecutionRepo struct {
	getByTaskIDFn func(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error)
	countBeforeFn func(ctx context.Context, taskID uuid.UUID, cursorID uuid.UUID) (int, error)
}

func (m *mockExecutionRepo) GetByTaskID(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error) {
	return m.getByTaskIDFn(ctx, taskID, limit, offset)
}

func (m *mockExecutionRepo) CountBefore(ctx context.Context, taskID uuid.UUID, cursorID uuid.UUID) (int, error) {
	return m.countBeforeFn(ctx, taskID, cursorID)
}

func fakeExecution(taskID uuid.UUID) *models.ExecutionHistory {
	return &models.ExecutionHistory{
		ID:         uuid.New(),
		TaskID:     taskID,
		Status:     models.StatusSuccess,
		RetryCount: 0,
	}
}

func newExecutionService(tasks repository.TaskRepository, executions repository.ExecutionRepository) service.ExecutionService {
	return service.NewExecutionService(tasks, executions)
}

// --- tests ---

func TestListByTask_TaskNotFound(t *testing.T) {
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return nil, repository.ErrTaskNotFound
		},
	}
	svc := newExecutionService(tasks, &mockExecutionRepo{})

	_, err := svc.ListByTask(context.Background(), uuid.New(), service.ListExecutionsRequest{Limit: 20})
	if !errors.Is(err, service.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestListByTask_EmptyHistory(t *testing.T) {
	taskID := uuid.New()
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]*models.ExecutionHistory, error) {
			return nil, nil
		},
	}
	svc := newExecutionService(tasks, executions)

	resp, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 records, got %d", len(resp.Data))
	}
	if resp.HasMore {
		t.Error("expected HasMore=false for empty history")
	}
	if resp.NextCursor != "" {
		t.Error("expected empty NextCursor for empty history")
	}
}

func TestListByTask_NoCursor(t *testing.T) {
	taskID := uuid.New()
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, _, offset int) ([]*models.ExecutionHistory, error) {
			if offset != 0 {
				t.Errorf("expected offset 0, got %d", offset)
			}
			return []*models.ExecutionHistory{fakeExecution(taskID), fakeExecution(taskID)}, nil
		},
	}
	svc := newExecutionService(tasks, executions)

	resp, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.HasMore {
		t.Error("expected HasMore=false")
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 records, got %d", len(resp.Data))
	}
}

func TestListByTask_HasMore(t *testing.T) {
	taskID := uuid.New()
	records := make([]*models.ExecutionHistory, 21) // limit=20, fetch 21 -> has more
	for i := range records {
		records[i] = fakeExecution(taskID)
	}
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, limit, _ int) ([]*models.ExecutionHistory, error) {
			return records[:limit], nil
		},
	}
	svc := newExecutionService(tasks, executions)

	resp, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true")
	}
	if len(resp.Data) != 20 {
		t.Errorf("expected 20 records in response, got %d", len(resp.Data))
	}
	if resp.NextCursor == "" {
		t.Error("expected a non-empty NextCursor")
	}
}

func TestListByTask_WithCursor(t *testing.T) {
	taskID := uuid.New()
	cursorID := uuid.New()
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		countBeforeFn: func(_ context.Context, gotTaskID uuid.UUID, gotCursorID uuid.UUID) (int, error) {
			if gotTaskID != taskID {
				t.Errorf("wrong task id passed to CountBefore")
			}
			if gotCursorID != cursorID {
				t.Errorf("wrong cursor id passed to CountBefore")
			}
			return 5, nil // 5 rows before cursor -> offset=5
		},
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, _, offset int) ([]*models.ExecutionHistory, error) {
			if offset != 5 {
				t.Errorf("expected offset 5, got %d", offset)
			}
			return []*models.ExecutionHistory{fakeExecution(taskID)}, nil
		},
	}
	svc := newExecutionService(tasks, executions)

	_, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{
		Limit:  20,
		Cursor: cursorID.String(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListByTask_InvalidCursorFormat(t *testing.T) {
	taskID := uuid.New()
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	svc := newExecutionService(tasks, &mockExecutionRepo{})

	_, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{
		Limit:  20,
		Cursor: "not-a-uuid",
	})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError for malformed cursor, got %v", err)
	}
}

func TestListByTask_CursorNotFound(t *testing.T) {
	taskID := uuid.New()
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		countBeforeFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (int, error) {
			return 0, repository.ErrExecutionCursorNotFound
		},
	}
	svc := newExecutionService(tasks, executions)

	_, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{
		Limit:  20,
		Cursor: uuid.New().String(),
	})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError for a cursor that doesn't resolve, got %v", err)
	}
}

func TestListByTask_RepositoryError(t *testing.T) {
	taskID := uuid.New()
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]*models.ExecutionHistory, error) {
			return nil, errors.New("db connection lost")
		},
	}
	svc := newExecutionService(tasks, executions)

	_, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{Limit: 20})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *service.ValidationError
	if errors.As(err, &ve) {
		t.Fatal("expected a plain repository error, not a ValidationError")
	}
}
