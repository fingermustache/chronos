//go:build !integration

package scheduling_test

import (
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/fingermustache/chronos/scheduler/internal/scheduling"
	"github.com/google/uuid"
)

func taskWithTimezone(scheduleType models.ScheduleType, cfg models.JSONB, timezone *string) *models.Task {
	now := time.Now().Add(-time.Minute)
	return &models.Task{
		ID:                uuid.New(),
		ScheduleType:      scheduleType,
		ScheduleConfig:    cfg,
		Timezone:          timezone,
		TaskType:          models.TaskTypeHTTP,
		TaskConfig:        models.JSONB{},
		Enabled:           true,
		NextExecutionTime: &now,
	}
}

func ptr(s string) *string { return &s }

func TestNextExecution_CronRespectsTimezone(t *testing.T) {
	cases := []struct {
		name     string
		timezone *string
		nowUTC   time.Time
		wantUTC  time.Time
	}{
		{
			name:     "UTC nil",
			timezone: nil,
			nowUTC:   time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC),
			wantUTC:  time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		},
		{
			name:     "EST: 09:00 local = 14:00 UTC",
			timezone: ptr("America/New_York"),
			nowUTC:   time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC), // 03:00 EST
			wantUTC:  time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC),
		},
		{
			name:     "JST: 09:00 local = 00:00 UTC next day",
			timezone: ptr("Asia/Tokyo"),
			nowUTC:   time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC), // 10:00 JST, past 09:00
			wantUTC:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := taskWithTimezone(
				models.ScheduleTypeCron,
				models.JSONB{"expression": "0 9 * * *"},
				tc.timezone,
			)
			result, err := scheduling.NextExecutionForTest(task, tc.nowUTC)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsZero() {
				t.Fatal("expected non-zero next execution time")
			}
			if !result.Equal(tc.wantUTC) {
				t.Errorf("got %v, want %v", result.UTC(), tc.wantUTC)
			}
		})
	}
}

func TestNextExecution_IntervalIgnoresTimezone(t *testing.T) {
	task := taskWithTimezone(
		models.ScheduleTypeInterval,
		models.JSONB{"seconds": float64(300)},
		ptr("America/New_York"),
	)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	result, err := scheduling.NextExecutionForTest(task, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := now.Add(300 * time.Second)
	if !result.Equal(want) {
		t.Errorf("got %v, want %v", result, want)
	}
}
