//go:build !integration

package scheduling_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/models"
	"github.com/fingermustache/chronos/scheduler/internal/repository"
	"github.com/fingermustache/chronos/scheduler/internal/scheduling"
	"github.com/google/uuid"
)

// --- mocks ---

type mockDB struct{}

func (m *mockDB) WithTx(ctx context.Context, fn func(database.Querier) error) error {
	return fn(nil) // repo mock ignores the querier
}

type mockRepo struct {
	claimedTasks []*models.Task
	claimErr     error
	updatedID    uuid.UUID
	updatedNext  time.Time
	disabledID   uuid.UUID
	updateErr    error
	disableErr   error
	updateCalls  int
	disableCalls int
}

func (r *mockRepo) ClaimDueTasks(_ context.Context, _ database.Querier, _ int) ([]*models.Task, error) {
	return r.claimedTasks, r.claimErr
}

func (r *mockRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Task, error) {
	return nil, nil
}

func (r *mockRepo) UpdateNextExecutionTime(_ context.Context, _ database.Querier, id uuid.UUID, next time.Time) error {
	r.updateCalls++
	r.updatedID = id
	r.updatedNext = next
	return r.updateErr
}

func (r *mockRepo) DisableTask(_ context.Context, _ database.Querier, id uuid.UUID) error {
	r.disableCalls++
	r.disabledID = id
	return r.disableErr
}

type mockPublisher struct {
	published  []broker.TaskTriggerEvent
	publishErr error
}

func (p *mockPublisher) Publish(_ context.Context, evt broker.TaskTriggerEvent) error {
	if p.publishErr != nil {
		return p.publishErr
	}
	p.published = append(p.published, evt)
	return nil
}

func (p *mockPublisher) Close() error { return nil }

// --- helpers ---

func makeTask(scheduleType models.ScheduleType, cfg models.JSONB) *models.Task {
	t := time.Now().Add(-time.Minute)
	return &models.Task{
		ID:                uuid.New(),
		ScheduleType:      scheduleType,
		ScheduleConfig:    cfg,
		TaskType:          models.TaskTypeHTTP,
		TaskConfig:        models.JSONB{},
		Enabled:           true,
		NextExecutionTime: &t,
	}
}

// --- tests ---

func TestPoll_NoTasks_NoPublish(t *testing.T) {
	repo := &mockRepo{claimedTasks: nil}
	pub := &mockPublisher{}
	sched := scheduling.New(&mockDB{}, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(context.Background())

	if len(pub.published) != 0 {
		t.Errorf("expected 0 publishes, got %d", len(pub.published))
	}
}

func TestPoll_IntervalTask_PublishesAndAdvancesTime(t *testing.T) {
	task := makeTask(models.ScheduleTypeInterval, models.JSONB{"seconds": float64(60)})
	repo := &mockRepo{claimedTasks: []*models.Task{task}}
	pub := &mockPublisher{}
	sched := scheduling.New(&mockDB{}, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	before := time.Now()
	sched.PollOnce(context.Background())
	after := time.Now()

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pub.published))
	}
	if pub.published[0].TaskID != task.ID {
		t.Errorf("published wrong task ID")
	}
	if repo.updateCalls != 1 {
		t.Errorf("expected UpdateNextExecutionTime called once, got %d", repo.updateCalls)
	}
	if repo.disableCalls != 0 {
		t.Errorf("expected DisableTask not called, got %d", repo.disableCalls)
	}
	// next should be ~60 seconds from now
	expectedMin := before.Add(60 * time.Second)
	expectedMax := after.Add(60 * time.Second)
	if repo.updatedNext.Before(expectedMin) || repo.updatedNext.After(expectedMax) {
		t.Errorf("next execution time %v not in expected range [%v, %v]", repo.updatedNext, expectedMin, expectedMax)
	}
}

