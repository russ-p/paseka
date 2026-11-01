package taskledger

import (
	"sort"

	"github.com/russ-p/paseka/internal/protocol"
)

// IsOpenStandingTickStatus reports whether a ledger task still occupies a standing tick.
// blocked is allowed: the previous tick burned its stipend and the next tick is a new task.
// skipped is terminal for overlap: a superseded tick no longer occupies the trail.
func IsOpenStandingTickStatus(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusPlanned, protocol.TaskStatusReady, protocol.TaskStatusRunning,
		protocol.TaskStatusWaitingReview:
		return true
	default:
		return false
	}
}

// EnergyDeferredTaskIDs lists the tasks whose dispatch is waiting on the honey
// reserve rather than on a lifecycle gate. Such work does not occupy a standing
// tick: the next stipend supersedes it atomically instead of refusing to start.
// Configuration-barrier work is not energy waiting and stays owned by its
// transition, so it keeps the tick open.
func EnergyDeferredTaskIDs(trace TraceSnapshot) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, dispatch := range trace.PendingDispatches {
		if dispatch.Kind != PendingDispatchTask || !dispatch.Status.Active() {
			continue
		}
		if dispatch.Reason == protocol.ConfigBarrierHoldReason || dispatch.TaskID == "" {
			continue
		}
		ids[dispatch.TaskID] = struct{}{}
	}
	return ids
}

// OpenStandingTick returns one open tick task if any exist (stable by taskId).
// A task with an active energy-deferred entry is not open: it is waiting for
// honey, which a new tick replaces rather than queues behind.
func OpenStandingTick(trace TraceSnapshot) (TaskSnapshot, bool) {
	energyDeferred := EnergyDeferredTaskIDs(trace)
	var candidates []TaskSnapshot
	for _, task := range trace.Tasks {
		if _, deferred := energyDeferred[task.TaskID]; deferred {
			continue
		}
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
