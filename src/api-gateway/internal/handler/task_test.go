package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/fingermustache/chronos/api-gateway/internal/handler"
	"github.com/fingermustache/chronos/api-gateway/internal/service"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// mock service
// ---------------------------------------------------------------------------

type mockTaskService struct {
	createFn  func(ctx context.Context, req service.CreateTaskRequest) (*models.Task, error)
	getByIDFn func(ctx context.Context, id uuid.UUID) (*models.Task, error)
	listFn    func(ctx context.Context, req service.ListTasksRequest) (*service.ListTasksResponse, error)
	updateFn  func(ctx context.Context, id uuid.UUID, req service.UpdateTaskRequest) (*models.Task, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockTaskService) Create(ctx context.Context, req service.CreateTaskRequest) (*models.Task, error) {
	if m.createFn == nil {
		return nil, errors.New("unexpected Create call")
	}
	return m.createFn(ctx, req)
}

func (m *mockTaskService) GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	if m.getByIDFn == nil {
		return nil, errors.New("unexpected GetByID call")
	}
	return m.getByIDFn(ctx, id)
}

func (m *mockTaskService) List(ctx context.Context, req service.ListTasksRequest) (*service.ListTasksResponse, error) {
	if m.listFn == nil {
		return nil, errors.New("unexpected List call")
	}
	return m.listFn(ctx, req)
}

func (m *mockTaskService) Update(ctx context.Context, id uuid.UUID, req service.UpdateTaskRequest) (*models.Task, error) {
	if m.updateFn == nil {
		return nil, errors.New("unexpected Update call")
	}
	return m.updateFn(ctx, id, req)
}

func (m *mockTaskService) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn == nil {
		return errors.New("unexpected Delete call")
	}
	return m.deleteFn(ctx, id)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var testLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelError,
}))

func newRouter(svc service.TaskService) http.Handler {
	h := handler.NewTaskHandler(svc, testLogger)
	r := chi.NewRouter()
	r.Post("/tasks", h.Create)
	r.Get("/tasks", h.List)
	r.Get("/tasks/{id}", h.GetByID)
	r.Put("/tasks/{id}", h.Update)
	r.Delete("/tasks/{id}", h.Delete)
	return r
}

func fakeTask() *models.Task {
	return &models.Task{
		ID:             uuid.New(),
		Name:           "test-task",
		ScheduleType:   models.ScheduleTypeCron,
		ScheduleConfig: models.JSONB{"expression": "* * * * *"},
		TaskType:       models.TaskTypeHTTP,
		TaskConfig:     models.JSONB{"url": "https://example.com"},
		MaxRetries:     3,
		TimeoutSeconds: 60,
		Enabled:        true,
	}
}

func toJSON(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) (json.RawMessage, string) {
	t.Helper()
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env.Data, env.Error
}

// ---------------------------------------------------------------------------
// POST /tasks
// ---------------------------------------------------------------------------

func TestCreate_Returns201(t *testing.T) {
	task := fakeTask()
	svc := &mockTaskService{
		createFn: func(_ context.Context, _ service.CreateTaskRequest) (*models.Task, error) {
			return task, nil
		},
	}

	body := toJSON(t, map[string]any{
		"name":            "test-task",
		"schedule_type":   "cron",
		"schedule_config": map[string]any{"expression": "* * * * *"},
		"task_type":       "http",
		"task_config":     map[string]any{"url": "https://example.com"},
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", body)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("got %d, want 201", rec.Code)
	}

	data, errMsg := decodeEnvelope(t, rec)
	if errMsg != "" {
		t.Errorf("unexpected error in envelope: %s", errMsg)
	}

	var got models.Task
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("got ID %s, want %s", got.ID, task.ID)
	}
}

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	svc := &mockTaskService{}
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestCreate_ValidationError_Returns422(t *testing.T) {
	svc := &mockTaskService{
		createFn: func(_ context.Context, _ service.CreateTaskRequest) (*models.Task, error) {
			return nil, &service.ValidationError{Message: "name is required"}
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/tasks", toJSON(t, map[string]any{}))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", rec.Code)
	}

	_, errMsg := decodeEnvelope(t, rec)
	if errMsg == "" {
		t.Error("expected error message in envelope")
	}
}

func TestCreate_ServiceError_Returns500(t *testing.T) {
	svc := &mockTaskService{
		createFn: func(_ context.Context, _ service.CreateTaskRequest) (*models.Task, error) {
			return nil, errors.New("db connection lost")
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/tasks", toJSON(t, map[string]any{
		"name": "test",
	}))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /tasks/:id
// ---------------------------------------------------------------------------

