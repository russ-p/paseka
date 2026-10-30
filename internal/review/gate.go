package review

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/worktree"
)

// ApproveInput describes a human approval of a review-gated task.
type ApproveInput struct {
	TraceID      string
	TaskID       string
	Summary      string
	MergeMessage string
	PRTitle      string
	PRBody       string
	Draft        bool
	RunHooks     bool
	AgentID      string
}

// ApproveResult reports the outcome of a successful approve.
type ApproveResult struct {
	CommitSHA    string
	StashOutcome worktree.StashOutcome
	Published    bool
	PRURL        string
	PRNumber     int
	PRState      string
	// Experimental marks a pinned fork approval. Its work is accepted internally,
	// but the branch is never merged, pushed, or published.
	Experimental bool
}

// Approve merges the trace worktree or publishes a pull request, then completes the review task
// except for pull_request delivery, which stays waiting_review until the forge reports merged.
func Approve(ctx context.Context, colonyCtx colony.Context, pub bus.Publisher, ledger taskledger.Ledger, in ApproveInput, opts WriteOptions) (ApproveResult, error) {
	if in.TraceID == "" || in.TaskID == "" {
		return ApproveResult{}, fmt.Errorf("trace and task id are required")
	}
	if ledger == nil {
		return ApproveResult{}, fmt.Errorf("task ledger is required")
	}

	snap, err := ledger.Snapshot(in.TraceID)
	if err != nil {
		return ApproveResult{}, err
	}
	task, ok := snap.Tasks[in.TaskID]
	if !ok {
		return ApproveResult{}, fmt.Errorf("task %q not found in trace %s", in.TaskID, in.TraceID)
	}
	if task.Status != protocol.TaskStatusWaitingReview {
		return ApproveResult{}, fmt.Errorf("task %q is %q, expected waiting_review", in.TaskID, task.Status)
	}
	if !taskledger.IsReviewGate(task) {
		return ApproveResult{}, fmt.Errorf("task %q is not a review gate task", in.TaskID)
	}
	if err := EnsureApprovable(snap, in.TaskID); err != nil {
		return ApproveResult{}, err
	}

	bees, err := colonyCtx.LoadAllBees()
	if err != nil {
		return ApproveResult{}, err
	}
	manifest, err := colony.LoadColony(colonyCtx.ColonyRoot)
	if err != nil {
		return ApproveResult{}, err
	}

	experimental, err := IsExperimentalTrail(colonyCtx.ColonyRoot, in.TraceID)
	if err != nil {
		return ApproveResult{}, err
	}
	result := ApproveResult{Experimental: experimental}
	// Experimental work may complete its internal review, but its branch is
	// never delivered. Promotion is a separate ordinary-trail proposal.
	if !experimental {
		if ShouldPublishOnApprove(task, bees, manifest.Defaults) {
			published, err := publishPullRequest(ctx, colonyCtx, in.TraceID, in)
			if err != nil {
				return ApproveResult{}, err
			}
			return published, nil
		}
		if ShouldMergeOnApprove(task, bees, manifest.Defaults) {
			wtPath := worktree.Path(colonyCtx.ColonyRoot, in.TraceID)
			if gitroot.IsInsideWorkTree(wtPath) {
				traceSummary, err := runs.ResolveTraceSummary(colonyCtx.ColonyRoot, in.TraceID)
				if err != nil {
					return ApproveResult{}, err
				}
				mergeMessage := ComposeMergeMessage(in.TraceID, in.MergeMessage, traceSummary).FormatMessage()
				mergeRes, err := worktree.Merge(worktree.MergeOptions{
					ColonyRoot: colonyCtx.ColonyRoot,
					TraceID:    in.TraceID,
					Slug:       colonyCtx.Slug,
					Message:    mergeMessage,
				})
				if err != nil {
					return ApproveResult{StashOutcome: mergeRes.StashOutcome}, err
				}
				result.CommitSHA = mergeRes.CommitSHA
				result.StashOutcome = mergeRes.StashOutcome
			}
		}
	}

	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		summary = "approved by human"
	}
	agentID := in.AgentID
	if agentID == "" {
		agentID = "human"
	}

	completed, err := protocol.NewEvent(in.TraceID, agentID, 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      in.TaskID,
		Status:      protocol.TaskStatusCompleted,
		Summary:     summary,
		Commit:      result.CommitSHA,
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		return ApproveResult{}, err
	}
	if err := WriteEvent(ctx, pub, ledger, completed, opts); err != nil {
		return ApproveResult{}, err
	}
	if err := ActivateFinalReviewGate(ctx, pub, ledger, colonyCtx, in.TraceID, opts); err != nil {
		return ApproveResult{}, err
	}
	return result, nil
}

