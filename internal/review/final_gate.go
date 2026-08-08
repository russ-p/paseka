package review

import (
	"context"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/worktree"
)

// ActivateFinalReviewGate opens the trace-level merge gate when every task except
// review: final entries has reached completed and there is something to merge
// (an isolated code.proposal and/or a non-empty worktree merge diff).
// When AFK work is done but there is nothing to merge, any planned/ready/waiting
// final-review task is auto-completed instead of leaving a hollow HITL gate.
func ActivateFinalReviewGate(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, colonyCtx colony.Context, traceID string, opts WriteOptions) error {
	if ledger == nil || traceID == "" {
		return nil
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if !taskledger.AllAFKTasksCompleted(snap) {
		return nil
	}

	needsMerge := needsIsolatedMerge(snap, colonyCtx, traceID)
	if final, ok := taskledger.FindFinalReviewTask(snap); ok {
		if !needsMerge {
			return completeEmptyFinalGate(ctx, pub, ledger, traceID, final, opts)
		}
		if taskledger.HasWaitingReview(snap) && final.Status != protocol.TaskStatusWaitingReview {
			return nil
		}
		switch final.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady:
			ev, err := statusEvent(traceID, final.TaskID, protocol.TaskStatusWaitingReview, "All tasks completed — awaiting human review and merge")
			if err != nil {
				return err
			}
			return WriteEvent(ctx, pub, ledger, ev, opts)
		}
		return nil
	}

	if !needsMerge || taskledger.HasWaitingReview(snap) {
		return nil
	}

	planEv, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: taskledger.FinalReviewTaskID,
			Title:  "Human review and merge",
			Body:   "Review accumulated changes and merge into the default branch.",
			Review: protocol.TaskReviewFinal,
		}},
	})
	if err != nil {
		return err
	}
	if err := WriteEvent(ctx, pub, ledger, planEv, opts); err != nil {
		return err
	}
	ev, err := statusEvent(traceID, taskledger.FinalReviewTaskID, protocol.TaskStatusWaitingReview, "All tasks completed — awaiting human review and merge")
	if err != nil {
		return err
	}
	return WriteEvent(ctx, pub, ledger, ev, opts)
}

func needsIsolatedMerge(snap taskledger.TraceSnapshot, colonyCtx colony.Context, traceID string) bool {
	if taskledger.HasIsolatedProposal(snap) {
		return true
	}
	if colonyCtx.ColonyRoot == "" || traceID == "" {
		return false
	}
	diff, err := worktree.MergeDiff(worktree.MergeDiffOptions{
		ColonyRoot: colonyCtx.ColonyRoot,
		TraceID:    traceID,
		Slug:       colonyCtx.Slug,
	})
	if err != nil {
		return false
	}
	return !diff.Missing && !diff.Empty
}

func completeEmptyFinalGate(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, traceID string, final taskledger.TaskSnapshot, opts WriteOptions) error {
	switch final.Status {
	case protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusBlocked:
		return nil
	}
	completed, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      final.TaskID,
		Status:      protocol.TaskStatusCompleted,
		Summary:     "Nothing to merge — skipped final review gate",
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return WriteEvent(ctx, pub, ledger, completed, opts)
}

func statusEvent(traceID, taskID string, status protocol.TaskStatus, summary string) (protocol.Event, error) {
	return protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  taskID,
		Status:  status,
		Summary: summary,
	})
}
