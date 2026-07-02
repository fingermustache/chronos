package execution

import (
	"context"

	"github.com/fingermustache/chronos/pkg/broker"
)

var NewWithRunners = newWithRunners

func Handle(e *Executor, evt broker.TaskTriggerEvent) error {
	return e.handle(context.Background(), evt)
}

func HandleWithContext(e *Executor, shutdownCtx context.Context, evt broker.TaskTriggerEvent) error {
	return e.handle(shutdownCtx, evt)
}
