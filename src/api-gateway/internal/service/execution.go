package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

type ListExecutionsRequest struct {
	Limit  int
	Cursor string // UUID of the last seen execution row — empty means start from beginning
}

type ListExecutionsResponse struct {
	Data       []*models.ExecutionHistory `json:"data"`
	NextCursor string                     `json:"next_cursor,omitempty"`
	HasMore    bool                       `json:"has_more"`
}

// ExecutionService reads execution_history for a task. The executor is the
// only service that writes to that table.
type ExecutionService interface {
	ListByTask(ctx context.Context, taskID uuid.UUID, req ListExecutionsRequest) (*ListExecutionsResponse, error)
}

type executionService struct {
	tasks      repository.TaskRepository
	executions repository.ExecutionRepository
}

func NewExecutionService(tasks repository.TaskRepository, executions repository.ExecutionRepository) ExecutionService {
	return &executionService{tasks: tasks, executions: executions}
}

func (s *executionService) ListByTask(ctx context.Context, taskID uuid.UUID, req ListExecutionsRequest) (*ListExecutionsResponse, error) {
	// GetByID already excludes soft-deleted tasks, so a soft-deleted task_id
	// correctly reports not-found here too.
	if _, err := s.tasks.GetByID(ctx, taskID); err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("get task: %w", err)
	}

	// Fetch one extra to determine if there are more pages
	limit := req.Limit + 1

	offset := 0
	if req.Cursor != "" {
		cursorID, err := uuid.Parse(req.Cursor)
		if err != nil {
			return nil, &ValidationError{Message: "invalid cursor"}
		}
		count, err := s.executions.CountBefore(ctx, taskID, cursorID)
		if err != nil {
			if errors.Is(err, repository.ErrExecutionCursorNotFound) {
				return nil, &ValidationError{Message: "invalid cursor"}
			}
			return nil, fmt.Errorf("resolve cursor: %w", err)
		}
		offset = count
	}

	records, err := s.executions.GetByTaskID(ctx, taskID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}

	hasMore := len(records) == limit
	if hasMore {
		records = records[:req.Limit] // trim the extra sentinel row
	}

	var nextCursor string
	if hasMore && len(records) > 0 {
		nextCursor = records[len(records)-1].ID.String()
	}

	return &ListExecutionsResponse{
		Data:       records,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
