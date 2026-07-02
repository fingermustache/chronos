package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/fingermustache/chronos/executor/internal/runners"
	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/models"
)

// Executor consumes TaskTriggerEvents from the broker and dispatches them to
// task-type runners.
type Executor struct {
	consumer broker.Consumer
	runners  map[string]runners.TaskRunner
	logger   *slog.Logger
}

func New(consumer broker.Consumer, logger *slog.Logger) *Executor {
	return newWithRunners(consumer, logger, defaultRunners())
}

func defaultRunners() map[string]runners.TaskRunner {
	return map[string]runners.TaskRunner{
		string(models.TaskTypeHTTP): runners.NewHTTPRunner(),
	}
}

func newWithRunners(consumer broker.Consumer, logger *slog.Logger, r map[string]runners.TaskRunner) *Executor {
	return &Executor{consumer: consumer, runners: r, logger: logger}
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

func (e *Executor) handle(evt broker.TaskTriggerEvent) error {
	runner, ok := e.runners[evt.TaskType]
	if !ok {
		e.logger.Error("unsupported task type — nacking to DLQ",
			"task_id", evt.TaskID,
			"task_type", evt.TaskType,
		)
		return fmt.Errorf("%w: %s", runners.ErrUnsupportedTaskType, evt.TaskType)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(evt.TimeoutSeconds)*time.Second)
	defer cancel()

	result, err := runner.Run(ctx, evt.TaskConfig)
	if err != nil {
		e.logger.Error("task execution failed",
			"task_id", evt.TaskID,
			"task_type", evt.TaskType,
			"error", err,
		)
		return err
	}

	e.logger.Info("task executed successfully",
		"task_id", evt.TaskID,
		"task_type", evt.TaskType,
		"status_code", result.StatusCode,
	)
	return nil
}
