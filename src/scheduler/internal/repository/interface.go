package repository

import (
	"context"
	"time"

	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// TaskRepository defines the scheduler's narrow view of the tasks table.
// It reads task definitions but only writes next_execution_time and enabled —
// it never mutates task configuration or lifecycle state.
type TaskRepository interface {
	// ClaimDueTasks selects up to limit tasks that are due and locks them with
	// SELECT FOR UPDATE SKIP LOCKED. Must be called within a transaction so that
	// concurrent schedulers claim disjoint sets of tasks.
	ClaimDueTasks(ctx context.Context, q database.Querier, limit int) ([]*models.Task, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error)
	UpdateNextExecutionTime(ctx context.Context, q database.Querier, id uuid.UUID, next time.Time) error
	// DisableTask sets enabled = false for once-type tasks after they fire.
	DisableTask(ctx context.Context, q database.Querier, id uuid.UUID) error
}
