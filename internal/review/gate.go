package review

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/worktree"
)

// ApproveInput describes a human approval of a review-gated task.
type ApproveInput struct {
	TraceID      string
	TaskID       string
	Summary      string
	MergeMessage string
	AgentID      string
}

// Approve merges the trace worktree when present and completes the review task.
func Approve(ctx context.Context, colonyCtx colony.Context, client *bus.Client, ledger taskledger.Ledger, in ApproveInput) (string, error) {
	if in.TraceID == "" || in.TaskID == "" {
		return "", fmt.Errorf("trace and task id are required")
	}
	if ledger == nil {
		return "", fmt.Errorf("task ledger is required")
	}

	snap, err := ledger.Snapshot(in.TraceID)
	if err != nil {
		return "", err
	}
	task, ok := snap.Tasks[in.TaskID]
	if !ok {
		return "", fmt.Errorf("task %q not found in trace %s", in.TaskID, in.TraceID)
	}
	if task.Status != protocol.TaskStatusWaitingReview {
		return "", fmt.Errorf("task %q is %q, expected waiting_review", in.TaskID, task.Status)
	}
	if !taskledger.IsReviewGate(task) {
		return "", fmt.Errorf("task %q is not a review gate task", in.TaskID)
	}

	bees, err := colony.LoadAllBees(colonyCtx.ColonyRoot)
	if err != nil {
		return "", err
	}

	commitSHA := ""
	if ShouldMergeOnApprove(task, bees) {
		wtPath := worktree.Path(colonyCtx.ColonyRoot, in.TraceID)
		if gitroot.IsInsideWorkTree(wtPath) {
			mergeRes, err := worktree.Merge(worktree.MergeOptions{
				ColonyRoot: colonyCtx.ColonyRoot,
				TraceID:    in.TraceID,
				Slug:       colonyCtx.Slug,
				Message:    in.MergeMessage,
			})
			if err != nil {
				return "", err
			}
			commitSHA = mergeRes.CommitSHA
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
		Commit:      commitSHA,
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		return "", err
	}
	if client != nil {
		if err := client.PublishEvent(ctx, completed); err != nil {
			return "", err
		}
	}
	if _, err := ledger.Apply(completed); err != nil {
		return "", err
	}
	if err := ActivateFinalReviewGate(ctx, client, ledger, in.TraceID); err != nil {
		return "", err
	}
	return commitSHA, nil
}

// RejectInput describes a human rejection of a review-gated task.
type RejectInput struct {
	TraceID  string
	TaskID   string
	Feedback string
	AgentID  string
}

// Reject publishes human feedback for a review-gated task.
func Reject(ctx context.Context, client *bus.Client, ledger taskledger.Ledger, in RejectInput) error {
	if in.TraceID == "" || in.TaskID == "" {
		return fmt.Errorf("trace and task id are required")
	}
	if ledger == nil {
		return fmt.Errorf("task ledger is required")
	}

	snap, err := ledger.Snapshot(in.TraceID)
	if err != nil {
		return err
	}
	task, ok := snap.Tasks[in.TaskID]
	if !ok {
		return fmt.Errorf("task %q not found in trace %s", in.TaskID, in.TraceID)
	}
	if task.Status != protocol.TaskStatusWaitingReview {
		return fmt.Errorf("task %q is %q, expected waiting_review", in.TaskID, task.Status)
	}
	if !taskledger.IsReviewGate(task) {
		return fmt.Errorf("task %q is not a review gate task", in.TaskID)
	}
	if client == nil {
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
	ev, err := protocol.NewEvent(in.TraceID, agentID, 0, protocol.EventInsight, protocol.HumanFeedbackPayload{
		Kind:    protocol.InsightHumanFeedback,
		TaskID:  in.TaskID,
		Message: feedback,
	})
	if err != nil {
		return err
	}
	return client.PublishEvent(ctx, ev)
}
