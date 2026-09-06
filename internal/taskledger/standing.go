package taskledger

import (
	"sort"

	"github.com/russ-p/paseka/internal/protocol"
)

// IsOpenStandingTickStatus reports whether a ledger task still occupies a standing tick.
// blocked is allowed: the previous tick burned its stipend and the next tick is a new task.
func IsOpenStandingTickStatus(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusPlanned, protocol.TaskStatusReady, protocol.TaskStatusRunning,
		protocol.TaskStatusWaitingReview:
		return true
	default:
		return false
	}
}

// OpenStandingTick returns one open tick task if any exist (stable by taskId).
func OpenStandingTick(trace TraceSnapshot) (TaskSnapshot, bool) {
	var candidates []TaskSnapshot
	for _, task := range trace.Tasks {
		if IsOpenStandingTickStatus(task.Status) {
			candidates = append(candidates, task)
		}
	}
	if len(candidates) == 0 {
		return TaskSnapshot{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TaskID < candidates[j].TaskID
	})
	return candidates[0], true
}
