//go:build !integration

package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

type mockExecutionService struct {
	listByTaskFn func(ctx context.Context, taskID uuid.UUID, req service.ListExecutionsRequest) (*service.ListExecutionsResponse, error)
}

func (m *mockExecutionService) ListByTask(ctx context.Context, taskID uuid.UUID, req service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
	if m.listByTaskFn == nil {
		return nil, errors.New("unexpected ListByTask call")
	}
	return m.listByTaskFn(ctx, taskID, req)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newExecutionRouter(svc service.ExecutionService) http.Handler {
	h := handler.NewExecutionHandler(svc, testLogger)
	r := chi.NewRouter()
	r.Get("/tasks/{id}/executions", h.List)
	return r
}

func fakeExecutionRecord(taskID uuid.UUID) *models.ExecutionHistory {
	return &models.ExecutionHistory{
		ID:         uuid.New(),
		TaskID:     taskID,
		Status:     models.StatusSuccess,
		RetryCount: 0,
	}
}

// ---------------------------------------------------------------------------
// GET /tasks/:id/executions
// ---------------------------------------------------------------------------

func TestExecutionsList_Returns200(t *testing.T) {
	taskID := uuid.New()
	svc := &mockExecutionService{
		listByTaskFn: func(_ context.Context, _ uuid.UUID, _ service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
			return &service.ListExecutionsResponse{
				Data:    []*models.ExecutionHistory{fakeExecutionRecord(taskID), fakeExecutionRecord(taskID)},
				HasMore: false,
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID.String()+"/executions", nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}

	data, errMsg := decodeEnvelope(t, rec)
	if errMsg != "" {
		t.Errorf("unexpected error in envelope: %s", errMsg)
	}

	var got service.ListExecutionsResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Data) != 2 {
		t.Errorf("got %d records, want 2", len(got.Data))
	}
}

func TestExecutionsList_InvalidTaskUUID_Returns400(t *testing.T) {
	svc := &mockExecutionService{}
	req := httptest.NewRequest(http.MethodGet, "/tasks/not-a-uuid/executions", nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestExecutionsList_TaskNotFound_Returns404(t *testing.T) {
	svc := &mockExecutionService{
		listByTaskFn: func(_ context.Context, _ uuid.UUID, _ service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
			return nil, service.ErrTaskNotFound
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+uuid.New().String()+"/executions", nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestExecutionsList_InvalidCursor_Returns400(t *testing.T) {
	svc := &mockExecutionService{
		listByTaskFn: func(_ context.Context, _ uuid.UUID, _ service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
			return nil, &service.ValidationError{Message: "invalid cursor"}
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+uuid.New().String()+"/executions?cursor=bad", nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestExecutionsList_ServiceError_Returns500(t *testing.T) {
	svc := &mockExecutionService{
		listByTaskFn: func(_ context.Context, _ uuid.UUID, _ service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
			return nil, errors.New("db down")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+uuid.New().String()+"/executions", nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}
}

func TestExecutionsList_PassesLimitAndCursor(t *testing.T) {
	taskID := uuid.New()
	cursorID := uuid.New().String()
	var capturedReq service.ListExecutionsRequest
	var capturedTaskID uuid.UUID

	svc := &mockExecutionService{
		listByTaskFn: func(_ context.Context, gotTaskID uuid.UUID, req service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
			capturedTaskID = gotTaskID
			capturedReq = req
			return &service.ListExecutionsResponse{Data: nil, HasMore: false}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID.String()+"/executions?limit=5&cursor="+cursorID, nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if capturedTaskID != taskID {
		t.Errorf("got task id %s, want %s", capturedTaskID, taskID)
	}
	if capturedReq.Limit != 5 {
		t.Errorf("got limit %d, want 5", capturedReq.Limit)
	}
	if capturedReq.Cursor != cursorID {
		t.Errorf("got cursor %q, want %q", capturedReq.Cursor, cursorID)
	}
}

func TestExecutionsList_LimitClamped(t *testing.T) {
	var capturedReq service.ListExecutionsRequest

	svc := &mockExecutionService{
		listByTaskFn: func(_ context.Context, _ uuid.UUID, req service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
			capturedReq = req
			return &service.ListExecutionsResponse{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+uuid.New().String()+"/executions?limit=999", nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if capturedReq.Limit != 20 {
		t.Errorf("got limit %d, want 20 (clamped to default)", capturedReq.Limit)
	}
}

func TestExecutionsList_EmptyHistory_Returns200WithEmptyData(t *testing.T) {
	svc := &mockExecutionService{
		listByTaskFn: func(_ context.Context, _ uuid.UUID, _ service.ListExecutionsRequest) (*service.ListExecutionsResponse, error) {
			return &service.ListExecutionsResponse{Data: nil, HasMore: false}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+uuid.New().String()+"/executions", nil)
	rec := httptest.NewRecorder()
	newExecutionRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}

	data, _ := decodeEnvelope(t, rec)
	var got service.ListExecutionsResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Data) != 0 {
		t.Errorf("expected empty data, got %d records", len(got.Data))
	}
}