func TestPoll_CronTask_PublishesAndAdvancesTime(t *testing.T) {
	task := makeTask(models.ScheduleTypeCron, models.JSONB{"expression": "* * * * *"})
	repo := &mockRepo{claimedTasks: []*models.Task{task}}
	pub := &mockPublisher{}
	sched := scheduling.New(&mockDB{}, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(context.Background())

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pub.published))
	}
	if repo.updateCalls != 1 {
		t.Errorf("expected UpdateNextExecutionTime called once, got %d", repo.updateCalls)
	}
	// next should be within the next 2 minutes for "* * * * *"
	if repo.updatedNext.IsZero() {
		t.Error("expected non-zero next execution time")
	}
}

func TestPoll_OnceTask_PublishesAndDisables(t *testing.T) {
	task := makeTask(models.ScheduleTypeOnce, models.JSONB{"run_at": time.Now().Add(-time.Minute).Format(time.RFC3339)})
	repo := &mockRepo{claimedTasks: []*models.Task{task}}
	pub := &mockPublisher{}
	sched := scheduling.New(&mockDB{}, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(context.Background())

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pub.published))
	}
	if repo.disableCalls != 1 {
		t.Errorf("expected DisableTask called once, got %d", repo.disableCalls)
	}
	if repo.disabledID != task.ID {
		t.Errorf("disabled wrong task ID")
	}
	if repo.updateCalls != 0 {
		t.Errorf("expected UpdateNextExecutionTime not called, got %d", repo.updateCalls)
	}
}

func TestPoll_PublishError_RollsBack(t *testing.T) {
	task := makeTask(models.ScheduleTypeInterval, models.JSONB{"seconds": float64(60)})
	repo := &mockRepo{claimedTasks: []*models.Task{task}}
	pub := &mockPublisher{publishErr: errors.New("broker unavailable")}
	sched := scheduling.New(&mockDB{}, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(context.Background())

	// The transaction rolls back on publish error, so repo update is not committed.
	// mockDB.WithTx propagates the error from fn; because we pass nil Querier,
	// the mock repo's state reflects what was called but the tx would be rolled
	// back in a real DB.
	if repo.updateCalls != 0 {
		t.Errorf("expected UpdateNextExecutionTime not called on publish error, got %d", repo.updateCalls)
	}
}

func TestPoll_MultipleTasks_AllPublished(t *testing.T) {
	tasks := []*models.Task{
		makeTask(models.ScheduleTypeInterval, models.JSONB{"seconds": float64(30)}),
		makeTask(models.ScheduleTypeInterval, models.JSONB{"seconds": float64(60)}),
		makeTask(models.ScheduleTypeOnce, models.JSONB{"run_at": time.Now().Add(-time.Minute).Format(time.RFC3339)}),
	}
	repo := &mockRepo{claimedTasks: tasks}
	pub := &mockPublisher{}
	sched := scheduling.New(&mockDB{}, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(context.Background())

	if len(pub.published) != 3 {
		t.Errorf("expected 3 publishes, got %d", len(pub.published))
	}
	if repo.updateCalls != 2 {
		t.Errorf("expected 2 UpdateNextExecutionTime calls, got %d", repo.updateCalls)
	}
	if repo.disableCalls != 1 {
		t.Errorf("expected 1 DisableTask call, got %d", repo.disableCalls)
	}
}

func TestPoll_UsesBatchSizeFromConfig(t *testing.T) {
	// This test verifies that the batch size from config is passed to ClaimDueTasks.
	capturedLimit := 0
	repo := &capturingRepo{}
	repo.onClaim = func(limit int) { capturedLimit = limit }
	pub := &mockPublisher{}
	sched := scheduling.New(&mockDB{}, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 7,
	})

	sched.PollOnce(context.Background())

	if capturedLimit != 7 {
		t.Errorf("expected batch size 7, got %d", capturedLimit)
	}
}

type capturingRepo struct {
	mockRepo
	onClaim func(limit int)
}

func (r *capturingRepo) ClaimDueTasks(_ context.Context, _ database.Querier, limit int) ([]*models.Task, error) {
	if r.onClaim != nil {
		r.onClaim(limit)
	}
	return nil, nil
}

// Compile-time: verify mock satisfies repository.TaskRepository interface.
var _ repository.TaskRepository = (*mockRepo)(nil)
var _ repository.TaskRepository = (*capturingRepo)(nil)
