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

type executionRepository struct {
	db database.Querier
}

func NewExecutionRepository(db database.Querier) ExecutionRepository {
	return &executionRepository{db: db}
}

func (r *executionRepository) Create(ctx context.Context, params CreateExecutionParams) (*models.ExecutionHistory, error) {
	query := `
		INSERT INTO execution_history (
			id, task_id, status, retry_count, metadata
		) VALUES (
			:id, :task_id, :status, :retry_count, :metadata
		)
		RETURNING *
	`

	row := &models.ExecutionHistory{
		ID:         uuid.New(),
		TaskID:     params.TaskID,
		Status:     params.Status,
		RetryCount: params.RetryCount,
		Metadata:   params.Metadata,
	}

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare create execution: %w", err)
	}
	defer stmt.Close()

	var record models.ExecutionHistory
	if err := stmt.QueryRowxContext(ctx, row).StructScan(&record); err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	return &record, nil
}

func (r *executionRepository) UpdateStatus(ctx context.Context, id uuid.UUID, params UpdateExecutionParams) error {
	query := `
		UPDATE execution_history SET
			status        = :status,
			completed_at  = :completed_at,
			error_message = :error_message,
			output        = :output,
			duration_ms   = :duration_ms
		WHERE id = :id
	`

	args := map[string]interface{}{
		"id":            id,
		"status":        params.Status,
		"completed_at":  params.CompletedAt,
		"error_message": params.ErrorMessage,
		"output":        params.Output,
		"duration_ms":   params.DurationMs,
	}

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare update execution: %w", err)
	}
	defer stmt.Close()

	result, err := stmt.ExecContext(ctx, args)
	if err != nil {
		return fmt.Errorf("failed to update execution status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm update: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("execution record not found: %s", id)
	}

	return nil
}

func (r *executionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.ExecutionHistory, error) {
	query := `
		SELECT * FROM execution_history
		WHERE id = $1
	`

	var record models.ExecutionHistory
	if err := r.db.GetContext(ctx, &record, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("execution record not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	return &record, nil
}

func (r *executionRepository) GetByTaskID(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error) {
	query := `
		SELECT * FROM execution_history
		WHERE task_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`

	var records []*models.ExecutionHistory
	if err := r.db.SelectContext(ctx, &records, query, taskID, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get executions for task %s: %w", taskID, err)
	}

	return records, nil
}
