package models

import (
	"time"

	"github.com/google/uuid"
)

type ExecutionStatus string

const (
	StatusPending ExecutionStatus = "pending"
	StatusRunning ExecutionStatus = "running"
	StatusSuccess ExecutionStatus = "success"
	StatusFailed  ExecutionStatus = "failed"
	StatusTimeout ExecutionStatus = "timeout"
)

func (s ExecutionStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusRunning, StatusSuccess, StatusFailed, StatusTimeout:
		return true
	}
	return false
}

// ExecutionHistory represents a single task execution attempt.
type ExecutionHistory struct {
	ID           uuid.UUID       `db:"id"            json:"id"`
	TaskID       uuid.UUID       `db:"task_id"       json:"task_id"`
	StartedAt    time.Time       `db:"started_at"    json:"started_at"`
	CompletedAt  *time.Time      `db:"completed_at"  json:"completed_at,omitempty"`
	Status       ExecutionStatus `db:"status"        json:"status"`
	ErrorMessage *string         `db:"error_message" json:"error_message,omitempty"`
	Output       *string         `db:"output"        json:"output,omitempty"`
	RetryCount   int             `db:"retry_count"   json:"retry_count"`
	DurationMs   *int            `db:"duration_ms"   json:"duration_ms,omitempty"`
	Metadata     JSONB           `db:"metadata"      json:"metadata,omitempty"`
}
