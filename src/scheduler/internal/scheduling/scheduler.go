package scheduling

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/scheduler/internal/repository"
)

// DB is the minimal interface the scheduler needs for transaction management.
// Satisfied by *database.DB.
type DB interface {
	WithTx(ctx context.Context, fn func(database.Querier) error) error
}

// Config holds runtime-tunable scheduler parameters.
type Config struct {
	PollInterval   time.Duration
	ClaimBatchSize int
}

// Scheduler polls the database for due tasks, claims them with SELECT FOR
// UPDATE SKIP LOCKED, publishes a trigger event per task, then advances
// next_execution_time (or disables once tasks). All three steps run inside
// a single transaction so that a publish failure rolls back the claim.
type Scheduler struct {
	db     DB
	repo   repository.TaskRepository
	pub    broker.Publisher
	logger *slog.Logger
	cfg    Config
}

func New(db DB, repo repository.TaskRepository, pub broker.Publisher, logger *slog.Logger, cfg Config) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{db: db, repo: repo, pub: pub, logger: logger, cfg: cfg}
}

// Run starts the poll loop and blocks until SIGTERM or SIGINT.
func (s *Scheduler) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	s.logger.Info("scheduler poll loop started",
		"poll_interval", s.cfg.PollInterval,
		"claim_batch_size", s.cfg.ClaimBatchSize,
	)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler shutting down")
			return nil
		case <-ticker.C:
			s.PollOnce(ctx)
		}
	}
}

// PollOnce claims due tasks and publishes trigger events in a single
// transaction. Exported so tests can invoke it directly.
func (s *Scheduler) PollOnce(ctx context.Context) {
	err := s.db.WithTx(ctx, func(q database.Querier) error {
		tasks, err := s.repo.ClaimDueTasks(ctx, q, s.cfg.ClaimBatchSize)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		for _, task := range tasks {
			evt := broker.NewTaskTriggerEvent(task)
			if err := s.pub.Publish(ctx, evt); err != nil {
				return err // rolls back claim + any preceding updates
			}

			result, err := nextExecution(task, now)
			if err != nil {
				s.logger.Warn("scheduler: could not compute next execution, skipping advance",
					"task_id", task.ID, "error", err)
				continue
			}

			if result.disable {
				if err := s.repo.DisableTask(ctx, q, task.ID); err != nil {
					return err
				}
			} else {
				if err := s.repo.UpdateNextExecutionTime(ctx, q, task.ID, result.next); err != nil {
					return err
				}
			}

			s.logger.Info("scheduler: triggered task",
				"task_id", task.ID,
				"schedule_type", task.ScheduleType,
			)
		}
		return nil
	})

	if err != nil {
		s.logger.Error("scheduler: poll transaction failed", "error", err)
	}
}
