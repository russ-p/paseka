package review_test

import (
	"context"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestApproveRequiredSkipsFinalGateWhenNothingToMerge(t *testing.T) {
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
	if _, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
		t.Fatal("expected no synthetic _review when approve closes a soft gate with nothing to merge")
	}
}

func TestApproveRequiredOpensFinalGateWhenIsolatedProposal(t *testing.T) {
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
	mutation, err := protocol.NewEvent(traceID, "builder-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalIsolated,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceIsolated,
		Diff:      "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(mutation); err != nil {
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

	approveRes, err := review.Approve(context.Background(), colonyCtx(t), nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if approveRes.CommitSHA != "" {
		t.Fatalf("commit = %q, want empty (no merge for required review)", approveRes.CommitSHA)
	}
}

func TestRejectRequiresWaitingReviewGate(t *testing.T) {
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

	err = review.Reject(context.Background(), nil, ledger, review.RejectInput{
		TraceID: traceID,
		TaskID:  "task-1",
	})
	if err == nil || !strings.Contains(err.Error(), "waiting_review") {
		t.Fatalf("Reject planned task err = %v", err)
	}

	err = review.Reject(context.Background(), nil, ledger, review.RejectInput{
		TraceID: traceID,
		TaskID:  "missing",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Reject missing task err = %v", err)
	}
}

func colonyCtx(t *testing.T) colony.Context {
	t.Helper()
	return colony.Context{ColonyRoot: t.TempDir(), Slug: "test"}
}
