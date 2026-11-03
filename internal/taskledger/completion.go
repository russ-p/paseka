package taskledger

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// WorkCompletionVerdict is the ledger's answer to "is the accepted working set
// complete?". Complete is only true when at least one ordinary work task exists
// and every non-merge task — ordinary work and every closing round — reached
// completed, with no deferred admission still waiting.
//
// Reason names the first blocker in stable task-id order. It is evidence for a
// log line, never a task state.
type WorkCompletionVerdict struct {
	Complete  bool
	Reason    string
	WorkTasks int
}

// EvaluateWorkCompletion applies the work-completion predicate.
//
// A merge task at waiting_review does not block: final acceptance is a separate
// fact. Only a task proven to be the merge phase is excluded. A task whose
// phase/review evidence cannot prove it is final acceptance reads as work and
// blocks, so an unknown gate purpose holds the milestone instead of being
// guessed. A task with an active energy-deferred pending entry has not started
// and blocks whatever its lifecycle status says. The same rule keeps skipped,
// failed, cancelled, and blocked work from ever reading as completion.
//
// A trail with no work-phase task is not complete by vacuous truth: a taskless
// or event-only trail needs its own completion contract.
func EvaluateWorkCompletion(trace TraceSnapshot) WorkCompletionVerdict {
	if trace.Killed {
		return WorkCompletionVerdict{Reason: "trace is killed"}
	}
	workTasks := WorkTaskCount(trace)
	if workTasks == 0 {
		return WorkCompletionVerdict{Reason: "no working task"}
	}
	deferred := activeWorkPendingTaskIDs(trace)
	if len(deferred) > 0 {
		return WorkCompletionVerdict{
			Reason:    fmt.Sprintf("deferred work dispatch is still waiting (%d)", len(deferred)),
			WorkTasks: workTasks,
		}
	}
	for _, dispatch := range trace.PendingDispatches {
		if !dispatch.Status.Active() || dispatch.Purpose == protocol.PurposeAnalysis {
			continue
		}
		if dispatch.Kind == PendingDispatchDirect || dispatch.TaskID == "" {
			return WorkCompletionVerdict{Reason: "deferred dispatch is still waiting", WorkTasks: workTasks}
		}
	}

	var ids []string
	for taskID := range trace.Tasks {
		ids = append(ids, taskID)
	}
	sort.Strings(ids)
	for _, taskID := range ids {
		task := trace.Tasks[taskID]
		if IsMergeTask(task) || IsAnalysisTask(task) {
			// The merge gate and final acceptance are later observations, and
			// analysis studies a result instead of producing one.
			continue
		}
		if task.Status != protocol.TaskStatusCompleted {
			status := task.Status
			if status == "" {
				status = protocol.TaskStatusPlanned
			}
			return WorkCompletionVerdict{
				Reason:    fmt.Sprintf("task %s is %s", taskID, status),
				WorkTasks: workTasks,
			}
		}
	}
	return WorkCompletionVerdict{Complete: true, WorkTasks: workTasks}
}

// activeWorkPendingTaskIDs lists tasks whose admission is still deferred for a
// non-analysis reason. The task lifecycle status is deliberately not consulted:
// a deferred entry means the work has not started.
func activeWorkPendingTaskIDs(trace TraceSnapshot) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, dispatch := range trace.PendingDispatches {
		if !dispatch.Status.Active() || dispatch.Purpose == protocol.PurposeAnalysis {
			continue
		}
		if dispatch.TaskID == "" || dispatch.Kind != PendingDispatchTask {
			continue
		}
		ids[dispatch.TaskID] = struct{}{}
	}
	return ids
}

// AdvanceWorkRevision opens a new accepted-work revision. It is called only by
// the reducers that accept substantive work; the completion milestone and the
// closing seed never call it.
func AdvanceWorkRevision(trace TraceSnapshot) TraceSnapshot {
	trace.WorkRevision++
	if trace.WorkRevision < 1 {
		trace.WorkRevision = 1
	}
	return trace
}

// isAcceptedWorkTask reports whether a task snapshot represents ordinary goal
// work for revision accounting. Closing rounds are overhead, merge gates are
// acceptance, and analysis studies a result, so none of them reopens the work
// revision.
func isAcceptedWorkTask(task TaskSnapshot) bool {
	return !IsAnalysisTask(task) && TaskPhaseOf(task) == protocol.TaskPhaseWork
}

// isReworkReturn reports whether a previous status means the task had already
// entered, or tried to enter, the accepted working set. A return from one of
// these to ready/running is substantive rework, not the first start of a task.
func isReworkReturn(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusCancelled,
		protocol.TaskStatusWaitingReview:
		return true
	default:
		return false
	}
}

// IsWorkCompletionMilestone reports whether a payload kind is the milestone.
func IsWorkCompletionMilestone(kind string) bool {
	return strings.TrimSpace(kind) == protocol.SignalTrailWorkCompleted
}

// CompletionIDForRevision derives the durable identity of one trail's
// completion for one work revision. It is deterministic so a retry, a duplicate
// delivery, or a restart reuses the recorded identity instead of minting a
// second completion for the same accepted work.
func CompletionIDForRevision(traceID string, revision int) string {
	if revision < 1 {
		revision = 1
	}
	return fmt.Sprintf("completion-r%d-%s", revision, stableIDDigest(traceID))
}

func stableIDDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:6])
}

// RecordWorkCompletion stores one completion episode unless the trace already
// holds a completion for the same revision. The first recorded identity wins so
// a retry after restart cannot mint a second completion for the same work.
func RecordWorkCompletion(trace TraceSnapshot, completion WorkCompletionSnapshot) (TraceSnapshot, WorkCompletionSnapshot, bool, error) {
	completion.CompletionID = strings.TrimSpace(completion.CompletionID)
	if completion.CompletionID == "" {
		return trace, WorkCompletionSnapshot{}, false, fmt.Errorf("taskledger: completion id is required")
	}
	if completion.WorkRevision < 1 {
		return trace, WorkCompletionSnapshot{}, false, fmt.Errorf("taskledger: completion work revision is required")
	}
	if completion.CompletedAt.IsZero() {
		completion.CompletedAt = time.Now().UTC()
	}
	if trace.WorkCompletion != nil && trace.WorkCompletion.WorkRevision == completion.WorkRevision {
		return trace, *trace.WorkCompletion, false, nil
	}
	trace.WorkCompletion = &completion
	return trace, completion, true, nil
}

// MarkWorkCompletionPublished records that the milestone reached the publisher.
// It is idempotent and refuses a different identity, so a retry reuses the
// recorded completion rather than selecting another.
func MarkWorkCompletionPublished(trace TraceSnapshot, completionID string) (TraceSnapshot, bool, error) {
	if trace.WorkCompletion == nil {
		return trace, false, fmt.Errorf("taskledger: no work completion recorded")
	}
	if trace.WorkCompletion.CompletionID != completionID {
		return trace, false, fmt.Errorf("taskledger: completion %q does not match recorded %q", completionID, trace.WorkCompletion.CompletionID)
	}
	if trace.WorkCompletion.Published {
		return trace, false, nil
	}
	trace.WorkCompletion.Published = true
	return trace, true, nil
}