func TestGetByID_Returns200(t *testing.T) {
	task := fakeTask()
	svc := &mockTaskService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return task, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+task.ID.String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}

	data, _ := decodeEnvelope(t, rec)
	var got models.Task
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("wrong task returned")
	}
}

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	svc := &mockTaskService{}
	req := httptest.NewRequest(http.MethodGet, "/tasks/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	svc := &mockTaskService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return nil, service.ErrTaskNotFound
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /tasks
// ---------------------------------------------------------------------------

func TestList_Returns200(t *testing.T) {
	svc := &mockTaskService{
		listFn: func(_ context.Context, _ service.ListTasksRequest) (*service.ListTasksResponse, error) {
			return &service.ListTasksResponse{
				Data:    []*models.Task{fakeTask(), fakeTask()},
				HasMore: false,
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestList_PassesLimitAndCursor(t *testing.T) {
	cursorID := uuid.New().String()
	var capturedReq service.ListTasksRequest

	svc := &mockTaskService{
		listFn: func(_ context.Context, req service.ListTasksRequest) (*service.ListTasksResponse, error) {
			capturedReq = req
			return &service.ListTasksResponse{Data: nil, HasMore: false}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=5&cursor="+cursorID, nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if capturedReq.Limit != 5 {
		t.Errorf("got limit %d, want 5", capturedReq.Limit)
	}
	if capturedReq.Cursor != cursorID {
		t.Errorf("got cursor %q, want %q", capturedReq.Cursor, cursorID)
	}
}

func TestList_LimitClamped(t *testing.T) {
	var capturedReq service.ListTasksRequest

	svc := &mockTaskService{
		listFn: func(_ context.Context, req service.ListTasksRequest) (*service.ListTasksResponse, error) {
			capturedReq = req
			return &service.ListTasksResponse{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=999", nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if capturedReq.Limit != 20 {
		t.Errorf("got limit %d, want 20 (clamped to default)", capturedReq.Limit)
	}
}

func TestList_InvalidCursor_Returns400(t *testing.T) {
	svc := &mockTaskService{
		listFn: func(_ context.Context, _ service.ListTasksRequest) (*service.ListTasksResponse, error) {
			return nil, &service.ValidationError{Message: "invalid cursor"}
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks?cursor=bad", nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestList_ServiceError_Returns500(t *testing.T) {
	svc := &mockTaskService{
		listFn: func(_ context.Context, _ service.ListTasksRequest) (*service.ListTasksResponse, error) {
			return nil, errors.New("db down")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// PUT /tasks/:id
// ---------------------------------------------------------------------------

func TestUpdate_Returns200(t *testing.T) {
	task := fakeTask()
	svc := &mockTaskService{
		updateFn: func(_ context.Context, _ uuid.UUID, _ service.UpdateTaskRequest) (*models.Task, error) {
			return task, nil
		},
	}

	body := toJSON(t, map[string]any{
		"name": "updated-name",
	})

	req := httptest.NewRequest(http.MethodPut, "/tasks/"+task.ID.String(), body)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}

	data, errMsg := decodeEnvelope(t, rec)
	if errMsg != "" {
		t.Errorf("unexpected error in envelope: %s", errMsg)
	}

	var got models.Task
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("wrong task returned")
	}
}

func TestUpdate_InvalidUUID_Returns400(t *testing.T) {
	svc := &mockTaskService{}
	req := httptest.NewRequest(http.MethodPut, "/tasks/not-a-uuid", toJSON(t, map[string]any{}))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestUpdate_InvalidJSON_Returns400(t *testing.T) {
	svc := &mockTaskService{}
	req := httptest.NewRequest(http.MethodPut, "/tasks/"+uuid.New().String(), bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestUpdate_NotFound_Returns404(t *testing.T) {
	svc := &mockTaskService{
		updateFn: func(_ context.Context, _ uuid.UUID, _ service.UpdateTaskRequest) (*models.Task, error) {
			return nil, service.ErrTaskNotFound
		},
	}

	req := httptest.NewRequest(http.MethodPut, "/tasks/"+uuid.New().String(), toJSON(t, map[string]any{
		"name": "new-name",
	}))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestUpdate_ValidationError_Returns422(t *testing.T) {
	svc := &mockTaskService{
		updateFn: func(_ context.Context, _ uuid.UUID, _ service.UpdateTaskRequest) (*models.Task, error) {
			return nil, &service.ValidationError{Message: "schedule_type must be one of: cron, interval, once"}
		},
	}

	req := httptest.NewRequest(http.MethodPut, "/tasks/"+uuid.New().String(), toJSON(t, map[string]any{
		"schedule_type": "daily",
	}))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", rec.Code)
	}

	_, errMsg := decodeEnvelope(t, rec)
	if errMsg == "" {
		t.Error("expected error message in envelope")
	}
}

func TestUpdate_ServiceError_Returns500(t *testing.T) {
	svc := &mockTaskService{
		updateFn: func(_ context.Context, _ uuid.UUID, _ service.UpdateTaskRequest) (*models.Task, error) {
			return nil, errors.New("db connection lost")
		},
	}

	req := httptest.NewRequest(http.MethodPut, "/tasks/"+uuid.New().String(), toJSON(t, map[string]any{
		"name": "new-name",
	}))
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// DELETE /tasks/:id
// ---------------------------------------------------------------------------

func TestDelete_Returns204(t *testing.T) {
	svc := &mockTaskService{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/tasks/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("got %d, want 204", rec.Code)
	}
}

func TestDelete_InvalidUUID_Returns400(t *testing.T) {
	svc := &mockTaskService{}

	req := httptest.NewRequest(http.MethodDelete, "/tasks/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	svc := &mockTaskService{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return service.ErrTaskNotFound
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/tasks/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestDelete_ServiceError_Returns500(t *testing.T) {
	svc := &mockTaskService{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return errors.New("db connection lost")
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/tasks/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// response envelope shape
// ---------------------------------------------------------------------------

func TestResponseEnvelope_SuccessHasDataKey(t *testing.T) {
	task := fakeTask()
	svc := &mockTaskService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return task, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+task.ID.String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["data"]; !ok {
		t.Error("expected 'data' key in success response")
	}
	if _, ok := raw["error"]; ok {
		// error key should be omitted entirely on success, not present as null
		t.Error("expected 'error' key to be absent on success")
	}
}

func TestResponseEnvelope_ErrorHasErrorKey(t *testing.T) {
	svc := &mockTaskService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return nil, service.ErrTaskNotFound
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["error"]; !ok {
		t.Error("expected 'error' key in error response")
	}
}

func TestResponseEnvelope_ContentTypeIsJSON(t *testing.T) {
	task := fakeTask()
	svc := &mockTaskService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*models.Task, error) {
			return task, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+task.ID.String(), nil)
	rec := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("got Content-Type %q, want application/json", ct)
	}
}
