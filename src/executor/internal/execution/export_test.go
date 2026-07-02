package execution

import "github.com/fingermustache/chronos/pkg/broker"

var NewWithRunners = newWithRunners

func Handle(e *Executor, evt broker.TaskTriggerEvent) error {
	return e.handle(evt)
}
