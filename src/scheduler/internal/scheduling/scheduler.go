package scheduling

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/scheduler/internal/repository"
)

// Scheduler polls the database for due tasks, claims them with SELECT FOR UPDATE
// SKIP LOCKED, publishes a trigger event per task, then advances next_execution_time.
// The poll loop and claim logic are implemented in Issue #8.
type Scheduler struct {
	repo   repository.TaskRepository
	pub    broker.Publisher
	logger *slog.Logger
}

func New(repo repository.TaskRepository, pub broker.Publisher, logger *slog.Logger) *Scheduler {
	return &Scheduler{repo: repo, pub: pub, logger: logger}
}

// Run starts the poll loop and blocks until SIGTERM or SIGINT.
func (s *Scheduler) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	s.logger.Info("scheduler poll loop started")
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler shutting down")
			return nil
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll claims due tasks and publishes trigger events.
// Full claim + next_execution_time advance logic: Issue #8.
func (s *Scheduler) poll(ctx context.Context) {
	tasks, err := s.repo.GetDueTasks(ctx, 10)
	if err != nil {
		s.logger.Error("poll: get due tasks", "error", err)
		return
	}
	for _, task := range tasks {
		evt := broker.NewTaskTriggerEvent(task)
		if err := s.pub.Publish(ctx, evt); err != nil {
			s.logger.Error("poll: publish trigger", "task_id", task.ID, "error", err)
			continue
		}
		// TODO(Issue #8): advance next_execution_time after successful publish
		// to prevent duplicate triggers on the next poll cycle.
		s.logger.Info("poll: triggered task", "task_id", task.ID)
	}
}
