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

// backoffBase and backoffCap bound the exponential backoff between retry
// attempts: delay = min(backoffBase * 2^attempt, backoffCap).
const (
	backoffBase = 1 * time.Second
	backoffCap  = 30 * time.Second
)

// maxAllowedRetries caps the number of retries the executor will actually
// perform, regardless of what a trigger event claims. It mirrors the
// api-gateway's validated max_retries ceiling (see
// api-gateway/internal/service/task.go); the database only enforces
// max_retries >= 0, so this is a defense-in-depth backstop against an
// out-of-range value reaching the executor and stalling the single-message
// consumer for an unbounded number of attempts.
const maxAllowedRetries = 10

// Executor consumes TaskTriggerEvents from the broker and dispatches them to
// task-type runners.
type Executor struct {
	consumer broker.Consumer
	repo     repository.ExecutionRepository
	runners  map[string]runners.TaskRunner
	logger   *slog.Logger
	sleep    func(context.Context, time.Duration)
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
	return &Executor{consumer: consumer, repo: repo, runners: r, logger: logger, sleep: sleepUnlessDone}
}

// sleepUnlessDone waits for d, but returns early if ctx is done first — so a
// SIGTERM during shutdown doesn't have to wait out a long backoff delay.
func sleepUnlessDone(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
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

// handle dispatches a single task trigger, retrying on failure up to
// evt.MaxRetries times with exponential backoff. Attempt 0 is the first try,
// not a retry. Each attempt gets its own execution_history row via
// attemptOnce; the loop stops as soon as an attempt succeeds, or after
// max_retries is exhausted (the message is then nacked to the DLQ).
func (e *Executor) handle(shutdownCtx context.Context, evt broker.TaskTriggerEvent) error {
	runner, ok := e.runners[evt.TaskType]
	if !ok {
		e.logger.Error("unsupported task type — nacking to DLQ",
			"task_id", evt.TaskID,
			"task_type", evt.TaskType,
		)
		return fmt.Errorf("%w: %s", runners.ErrUnsupportedTaskType, evt.TaskType)
	}

	maxRetries := evt.MaxRetries
	if maxRetries > maxAllowedRetries {
		e.logger.Warn("max_retries exceeds the allowed ceiling — clamping",
			"task_id", evt.TaskID,
			"max_retries", evt.MaxRetries,
			"clamped_to", maxAllowedRetries,
		)
		maxRetries = maxAllowedRetries
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			e.sleep(shutdownCtx, backoffDelay(attempt-1))
		}

		hardNack, runErr := e.attemptOnce(shutdownCtx, evt, runner, attempt)
		if runErr == nil {
			return nil
		}
		if hardNack {
			// The very first attempt's history write failed: nack immediately
			// rather than spend retries on an attempt we can't even audit.
			return runErr
		}
		lastErr = runErr
	}

	e.logger.Error("task failed after exhausting all retries — nacking to DLQ",
		"task_id", evt.TaskID,
		"task_type", evt.TaskType,
		"max_retries", maxRetries,
		"error", lastErr,
	)
	return lastErr
}

// attemptOnce runs a single attempt: it records a running row (attempt 0's
// write failure is a hard nack so the trigger isn't silently lost without any
// audit trail at all; on retry paths a write failure is logged and the
// attempt still runs, since history is best-effort there), executes the
// runner, and records the terminal status. The returned bool is true only
// when the caller should nack immediately rather than continue retrying.
func (e *Executor) attemptOnce(shutdownCtx context.Context, evt broker.TaskTriggerEvent, runner runners.TaskRunner, attempt int) (bool, error) {
	createCtx, cancelCreate := e.independentCtx()
	record, err := e.repo.Create(createCtx, repository.CreateExecutionParams{
		TaskID:     evt.TaskID,
		Status:     models.StatusRunning,
		RetryCount: attempt,
	})
	cancelCreate()
	if err != nil {
		if attempt == 0 {
			e.logger.Error("failed to record execution start — nacking",
				"task_id", evt.TaskID,
				"error", err,
			)
			return true, fmt.Errorf("record execution start: %w", err)
		}
		e.logger.Error("failed to record retry attempt start — continuing without a history row",
			"task_id", evt.TaskID,
			"attempt", attempt,
			"error", err,
		)
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

	if record != nil {
		e.recordCompletion(evt.TaskID, record.ID, params)
	}

	if runErr != nil {
		e.logger.Error("task attempt failed",
			"task_id", evt.TaskID,
			"task_type", evt.TaskType,
			"attempt", attempt,
			"max_retries", evt.MaxRetries,
			"error", runErr,
		)
		return false, runErr
	}

	e.logger.Info("task executed successfully",
		"task_id", evt.TaskID,
		"task_type", evt.TaskType,
		"attempt", attempt,
		"status_code", result.StatusCode,
	)
	return false, nil
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

// backoffDelay returns the exponential backoff delay after the given
// (zero-indexed) failed attempt: min(backoffBase * 2^attempt, backoffCap).
func backoffDelay(attempt int) time.Duration {
	if attempt > 20 { // guard against overflow; the cap dominates long before this
		return backoffCap
	}
	d := backoffBase * time.Duration(uint64(1)<<uint(attempt))
	if d > backoffCap {
		return backoffCap
	}
	return d
}
