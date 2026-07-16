package runtime_test

import (
	"context"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func builderBeeWithProposalPublish() colony.Bee {
	return colony.Bee{
		Role: "builder",
		Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		},
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
		},
	}
}

func receiverBeeWithTaskCompleted() colony.Bee {
	return colony.Bee{
		Role: "receiver",
		Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.success"}, Dispatch: colony.DispatchDirect},
		},
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: string(protocol.TaskEventCompleted)}},
		},
	}
}

func TestReactorDefersWhenPublisherAndCodeProposal(t *testing.T) {
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
		"builder":  builderBeeWithProposalPublish(),
		"receiver": receiverBeeWithTaskCompleted(),
	})
	rec := &recordingAdapter{result: &adapters.RunResult{
		Status:  "completed",
		Summary: "done",
		Artifacts: []adapters.Artifact{{
			Kind:    "diff",
			Content: "+func Login() {}",
		}},
	}}
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
	if _, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
		t.Fatal("final review gate should not open before task.completed")
	}
}

func TestReactorFallbackCompletesWhenNoTaskCompletedPublisher(t *testing.T) {
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
		"builder": builderBeeWithProposalPublish(),
	})
	rec := &recordingAdapter{result: &adapters.RunResult{
		Status: "completed",
		Artifacts: []adapters.Artifact{{
			Kind:    "diff",
			Content: "+line",
		}},
	}}
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
}

func TestReactorScoutLikeDoesNotDeferWithIncidentalDiff(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "classify",
			Bee:    "scout",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1", Bee: "scout",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"scout": {
			Role: "scout",
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
			},
		},
		"receiver": receiverBeeWithTaskCompleted(),
	})
	rec := &recordingAdapter{result: &adapters.RunResult{
		Status: "completed",
		Artifacts: []adapters.Artifact{{
			Kind:    "diff",
			Content: "+incidental",
		}},
	}}
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
		t.Fatalf("status = %q, want completed (scout empty publishes must not open gate)", snap.Tasks["task-1"].Status)
	}
}

func TestReactorPublisherWithoutProposalAutoCompletes(t *testing.T) {
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
		"builder":  builderBeeWithProposalPublish(),
		"receiver": receiverBeeWithTaskCompleted(),
	})
	rec := &recordingAdapter{result: &adapters.RunResult{Status: "completed", Summary: "no diff"}}
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
}

func TestReactorReceiverClosesDeferredTask(t *testing.T) {
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
		"builder":  builderBeeWithProposalPublish(),
		"receiver": receiverBeeWithTaskCompleted(),
	})
	rec := &recordingAdapter{result: &adapters.RunResult{
		Status: "completed",
		Artifacts: []adapters.Artifact{{
			Kind:    "diff",
			Content: "+line",
		}},
	}}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	completed, err := protocol.NewEvent("trace-1", "receiver-1", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:    protocol.TaskEventCompleted,
		TaskID:  "task-1",
		Status:  protocol.TaskStatusCompleted,
		Summary: "committed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), completed); err != nil {
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

func TestReactorRequiredWinsOverDefer(t *testing.T) {
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
		"builder":  builderBeeWithProposalPublish(),
		"receiver": receiverBeeWithTaskCompleted(),
	})
	rec := &recordingAdapter{result: &adapters.RunResult{
		Status:  "completed",
		Summary: "done",
		Artifacts: []adapters.Artifact{{
			Kind:    "diff",
			Content: "+line",
		}},
	}}
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
	task := snap.Tasks["task-1"]
	if task.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("status = %q, want waiting_review", task.Status)
	}
	if task.Summary != "done" {
		t.Fatalf("summary = %q, want human review summary from run", task.Summary)
	}
}
