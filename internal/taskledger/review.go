package taskledger

import (
	"sort"

	"github.com/russ-p/paseka/internal/protocol"
)

// FinalReviewTaskID is the synthetic task id when no explicit final review task was planned.
const FinalReviewTaskID = "_review"

// TaskPhaseOf resolves a snapshot's ledger phase. A snapshot written before
// phase existed carries an empty phase and is read through its review policy:
// review: final is merge, anything else is work.
func TaskPhaseOf(task TaskSnapshot) protocol.TaskPhase {
	return protocol.DeriveTaskPhase(task.Phase, task.Review)
}

// IsMergeTask reports whether a task is the trace-level merge gate. Merge tasks
// are never dispatched and never open the gate early.
func IsMergeTask(task TaskSnapshot) bool {
	return TaskPhaseOf(task) == protocol.TaskPhaseMerge
}

// IsClosingTask reports whether a task is a runtime-seeded closing round.
func IsClosingTask(task TaskSnapshot) bool {
	return TaskPhaseOf(task) == protocol.TaskPhaseClosing
}

// IsReviewGate reports whether a task requires human review before completion.
func IsReviewGate(task TaskSnapshot) bool {
	if IsMergeTask(task) {
		return true
	}
	return protocol.NormalizeTaskReviewPolicy(task.Review) == protocol.TaskReviewRequired
}

// IsFinalReviewTask reports whether a task carries review: final. After reduce
// coercion that is exactly the merge phase; kept as the review-policy view for
// callers that reason about the policy rather than the phase.
func IsFinalReviewTask(task TaskSnapshot) bool {
	return protocol.NormalizeTaskReviewPolicy(task.Review) == protocol.TaskReviewFinal
}

// AllWorkCompleted reports whether every work-phase task has completed. It is
// the closing trigger: a closing round becomes eligible only once this holds.
func AllWorkCompleted(trace TraceSnapshot) bool {
	sawWork := false
	for _, task := range trace.Tasks {
		if TaskPhaseOf(task) != protocol.TaskPhaseWork {
			continue
		}
		sawWork = true
		if task.Status != protocol.TaskStatusCompleted {
			return false
		}
	}
	return sawWork
}

// AllPreMergeCompleted reports whether every non-merge task — work and closing
// alike — has completed. It is the final-gate condition.
func AllPreMergeCompleted(trace TraceSnapshot) bool {
	if len(trace.Tasks) == 0 {
		return false
	}
	for _, task := range trace.Tasks {
		if IsMergeTask(task) {
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

// HasIsolatedProposal reports whether any task recorded an isolated code.proposal.
// Used to decide whether a trace-level merge gate is warranted.
func HasIsolatedProposal(trace TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if task.ProposalWorkspace == protocol.ProposalWorkspaceIsolated {
			return true
		}
	}
	return false
}

// LastCompletedIsolatedProposalTask returns the newest completed non-final task
// that recorded an isolated code.proposal (the bee that should continue rework).
func LastCompletedIsolatedProposalTask(trace TraceSnapshot) (TaskSnapshot, bool) {
	var candidates []TaskSnapshot
	for _, task := range trace.Tasks {
		if IsFinalReviewTask(task) {
			continue
		}
		if task.Status != protocol.TaskStatusCompleted {
			continue
		}
		if task.ProposalWorkspace != protocol.ProposalWorkspaceIsolated {
			continue
		}
		candidates = append(candidates, task)
	}
	if len(candidates) == 0 {
		return TaskSnapshot{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
		}
		return candidates[i].TaskID > candidates[j].TaskID
	})
	return candidates[0], true
}

func isInFlightReworkStatus(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusPlanned, protocol.TaskStatusReady, protocol.TaskStatusRunning,
		protocol.TaskStatusWaitingReview, protocol.TaskStatusBlocked:
		return true
	default:
		return false
	}
}

// InFlightRework returns a non-final task that is still occupying the trail
// (planned through blocked). Failed and cancelled tasks do not count.
func InFlightRework(trace TraceSnapshot) (TaskSnapshot, bool) {
	var candidates []TaskSnapshot
	for _, task := range trace.Tasks {
		if IsFinalReviewTask(task) {
			continue
		}
		if isInFlightReworkStatus(task.Status) {
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

// CanRequestChanges reports whether a final merge gate may start a rework task.
func CanRequestChanges(trace TraceSnapshot, task TaskSnapshot) bool {
	if !IsFinalReviewTask(task) || task.Status != protocol.TaskStatusWaitingReview {
		return false
	}
	if _, ok := LastCompletedIsolatedProposalTask(trace); !ok {
		return false
	}
	if _, ok := InFlightRework(trace); ok {
		return false
	}
	return true
}

// ShouldSkipDispatch reports whether a ready task should bypass AFK dispatch.
// A merge task is presented to a human instead of being run.
func ShouldSkipDispatch(task TaskSnapshot) bool {
	return IsMergeTask(task)
}
