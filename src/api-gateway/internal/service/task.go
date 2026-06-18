package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
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

	params := repository.CreateTaskParams{
		Name:           req.Name,
		Description:    req.Description,
		ScheduleType:   models.ScheduleType(req.ScheduleType),
		ScheduleConfig: models.JSONB(req.ScheduleConfig),
		TaskType:       models.TaskType(req.TaskType),
		TaskConfig:     models.JSONB(req.TaskConfig),
		MaxRetries:     req.MaxRetries,
		TimeoutSeconds: req.TimeoutSeconds,
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
	if params.TimeoutSeconds == 0 {
		params.TimeoutSeconds = 300
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

	params := repository.UpdateTaskParams{
		Name:           req.Name,
		Description:    req.Description,
		ScheduleType:   scheduleType,
		ScheduleConfig: scheduleConfig,
		TaskType:       taskType,
		TaskConfig:     taskConfig,
		Enabled:        req.Enabled,
		MaxRetries:     req.MaxRetries,
		TimeoutSeconds: req.TimeoutSeconds,
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

func validateCreate(req CreateTaskRequest) error {
	if req.Name == "" {
		return &ValidationError{Message: "name is required"}
	}
	if !models.ScheduleType(req.ScheduleType).IsValid() {
		return &ValidationError{Message: "schedule_type must be one of: cron, interval, once"}
	}
	if !models.TaskType(req.TaskType).IsValid() {
		return &ValidationError{Message: "task_type must be one of: http, command, grpc"}
	}
	if req.MaxRetries < 0 {
		return &ValidationError{Message: "max_retries must be >= 0"}
	}
	if req.TimeoutSeconds < 0 {
		return &ValidationError{Message: "timeout_seconds must be >= 0"}
	}
	return nil
}

func validateUpdate(req UpdateTaskRequest) error {
	if req.ScheduleType != nil && !models.ScheduleType(*req.ScheduleType).IsValid() {
		return &ValidationError{Message: "schedule_type must be one of: cron, interval, once"}
	}
	if req.TaskType != nil && !models.TaskType(*req.TaskType).IsValid() {
		return &ValidationError{Message: "task_type must be one of: http, command, grpc"}
	}
	if req.MaxRetries != nil && *req.MaxRetries < 0 {
		return &ValidationError{Message: "max_retries must be >= 0"}
	}
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds < 0 {
		return &ValidationError{Message: "timeout_seconds must be >= 0"}
	}
	return nil
}
