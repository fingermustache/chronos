//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fingermustache/chronos/api-gateway/internal/config"
	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/api-gateway/internal/server"
	"github.com/fingermustache/chronos/api-gateway/internal/service"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/fingermustache/chronos/pkg/testutil"
	"github.com/google/uuid"
)

// newE2EServer spins up the full stack against a real DB container.
func newE2EServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	ts, _, cleanup := newE2EServerWithDB(t)
	return ts, cleanup
}

// newE2EServerWithDB is like newE2EServer but also returns the underlying DB
// handle, for tests that need to seed rows the API has no endpoint to create
// (e.g. execution_history, which only the executor writes to).
func newE2EServerWithDB(t *testing.T) (*httptest.Server, *database.DB, func()) {
	t.Helper()

	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	repo := repository.NewTaskRepository(db)
	svc := service.NewTaskService(repo)
	executionSvc := service.NewExecutionService(repo, repository.NewExecutionHistoryRepository(db))

	cfg := config.Config{Port: "0", RateLimitRPM: 1000}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ts := httptest.NewServer(server.New(cfg, logger, svc, executionSvc).Handler)
	return ts, db, ts.Close
}

func authHeader() string { return "Bearer test-token" }

func doJSON(t *testing.T, ts *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = bytes.NewBuffer(b)
	} else {
		buf = &bytes.Buffer{}
	}
	req, err := http.NewRequest(method, ts.URL+path, buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if buf.Len() > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", authHeader())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return res
}

func decodeTask(t *testing.T, res *http.Response) models.Task {
	t.Helper()
	defer res.Body.Close()
	var env struct {
		Data  models.Task `json:"data"`
		Error string      `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error != "" {
		t.Fatalf("unexpected error in response: %s", env.Error)
	}
	return env.Data
}

func validCreateBody() map[string]any {
	return map[string]any{
		"name":            "e2e-task",
		"schedule_type":   "cron",
		"schedule_config": map[string]any{"expression": "* * * * *"},
		"task_type":       "http",
		"task_config":     map[string]any{"url": "https://example.com"},
		"max_retries":     3,
		"timeout_seconds": 60,
	}
}

// --- tests ---

func TestE2E_CreateTask(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	res := doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody())
	if res.StatusCode != http.StatusCreated {
		res.Body.Close()
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	task := decodeTask(t, res)
	if task.ID == uuid.Nil {
		t.Error("expected non-nil task ID")
	}
	if task.Name != "e2e-task" {
		t.Errorf("got name %q, want %q", task.Name, "e2e-task")
	}
}

func TestE2E_GetTask(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	created := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))

	res := doJSON(t, ts, http.MethodGet, "/tasks/"+created.ID.String(), nil)
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	got := decodeTask(t, res)
	if got.ID != created.ID {
		t.Errorf("got ID %s, want %s", got.ID, created.ID)
	}
}

func TestE2E_ListTasks(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	for i := range 3 {
		body := validCreateBody()
		body["name"] = fmt.Sprintf("e2e-task-%d", i)
		doJSON(t, ts, http.MethodPost, "/tasks", body)
	}

	res := doJSON(t, ts, http.MethodGet, "/tasks", nil)
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	defer res.Body.Close()

	var env struct {
		Data struct {
			Data    []models.Task `json:"data"`
			HasMore bool          `json:"has_more"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data.Data) != 3 {
		t.Errorf("got %d tasks, want 3", len(env.Data.Data))
	}
}

func TestE2E_UpdateTask(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	created := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))

	res := doJSON(t, ts, http.MethodPut, "/tasks/"+created.ID.String(), map[string]any{
		"name": "updated-name",
	})
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	updated := decodeTask(t, res)
	if updated.Name != "updated-name" {
		t.Errorf("got name %q, want %q", updated.Name, "updated-name")
	}
	if updated.ID != created.ID {
		t.Errorf("ID changed unexpectedly")
	}
}

