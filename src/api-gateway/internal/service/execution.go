package service

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/fingermustache/chronos/api-gateway/internal/repository"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// maxOutputResponseBytes caps how much of a captured runner output the API
// returns. The executor's HTTP runner already caps what it captures at write
// time, but this is a defense-in-depth limit at the read/response layer too
// (e.g. against a future runner that doesn't cap the same way).
const maxOutputResponseBytes = 64 * 1024

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
	executions repository.ExecutionHistoryRepository
}

func NewExecutionService(tasks repository.TaskRepository, executions repository.ExecutionHistoryRepository) ExecutionService {
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
		count, err := s.executions.CountByTaskID(ctx, taskID, cursorID)
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

	for _, record := range records {
		truncateOutput(record)
	}

	return &ListExecutionsResponse{
		Data:       records,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// truncateOutput caps record.Output at maxOutputResponseBytes, cutting on a
// valid UTF-8 boundary so the response never contains a truncated multi-byte
// rune.
func truncateOutput(record *models.ExecutionHistory) {
	if record.Output == nil || len(*record.Output) <= maxOutputResponseBytes {
		return
	}

	b := (*record.Output)[:maxOutputResponseBytes]
	for len(b) > 0 {
		r, size := utf8.DecodeLastRuneInString(b)
		if r != utf8.RuneError || size != 1 {
			break
		}
		b = b[:len(b)-1]
	}
	record.Output = &b
}
