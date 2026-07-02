package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

var ErrTaskNotFound = errors.New("task not found")

// ValidationError is returned when request input fails validation.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// --- request/response types --------------------------------------------------

type CreateTaskRequest struct {
	Name           string                 `json:"name"`
	Description    *string                `json:"description"`
	ScheduleType   string                 `json:"schedule_type"`
	ScheduleConfig map[string]interface{} `json:"schedule_config"`
	TaskType       string                 `json:"task_type"`
	TaskConfig     map[string]interface{} `json:"task_config"`
	MaxRetries     int                    `json:"max_retries"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
}

type UpdateTaskRequest struct {
	Name           *string                 `json:"name"`
	Description    *string                 `json:"description"`
	ScheduleType   *string                 `json:"schedule_type"`
	ScheduleConfig *map[string]interface{} `json:"schedule_config"`
	TaskType       *string                 `json:"task_type"`
	TaskConfig     *map[string]interface{} `json:"task_config"`
	Enabled        *bool                   `json:"enabled"`
	MaxRetries     *int                    `json:"max_retries"`
	TimeoutSeconds *int                    `json:"timeout_seconds"`
}

type ListTasksRequest struct {
	Limit  int
	Cursor string // UUID of the last seen task — empty means start from beginning
}

type ListTasksResponse struct {
	Data       []*models.Task `json:"data"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

// --- interface ---------------------------------------------------------------

type TaskService interface {
	Create(ctx context.Context, req CreateTaskRequest) (*models.Task, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error)
	List(ctx context.Context, req ListTasksRequest) (*ListTasksResponse, error)
	Update(ctx context.Context, id uuid.UUID, req UpdateTaskRequest) (*models.Task, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// --- implementation ----------------------------------------------------------

type taskService struct {
	repo repository.TaskRepository
}

func NewTaskService(repo repository.TaskRepository) TaskService {
	return &taskService{repo: repo}
}

func (s *taskService) Create(ctx context.Context, req CreateTaskRequest) (*models.Task, error) {
	if err := validateCreate(req); err != nil {
		return nil, err
	}

	nextExec, err := calculateNextExecutionTime(models.ScheduleType(req.ScheduleType), req.ScheduleConfig, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	params := repository.CreateTaskParams{
		Name:              req.Name,
		Description:       req.Description,
		ScheduleType:      models.ScheduleType(req.ScheduleType),
		ScheduleConfig:    models.JSONB(req.ScheduleConfig),
		TaskType:          models.TaskType(req.TaskType),
		TaskConfig:        models.JSONB(req.TaskConfig),
		MaxRetries:        req.MaxRetries,
		TimeoutSeconds:    req.TimeoutSeconds,
		NextExecutionTime: &nextExec,
	}

	// Apply defaults
	if params.ScheduleConfig == nil {
		params.ScheduleConfig = models.JSONB{}
	}
	if params.TaskConfig == nil {
		params.TaskConfig = models.JSONB{}
	}
	if params.MaxRetries == 0 {
		params.MaxRetries = 3
	}

	task, err := s.repo.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	return task, nil
}

func (s *taskService) GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

func (s *taskService) List(ctx context.Context, req ListTasksRequest) (*ListTasksResponse, error) {
	// Fetch one extra to determine if there are more pages
	limit := req.Limit + 1

	offset := 0
	if req.Cursor != "" {
		cursorID, err := uuid.Parse(req.Cursor)
		if err != nil {
			return nil, &ValidationError{Message: "invalid cursor"}
		}
		// Count how many rows precede the cursor row
		count, err := s.repo.CountBefore(ctx, cursorID)
		if err != nil {
			return nil, fmt.Errorf("resolve cursor: %w", err)
		}
		offset = count
	}

	tasks, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	hasMore := len(tasks) == limit
	if hasMore {
		tasks = tasks[:req.Limit] // trim the extra sentinel row
	}

	var nextCursor string
	if hasMore && len(tasks) > 0 {
		nextCursor = tasks[len(tasks)-1].ID.String()
	}

	return &ListTasksResponse{
		Data:       tasks,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *taskService) Update(ctx context.Context, id uuid.UUID, req UpdateTaskRequest) (*models.Task, error) {
	if err := validateUpdate(req); err != nil {
		return nil, err
	}

	var scheduleType *models.ScheduleType
	if req.ScheduleType != nil {
		st := models.ScheduleType(*req.ScheduleType)
		scheduleType = &st
	}

	var taskType *models.TaskType
	if req.TaskType != nil {
		tt := models.TaskType(*req.TaskType)
		taskType = &tt
	}

	var scheduleConfig *models.JSONB
	if req.ScheduleConfig != nil {
		sc := models.JSONB(*req.ScheduleConfig)
		scheduleConfig = &sc
	}

	var taskConfig *models.JSONB
	if req.TaskConfig != nil {
		tc := models.JSONB(*req.TaskConfig)
		taskConfig = &tc
	}

	var nextExecTime *time.Time
	if req.ScheduleType != nil && req.ScheduleConfig != nil {
		t, err := calculateNextExecutionTime(models.ScheduleType(*req.ScheduleType), *req.ScheduleConfig, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		nextExecTime = &t
	}

	params := repository.UpdateTaskParams{
		Name:              req.Name,
		Description:       req.Description,
		ScheduleType:      scheduleType,
		ScheduleConfig:    scheduleConfig,
		TaskType:          taskType,
		TaskConfig:        taskConfig,
		Enabled:           req.Enabled,
		MaxRetries:        req.MaxRetries,
		TimeoutSeconds:    req.TimeoutSeconds,
		NextExecutionTime: nextExecTime,
	}

	task, err := s.repo.Update(ctx, id, params)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("update task: %w", err)
	}

	return task, nil
}

func (s *taskService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// --- validation --------------------------------------------------------------

const (
	maxNameLength      = 255
	maxRetries         = 10
	maxTimeoutSeconds  = 600
	minTimeoutSeconds  = 1
)

func validateCreate(req CreateTaskRequest) error {
	if req.Name == "" {
		return &ValidationError{Message: "name is required"}
	}
	if req.Name != strings.TrimSpace(req.Name) {
		return &ValidationError{Message: "name must not have leading or trailing whitespace"}
	}
	if len(req.Name) > maxNameLength {
		return &ValidationError{Message: fmt.Sprintf("name must be %d characters or fewer", maxNameLength)}
	}
	if !models.ScheduleType(req.ScheduleType).IsValid() {
		return &ValidationError{Message: "schedule_type must be one of: cron, interval, once"}
	}
	if err := validateScheduleConfig(models.ScheduleType(req.ScheduleType), req.ScheduleConfig); err != nil {
		return err
	}
	if !models.TaskType(req.TaskType).IsValid() {
		return &ValidationError{Message: "task_type must be one of: http, command, grpc"}
	}
	if req.MaxRetries < 0 || req.MaxRetries > maxRetries {
		return &ValidationError{Message: fmt.Sprintf("max_retries must be between 0 and %d", maxRetries)}
	}
	if req.TimeoutSeconds < minTimeoutSeconds || req.TimeoutSeconds > maxTimeoutSeconds {
		return &ValidationError{Message: fmt.Sprintf("timeout_seconds must be between %d and %d", minTimeoutSeconds, maxTimeoutSeconds)}
	}
	return nil
}

func validateUpdate(req UpdateTaskRequest) error {
	if req.Name == nil && req.Description == nil && req.ScheduleType == nil &&
		req.ScheduleConfig == nil && req.TaskType == nil && req.TaskConfig == nil &&
		req.Enabled == nil && req.MaxRetries == nil && req.TimeoutSeconds == nil {
		return &ValidationError{Message: "request body must include at least one field to update"}
	}
	if req.Name != nil {
		if *req.Name == "" {
			return &ValidationError{Message: "name must not be empty"}
		}
		if *req.Name != strings.TrimSpace(*req.Name) {
			return &ValidationError{Message: "name must not have leading or trailing whitespace"}
		}
		if len(*req.Name) > maxNameLength {
			return &ValidationError{Message: fmt.Sprintf("name must be %d characters or fewer", maxNameLength)}
		}
	}
	if req.ScheduleType != nil && !models.ScheduleType(*req.ScheduleType).IsValid() {
		return &ValidationError{Message: "schedule_type must be one of: cron, interval, once"}
	}
	if req.ScheduleConfig != nil && req.ScheduleType == nil {
		return &ValidationError{Message: "schedule_type is required when schedule_config is provided"}
	}
	if req.ScheduleType != nil && req.ScheduleConfig != nil {
		if err := validateScheduleConfig(models.ScheduleType(*req.ScheduleType), *req.ScheduleConfig); err != nil {
			return err
		}
	}
	if req.TaskType != nil && !models.TaskType(*req.TaskType).IsValid() {
		return &ValidationError{Message: "task_type must be one of: http, command, grpc"}
	}
	if req.MaxRetries != nil && (*req.MaxRetries < 0 || *req.MaxRetries > maxRetries) {
		return &ValidationError{Message: fmt.Sprintf("max_retries must be between 0 and %d", maxRetries)}
	}
	if req.TimeoutSeconds != nil && (*req.TimeoutSeconds < minTimeoutSeconds || *req.TimeoutSeconds > maxTimeoutSeconds) {
		return &ValidationError{Message: fmt.Sprintf("timeout_seconds must be between %d and %d", minTimeoutSeconds, maxTimeoutSeconds)}
	}
	return nil
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func calculateNextExecutionTime(scheduleType models.ScheduleType, cfg map[string]interface{}, now time.Time) (time.Time, error) {
	switch scheduleType {
	case models.ScheduleTypeCron:
		expr := cfg["expression"].(string)
		schedule, err := cronParser.Parse(expr)
		if err != nil {
			return time.Time{}, &ValidationError{Message: fmt.Sprintf("invalid cron expression: %s", err)}
		}
		return schedule.Next(now), nil
	case models.ScheduleTypeInterval:
		seconds := int64(cfg["seconds"].(float64))
		return now.Add(time.Duration(seconds) * time.Second), nil
	case models.ScheduleTypeOnce:
		t, err := time.Parse(time.RFC3339, cfg["run_at"].(string))
		if err != nil {
			return time.Time{}, &ValidationError{Message: "invalid run_at timestamp"}
		}
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unknown schedule type: %s", scheduleType)
}

func validateScheduleConfig(scheduleType models.ScheduleType, cfg map[string]interface{}) error {
	switch scheduleType {
	case models.ScheduleTypeCron:
		raw, ok := cfg["expression"]
		if !ok {
			return &ValidationError{Message: "schedule_config must include 'expression' for cron schedule_type"}
		}
		expr, ok := raw.(string)
		if !ok || expr == "" {
			return &ValidationError{Message: "schedule_config.expression must be a non-empty string"}
		}
		if _, err := cronParser.Parse(expr); err != nil {
			return &ValidationError{Message: fmt.Sprintf("schedule_config.expression is not a valid cron expression: %s", err)}
		}
	case models.ScheduleTypeInterval:
		raw, ok := cfg["seconds"]
		if !ok {
			return &ValidationError{Message: "schedule_config must include 'seconds' for interval schedule_type"}
		}
		var seconds float64
		switch v := raw.(type) {
		case float64:
			seconds = v
		case int:
			seconds = float64(v)
		case int64:
			seconds = float64(v)
		default:
			return &ValidationError{Message: "schedule_config.seconds must be a positive integer"}
		}
		if seconds <= 0 || seconds != float64(int64(seconds)) {
			return &ValidationError{Message: "schedule_config.seconds must be a positive integer"}
		}
	case models.ScheduleTypeOnce:
		raw, ok := cfg["run_at"]
		if !ok {
			return &ValidationError{Message: "schedule_config must include 'run_at' for once schedule_type"}
		}
		s, ok := raw.(string)
		if !ok || s == "" {
			return &ValidationError{Message: "schedule_config.run_at must be a non-empty string"}
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return &ValidationError{Message: "schedule_config.run_at must be a valid RFC3339 timestamp (e.g. 2026-01-01T00:00:00Z)"}
		}
		if !t.After(time.Now().UTC()) {
			return &ValidationError{Message: "schedule_config.run_at must be in the future"}
		}
	}
	return nil
}
