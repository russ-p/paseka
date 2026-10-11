package review_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/taskledger"
)

func waitingReviewLedger(t *testing.T, traceID, taskID string, policy protocol.TaskReviewPolicy) taskledger.Ledger {
	t.Helper()
	ledger := taskledger.NewMemoryLedger()
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: taskID, Title: "Work", Review: policy}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: taskID, Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}
	return ledger
}

func TestDiscardClosesWaitingReviewTaskTerminally(t *testing.T) {
	traceID := "trace-1"
	ledger := waitingReviewLedger(t, traceID, "task-1", protocol.TaskReviewRequired)
	pub := &recordingPublisher{}

	if err := review.Discard(context.Background(), pub, ledger, review.DiscardInput{
		TraceID: traceID,
		TaskID:  "task-1",
		Reason:  "Superseded by a different approach.",
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	task := snap.Tasks["task-1"]
	if task.Status != protocol.TaskStatusCancelled {
		t.Fatalf("status = %q, want cancelled", task.Status)
	}
	if task.Summary != "Superseded by a different approach." {
		t.Fatalf("summary = %q", task.Summary)
	}

	// No rework task, no human.feedback, no completion.
	for _, ev := range pub.events {
		if ev.Type == protocol.EventVerification {
			t.Fatalf("discard must not publish a completion: %+v", ev)
		}
		if ev.Type == protocol.EventInsight {
			var payload protocol.NarrativeInsightPayload
			if err := json.Unmarshal(ev.Payload, &payload); err == nil && payload.Kind == protocol.InsightHumanFeedback {
				t.Fatalf("discard must not publish human.feedback: %+v", ev)
			}
		}
	}
}

func TestDiscardDefaultsSummaryAndDoesNotActivateFinalGate(t *testing.T) {
	traceID := "trace-1"
	ledger := waitingReviewLedger(t, traceID, "task-1", protocol.TaskReviewRequired)

	if err := review.Discard(context.Background(), nil, ledger, review.DiscardInput{
		TraceID: traceID,
		TaskID:  "task-1",
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks["task-1"].Summary; got != review.DiscardSummary {
		t.Fatalf("summary = %q, want %q", got, review.DiscardSummary)
	}
	if _, ok := taskledger.FindFinalReviewTask(snap); ok {
		t.Fatal("discard must not open a final merge gate")
	}
}

func TestDiscardRejectsNonWaitingReviewTargets(t *testing.T) {
	traceID := "trace-1"
	ledger := taskledger.NewMemoryLedger()
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: "task-1", Title: "Work", Review: protocol.TaskReviewRequired}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}

	if err := review.Discard(context.Background(), nil, ledger, review.DiscardInput{TraceID: traceID, TaskID: "task-1"}); err == nil {
		t.Fatal("expected planned task discard to fail")
	}
	if err := review.Discard(context.Background(), nil, ledger, review.DiscardInput{TraceID: traceID, TaskID: "missing"}); err == nil {
		t.Fatal("expected missing task discard to fail")
	}
}
