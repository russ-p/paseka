package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestReactorDirectDispatchMutation(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposal,
		Summary: "add auth endpoint",
		Diff:    "+func Login() {}",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"guard": {Role: "guard", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "guard" {
		t.Fatalf("bee = %q, want guard", rec.lastReq.Bee)
	}
	if rec.lastReq.TaskID != "task-1" {
		t.Fatalf("taskId = %q", rec.lastReq.TaskID)
	}
	if rec.lastReq.Task == "" {
		t.Fatal("expected task body from mutation payload")
	}
}

func TestReactorSkipsDuplicateDirectEvent(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposal,
		Summary: "proposal",
		Diff:    "+line",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"guard": {Role: "guard", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1 (duplicate bus delivery should be skipped)", rec.calls)
	}
}

func TestReactorDirectDispatchVerificationFailed(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationFailed,
		TaskID:  "task-2",
		Summary: "tests failed",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "builder" {
		t.Fatalf("bee = %q, want builder", rec.lastReq.Bee)
	}
}

func TestReactorSkipsTaskReadyWhenBeeNotSubscribed(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "work",
			Bee:    "guard",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "guard",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"guard": {Role: "guard", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "" {
		t.Fatalf("guard should not run task.ready, got bee %q", rec.lastReq.Bee)
	}
}

func TestReactorTaskReadyDispatchWithSubscribe(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "implement",
			Body:   "do work",
			Bee:    "builder",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "builder" {
		t.Fatalf("bee = %q, want builder", rec.lastReq.Bee)
	}
}

func TestAdvisoryPublishWarning(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	content := `role: builder
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
  trust: true
  force: true
publishes:
  - type: MUTATION
    kind: code.proposal
`
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	d.SetPublisher(pub, false)
	d.SetBeeRegistry(reg)

	rec.result = &adapters.RunResult{
		Status: "completed",
		Events: []protocol.Event{
			mustEvent(t, "trace-abc", "agent-1", protocol.EventInsight, `{"kind":"task.plan","tasks":[]}`),
		},
	}

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-abc",
		Task:       "implement auth",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected advisory warning for undeclared publish")
	}
}

func newTestReactor(t *testing.T, bees map[string]colony.Bee) *runtime.Reactor {
	t.Helper()
	root := t.TempDir()
	mustWriteTestColony(t, root, bees)
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	return runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
	})
}
