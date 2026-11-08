package runs

import (
	"sort"
	"time"
)

// trailRunOutcome is one terminal run as the trail's attention projection sees
// it: the task it belongs to, when it finished, and which side of the resolution
// rule it sits on. A run that is neither a failure nor a success — cancelled,
// or still in flight — is not one of these and participates in neither
// direction.
type trailRunOutcome struct {
	// TaskID is the task recorded on the run. An empty id is the trail-level
	// group for direct runs, which have no finer identity than the trail.
	TaskID  string
	AgentID string
	// At is the completion instant, or the start instant for a record that
	// never finished.
	At        time.Time
	Failed    bool
	Succeeded bool
}

// openFailuresOf groups terminal runs by the task id recorded on the run and
// reports how many tasks still carry an unresolved failure, plus the instant of
// the newest one. A failure stays open until a later successful run of the same
// task exists; a success on one task never clears a failure on another.
//
// Each group is ordered by completion instant, falling back to the start
// instant when the record never finished, then by agent id. The tie-break makes
// the answer a total order, so the same runs always produce the same projection
// whatever order the directory read returned them in. Retries within one task
// collapse: a group is open or it is not, and the newest open failure is the
// last failure not followed by a success.
//
// The result is (0, nil) when nothing is open. The count and instant are
// additive on the wire: an older client that ignores them still reads the
// historical flag beside them.
func openFailuresOf(outcomes []trailRunOutcome) (int, *time.Time) {
	groups := make(map[string][]trailRunOutcome, len(outcomes))
	for _, outcome := range outcomes {
		groups[outcome.TaskID] = append(groups[outcome.TaskID], outcome)
	}

	open := 0
	var newest time.Time
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			if !group[i].At.Equal(group[j].At) {
				return group[i].At.Before(group[j].At)
			}
			return group[i].AgentID < group[j].AgentID
		})
		var openedAt time.Time
		isOpen := false
		for _, outcome := range group {
			switch {
			case outcome.Failed:
				isOpen = true
				openedAt = outcome.At
			case outcome.Succeeded:
				isOpen = false
				openedAt = time.Time{}
			}
		}
		if !isOpen {
			continue
		}
		open++
		if openedAt.After(newest) {
			newest = openedAt
		}
	}

	if open == 0 || newest.IsZero() {
		return open, nil
	}
	return open, &newest
}
