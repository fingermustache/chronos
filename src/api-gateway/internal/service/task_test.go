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

// --- mock repository ---

type mockTaskRepo struct {
	createFn      func(ctx context.Context, params repository.CreateTaskParams) (*models.Task, error)
	getByIDFn     func(ctx context.Context, id uuid.UUID) (*models.Task, error)
	listFn        func(ctx context.Context, limit, offset int) ([]*models.Task, error)
	updateFn      func(ctx context.Context, id uuid.UUID, params repository.UpdateTaskParams) (*models.Task, error)
	deleteFn      func(ctx context.Context, id uuid.UUID) error
	countFn       func(ctx context.Context) (int, error)
	countBeforeFn func(ctx context.Context, id uuid.UUID) (int, error)
}

func (m *mockTaskRepo) Create(ctx context.Context, p repository.CreateTaskParams) (*models.Task, error) {
	return m.createFn(ctx, p)
}
func (m *mockTaskRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockTaskRepo) List(ctx context.Context, limit, offset int) ([]*models.Task, error) {
	return m.listFn(ctx, limit, offset)
}
func (m *mockTaskRepo) Update(ctx context.Context, id uuid.UUID, p repository.UpdateTaskParams) (*models.Task, error) {
	return m.updateFn(ctx, id, p)
}
func (m *mockTaskRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}
func (m *mockTaskRepo) Count(ctx context.Context) (int, error) {
	return m.countFn(ctx)
}
func (m *mockTaskRepo) CountBefore(ctx context.Context, id uuid.UUID) (int, error) {
	return m.countBeforeFn(ctx, id)
}

// --- helpers ---

func validCreateReq() service.CreateTaskRequest {
	return service.CreateTaskRequest{
		Name:           "test-task",
		ScheduleType:   "cron",
		ScheduleConfig: map[string]interface{}{"expression": "* * * * *"},
		TaskType:       "http",
		TaskConfig:     map[string]interface{}{"url": "https://example.com"},
		MaxRetries:     3,
		TimeoutSeconds: 60,
	}
}

func fakeTask(id uuid.UUID) *models.Task {
	return &models.Task{
		ID:             id,
		Name:           "test-task",
		ScheduleType:   models.ScheduleTypeCron,
		TaskType:       models.TaskTypeHTTP,
		MaxRetries:     3,
		TimeoutSeconds: 60,
		Enabled:        true,
	}
}

// --- Create tests ---

