package taskledger

import (
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

// IsKillableTaskStatus reports whether a task should transition to cancelled on system.kill.
func IsKillableTaskStatus(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusCancelled:
		return false
	default:
		return true
	}
}

// applySystemKill marks a trace killed and cancels non-terminal tasks.
func applySystemKill(trace TraceSnapshot, reason string, now time.Time) TraceSnapshot {
	if trace.Killed {
		return trace
	}
	if trace.Tasks == nil {
		trace.Tasks = make(map[string]TaskSnapshot)
	}
	summary := reason
	if summary == "" {
		summary = protocol.TraceKilledSummary
	}
	trace.Killed = true
	for id, task := range trace.Tasks {
		if !IsKillableTaskStatus(task.Status) {
			continue
		}
		task.Status = protocol.TaskStatusCancelled
		task.Summary = summary
		task.UpdatedAt = now
		trace.Tasks[id] = task
	}
	return trace
}
