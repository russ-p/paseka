package taskledger

import (
	"sort"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

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
	sort.Slice(out, func(i, j int) bool {
		return out[i].TaskID < out[j].TaskID
	})
	return out
}

// FirstEligiblePlanned returns the lexicographically first planned task whose
// dependencies are all completed, or false when none exist.
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
