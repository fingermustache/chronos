//go:build !integration

// Internal package test — uses package service (not service_test) to access
// the unexported calculateNextExecutionTime directly.
package service

import (
	"testing"
	"time"

	"github.com/fingermustache/chronos/pkg/models"
)

// Standard 5-field cron expressions always have a next occurrence — robfig/cron/v3
// does not return a zero time for any valid recurring expression. The tests below
// verify the non-zero guard holds across common and pathological expressions, using
// a fixed `now` so results are deterministic.
func TestCalculateNextExecutionTime_CronNeverReturnsZero(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		expr string
	}{
		{"every minute", "* * * * *"},
		{"daily at 9am", "0 9 * * *"},
		{"once a year Jan 1 midnight", "0 0 1 1 *"},
		{"rare: Dec 31 23:59 on a Friday", "59 23 31 12 5"},
		{"weekdays only", "0 9 * * 1-5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := map[string]interface{}{"expression": tc.expr}
			got, err := calculateNextExecutionTime(models.ScheduleTypeCron, cfg, now)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.expr, err)
			}
			if got.IsZero() {
				t.Errorf("expression %q returned zero next execution time", tc.expr)
			}
			if !got.After(now) {
				t.Errorf("expression %q: expected next time after %v, got %v", tc.expr, now, got)
			}
		})
	}
}

func TestCalculateNextExecutionTime_IntervalIsExact(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := map[string]interface{}{"seconds": float64(300)}

	got, err := calculateNextExecutionTime(models.ScheduleTypeInterval, cfg, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := now.Add(300 * time.Second)
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestCalculateNextExecutionTime_OnceEqualsRunAt(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	runAt := "2030-12-31T23:59:00Z"
	cfg := map[string]interface{}{"run_at": runAt}

	got, err := calculateNextExecutionTime(models.ScheduleTypeOnce, cfg, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected, _ := time.Parse(time.RFC3339, runAt)
	if !got.Equal(expected.UTC()) {
		t.Errorf("expected %v, got %v", expected.UTC(), got)
	}
}
