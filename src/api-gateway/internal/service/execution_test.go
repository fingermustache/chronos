//go:build !integration

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/api-gateway/internal/service"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// --- mock execution repository ---

type mockExecutionRepo struct {
	getByTaskIDFn   func(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error)
	countByTaskIDFn func(ctx context.Context, taskID uuid.UUID, cursorID uuid.UUID) (int, error)
}

func (m *mockExecutionRepo) GetByTaskID(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error) {
	return m.getByTaskIDFn(ctx, taskID, limit, offset)
}

func (m *mockExecutionRepo) CountByTaskID(ctx context.Context, taskID uuid.UUID, cursorID uuid.UUID) (int, error) {
	return m.countByTaskIDFn(ctx, taskID, cursorID)
}

func fakeExecution(taskID uuid.UUID) *models.ExecutionHistory {
	return &models.ExecutionHistory{
		ID:         uuid.New(),
		TaskID:     taskID,
		Status:     models.StatusSuccess,
		RetryCount: 0,
	}
}

func newExecutionService(tasks repository.TaskRepository, executions repository.ExecutionHistoryRepository) service.ExecutionService {
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
		countByTaskIDFn: func(_ context.Context, gotTaskID uuid.UUID, gotCursorID uuid.UUID) (int, error) {
			if gotTaskID != taskID {
				t.Errorf("wrong task id passed to CountByTaskID")
			}
			if gotCursorID != cursorID {
				t.Errorf("wrong cursor id passed to CountByTaskID")
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
		countByTaskIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (int, error) {
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

func TestListByTask_TruncatesLargeOutput(t *testing.T) {
	taskID := uuid.New()
	large := strings.Repeat("a", 100_000) // well over the 64KB cap
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]*models.ExecutionHistory, error) {
			record := fakeExecution(taskID)
			record.Output = &large
			return []*models.ExecutionHistory{record}, nil
		},
	}
	svc := newExecutionService(tasks, executions)

	resp, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.Data))
	}
	if resp.Data[0].Output == nil {
		t.Fatal("expected output to still be present, just truncated")
	}
	if len(*resp.Data[0].Output) > 64*1024 {
		t.Errorf("expected output truncated to at most 64KB, got %d bytes", len(*resp.Data[0].Output))
	}
}

func TestListByTask_SmallOutputNotTruncated(t *testing.T) {
	taskID := uuid.New()
	small := `{"ok":true}`
	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]*models.ExecutionHistory, error) {
			record := fakeExecution(taskID)
			record.Output = &small
			return []*models.ExecutionHistory{record}, nil
		},
	}
	svc := newExecutionService(tasks, executions)

	resp, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data[0].Output == nil || *resp.Data[0].Output != small {
		t.Errorf("expected output unchanged for small output, got %v", resp.Data[0].Output)
	}
}

func TestListByTask_TruncationCutsOnValidUTF8Boundary(t *testing.T) {
	taskID := uuid.New()

	// Place a multi-byte rune straddling the exact 64KB cut point: 65535
	// ASCII bytes, then a 2-byte rune (bytes 65535-65536), then more content.
	// A naive byte-slice truncation at 65536 would split "é" in half and
	// leave an invalid UTF-8 tail.
	prefix := strings.Repeat("a", 64*1024-1)
	withSplitRune := prefix + "é" + strings.Repeat("b", 1000)

	tasks := &mockTaskRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	executions := &mockExecutionRepo{
		getByTaskIDFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]*models.ExecutionHistory, error) {
			record := fakeExecution(taskID)
			record.Output = &withSplitRune
			return []*models.ExecutionHistory{record}, nil
		},
	}
	svc := newExecutionService(tasks, executions)

	resp, err := svc.ListByTask(context.Background(), taskID, service.ListExecutionsRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data[0].Output == nil {
		t.Fatal("expected output to still be present, just truncated")
	}
	got := *resp.Data[0].Output
	if len(got) > 64*1024 {
		t.Errorf("expected output truncated to at most 64KB, got %d bytes", len(got))
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncated output is not valid UTF-8: %q", got)
	}
	if got != prefix {
		t.Errorf("expected the split rune to be dropped entirely, leaving just the ASCII prefix (%d bytes), got %d bytes", len(prefix), len(got))
	}
}
