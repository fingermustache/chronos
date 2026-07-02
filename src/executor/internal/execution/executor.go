package execution

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/fingermustache/chronos/pkg/broker"
)

// Executor consumes TaskTriggerEvents from the broker and dispatches them to
// task-type handlers (HTTP, gRPC, etc.).
// Handler implementations are added in Phase 3.
type Executor struct {
	consumer broker.Consumer
	logger   *slog.Logger
}

func New(consumer broker.Consumer, logger *slog.Logger) *Executor {
	return &Executor{consumer: consumer, logger: logger}
}

// Run starts the consumer loop and blocks until SIGTERM or SIGINT.
func (e *Executor) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		e.logger.Info("executor consumer loop started")
		errCh <- e.consumer.Consume(e.handle)
	}()

	select {
	case <-ctx.Done():
		e.logger.Info("executor shutting down")
		return e.consumer.Close()
	case err := <-errCh:
		return err
	}
}

// handle dispatches a single trigger event.
// Full implementation: Phase 3 (task-type handlers).
func (e *Executor) handle(evt broker.TaskTriggerEvent) error {
	e.logger.Info("received trigger event",
		"task_id", evt.TaskID,
		"task_type", evt.TaskType,
		"schedule_type", evt.ScheduleType,
	)
	// TODO(Phase 3): dispatch to task-type handler (HTTP, gRPC, etc.)
	return nil
}
