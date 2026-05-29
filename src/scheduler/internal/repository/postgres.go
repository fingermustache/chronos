package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

type taskRepository struct {
	db database.Querier
}

func NewTaskRepository(db database.Querier) TaskRepository {
	return &taskRepository{db: db}
}

// GetDueTasks fetches enabled tasks whose next_execution_time is due.
// Ordered by next_execution_time ASC so the most overdue tasks are handled first.
// limit prevents a single poll from overwhelming the executor.
func (r *taskRepository) GetDueTasks(ctx context.Context, limit int) ([]*models.Task, error) {
	query := `
		SELECT * FROM tasks
		WHERE enabled = true
		  AND deleted_at IS NULL
		  AND next_execution_time <= NOW()
		ORDER BY next_execution_time ASC
		LIMIT $1
	`

	var tasks []*models.Task
	if err := r.db.SelectContext(ctx, &tasks, query, limit); err != nil {
		return nil, fmt.Errorf("failed to get due tasks: %w", err)
	}

	return tasks, nil
}

func (r *taskRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	query := `
		SELECT * FROM tasks
		WHERE id = $1 AND deleted_at IS NULL
	`

	var task models.Task
	if err := r.db.GetContext(ctx, &task, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("task not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return &task, nil
}

func (r *taskRepository) UpdateNextExecutionTime(ctx context.Context, id uuid.UUID, next time.Time) error {
	query := `
		UPDATE tasks SET next_execution_time = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, next, id)
	if err != nil {
		return fmt.Errorf("failed to update next execution time: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm update: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}
