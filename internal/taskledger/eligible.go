package taskledger

import (
	"cmp"
	"slices"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// comparePlanned orders tasks oldest-planned first (FIFO, spec 031), with the
// task id breaking ties. A task with no createdAt — a trail planned before the
// ledger stamped one — sorts last, so a legacy task never jumps ahead of one
// whose wait is actually measured.
func comparePlanned(a, b TaskSnapshot) int {
	aStamped := !a.CreatedAt.IsZero()
	bStamped := !b.CreatedAt.IsZero()
	if aStamped != bStamped {
		if aStamped {
			return -1
		}
		return 1
	}
	if aStamped {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
	}
	return cmp.Compare(a.TaskID, b.TaskID)
}

// EligiblePlanned returns planned tasks whose dependencies are all completed.
func EligiblePlanned(trace TraceSnapshot) []TaskSnapshot {
	var out []TaskSnapshot
	afkDone := AllAFKTasksCompleted(trace)
	for _, task := range trace.Tasks {
		if task.Status != protocol.TaskStatusPlanned {
			continue
		}
		if IsFinalReviewTask(task) && !afkDone {
			continue
		}
		if !allDepsCompleted(trace, task.DependsOn) {
			continue
		}
		out = append(out, task)
	}
	slices.SortFunc(out, comparePlanned)
	return out
}

// FirstEligiblePlanned returns the oldest planned task whose dependencies are
// all completed, or false when none exist.
func FirstEligiblePlanned(trace TraceSnapshot) (TaskSnapshot, bool) {
	eligible := EligiblePlanned(trace)
	if len(eligible) == 0 {
		return TaskSnapshot{}, false
	}
	return eligible[0], true
}

// HasReadyTask reports whether any task in the trace is currently ready.
func HasReadyTask(trace TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if task.Status == protocol.TaskStatusReady {
			return true
		}
	}
	return false
}

// PromoteFirstEligible transitions the first eligible planned task to ready when
// no task is already ready. Returns the promoted task and true on success.
func PromoteFirstEligible(trace TraceSnapshot, now time.Time) (TraceSnapshot, TaskSnapshot, bool) {
	if trace.Killed {
		return trace, TaskSnapshot{}, false
	}
	if HasReadyTask(trace) {
		return trace, TaskSnapshot{}, false
	}
	candidate, ok := FirstEligiblePlanned(trace)
	if !ok {
		return trace, TaskSnapshot{}, false
	}
	candidate.Status = protocol.TaskStatusReady
	candidate.UpdatedAt = now
	trace.Tasks[candidate.TaskID] = candidate
	return trace, candidate, true
}

// CanStart reports whether a task may be enqueued via task.ready.
func CanStart(trace TraceSnapshot, taskID string) (TaskSnapshot, error) {
	task, ok := trace.Tasks[taskID]
	if !ok {
		return TaskSnapshot{}, ErrTaskNotFound
	}
	if task.Status == protocol.TaskStatusReady {
		return task, ErrTaskAlreadyReady
	}
	if task.Status == protocol.TaskStatusCompleted {
		return task, ErrTaskCompleted
	}
	if task.Status != protocol.TaskStatusPlanned {
		return task, ErrTaskNotEligible
	}
	if !allDepsCompleted(trace, task.DependsOn) {
		return task, ErrDependenciesIncomplete
	}
	return task, nil
}

// CanRetry reports whether a task may be re-queued via task.ready after failure or a stuck run.
func CanRetry(trace TraceSnapshot, taskID string) (TaskSnapshot, error) {
	task, ok := trace.Tasks[taskID]
	if !ok {
		return TaskSnapshot{}, ErrTaskNotFound
	}
	switch task.Status {
	case protocol.TaskStatusFailed, protocol.TaskStatusRunning:
		return task, nil
	default:
		return task, ErrTaskNotRetryable
	}
}