func TestE2E_DeleteTask(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	created := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))

	res := doJSON(t, ts, http.MethodDelete, "/tasks/"+created.ID.String(), nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}

	get := doJSON(t, ts, http.MethodGet, "/tasks/"+created.ID.String(), nil)
	get.Body.Close()
	if get.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", get.StatusCode)
	}
}

func TestE2E_ValidationError(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	res := doJSON(t, ts, http.MethodPost, "/tasks", map[string]any{
		"name":            "",
		"schedule_type":   "cron",
		"task_type":       "http",
		"timeout_seconds": 60,
	})
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", res.StatusCode)
	}
}

func TestE2E_GetTask_NotFound(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	res := doJSON(t, ts, http.MethodGet, "/tasks/"+uuid.New().String(), nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", res.StatusCode)
	}
}

func TestE2E_MissingAuthReturns401(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/tasks", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
}

// --- next_execution_time integration tests ---

func TestE2E_CreateTask_CronSetsNextExecutionTime(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "cron"
	body["schedule_config"] = map[string]any{"expression": "0 9 * * *"}

	before := time.Now().UTC()
	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", body))

	if task.NextExecutionTime == nil {
		t.Fatal("expected next_execution_time to be set for cron task, got nil")
	}
	if !task.NextExecutionTime.After(before) {
		t.Errorf("expected next_execution_time to be in the future, got %v", task.NextExecutionTime)
	}
}

func TestE2E_CreateTask_IntervalSetsNextExecutionTime(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "interval"
	body["schedule_config"] = map[string]any{"seconds": 3600}

	before := time.Now().UTC()
	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", body))

	if task.NextExecutionTime == nil {
		t.Fatal("expected next_execution_time to be set for interval task, got nil")
	}
	expected := before.Add(3600 * time.Second)
	if task.NextExecutionTime.Before(expected.Add(-2*time.Second)) || task.NextExecutionTime.After(expected.Add(2*time.Second)) {
		t.Errorf("expected next_execution_time ~%v, got %v", expected, task.NextExecutionTime)
	}
}

func TestE2E_CreateTask_OnceSetsNextExecutionTime(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "once"
	body["schedule_config"] = map[string]any{"run_at": "2030-06-15T12:00:00Z"}

	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", body))

	if task.NextExecutionTime == nil {
		t.Fatal("expected next_execution_time to be set for once task, got nil")
	}
	expected, _ := time.Parse(time.RFC3339, "2030-06-15T12:00:00Z")
	if !task.NextExecutionTime.Equal(expected) {
		t.Errorf("expected next_execution_time %v, got %v", expected, task.NextExecutionTime)
	}
}

func TestE2E_UpdateTask_RecalculatesNextExecutionTime(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	created := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))

	before := time.Now().UTC()
	updated := decodeTask(t, doJSON(t, ts, http.MethodPut, "/tasks/"+created.ID.String(), map[string]any{
		"schedule_type":   "interval",
		"schedule_config": map[string]any{"seconds": 300},
	}))

	if updated.NextExecutionTime == nil {
		t.Fatal("expected next_execution_time to be recalculated on update, got nil")
	}
	if !updated.NextExecutionTime.After(before) {
		t.Errorf("expected recalculated next_execution_time to be in the future, got %v", updated.NextExecutionTime)
	}
}

// --- schedule type integration tests ---

