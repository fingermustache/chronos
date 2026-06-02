package repository

import (
	"context"
	"time"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// TaskRepository defines the scheduler's narrow view of the tasks table.
// It reads task definitions but only writes next_execution_time — it never
// mutates task configuration or lifecycle state.
type TaskRepository interface {
	GetDueTasks(ctx context.Context, limit int) ([]*models.Task, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error)
	UpdateNextExecutionTime(ctx context.Context, id uuid.UUID, next time.Time) error
}
