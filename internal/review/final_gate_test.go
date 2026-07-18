package review_test

import (
	"context"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestActivateFinalReviewGateSkipsWhenNothingToMerge(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-1"
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Scout only"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	completed, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(completed); err != nil {
		t.Fatal(err)
	}

	if err := review.ActivateFinalReviewGate(context.Background(), nil, ledger, colony.Context{}, traceID); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
		t.Fatal("expected no synthetic _review when nothing to merge")
	}
}

func TestActivateFinalReviewGateSynthesizesWhenIsolatedProposal(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-1"
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Build", Bee: "builder"},
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
	completed, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(completed); err != nil {
		t.Fatal(err)
	}

	if err := review.ActivateFinalReviewGate(context.Background(), nil, ledger, colony.Context{}, traceID); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	final := snap.Tasks[taskledger.FinalReviewTaskID]
	if final.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("final status = %q, want waiting_review", final.Status)
	}
}

func TestActivateFinalReviewGateAutoCompletesEmptyExplicitFinal(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-1"
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Docs"},
			{TaskID: "merge", Title: "Merge", Review: protocol.TaskReviewFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	completed, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(completed); err != nil {
		t.Fatal(err)
	}

	if err := review.ActivateFinalReviewGate(context.Background(), nil, ledger, colony.Context{}, traceID); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	final := snap.Tasks["merge"]
	if final.Status != protocol.TaskStatusCompleted {
		t.Fatalf("explicit final status = %q, want completed", final.Status)
	}
	if final.Summary != "Nothing to merge — skipped final review gate" {
		t.Fatalf("summary = %q", final.Summary)
	}
}

func TestActivateFinalReviewGateAutoCompletesHollowWaitingReview(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-1"
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Scout"},
			{TaskID: taskledger.FinalReviewTaskID, Title: "Merge", Review: protocol.TaskReviewFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"task-1"} {
		completed, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
			Kind: protocol.TaskEventCompleted, TaskID: taskID, Status: protocol.TaskStatusCompleted,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.Apply(completed); err != nil {
			t.Fatal(err)
		}
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: taskledger.FinalReviewTaskID, Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}

	if err := review.ActivateFinalReviewGate(context.Background(), nil, ledger, colony.Context{}, traceID); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusCompleted {
		t.Fatalf("status = %q, want completed", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
}
