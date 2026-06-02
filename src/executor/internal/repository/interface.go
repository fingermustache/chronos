package repository

import (
	"context"
	"time"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// ExecutionRepository owns the execution_history table.
// The executor is the only service that writes to it.
type ExecutionRepository interface {
	Create(ctx context.Context, params CreateExecutionParams) (*models.ExecutionHistory, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, params UpdateExecutionParams) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.ExecutionHistory, error)
	GetByTaskID(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error)
}

type CreateExecutionParams struct {
	TaskID     uuid.UUID
	Status     models.ExecutionStatus
	RetryCount int
	Metadata   models.JSONB
}

type UpdateExecutionParams struct {
	Status       models.ExecutionStatus
	CompletedAt  *time.Time
	ErrorMessage *string
	Output       *string
	DurationMs   *int
}
