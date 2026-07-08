package taskledger

import (
	"sort"

	"github.com/paseka/paseka/internal/protocol"
)

// FinalReviewTaskID is the synthetic task id when no explicit final review task was planned.
const FinalReviewTaskID = "_review"

// IsReviewGate reports whether a task requires human review before completion.
func IsReviewGate(task TaskSnapshot) bool {
	p := protocol.NormalizeTaskReviewPolicy(task.Review)
	return p == protocol.TaskReviewRequired || p == protocol.TaskReviewFinal
}

// IsFinalReviewTask reports whether a task is the trace-level merge gate.
func IsFinalReviewTask(task TaskSnapshot) bool {
	return protocol.NormalizeTaskReviewPolicy(task.Review) == protocol.TaskReviewFinal
}

// AllAFKTasksCompleted reports whether every task except review: final gates has completed.
func AllAFKTasksCompleted(trace TraceSnapshot) bool {
	if len(trace.Tasks) == 0 {
		return false
	}
	for _, task := range trace.Tasks {
		if IsFinalReviewTask(task) {
			continue
		}
		if task.Status != protocol.TaskStatusCompleted {
			return false
		}
	}
	return true
}

// FindFinalReviewTask returns the first planned final-review task, if any.
func FindFinalReviewTask(trace TraceSnapshot) (TaskSnapshot, bool) {
	var candidates []TaskSnapshot
	for _, task := range trace.Tasks {
		if IsFinalReviewTask(task) {
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

// HasWaitingReview reports whether any task is awaiting human review.
func HasWaitingReview(trace TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if task.Status == protocol.TaskStatusWaitingReview {
			return true
		}
	}
	return false
}

// ShouldSkipDispatch reports whether a ready task should bypass AFK dispatch.
func ShouldSkipDispatch(task TaskSnapshot) bool {
	return IsFinalReviewTask(task)
}
