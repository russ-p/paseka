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

// TaskPurposeOf resolves the purpose a task's executions carry. A recorded
// purpose is authoritative; a legacy task without one is read through its
// phase, so an old closing round stays closing and an old work task stays work.
// It never infers analysis from a Bee name or a task title.
func TaskPurposeOf(task TaskSnapshot) protocol.ExecutionPurpose {
	if protocol.IsKnownExecutionPurpose(task.Purpose) {
		return task.Purpose
	}
	return protocol.PurposeForTaskPhase(TaskPhaseOf(task))
}

// IsAnalysisTask reports whether the task studies an already-recorded result
// rather than producing goal work. Analysis tasks are excluded from the
// working set: they do not advance the work revision, satisfy or block
// completion, or hold the final gate and change requests.
func IsAnalysisTask(task TaskSnapshot) bool {
	return TaskPurposeOf(task) == protocol.PurposeAnalysis
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

// AllWorkCompleted reports whether the trail has work and all of it completed.
// It is the closing trigger: a closing round becomes eligible only once this
// holds, and a trail with no work-phase task (a merge-only or closing-only
// record) must not seed a wrap-up of nothing.
func AllWorkCompleted(trace TraceSnapshot) bool {
	sawWork := false
	for _, task := range trace.Tasks {
		if IsAnalysisTask(task) {
			// Studying a result is not producing it.
			continue
		}
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
// alike — has completed. It is the final-gate condition. Analysis tasks are not
// product work and neither suppress the gate nor hold it open.
func AllPreMergeCompleted(trace TraceSnapshot) bool {
	sawAny := false
	for _, task := range trace.Tasks {
		if IsAnalysisTask(task) {
			continue
		}
		sawAny = true
		if IsMergeTask(task) {
			continue
		}
		if task.Status != protocol.TaskStatusCompleted {
			return false
		}
	}
	return sawAny
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

// HasWaitingReview reports whether any product task is awaiting human review.
// An analysis task's review is retrospective and never holds the product gate.
func HasWaitingReview(trace TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if IsAnalysisTask(task) {
			continue
		}
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
		if IsAnalysisTask(task) {
			// Analysis inspects the candidate; it does not become it.
			continue
		}
		if task.ProposalWorkspace == protocol.ProposalWorkspaceIsolated {
			return true
		}
	}
	return false
}

// LastCompletedIsolatedProposalTask returns the newest completed non-final task
// that recorded an isolated code.proposal (the bee that should continue rework).
// Closing-phase tasks are excluded: a closing round that opens a proposal is
// recorded but must never become the source of "apply review comments" work.
func LastCompletedIsolatedProposalTask(trace TraceSnapshot) (TaskSnapshot, bool) {
	var candidates []TaskSnapshot
	for _, task := range trace.Tasks {
		if IsFinalReviewTask(task) || IsClosingTask(task) || IsAnalysisTask(task) {
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
		if IsFinalReviewTask(task) || IsAnalysisTask(task) {
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
