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

type executionHistoryRepository struct {
	db database.Querier
}

func NewExecutionHistoryRepository(db database.Querier) ExecutionHistoryRepository {
	return &executionHistoryRepository{db: db}
}

func (r *executionHistoryRepository) GetByTaskID(ctx context.Context, taskID uuid.UUID, limit, offset int) ([]*models.ExecutionHistory, error) {
	query := `
		SELECT * FROM execution_history
		WHERE task_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`

	var records []*models.ExecutionHistory
	if err := r.db.SelectContext(ctx, &records, query, taskID, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to list executions for task %s: %w", taskID, err)
	}

	return records, nil
}

func (r *executionHistoryRepository) CountByTaskID(ctx context.Context, taskID uuid.UUID, cursorID uuid.UUID) (int, error) {
	var startedAt time.Time
	err := r.db.GetContext(ctx, &startedAt, `
		SELECT started_at FROM execution_history
		WHERE id = $1 AND task_id = $2
	`, cursorID, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrExecutionCursorNotFound
		}
		return 0, fmt.Errorf("resolve execution cursor: %w", err)
	}

	// >= (not >) so the cursor row itself counts toward the offset — otherwise
	// the cursor row would be skipped by 0 rows too few and reappear as the
	// first row of the next page. execution_history has UNIQUE(task_id,
	// started_at), so a strict started_at comparison within one task_id
	// needs no tiebreak beyond that.
	query := `
		SELECT COUNT(*) FROM execution_history
		WHERE task_id = $1 AND started_at >= $2
	`

	var count int
	if err := r.db.GetContext(ctx, &count, query, taskID, startedAt); err != nil {
		return 0, fmt.Errorf("count executions before cursor: %w", err)
	}

	return count, nil
}
