package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ScheduleType string
type TaskType string

const (
	ScheduleTypeCron     ScheduleType = "cron"
	ScheduleTypeInterval ScheduleType = "interval"
	ScheduleTypeOnce     ScheduleType = "once"
)

const (
	TaskTypeHTTP    TaskType = "http"
	TaskTypeCommand TaskType = "command"
	TaskTypeGRPC    TaskType = "grpc"
)

func (s ScheduleType) IsValid() bool {
	switch s {
	case ScheduleTypeCron, ScheduleTypeInterval, ScheduleTypeOnce:
		return true
	}
	return false
}

func (t TaskType) IsValid() bool {
	switch t {
	case TaskTypeHTTP, TaskTypeCommand, TaskTypeGRPC:
		return true
	}
	return false
}

// JSONB maps to PostgreSQL JSONB columns. Defaults to an empty map (never nil)
// to satisfy NOT NULL constraints on schedule_config and task_config.
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return json.Marshal(map[string]interface{}{})
	}
	return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = JSONB{}
		return nil
	}

	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("unsupported JSONB source type: %T", value)
	}

	return json.Unmarshal(b, j)
}

// Task represents a scheduled task definition.
type Task struct {
	ID                uuid.UUID    `db:"id"                  json:"id"`
	Name              string       `db:"name"                json:"name"`
	Description       *string      `db:"description"         json:"description,omitempty"`
	ScheduleType      ScheduleType `db:"schedule_type"       json:"schedule_type"`
	ScheduleConfig    JSONB        `db:"schedule_config"     json:"schedule_config"`
	TaskType          TaskType     `db:"task_type"           json:"task_type"`
	TaskConfig        JSONB        `db:"task_config"         json:"task_config"`
	Enabled           bool         `db:"enabled"             json:"enabled"`
	MaxRetries        int          `db:"max_retries"         json:"max_retries"`
	TimeoutSeconds    int          `db:"timeout_seconds"     json:"timeout_seconds"`
	NextExecutionTime *time.Time   `db:"next_execution_time" json:"next_execution_time,omitempty"`
	CreatedAt         time.Time    `db:"created_at"          json:"created_at"`
	UpdatedAt         time.Time    `db:"updated_at"          json:"updated_at"`
	DeletedAt         *time.Time   `db:"deleted_at"          json:"deleted_at,omitempty"`
}
