package runners

import (
	"context"
	"errors"
)

// ErrUnsupportedTaskType is returned when the executor receives a task_type
// it has no runner for. The message should be nacked to the DLQ.
var ErrUnsupportedTaskType = errors.New("unsupported task type")

// Result holds the output of a successful or partially-successful task run.
type Result struct {
	StatusCode int
	Output     string
}

// TaskRunner executes a single task attempt.
// config is the task_config map from the trigger event.
// The caller is responsible for setting a deadline on ctx.
type TaskRunner interface {
	Run(ctx context.Context, config map[string]any) (Result, error)
}
