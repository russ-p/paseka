package runtime_test

import (
	"context"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestReactorReviewRequiredWaitsAfterAFKRun(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "implement",
			Bee:    "builder",
			Review: protocol.TaskReviewRequired,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1", Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	rec := &recordingAdapter{result: &adapters.RunResult{Status: "completed", Summary: "done"}}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	snap, err := r.Ledger().Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("status = %q, want waiting_review", snap.Tasks["task-1"].Status)
	}
}

func TestReactorNormalTaskAutoCompletes(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "implement",
			Bee:    "builder",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1", Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	rec := &recordingAdapter{result: &adapters.RunResult{Status: "completed", Summary: "done"}}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	snap, err := r.Ledger().Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("status = %q, want completed", snap.Tasks["task-1"].Status)
	}
	final := snap.Tasks[taskledger.FinalReviewTaskID]
	if final.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("final review status = %q, want waiting_review", final.Status)
	}
}

func TestReactorActivatesFinalReviewAfterAFKChain(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "work", Bee: "builder"},
			{TaskID: "review", Title: "Merge gate", Review: protocol.TaskReviewFinal, DependsOn: []string{"task-1"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1", Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	rec := &recordingAdapter{result: &adapters.RunResult{Status: "completed", Summary: "done"}}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	snap, err := r.Ledger().Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["review"].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("review status = %q, want waiting_review", snap.Tasks["review"].Status)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1 (final gate should not dispatch)", rec.calls)
	}
}
