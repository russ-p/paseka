package taskledger

import (
	"sort"

	"github.com/paseka/paseka/internal/protocol"
)

// EligiblePlanned returns planned tasks whose dependencies are all completed.
func EligiblePlanned(trace TraceSnapshot) []TaskSnapshot {
	var out []TaskSnapshot
	for _, task := range trace.Tasks {
		if task.Status != protocol.TaskStatusPlanned {
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
