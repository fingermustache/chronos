package repository

import (
	"context"
	"time"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// TaskRepository is read-only. The scheduler never mutates task definitions —
// it only reads due tasks and writes back the next execution time.
type TaskRepository interface {
	GetDueTasks(ctx context.Context, limit int) ([]*models.Task, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error)
	UpdateNextExecutionTime(ctx context.Context, id uuid.UUID, next time.Time) error
}
