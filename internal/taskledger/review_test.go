package taskledger_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestApplyEventTaskStatus(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusRunning},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  "task-1",
		Status:  protocol.TaskStatusWaitingReview,
		Summary: "needs eyes",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("status = %q", res.Trace.Tasks["task-1"].Status)
	}
	if res.Trace.Tasks["task-1"].Summary != "needs eyes" {
		t.Fatalf("summary = %q", res.Trace.Tasks["task-1"].Summary)
	}
}

func TestApplyEventTaskStatusClearsSummary(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID:  "task-1",
				Status:  protocol.TaskStatusBlocked,
				Summary: protocol.HoneyReserveExhaustedSummary,
			},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: "task-1",
		Status: protocol.TaskStatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q", res.Trace.Tasks["task-1"].Status)
	}
	if res.Trace.Tasks["task-1"].Summary != "" {
		t.Fatalf("summary = %q, want cleared", res.Trace.Tasks["task-1"].Summary)
	}
}

func TestApplyEventTaskPlanPreservesReview(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1"}
	ev, err := protocol.NewEvent("trace-1", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Gate", Review: protocol.TaskReviewFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Review != protocol.TaskReviewFinal {
		t.Fatalf("review = %q", res.Trace.Tasks["task-1"].Review)
	}
}

func TestAllAFKTasksCompletedRequiresReviewRequiredDone(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"task-2": {TaskID: "task-2", Review: protocol.TaskReviewRequired, Status: protocol.TaskStatusWaitingReview},
		},
	}
	if taskledger.AllAFKTasksCompleted(trace) {
		t.Fatal("expected false while review: required task is not completed")
	}
	trace.Tasks["task-2"] = taskledger.TaskSnapshot{TaskID: "task-2", Review: protocol.TaskReviewRequired, Status: protocol.TaskStatusCompleted}
	if !taskledger.AllAFKTasksCompleted(trace) {
		t.Fatal("expected true when all non-final tasks are completed")
	}
}

func TestAllAFKTasksCompletedIgnoresReviewGate(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"review": {TaskID: "review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusPlanned},
		},
	}
	if !taskledger.AllAFKTasksCompleted(trace) {
		t.Fatal("expected AFK tasks completed")
	}
}

func TestEligiblePlannedSkipsFinalUntilAFKDone(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusPlanned},
			"review": {TaskID: "review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusPlanned},
		},
	}
	if len(taskledger.EligiblePlanned(trace)) != 1 {
		t.Fatalf("eligible = %d, want 1 non-final task", len(taskledger.EligiblePlanned(trace)))
	}
	trace.Tasks["task-1"] = taskledger.TaskSnapshot{TaskID: "task-1", Status: protocol.TaskStatusCompleted}
	eligible := taskledger.EligiblePlanned(trace)
	if len(eligible) != 1 || eligible[0].TaskID != "review" {
		t.Fatalf("eligible = %+v, want final review task", eligible)
	}
}

func TestHasIsolatedProposal(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", ProposalWorkspace: protocol.ProposalWorkspaceRoot},
		},
	}
	if taskledger.HasIsolatedProposal(trace) {
		t.Fatal("root proposal must not count as isolated merge candidate")
	}
	trace.Tasks["task-1"] = taskledger.TaskSnapshot{
		TaskID:            "task-1",
		ProposalWorkspace: protocol.ProposalWorkspaceIsolated,
	}
	if !taskledger.HasIsolatedProposal(trace) {
		t.Fatal("expected isolated proposal")
	}
}
