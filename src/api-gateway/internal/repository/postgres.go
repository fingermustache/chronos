package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

func (r *taskRepository) Create(ctx context.Context, params CreateTaskParams) (*models.Task, error) {
	query := `
		INSERT INTO tasks (
			id, name, description, schedule_type, schedule_config,
			task_type, task_config, enabled, max_retries, timeout_seconds
		) VALUES (
			:id, :name, :description, :schedule_type, :schedule_config,
			:task_type, :task_config, true, :max_retries, :timeout_seconds
		)
		RETURNING *
	`

	row := &models.Task{
		ID:             uuid.New(),
		Name:           params.Name,
		Description:    params.Description,
		ScheduleType:   params.ScheduleType,
		ScheduleConfig: params.ScheduleConfig,
		TaskType:       params.TaskType,
		TaskConfig:     params.TaskConfig,
		MaxRetries:     params.MaxRetries,
		TimeoutSeconds: params.TimeoutSeconds,
	}

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare create task: %w", err)
	}
	defer stmt.Close()

	var task models.Task
	if err := stmt.QueryRowxContext(ctx, row).StructScan(&task); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return &task, nil
}

func (r *taskRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	query := `
	SELECT id, name, description, schedule_type, schedule_config, task_type, task_config,
	       enabled, max_retries, timeout_seconds, created_at, updated_at, deleted_at
	FROM tasks
	WHERE id = $1 AND deleted_at IS NULL
	`

	var task models.Task
	if err := r.db.GetContext(ctx, &task, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("get task by id: %w", err)
	}

	return &task, nil
}

func (r *taskRepository) List(ctx context.Context, limit, offset int) ([]*models.Task, error) {
	query := `
		SELECT * FROM tasks
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	var tasks []*models.Task
	if err := r.db.SelectContext(ctx, &tasks, query, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	return tasks, nil
}

func (r *taskRepository) Update(ctx context.Context, id uuid.UUID, params UpdateTaskParams) (*models.Task, error) {
	query := `
		UPDATE tasks SET
			name            = COALESCE(:name,            name),
			description     = COALESCE(:description,     description),
			schedule_type   = COALESCE(:schedule_type,   schedule_type),
			schedule_config = COALESCE(:schedule_config, schedule_config),
			task_type       = COALESCE(:task_type,       task_type),
			task_config     = COALESCE(:task_config,     task_config),
			enabled         = COALESCE(:enabled,         enabled),
			max_retries     = COALESCE(:max_retries,     max_retries),
			timeout_seconds = COALESCE(:timeout_seconds, timeout_seconds)
		WHERE id = :id AND deleted_at IS NULL
		RETURNING *
	`

	args := map[string]interface{}{
		"id":              id,
		"name":            params.Name,
		"description":     params.Description,
		"schedule_type":   params.ScheduleType,
		"schedule_config": params.ScheduleConfig,
		"task_type":       params.TaskType,
		"task_config":     params.TaskConfig,
		"enabled":         params.Enabled,
		"max_retries":     params.MaxRetries,
		"timeout_seconds": params.TimeoutSeconds,
	}

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare update task: %w", err)
	}
	defer stmt.Close()

	var task models.Task
	if err := r.db.GetContext(ctx, &task, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("update task: %w", err)
	}

	return &task, nil
}

func (r *taskRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
	UPDATE tasks
	SET deleted_at = NOW()
	WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete task rows affected: %w", err)
	}
	if rows == 0 {
		return ErrTaskNotFound
	}

	return nil
}

func (r *taskRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM tasks WHERE deleted_at IS NULL`

	var count int
	if err := r.db.GetContext(ctx, &count, query); err != nil {
		return 0, fmt.Errorf("failed to count tasks: %w", err)
	}

	return count, nil
}

func (r *taskRepository) CountBefore(ctx context.Context, id uuid.UUID) (int, error) {
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `
		SELECT EXISTS (
			SELECT 1 FROM tasks WHERE id = $1 AND deleted_at IS NULL
		)
	`, id); err != nil {
		return 0, fmt.Errorf("check cursor existence: %w", err)
	}
	if !exists {
		return 0, ErrTaskNotFound
	}

	query := `
	WITH cursor_task AS (
		SELECT id, created_at
		FROM tasks
		WHERE id = $1
		  AND deleted_at IS NULL
	)
	SELECT COUNT(*)
	FROM tasks t
	CROSS JOIN cursor_task c
	WHERE t.deleted_at IS NULL
	  AND (
		t.created_at > c.created_at OR
		(t.created_at = c.created_at AND t.id > c.id)
	  )
	`

	var count int
	if err := r.db.GetContext(ctx, &count, query, id); err != nil {
		return 0, fmt.Errorf("failed to count before cursor: %w", err)
	}

	return count, nil
}