func TestE2E_CreateTask_IntervalSchedule(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "interval"
	body["schedule_config"] = map[string]any{"seconds": 3600}

	res := doJSON(t, ts, http.MethodPost, "/tasks", body)
	if res.StatusCode != http.StatusCreated {
		res.Body.Close()
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	task := decodeTask(t, res)
	if string(task.ScheduleType) != "interval" {
		t.Errorf("got schedule_type %q, want %q", task.ScheduleType, "interval")
	}
}

func TestE2E_CreateTask_OnceSchedule(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "once"
	body["schedule_config"] = map[string]any{"run_at": "2030-01-01T00:00:00Z"}

	res := doJSON(t, ts, http.MethodPost, "/tasks", body)
	if res.StatusCode != http.StatusCreated {
		res.Body.Close()
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	task := decodeTask(t, res)
	if string(task.ScheduleType) != "once" {
		t.Errorf("got schedule_type %q, want %q", task.ScheduleType, "once")
	}
}

func TestE2E_CreateTask_InvalidCronExpression(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "cron"
	body["schedule_config"] = map[string]any{"expression": "not-a-cron"}

	res := doJSON(t, ts, http.MethodPost, "/tasks", body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid cron expression, got %d", res.StatusCode)
	}
}

func TestE2E_CreateTask_IntervalZeroSeconds(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "interval"
	body["schedule_config"] = map[string]any{"seconds": 0}

	res := doJSON(t, ts, http.MethodPost, "/tasks", body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for zero interval seconds, got %d", res.StatusCode)
	}
}

func TestE2E_CreateTask_OncePastTimestamp(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	body := validCreateBody()
	body["schedule_type"] = "once"
	body["schedule_config"] = map[string]any{"run_at": "2020-01-01T00:00:00Z"}

	res := doJSON(t, ts, http.MethodPost, "/tasks", body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for past run_at, got %d", res.StatusCode)
	}
}

// --- execution history integration tests ---

// seedExecution inserts an execution_history row directly (only the executor
// writes to this table via the API), offsetting started_at so ordering
// across seeded rows is deterministic.
func seedExecution(t *testing.T, db *database.DB, taskID uuid.UUID, status string, startedAtOffset time.Duration) uuid.UUID {
	t.Helper()
	id := uuid.New()
	startedAt := time.Now().UTC().Add(startedAtOffset)
	completedAt := startedAt.Add(2 * time.Second)
	durationMs := 2000
	output := `{"ok":true}`
	_, err := db.Exec(`
		INSERT INTO execution_history (
			id, task_id, started_at, completed_at, status, retry_count, duration_ms, output
		) VALUES ($1, $2, $3, $4, $5, 0, $6, $7)`,
		id, taskID, startedAt, completedAt, status, durationMs, output,
	)
	if err != nil {
		t.Fatalf("seedExecution: %v", err)
	}
	return id
}

type executionsEnvelope struct {
	Data struct {
		Data       []models.ExecutionHistory `json:"data"`
		NextCursor string                    `json:"next_cursor"`
		HasMore    bool                      `json:"has_more"`
	} `json:"data"`
	Error string `json:"error"`
}

func decodeExecutions(t *testing.T, res *http.Response) executionsEnvelope {
	t.Helper()
	defer res.Body.Close()
	var env executionsEnvelope
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env
}

func TestE2E_ListExecutions_ResponseShape(t *testing.T) {
	ts, db, cleanup := newE2EServerWithDB(t)
	defer cleanup()

	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))
	execID := seedExecution(t, db, task.ID, "success", -1*time.Minute)

	res := doJSON(t, ts, http.MethodGet, "/tasks/"+task.ID.String()+"/executions", nil)
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	env := decodeExecutions(t, res)
	if len(env.Data.Data) != 1 {
		t.Fatalf("expected 1 execution record, got %d", len(env.Data.Data))
	}

	record := env.Data.Data[0]
	if record.ID != execID {
		t.Errorf("got ID %s, want %s", record.ID, execID)
	}
	if record.TaskID != task.ID {
		t.Errorf("got task_id %s, want %s", record.TaskID, task.ID)
	}
	if record.Status != models.StatusSuccess {
		t.Errorf("got status %q, want %q", record.Status, models.StatusSuccess)
	}
	if record.RetryCount != 0 {
		t.Errorf("got retry_count %d, want 0", record.RetryCount)
	}
	if record.StartedAt.IsZero() {
		t.Error("expected started_at to be set")
	}
	if record.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
	if record.DurationMs == nil || *record.DurationMs != 2000 {
		t.Errorf("expected duration_ms 2000, got %v", record.DurationMs)
	}
	if record.Output == nil || *record.Output != `{"ok":true}` {
		t.Errorf("expected output to be recorded, got %v", record.Output)
	}
	if record.ErrorMessage != nil {
		t.Errorf("expected no error_message, got %v", *record.ErrorMessage)
	}
}

