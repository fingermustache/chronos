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
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/fingermustache/chronos/pkg/testutil"
	"github.com/google/uuid"
)

// newE2EServer spins up the full stack against a real DB container.
func newE2EServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	repo := repository.NewTaskRepository(db)
	svc := service.NewTaskService(repo)

	cfg := config.Config{Port: "0", RateLimitRPM: 1000}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ts := httptest.NewServer(server.New(cfg, logger, svc).Handler)
	return ts, ts.Close
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
