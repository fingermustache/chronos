package repository

import (
	"context"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

type TaskRepository interface {
	Create(ctx context.Context, params CreateTaskParams) (*models.Task, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error)
	List(ctx context.Context, limit, offset int) ([]*models.Task, error)
	Update(ctx context.Context, id uuid.UUID, params UpdateTaskParams) (*models.Task, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int, error)
}

type CreateTaskParams struct {
	Name           string
	Description    *string
	ScheduleType   models.ScheduleType
	ScheduleConfig models.JSONB
	TaskType       models.TaskType
	TaskConfig     models.JSONB
	MaxRetries     int
	TimeoutSeconds int
}

type UpdateTaskParams struct {
	Name           *string
	Description    *string
	ScheduleType   *models.ScheduleType
	ScheduleConfig *models.JSONB
	TaskType       *models.TaskType
	TaskConfig     *models.JSONB
	Enabled        *bool
	MaxRetries     *int
	TimeoutSeconds *int
}