func TestE2E_ListExecutions_NewestFirst(t *testing.T) {
	ts, db, cleanup := newE2EServerWithDB(t)
	defer cleanup()

	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))
	older := seedExecution(t, db, task.ID, "failed", -2*time.Minute)
	newer := seedExecution(t, db, task.ID, "success", -1*time.Minute)

	res := doJSON(t, ts, http.MethodGet, "/tasks/"+task.ID.String()+"/executions", nil)
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	env := decodeExecutions(t, res)
	if len(env.Data.Data) != 2 {
		t.Fatalf("expected 2 records, got %d", len(env.Data.Data))
	}
	if env.Data.Data[0].ID != newer {
		t.Errorf("expected newest record first, got %s", env.Data.Data[0].ID)
	}
	if env.Data.Data[1].ID != older {
		t.Errorf("expected oldest record second, got %s", env.Data.Data[1].ID)
	}
}

func TestE2E_ListExecutions_Pagination(t *testing.T) {
	ts, db, cleanup := newE2EServerWithDB(t)
	defer cleanup()

	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))
	for i := 0; i < 3; i++ {
		seedExecution(t, db, task.ID, "success", -time.Duration(3-i)*time.Minute)
	}

	firstPage := doJSON(t, ts, http.MethodGet, "/tasks/"+task.ID.String()+"/executions?limit=2", nil)
	if firstPage.StatusCode != http.StatusOK {
		firstPage.Body.Close()
		t.Fatalf("expected 200, got %d", firstPage.StatusCode)
	}
	env := decodeExecutions(t, firstPage)
	if len(env.Data.Data) != 2 {
		t.Fatalf("expected 2 records on first page, got %d", len(env.Data.Data))
	}
	if !env.Data.HasMore {
		t.Fatal("expected has_more=true on first page")
	}
	if env.Data.NextCursor == "" {
		t.Fatal("expected a non-empty next_cursor")
	}

	secondPage := doJSON(t, ts, http.MethodGet, "/tasks/"+task.ID.String()+"/executions?limit=2&cursor="+env.Data.NextCursor, nil)
	if secondPage.StatusCode != http.StatusOK {
		secondPage.Body.Close()
		t.Fatalf("expected 200, got %d", secondPage.StatusCode)
	}
	env2 := decodeExecutions(t, secondPage)
	if len(env2.Data.Data) != 1 {
		t.Fatalf("expected 1 record on second page, got %d", len(env2.Data.Data))
	}
	if env2.Data.HasMore {
		t.Error("expected has_more=false on second page")
	}
}

func TestE2E_ListExecutions_EmptyHistory(t *testing.T) {
	ts, _, cleanup := newE2EServerWithDB(t)
	defer cleanup()

	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))

	res := doJSON(t, ts, http.MethodGet, "/tasks/"+task.ID.String()+"/executions", nil)
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	env := decodeExecutions(t, res)
	if len(env.Data.Data) != 0 {
		t.Errorf("expected 0 records, got %d", len(env.Data.Data))
	}
	if env.Data.HasMore {
		t.Error("expected has_more=false for empty history")
	}
}

func TestE2E_ListExecutions_TaskNotFound(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	res := doJSON(t, ts, http.MethodGet, "/tasks/"+uuid.New().String()+"/executions", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", res.StatusCode)
	}
}

func TestE2E_ListExecutions_SoftDeletedTaskReturns404(t *testing.T) {
	ts, cleanup := newE2EServer(t)
	defer cleanup()

	task := decodeTask(t, doJSON(t, ts, http.MethodPost, "/tasks", validCreateBody()))

	del := doJSON(t, ts, http.MethodDelete, "/tasks/"+task.ID.String(), nil)
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 from delete, got %d", del.StatusCode)
	}

	res := doJSON(t, ts, http.MethodGet, "/tasks/"+task.ID.String()+"/executions", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for soft-deleted task, got %d", res.StatusCode)
	}
}
