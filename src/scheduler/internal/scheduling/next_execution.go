package scheduling

import (
	"fmt"
	"time"

	"github.com/fingermustache/chronos/pkg/models"
	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// advanceResult describes what to do with a task after it fires.
type advanceResult struct {
	// next is the next execution time. Zero if the task should be disabled.
	next    time.Time
	disable bool
}

// nextExecution computes the post-fire action for a task.
// For once: disable the task (no reschedule).
// For interval: reschedule at now + interval.
// For cron: reschedule at the next cron occurrence after now.
func nextExecution(task *models.Task, now time.Time) (advanceResult, error) {
	switch task.ScheduleType {
	case models.ScheduleTypeOnce:
		return advanceResult{disable: true}, nil

	case models.ScheduleTypeInterval:
		seconds, ok := extractIntervalSeconds(task.ScheduleConfig)
		if !ok {
			return advanceResult{}, fmt.Errorf("invalid interval config for task %s", task.ID)
		}
		return advanceResult{next: now.Add(time.Duration(seconds) * time.Second)}, nil

	case models.ScheduleTypeCron:
		expr, ok := extractCronExpression(task.ScheduleConfig)
		if !ok {
			return advanceResult{}, fmt.Errorf("invalid cron config for task %s", task.ID)
		}
		schedule, err := cronParser.Parse(expr)
		if err != nil {
			return advanceResult{}, fmt.Errorf("unparseable cron expression %q for task %s: %w", expr, task.ID, err)
		}
		loc := time.UTC
		if task.Timezone != nil && *task.Timezone != "" {
			loc, err = time.LoadLocation(*task.Timezone)
			if err != nil {
				return advanceResult{}, fmt.Errorf("invalid timezone %q for task %s: %w", *task.Timezone, task.ID, err)
			}
		}
		next := schedule.Next(now.In(loc))
		if next.IsZero() {
			return advanceResult{}, fmt.Errorf("cron expression %q has no future occurrences for task %s", expr, task.ID)
		}
		return advanceResult{next: next.UTC()}, nil

	default:
		return advanceResult{}, fmt.Errorf("unknown schedule type %q for task %s", task.ScheduleType, task.ID)
	}
}

func extractCronExpression(cfg models.JSONB) (string, bool) {
	v, ok := cfg["expression"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func extractIntervalSeconds(cfg models.JSONB) (int, bool) {
	v, ok := cfg["seconds"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		if n <= 0 {
			return 0, false
		}
		return int(n), true
	case int:
		if n <= 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