func TestCreate_Success(t *testing.T) {
	id := uuid.New()
	repo := &mockTaskRepo{
		createFn: func(_ context.Context, _ repository.CreateTaskParams) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	svc := service.NewTaskService(repo)
	task, err := svc.Create(context.Background(), validCreateReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID != id {
		t.Errorf("expected id %s, got %s", id, task.ID)
	}
}

func TestCreate_MissingName(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.Name = ""
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_InvalidScheduleType(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.ScheduleType = "daily" // not a valid value
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_InvalidTaskType(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.TaskType = "websocket"
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_WhitespaceOnlyName(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.Name = "   "
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_NameTooLong(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.Name = string(make([]byte, 256))
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_NegativeMaxRetries(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.MaxRetries = -1
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_MaxRetriesExceedsLimit(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.MaxRetries = 11
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_TimeoutTooLow(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.TimeoutSeconds = -1
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_TimeoutExceedsLimit(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	req := validCreateReq()
	req.TimeoutSeconds = 601
	_, err := svc.Create(context.Background(), req)
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_AppliesDefaults(t *testing.T) {
	var captured repository.CreateTaskParams
	repo := &mockTaskRepo{
		createFn: func(_ context.Context, p repository.CreateTaskParams) (*models.Task, error) {
			captured = p
			return fakeTask(uuid.New()), nil
		},
	}
	svc := service.NewTaskService(repo)
	req := validCreateReq()
	req.MaxRetries = 0
	req.TimeoutSeconds = 0
	_, _ = svc.Create(context.Background(), req)
	if captured.MaxRetries != 3 {
		t.Errorf("expected default MaxRetries=3, got %d", captured.MaxRetries)
	}
	if captured.TimeoutSeconds != 300 {
		t.Errorf("expected default TimeoutSeconds=300, got %d", captured.TimeoutSeconds)
	}
}

// --- GetByID tests ---

func TestGetByID_Found(t *testing.T) {
	id := uuid.New()
	repo := &mockTaskRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return fakeTask(id), nil
		},
	}
	svc := service.NewTaskService(repo)
	task, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID != id {
		t.Errorf("wrong task returned")
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &mockTaskRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return nil, repository.ErrTaskNotFound
		},
	}
	svc := service.NewTaskService(repo)

	_, err := svc.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, service.ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

// --- List tests ---

func TestList_NoCursor(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	repo := &mockTaskRepo{
		listFn: func(_ context.Context, limit, offset int) ([]*models.Task, error) {
			if offset != 0 {
				t.Errorf("expected offset 0, got %d", offset)
			}
			// return limit items (no extra — has_more false)
			return []*models.Task{fakeTask(id1), fakeTask(id2)}, nil
		},
	}
	svc := service.NewTaskService(repo)
	resp, err := svc.List(context.Background(), service.ListTasksRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.HasMore {
		t.Error("expected HasMore=false")
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resp.Data))
	}
}

func TestList_HasMore(t *testing.T) {
	tasks := make([]*models.Task, 21) // limit=20, fetch 21 → has more
	for i := range tasks {
		tasks[i] = fakeTask(uuid.New())
	}
	repo := &mockTaskRepo{
		listFn: func(_ context.Context, limit, offset int) ([]*models.Task, error) {
			return tasks[:limit], nil
		},
	}
	svc := service.NewTaskService(repo)
	resp, err := svc.List(context.Background(), service.ListTasksRequest{Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.HasMore {
		t.Error("expected HasMore=true")
	}
	if len(resp.Data) != 20 {
		t.Errorf("expected 20 tasks in response, got %d", len(resp.Data))
	}
	if resp.NextCursor == "" {
		t.Error("expected a non-empty NextCursor")
	}
}

func TestList_WithCursor(t *testing.T) {
	cursorID := uuid.New()
	repo := &mockTaskRepo{
		countBeforeFn: func(_ context.Context, id uuid.UUID) (int, error) {
			if id != cursorID {
				t.Errorf("wrong cursor id passed to CountBefore")
			}
			return 5, nil // 5 rows before cursor → offset=5
		},
		listFn: func(_ context.Context, limit, offset int) ([]*models.Task, error) {
			if offset != 5 {
				t.Errorf("expected offset 5, got %d", offset)
			}
			return []*models.Task{fakeTask(uuid.New())}, nil
		},
	}
	svc := service.NewTaskService(repo)
	_, err := svc.List(context.Background(), service.ListTasksRequest{
		Limit:  20,
		Cursor: cursorID.String(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_InvalidCursor(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	_, err := svc.List(context.Background(), service.ListTasksRequest{
		Limit:  20,
		Cursor: "not-a-uuid",
	})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError for bad cursor, got %v", err)
	}
}

// --- Update tests ---

func TestUpdate_EmptyBody(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	_, err := svc.Update(context.Background(), uuid.New(), service.UpdateTaskRequest{})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError for empty update, got %T: %v", err, err)
	}
}

func TestUpdate_WhitespaceOnlyName(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	name := "   "
	_, err := svc.Update(context.Background(), uuid.New(), service.UpdateTaskRequest{Name: &name})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestUpdate_NameTooLong(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	name := string(make([]byte, 256))
	_, err := svc.Update(context.Background(), uuid.New(), service.UpdateTaskRequest{Name: &name})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestUpdate_MaxRetriesExceedsLimit(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	retries := 11
	_, err := svc.Update(context.Background(), uuid.New(), service.UpdateTaskRequest{MaxRetries: &retries})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestUpdate_TimeoutExceedsLimit(t *testing.T) {
	svc := service.NewTaskService(&mockTaskRepo{})
	timeout := 601
	_, err := svc.Update(context.Background(), uuid.New(), service.UpdateTaskRequest{TimeoutSeconds: &timeout})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

// --- Delete tests ---

func TestDelete_NotFound(t *testing.T) {
	repo := &mockTaskRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return repository.ErrTaskNotFound
		},
	}
	svc := service.NewTaskService(repo)

	err := svc.Delete(context.Background(), uuid.New())
	if !errors.Is(err, service.ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestDelete_Success(t *testing.T) {
	repo := &mockTaskRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error { return nil },
	}
	svc := service.NewTaskService(repo)
	if err := svc.Delete(context.Background(), uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
