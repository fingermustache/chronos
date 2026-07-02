//go:build !integration

package service

import (
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/fingermustache/chronos/pkg/models"
)

// resolveLocation is a helper used by tests to get a *time.Location by name.
func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic("unknown timezone: " + name)
	}
	return loc
}

// TestCalculateNextExecutionTime_CronWithTimezone verifies that the cron
// expression is evaluated in the task's local timezone and the result is
// stored as UTC.
func TestCalculateNextExecutionTime_CronWithTimezone(t *testing.T) {
	cases := []struct {
		name     string
		timezone string
		expr     string
		// now is expressed in UTC; the cron must fire relative to local time
		nowUTC time.Time
		// wantUTC is the expected next execution time in UTC
		wantUTC time.Time
	}{
		{
			name:     "UTC explicit",
			timezone: "UTC",
			expr:     "0 9 * * *", // 09:00 daily
			nowUTC:   time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC),
			wantUTC:  time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		},
		{
			name:     "EST (UTC-5): 09:00 local = 14:00 UTC",
			timezone: "America/New_York",
			expr:     "0 9 * * *",
			nowUTC:   time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC), // 03:00 EST
			wantUTC:  time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC),
		},
		{
			name:     "PST (UTC-8): 09:00 local = 17:00 UTC",
			timezone: "America/Los_Angeles",
			expr:     "0 9 * * *",
			nowUTC:   time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC), // 00:00 PST
			wantUTC:  time.Date(2026, 1, 1, 17, 0, 0, 0, time.UTC),
		},
		{
			name:     "JST (UTC+9): 09:00 local = 00:00 UTC",
			timezone: "Asia/Tokyo",
			expr:     "0 9 * * *",
			nowUTC:   time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC), // 10:00 JST, already past 09:00
			wantUTC:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), // next day 09:00 JST
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := map[string]interface{}{"expression": tc.expr}
			tz := tc.timezone
			got, err := calculateNextExecutionTime(models.ScheduleTypeCron, cfg, tc.nowUTC, &tz)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.wantUTC) {
				t.Errorf("got %v, want %v", got.UTC(), tc.wantUTC)
			}
			if got.Location() != time.UTC {
				t.Errorf("result must be UTC, got location %v", got.Location())
			}
		})
	}
}

// TestCalculateNextExecutionTime_NilTimezoneDefaultsUTC verifies that nil
// timezone behaves identically to explicit UTC.
func TestCalculateNextExecutionTime_NilTimezoneDefaultsUTC(t *testing.T) {
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	cfg := map[string]interface{}{"expression": "0 9 * * *"}

	withNil, err := calculateNextExecutionTime(models.ScheduleTypeCron, cfg, now, nil)
	if err != nil {
		t.Fatalf("nil tz: %v", err)
	}
	utc := "UTC"
	withUTC, err := calculateNextExecutionTime(models.ScheduleTypeCron, cfg, now, &utc)
	if err != nil {
		t.Fatalf("UTC tz: %v", err)
	}
	if !withNil.Equal(withUTC) {
		t.Errorf("nil timezone %v != explicit UTC %v", withNil, withUTC)
	}
}

// TestCalculateNextExecutionTime_IntervalIgnoresTimezone verifies that interval
// tasks are not affected by timezone (they are always relative to now UTC).
func TestCalculateNextExecutionTime_IntervalIgnoresTimezone(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := map[string]interface{}{"seconds": float64(3600)}
	tz := "America/New_York"

	got, err := calculateNextExecutionTime(models.ScheduleTypeInterval, cfg, now, &tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := now.Add(time.Hour)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestCalculateNextExecutionTime_InvalidTimezone verifies that an unrecognised
// timezone name is rejected at calculation time.
func TestCalculateNextExecutionTime_InvalidTimezone(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := map[string]interface{}{"expression": "0 9 * * *"}
	bad := "Not/ATimezone"

	_, err := calculateNextExecutionTime(models.ScheduleTypeCron, cfg, now, &bad)
	if err == nil {
		t.Fatal("expected error for invalid timezone, got nil")
	}
}

// TestCalculateNextExecutionTime_DST_SpringForward verifies that when a cron
// expression targets an hour that is skipped by DST (02:30 on spring-forward
// night in America/New_York), robfig/cron skips that occurrence and fires at
// the next valid time.
func TestCalculateNextExecutionTime_DST_SpringForward(t *testing.T) {
	tz := "America/New_York"
	loc := mustLocation(tz)

	// Spring forward: 2026-03-08 02:00 EST → 03:00 EDT (clocks skip 02:xx)
	// "30 2 * * *" targets 02:30, which does not exist on this night.
	// robfig/cron should advance to 02:30 the following day (now in EDT = UTC-4).
	beforeGap := time.Date(2026, 3, 8, 1, 0, 0, 0, loc) // 01:00 EST

	cfg := map[string]interface{}{"expression": "30 2 * * *"}
	got, err := calculateNextExecutionTime(models.ScheduleTypeCron, cfg, beforeGap.UTC(), &tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next valid 02:30 in Eastern time is 2026-03-09 02:30 EDT = 06:30 UTC
	want := time.Date(2026, 3, 9, 6, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("spring-forward: got %v, want %v", got.UTC(), want)
	}
}

// TestCalculateNextExecutionTime_DST_FallBack verifies that when clocks fall
// back (02:30 occurs twice), the scheduler fires at the first occurrence.
func TestCalculateNextExecutionTime_DST_FallBack(t *testing.T) {
	tz := "America/New_York"
	loc := mustLocation(tz)

	// Fall back: 2026-11-01 02:00 EDT → 01:00 EST (clocks repeat 01:xx)
	// "30 1 * * *" targets 01:30, which occurs twice.
	// robfig/cron fires at the first occurrence (EDT = UTC-4).
	before := time.Date(2026, 11, 1, 0, 0, 0, 0, loc) // midnight EDT

	cfg := map[string]interface{}{"expression": "30 1 * * *"}
	got, err := calculateNextExecutionTime(models.ScheduleTypeCron, cfg, before.UTC(), &tz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 01:30 EDT = 05:30 UTC (first occurrence, before the clock falls back)
	want := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("fall-back: got %v, want %v", got.UTC(), want)
	}
}

// TestValidateTimezone verifies the validation helper rejects unknown names
// and accepts valid IANA names.
func TestValidateTimezone(t *testing.T) {
	valid := []string{"UTC", "America/New_York", "America/Los_Angeles", "Asia/Tokyo", "Europe/London"}
	for _, tz := range valid {
		t.Run("valid/"+tz, func(t *testing.T) {
			if err := validateTimezone(&tz); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}

	invalid := []string{"Not/Real", "Blah/Blah", "UTC+5", "garbage"}
	for _, tz := range invalid {
		t.Run("invalid/"+tz, func(t *testing.T) {
			if err := validateTimezone(&tz); err == nil {
				t.Errorf("expected error for %q, got nil", tz)
			}
		})
	}

	if err := validateTimezone(nil); err != nil {
		t.Errorf("nil timezone should be valid (defaults to UTC), got: %v", err)
	}
}
