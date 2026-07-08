package review

import (
	"context"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

// ActivateFinalReviewGate opens the trace-level merge gate when every task except
// review: final entries has reached completed.
func ActivateFinalReviewGate(ctx context.Context, client *bus.Client, ledger taskledger.Ledger, traceID string) error {
	if ledger == nil || traceID == "" {
		return nil
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if !taskledger.AllAFKTasksCompleted(snap) || taskledger.HasWaitingReview(snap) {
		return nil
	}
	if final, ok := taskledger.FindFinalReviewTask(snap); ok {
		switch final.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady:
			ev, err := statusEvent(traceID, final.TaskID, protocol.TaskStatusWaitingReview, "All tasks completed — awaiting human review and merge")
			if err != nil {
				return err
			}
			return publishAndApply(ctx, client, ledger, ev)
		}
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
	if err := publishAndApply(ctx, client, ledger, planEv); err != nil {
		return err
	}
	ev, err := statusEvent(traceID, taskledger.FinalReviewTaskID, protocol.TaskStatusWaitingReview, "All tasks completed — awaiting human review and merge")
	if err != nil {
		return err
	}
	return publishAndApply(ctx, client, ledger, ev)
}

func statusEvent(traceID, taskID string, status protocol.TaskStatus, summary string) (protocol.Event, error) {
	return protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  taskID,
		Status:  status,
		Summary: summary,
	})
}

func publishAndApply(ctx context.Context, client *bus.Client, ledger taskledger.Ledger, ev protocol.Event) error {
	if client != nil {
		if err := client.PublishEvent(ctx, ev); err != nil {
			return err
		}
	}
	_, err := ledger.Apply(ev)
	return err
}
