//go:build integration

package scheduling_test

import (
	"context"
	"testing"
	"time"

	"github.com/fingermustache/chronos/pkg/broker"
	"github.com/fingermustache/chronos/pkg/database"
	"github.com/fingermustache/chronos/pkg/testutil"
	schedulerrepo "github.com/fingermustache/chronos/scheduler/internal/repository"
	"github.com/fingermustache/chronos/scheduler/internal/scheduling"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

// seedTask inserts a task directly into the DB for test setup.
func seedTask(t *testing.T, db *database.DB, name, scheduleType, scheduleConfigJSON string, enabled bool, nextExec time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.Exec(`
		INSERT INTO tasks (id, name, schedule_type, schedule_config, task_type, task_config, enabled, next_execution_time)
		VALUES ($1, $2, $3, $4::jsonb, 'http', '{"url":"http://localhost"}'::jsonb, $5, $6)`,
		id, name, scheduleType, scheduleConfigJSON, enabled, nextExec,
	)
	if err != nil {
		t.Fatalf("seedTask: %v", err)
	}
	return id
}

// newTestBrokerPublisher spins up a RabbitMQ container and returns a publisher
// that also records what it publishes, plus a cleanup func.
func newTestBrokerPublisher(t *testing.T) (*recordingPublisher, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := rabbitmq.Run(ctx, "rabbitmq:3.13-management-alpine")
	if err != nil {
		t.Fatalf("start rabbitmq: %v", err)
	}

	amqpURL, err := container.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("get amqp url: %v", err)
	}

	conn, err := broker.NewConnection(broker.Config{URL: amqpURL})
	if err != nil {
		t.Fatalf("broker connect: %v", err)
	}
	if err := broker.SetupTopology(conn); err != nil {
		t.Fatalf("setup topology: %v", err)
	}
	inner, err := broker.NewPublisher(conn)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	cleanup := func() {
		inner.Close()
		conn.Close()
		container.Terminate(ctx)
	}

	return &recordingPublisher{Publisher: inner}, cleanup
}

type recordingPublisher struct {
	broker.Publisher
	published []broker.TaskTriggerEvent
}

func (p *recordingPublisher) Publish(ctx context.Context, evt broker.TaskTriggerEvent) error {
	if err := p.Publisher.Publish(ctx, evt); err != nil {
		return err
	}
	p.published = append(p.published, evt)
	return nil
}

// tests

func TestIntegration_PollOnce_IntervalTask_AdvancesTime(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	pub, cleanup := newTestBrokerPublisher(t)
	defer cleanup()

	taskID := seedTask(t, db, "interval-task", "interval", `{"seconds": 300}`, true, time.Now().Add(-time.Minute))

	repo := schedulerrepo.NewTaskRepository(db)
	sched := scheduling.New(db, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(ctx)

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].TaskID != taskID {
		t.Errorf("published wrong task ID: got %v, want %v", pub.published[0].TaskID, taskID)
	}

	var next time.Time
	if err := db.QueryRowx("SELECT next_execution_time FROM tasks WHERE id = $1", taskID).Scan(&next); err != nil {
		t.Fatalf("scan next_execution_time: %v", err)
	}
	if !next.After(time.Now().Add(290 * time.Second)) {
		t.Errorf("next_execution_time %v should be ~300s from now", next)
	}
}

func TestIntegration_PollOnce_OnceTask_DisablesAfterFire(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	pub, cleanup := newTestBrokerPublisher(t)
	defer cleanup()

	runAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	taskID := seedTask(t, db, "once-task", "once",
		`{"run_at": "`+runAt+`"}`, true, time.Now().Add(-time.Minute))

	repo := schedulerrepo.NewTaskRepository(db)
	sched := scheduling.New(db, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(ctx)

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}

	var enabled bool
	if err := db.QueryRowx("SELECT enabled FROM tasks WHERE id = $1", taskID).Scan(&enabled); err != nil {
		t.Fatalf("scan enabled: %v", err)
	}
	if enabled {
		t.Error("once task should be disabled after firing, got enabled=true")
	}

	// Second poll must not re-fire.
	pub.published = nil
	sched.PollOnce(ctx)
	if len(pub.published) != 0 {
		t.Errorf("expected 0 publishes on second poll, got %d", len(pub.published))
	}
}

func TestIntegration_PollOnce_CronTask_AdvancesTime(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	pub, cleanup := newTestBrokerPublisher(t)
	defer cleanup()

	taskID := seedTask(t, db, "cron-task", "cron", `{"expression": "* * * * *"}`, true, time.Now().Add(-time.Minute))

	repo := schedulerrepo.NewTaskRepository(db)
	sched := scheduling.New(db, repo, pub, nil, scheduling.Config{
		PollInterval:   time.Second,
		ClaimBatchSize: 10,
	})

	sched.PollOnce(ctx)

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}

	var next time.Time
	if err := db.QueryRowx("SELECT next_execution_time FROM tasks WHERE id = $1", taskID).Scan(&next); err != nil {
		t.Fatalf("scan next_execution_time: %v", err)
	}
	if !next.After(time.Now()) {
		t.Errorf("expected next_execution_time in the future, got %v", next)
	}
	if next.After(time.Now().Add(2 * time.Minute)) {
		t.Errorf("next_execution_time %v too far in future for '* * * * *'", next)
	}
}

func TestIntegration_PollOnce_SkipLockedConcurrency(t *testing.T) {
	ctx := context.Background()

	// A single connection pool suffices: each WithTx call opens its own
	// pooled connection, so two concurrent transactions get separate locks
	// and SKIP LOCKED works correctly.
	db := testutil.NewTestDB(t)
	testutil.Truncate(t, db, "execution_history", "tasks")

	pub1, cleanup1 := newTestBrokerPublisher(t)
	defer cleanup1()
	pub2, cleanup2 := newTestBrokerPublisher(t)
	defer cleanup2()

	past := time.Now().Add(-time.Minute)
	for i := range 4 {
		seedTask(t, db, "task-"+string(rune('A'+i)), "interval",
			`{"seconds": 60}`, true, past)
	}

	sched1 := scheduling.New(db, schedulerrepo.NewTaskRepository(db), pub1, nil, scheduling.Config{
		PollInterval: time.Second, ClaimBatchSize: 2,
	})
	sched2 := scheduling.New(db, schedulerrepo.NewTaskRepository(db), pub2, nil, scheduling.Config{
		PollInterval: time.Second, ClaimBatchSize: 2,
	})

	done := make(chan struct{}, 2)
	go func() { sched1.PollOnce(ctx); done <- struct{}{} }()
	go func() { sched2.PollOnce(ctx); done <- struct{}{} }()
	<-done
	<-done

	total := len(pub1.published) + len(pub2.published)
	if total != 4 {
		t.Errorf("expected 4 total publishes, got %d (%d + %d)", total, len(pub1.published), len(pub2.published))
	}

	// Verify no task was published more than once.
	seen := map[uuid.UUID]int{}
	for _, e := range pub1.published {
		seen[e.TaskID]++
	}
	for _, e := range pub2.published {
		seen[e.TaskID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("task %s published %d times, want exactly 1", id, count)
		}
	}
}
