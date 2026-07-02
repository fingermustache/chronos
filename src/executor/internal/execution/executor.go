package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/fingermustache/chronos/executor/internal/repository"
	"github.com/fingermustache/chronos/executor/internal/runners"
	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/google/uuid"
)

// recordWriteTimeout bounds execution-history writes. It is independent of
// the consumer's shutdown context so a SIGTERM mid-task doesn't cancel the
// write that is supposed to record what just happened.
const recordWriteTimeout = 5 * time.Second

// Executor consumes TaskTriggerEvents from the broker and dispatches them to
// task-type runners.
type Executor struct {
	consumer broker.Consumer
	repo     repository.ExecutionRepository
	runners  map[string]runners.TaskRunner
	logger   *slog.Logger
}

func New(consumer broker.Consumer, repo repository.ExecutionRepository, logger *slog.Logger) *Executor {
	return newWithRunners(consumer, repo, logger, defaultRunners())
}

func defaultRunners() map[string]runners.TaskRunner {
	return map[string]runners.TaskRunner{
		string(models.TaskTypeHTTP): runners.NewHTTPRunner(),
	}
}

func newWithRunners(consumer broker.Consumer, repo repository.ExecutionRepository, logger *slog.Logger, r map[string]runners.TaskRunner) *Executor {
	return &Executor{consumer: consumer, repo: repo, runners: r, logger: logger}
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
		errCh <- e.consumer.Consume(func(evt broker.TaskTriggerEvent) error {
			return e.handle(ctx, evt)
		})
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

func (e *Executor) handle(shutdownCtx context.Context, evt broker.TaskTriggerEvent) error {
	runner, ok := e.runners[evt.TaskType]
	if !ok {
		e.logger.Error("unsupported task type — nacking to DLQ",
			"task_id", evt.TaskID,
			"task_type", evt.TaskType,
		)
		return fmt.Errorf("%w: %s", runners.ErrUnsupportedTaskType, evt.TaskType)
	}

	createCtx, cancelCreate := e.independentCtx()
	record, err := e.repo.Create(createCtx, repository.CreateExecutionParams{
		TaskID: evt.TaskID,
		Status: models.StatusRunning,
	})
	cancelCreate()
	if err != nil {
		e.logger.Error("failed to record execution start — nacking",
			"task_id", evt.TaskID,
			"error", err,
		)
		return fmt.Errorf("record execution start: %w", err)
	}

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(shutdownCtx, time.Duration(evt.TimeoutSeconds)*time.Second)
	defer cancel()

	result, runErr := runner.Run(ctx, evt.TaskConfig)

	completedAt := time.Now()
	durationMs := int(completedAt.Sub(startedAt).Milliseconds())

	params := repository.UpdateExecutionParams{
		Status:      models.StatusSuccess,
		CompletedAt: &completedAt,
		DurationMs:  &durationMs,
		Output:      &result.Output,
	}

	if runErr != nil {
		params.Status = models.StatusFailed
		if errors.Is(runErr, context.DeadlineExceeded) {
			params.Status = models.StatusTimeout
		}
		errMsg := runErr.Error()
		params.ErrorMessage = &errMsg
	}

	e.recordCompletion(evt.TaskID, record.ID, params)

	if runErr != nil {
		e.logger.Error("task execution failed",
			"task_id", evt.TaskID,
			"task_type", evt.TaskType,
			"error", runErr,
		)
		return runErr
	}

	e.logger.Info("task executed successfully",
		"task_id", evt.TaskID,
		"task_type", evt.TaskType,
		"status_code", result.StatusCode,
	)
	return nil
}

// recordCompletion writes the terminal status for an execution attempt and
// logs, rather than fails, if the write itself fails — the task's own
// outcome (ack/nack) was already decided from the runner's result.
func (e *Executor) recordCompletion(taskID uuid.UUID, executionID uuid.UUID, params repository.UpdateExecutionParams) {
	ctx, cancel := e.independentCtx()
	defer cancel()

	if err := e.repo.UpdateStatus(ctx, executionID, params); err != nil {
		e.logger.Error("failed to record execution completion",
			"task_id", taskID,
			"execution_id", executionID,
			"status", params.Status,
			"error", err,
		)
	}
}

// independentCtx returns a context for execution-history writes that is
// bounded by recordWriteTimeout but not tied to the consumer's shutdown
// context, so a SIGTERM mid-task can't cancel the write that records what
// just happened.
func (e *Executor) independentCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), recordWriteTimeout)
}