// EnsureApprovable refuses approval of the merge gate while any other
// non-merge task is still incomplete, so a change set can never be merged
// underneath a running closing round or an unfinished rework task. A mid-trail
// review gate is not a merge task: its downstream work is planned by
// definition, and approving it is how that work becomes ready.
func EnsureApprovable(trace taskledger.TraceSnapshot, taskID string) error {
	task, ok := trace.Tasks[taskID]
	if !ok || !taskledger.IsMergeTask(task) {
		return nil
	}
	var blockers []string
	for id, other := range trace.Tasks {
		if id == taskID || taskledger.IsMergeTask(other) {
			continue
		}
		if other.Status == protocol.TaskStatusCompleted {
			continue
		}
		status := other.Status
		if status == "" {
			status = protocol.TaskStatusPlanned
		}
		blockers = append(blockers, fmt.Sprintf("%s (%s)", id, status))
	}
	if len(blockers) == 0 {
		return nil
	}
	sort.Strings(blockers)
	return fmt.Errorf("cannot approve merge gate %q: %d unfinished task(s): %s", taskID, len(blockers), strings.Join(blockers, ", "))
}

// RejectInput describes a human rejection of a review-gated task.
type RejectInput struct {
	TraceID  string
	TaskID   string
	Feedback string
	AgentID  string
	Ref      string
}

// EnsureRejectable reports whether the task can receive human.feedback (waiting review gate).
func EnsureRejectable(ledger taskledger.Ledger, traceID, taskID string) error {
	return ensureWaitingReviewGate(ledger, traceID, taskID)
}

// EnsureDiscardable reports whether the task can be discarded (waiting review gate).
func EnsureDiscardable(ledger taskledger.Ledger, traceID, taskID string) error {
	return ensureWaitingReviewGate(ledger, traceID, taskID)
}

// ensureWaitingReviewGate is the shared guard for approve, reject, and discard:
// the task must exist and be a review gate stopped at waiting_review.
func ensureWaitingReviewGate(ledger taskledger.Ledger, traceID, taskID string) error {
	if traceID == "" || taskID == "" {
		return fmt.Errorf("trace and task id are required")
	}
	if ledger == nil {
		return fmt.Errorf("task ledger is required")
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	task, ok := snap.Tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found in trace %s", taskID, traceID)
	}
	if task.Status != protocol.TaskStatusWaitingReview {
		return fmt.Errorf("task %q is %q, expected waiting_review", taskID, task.Status)
	}
	if !taskledger.IsReviewGate(task) {
		return fmt.Errorf("task %q is not a review gate task", taskID)
	}
	return nil
}

// Reject publishes human feedback for a review-gated task.
func Reject(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, in RejectInput) error {
	if err := EnsureRejectable(ledger, in.TraceID, in.TaskID); err != nil {
		return err
	}
	if !publisherAvailable(pub) {
		return fmt.Errorf("nats client is required")
	}
	feedback := strings.TrimSpace(in.Feedback)
	if feedback == "" {
		feedback = "Please revise the proposal."
	}
	agentID := in.AgentID
	if agentID == "" {
		agentID = "human"
	}
	payload := protocol.HumanFeedbackPayload{
		Kind:    protocol.InsightHumanFeedback,
		TaskID:  in.TaskID,
		Message: feedback,
	}
	if ref := strings.TrimSpace(in.Ref); ref != "" {
		payload.Ref = ref
	}
	ev, err := protocol.NewEvent(in.TraceID, agentID, 0, protocol.EventInsight, payload)
	if err != nil {
		return err
	}
	return pub.PublishEvent(ctx, ev)
}
