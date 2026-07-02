package execution

import (
	"context"
	"errors"
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

// Run starts the consumer loop and blocks until SIGTERM or SIGINT, then waits
// for the consumer goroutine to drain before returning. Returns a non-nil error
// if the broker closes the channel unexpectedly.
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
		// Signal the consumer to stop cleanly. Close() sets the closing flag
		// before closing the channel, so Consume() returns nil rather than
		// ErrConsumerClosed. We then wait for the goroutine to finish draining.
		e.logger.Info("executor shutting down")
		e.consumer.Close()
		return <-errCh
	case err := <-errCh:
		if errors.Is(err, broker.ErrConsumerClosed) {
			e.logger.Error("executor: broker closed consumer channel unexpectedly")
		}
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
