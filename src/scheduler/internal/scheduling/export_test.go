package scheduling

import (
	"time"

	"github.com/fingermustache/chronos/pkg/models"
)

// NextExecutionForTest exposes nextExecution to external test packages.
// Returns the next scheduled time (zero if task should be disabled).
func NextExecutionForTest(task *models.Task, now time.Time) (time.Time, error) {
	result, err := nextExecution(task, now)
	if err != nil {
		return time.Time{}, err
	}
	return result.next, nil
}
