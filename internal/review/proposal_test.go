package review_test

import (
	"context"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestApproveRootRequiredDoesNotMerge(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-1"

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Retune hive", Bee: "hivewright", Review: protocol.TaskReviewRequired},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	proposal, err := protocol.NewEvent(traceID, "hivewright-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalRoot,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceRoot,
		Summary:   "cfg change",
		Diff:      "+yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(proposal); err != nil {
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
		t.Fatalf("commit = %q, want empty (root R1 approve must not merge)", commit)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("status = %q, want completed", snap.Tasks["task-1"].Status)
	}
}

func TestShouldMergeOnApproveMatrix(t *testing.T) {
	bees := map[string]colony.Bee{
		"builder": {
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
			},
		},
		"hivewright": {
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
			},
		},
	}

	finalIsolated := taskledger.TaskSnapshot{
		TaskID:            taskledger.FinalReviewTaskID,
		Review:            protocol.TaskReviewFinal,
		ProposalWorkspace: protocol.ProposalWorkspaceIsolated,
	}
	if !review.ShouldMergeOnApprove(finalIsolated, bees) {
		t.Fatal("isolated final gate should merge when worktree present")
	}

	finalRoot := taskledger.TaskSnapshot{
		TaskID:            "task-root",
		Review:            protocol.TaskReviewFinal,
		Bee:               "hivewright",
		ProposalWorkspace: protocol.ProposalWorkspaceRoot,
	}
	if review.ShouldMergeOnApprove(finalRoot, bees) {
		t.Fatal("root proposal must never merge on approve")
	}

	requiredRoot := taskledger.TaskSnapshot{
		TaskID:            "task-1",
		Review:            protocol.TaskReviewRequired,
		ProposalWorkspace: protocol.ProposalWorkspaceRoot,
	}
	if review.ShouldMergeOnApprove(requiredRoot, bees) {
		t.Fatal("root required review must not merge")
	}
}
