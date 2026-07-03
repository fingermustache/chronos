package repository

import (
	"context"
	"errors"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// ErrExecutionCursorNotFound is returned when a pagination cursor does not
// resolve to an execution_history row for the given task.
var ErrExecutionCursorNotFound = errors.New("execution cursor not found")

// ExecutionHistoryRepository provides read-only access to execution_history.
// The executor is the only service that writes to this table.
type ExecutionHistoryRepository interface {
	GetByTaskID(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error)
	// CountByTaskID returns the number of rows for taskID that sort at or
	// before the row identified by cursorID (ordered started_at DESC) —
	// i.e. the offset to resume pagination immediately after cursorID, with
	// the cursor row itself not repeated. Returns ErrExecutionCursorNotFound
	// if cursorID doesn't resolve to a row belonging to taskID.
	CountByTaskID(ctx context.Context, taskID uuid.UUID, cursorID uuid.UUID) (int, error)
}
