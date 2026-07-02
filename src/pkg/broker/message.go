package broker

import (
	"time"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// TaskTriggerEvent is published by the scheduler when a task is due and
// consumed by the executor to run the task.
type TaskTriggerEvent struct {
	TaskID         uuid.UUID      `json:"task_id"`
	ScheduleType   string         `json:"schedule_type"`
	TaskType       string         `json:"task_type"`
	TaskConfig     map[string]any `json:"task_config"`
	MaxRetries     int            `json:"max_retries"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	TriggeredAt    time.Time      `json:"triggered_at"`
}

// NewTaskTriggerEvent builds a TaskTriggerEvent from a task model.
func NewTaskTriggerEvent(task *models.Task) TaskTriggerEvent {
	return TaskTriggerEvent{
		TaskID:         task.ID,
		ScheduleType:   string(task.ScheduleType),
		TaskType:       string(task.TaskType),
		TaskConfig:     map[string]any(task.TaskConfig),
		MaxRetries:     task.MaxRetries,
		TimeoutSeconds: task.TimeoutSeconds,
		TriggeredAt:    time.Now().UTC(),
	}
}
