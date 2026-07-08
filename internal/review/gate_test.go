package review_test

import (
	"context"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestApproveRequiredDoesNotCompleteFinalGateAlone(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-1"

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Work", Bee: "builder", Review: protocol.TaskReviewRequired},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-1", Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}

	_, err = review.Approve(context.Background(), colonyCtx(t), nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("task-1 status = %q", snap.Tasks["task-1"].Status)
	}
	final := snap.Tasks[taskledger.FinalReviewTaskID]
	if final.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("final review status = %q, want waiting_review", final.Status)
	}
}

func TestApproveRequiredSkipsMerge(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-1"

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Work", Review: protocol.TaskReviewRequired},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-1", Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}

	commit, err := review.Approve(context.Background(), colonyCtx(t), nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if commit != "" {
		t.Fatalf("commit = %q, want empty (no merge for required review)", commit)
	}
}

func colonyCtx(t *testing.T) colony.Context {
	t.Helper()
	return colony.Context{ColonyRoot: t.TempDir(), Slug: "test"}
}
