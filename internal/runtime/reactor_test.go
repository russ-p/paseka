package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
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
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
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
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
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

func TestReactorDirectDispatchVerificationSuccess(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "guard-1", 0, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationSuccess,
		TaskID:  "task-3",
		Summary: "all checks passed",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"receiver": {Role: "receiver", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.success"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "receiver" {
		t.Fatalf("bee = %q, want receiver", rec.lastReq.Bee)
	}
	if rec.lastReq.TaskID != "task-3" {
		t.Fatalf("taskId = %q", rec.lastReq.TaskID)
	}
	if !strings.Contains(rec.lastReq.Task, "commit") {
		t.Fatalf("task body = %q, want commit instructions", rec.lastReq.Task)
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

func TestReactorSkipsDirectDispatchSamePublisherBee(t *testing.T) {
	r := newTestReactor(t, map[string]colony.Bee{
		"receiver": {Role: "receiver", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.success"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	publisherID := "receiver-publisher"
	runDir := runs.Dir{ColonyRoot: r.ColonyRoot(), TraceID: "trace-1", AgentID: publisherID}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-1",
		AgentID:         publisherID,
		Bee:             "receiver",
		Adapter:         "cursor",
		Workspace:       r.ColonyRoot(),
		ColonyRoot:      r.ColonyRoot(),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	ev, err := protocol.NewEvent("trace-1", publisherID, 0, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationSuccess,
		Summary: "receiver echoed verification.success",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0 (same-bee publisher must not re-dispatch)", rec.calls)
	}
}

func TestReactorRedispatchesGuardOnReworkCodeProposal(t *testing.T) {
	r := newTestReactor(t, map[string]colony.Bee{
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	first, err := protocol.NewEvent("trace-1", "builder-1", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposal,
		Summary: "broken fix",
		Diff:    "+broken",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	rework, err := protocol.NewEvent("trace-1", "builder-2", 2, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposal,
		Summary: "correct fix",
		Diff:    "+fixed",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.ProcessEvent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), rework); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2 (rework code.proposal must re-dispatch guard)", rec.calls)
	}
}

func TestReactorRedispatchesBuilderOnReworkVerificationFailed(t *testing.T) {
	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	first, err := protocol.NewEvent("trace-1", "guard-1", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationFailed,
		TaskID:  "task-1",
		Summary: "tests failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := protocol.NewEvent("trace-1", "guard-2", 2, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationFailed,
		TaskID:  "task-1",
		Summary: "tests failed again",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.ProcessEvent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2 (rework verification.failed must re-dispatch builder)", rec.calls)
	}
}

func TestReactorDedupesDirectDispatchByTaskID(t *testing.T) {
	r := newTestReactor(t, map[string]colony.Bee{
		"receiver": {Role: "receiver", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.success"}, Dispatch: colony.DispatchDirect},
		}},
	})
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	first, err := protocol.NewEvent("trace-1", "guard-1", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationSuccess,
		TaskID:  "task-1",
		Summary: "approved",
	})
	if err != nil {
		t.Fatal(err)
	}
	echo, err := protocol.NewEvent("trace-1", "receiver-1", 2, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationSuccess,
		TaskID:  "task-1",
		Summary: "echoed success",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondTask, err := protocol.NewEvent("trace-1", "guard-2", 3, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationSuccess,
		TaskID:  "task-2",
		Summary: "another task approved",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.ProcessEvent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), echo); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d after echo, want 1", rec.calls)
	}
	if err := r.ProcessEvent(context.Background(), secondTask); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d after second task, want 2", rec.calls)
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
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
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

func TestReactorSyncsTaskProjection(t *testing.T) {
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

	r := newTestReactor(t, map[string]colony.Bee{})
	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}

	got, err := runs.LoadTraceTasksFromFS(r.ColonyRoot(), "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	task := got.Tasks["task-1"]
	if task.Status != protocol.TaskStatusPlanned || task.Body != "do work" {
		t.Fatalf("task = %+v", task)
	}
}

func TestAdvisoryPublishWarning(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	content := `role: builder
adapter: cursor
prompt_template: builder.md
worktree: true
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
