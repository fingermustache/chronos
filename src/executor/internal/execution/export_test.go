package execution

import (
	"context"
	"time"

	"github.com/fingermustache/chronos/pkg/broker"
)

var NewWithRunners = newWithRunners

func Handle(e *Executor, evt broker.TaskTriggerEvent) error {
	return e.handle(context.Background(), evt)
}

func HandleWithContext(e *Executor, shutdownCtx context.Context, evt broker.TaskTriggerEvent) error {
	return e.handle(shutdownCtx, evt)
}

// SetSleep overrides the executor's backoff sleep function, letting tests
// skip real delays or record the durations it was called with.
func SetSleep(e *Executor, fn func(time.Duration)) {
	e.sleep = fn
}
