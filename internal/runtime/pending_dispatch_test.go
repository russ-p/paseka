package runtime_test

import (
	"context"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

type fakeReplayer struct {
	events []protocol.Event
}

func (f fakeReplayer) ReplayTrace(string) ([]protocol.Event, error) {
	return f.events, nil
}

func TestReactorDefersDirectDispatchWithoutTouchingTaskStatus(t *testing.T) {
	cases := []struct {
		name string
		ev   func(t *testing.T) protocol.Event
	}{
		{
			name: "code proposal",
			ev: func(t *testing.T) protocol.Event {
				t.Helper()
				ev, err := protocol.NewEvent("trace-1", "builder-1", 1, protocol.EventMutation, protocol.MutationPayload{
					Kind:   protocol.MutationCodeProposal,
					TaskID: "task-1",
					Diff:   "+func Login() {}",
				})
				if err != nil {
					t.Fatal(err)
				}
				return ev
			},
		},
		{
			name: "verification failed",
			ev: func(t *testing.T) protocol.Event {
				t.Helper()
				ev, err := protocol.NewEvent("trace-1", "guard-1", 1, protocol.EventVerification, protocol.VerificationPayload{
					Kind:    protocol.VerificationFailed,
					TaskID:  "task-1",
					Summary: "tests failed",
				})
				if err != nil {
					t.Fatal(err)
				}
				return ev
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := tc.ev(t)
			ledger := taskledger.NewMemoryLedger()
			seedWaitingReviewTask(t, ledger, "task-1")
			exhaustEnergy(t, ledger)

			bees := map[string]colony.Bee{
				"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
					{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
				}},
			}
			first := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
			firstRec := &recordingAdapter{}
			first.Dispatcher().RegisterAdapter("cursor", firstRec)
			if err := first.ProcessEvent(context.Background(), ev); err != nil {
				t.Fatal(err)
			}
			if firstRec.calls != 0 {
				t.Fatalf("adapter calls = %d before energy.add, want 0", firstRec.calls)
			}
			assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
			assertPendingDirect(t, ledger, taskledger.PendingDispatchPending)

			// A new reactor over the same ledger proves deferred work survives a restart.
			second := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
			rec := &recordingAdapter{}
			second.Dispatcher().RegisterAdapter("cursor", rec)
			add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
			if err := second.ProcessEvent(context.Background(), add); err != nil {
				t.Fatal(err)
			}
			if rec.calls != 1 {
				t.Fatalf("adapter calls = %d after energy.add, want 1", rec.calls)
			}
			if rec.lastReq.Bee != "guard" {
				t.Fatalf("bee = %q, want guard", rec.lastReq.Bee)
			}
			if rec.lastReq.TaskID != "task-1" {
				t.Fatalf("taskId = %q, want task-1", rec.lastReq.TaskID)
			}
			assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
			assertPendingDirect(t, ledger, taskledger.PendingDispatchConsumed)
		})
	}
}

func seedWaitingReviewTask(t *testing.T, ledger *taskledger.MemoryLedger, taskID string) {
	t.Helper()
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: taskID,
			Title:  "implement",
			Body:   "do work",
			Bee:    "builder",
			Review: protocol.TaskReviewRequired,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: taskID,
		Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(status); err != nil {
		t.Fatal(err)
	}
}

func exhaustEnergy(t *testing.T, ledger *taskledger.MemoryLedger) {
	t.Helper()
	if err := ledger.SeedEnergy("trace-1", 1); err != nil {
		t.Fatal(err)
	}
	consume, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
		Reason: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(consume); err != nil {
		t.Fatal(err)
	}
}

func newPendingDispatchReactor(t *testing.T, ledger taskledger.Ledger, replayer fakeReplayer, bees map[string]colony.Bee) *runtime.Reactor {
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
		Ledger:     ledger,
		Replayer:   replayer,
	})
}

func assertTaskStatus(t *testing.T, ledger taskledger.Ledger, taskID string, want protocol.TaskStatus) {
	t.Helper()
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks[taskID].Status; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
}

func assertPendingDirect(t *testing.T, ledger taskledger.Ledger, want taskledger.PendingDispatchStatus) {
	t.Helper()
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.PendingDispatches {
		if item.Kind == taskledger.PendingDispatchDirect {
			if item.Status != want {
				t.Fatalf("pending direct status = %q, want %q", item.Status, want)
			}
			return
		}
	}
	t.Fatal("direct pending dispatch not found")
}

func TestKillAbandonsPendingDirectDispatchAndTopUpDoesNotRevive(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "builder-1", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:   protocol.MutationCodeProposal,
		TaskID: "task-1",
		Diff:   "+func Login() {}",
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)

	bees := map[string]colony.Bee{
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	}
	first := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	first.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := first.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchPending)

	kill := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"system.kill","reason":"stop"}`)
	if err := first.ProcessEvent(context.Background(), kill); err != nil {
		t.Fatal(err)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCancelled)
	assertPendingDirect(t, ledger, taskledger.PendingDispatchAbandoned)
	assertPendingDirectCount(t, ledger, 1)

	// A top-up on a fresh reactor over the same ledger must not revive the
	// work the kill closed, even though the source event is still replayable.
	second := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	rec := &recordingAdapter{}
	second.Dispatcher().RegisterAdapter("cursor", rec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := second.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d after top-up, want killed work to stay abandoned", rec.calls)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchAbandoned)
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCancelled)
}

func TestResumeDrainSkipsAbandonedPendingDirectDispatch(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "builder-1", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:   protocol.MutationCodeProposal,
		TaskID: "task-1",
		Diff:   "+func Login() {}",
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)

	bees := map[string]colony.Bee{
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	}
	first := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	first.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := first.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	dispatchID := pendingDirectID(t, ledger)

	// Abandon the entry directly, as stipend supersession does: the trail is
	// still alive and has a reserve after the top-up below, so only the drain's
	// status check can keep this work from being dispatched.
	if err := ledger.ResolvePendingDispatch("trace-1", dispatchID, taskledger.PendingDispatchAbandoned); err != nil {
		t.Fatal(err)
	}

	second := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	rec := &recordingAdapter{}
	second.Dispatcher().RegisterAdapter("cursor", rec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := second.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d after top-up, want abandoned entry skipped", rec.calls)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchAbandoned)
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
}

func TestStipendAbandonsPendingDirectDispatchAndTopUpDoesNotRevive(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "builder-1", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:   protocol.MutationCodeProposal,
		TaskID: "task-1",
		Diff:   "+func Login() {}",
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)

	bees := map[string]colony.Bee{
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	}
	first := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	first.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := first.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchPending)

	// A stipend starts a new allocation, so it closes the previous one's
	// pending direct reactions in the same reduction that sets the reserve.
	stipend := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.stipend","amount":4}`)
	if err := first.ProcessEvent(context.Background(), stipend); err != nil {
		t.Fatal(err)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchAbandoned)
	assertPendingDirectCount(t, ledger, 1)
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)

	// A top-up on a fresh reactor over the same ledger must not resurrect the
	// work the stipend closed, even though the source event is still replayable.
	second := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	rec := &recordingAdapter{}
	second.Dispatcher().RegisterAdapter("cursor", rec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := second.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d after top-up, want stipend-abandoned work to stay closed", rec.calls)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchAbandoned)
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
}

func pendingDirectID(t *testing.T, ledger taskledger.Ledger) string {
	t.Helper()
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.PendingDispatches {
		if item.Kind == taskledger.PendingDispatchDirect {
			return item.ID
		}
	}
	t.Fatal("direct pending dispatch not found")
	return ""
}

func assertPendingDirectCount(t *testing.T, ledger taskledger.Ledger, want int) {
	t.Helper()
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(snap.PendingDispatches); got != want {
		t.Fatalf("pending entries = %d, want %d", got, want)
	}
}

func TestResumeMarksMissingSourceEventUnresolved(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "builder-1", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:   protocol.MutationCodeProposal,
		TaskID: "task-1",
		Diff:   "+func Login() {}",
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)

	bees := map[string]colony.Bee{
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	}
	first := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	first.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := first.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchPending)

	// The replayer no longer holds the source event: the entry must become
	// visible unresolved work instead of being dispatched or dropped.
	second := newPendingDispatchReactor(t, ledger, fakeReplayer{}, bees)
	rec := &recordingAdapter{}
	second.Dispatcher().RegisterAdapter("cursor", rec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := second.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0 for missing source event", rec.calls)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.PendingDispatches) != 1 {
		t.Fatalf("pending entries = %d, want the unresolved entry to stay visible", len(snap.PendingDispatches))
	}
	item := snap.PendingDispatches[0]
	if item.Status != taskledger.PendingDispatchUnresolved {
		t.Fatalf("status = %q, want unresolved", item.Status)
	}
	if item.Diagnostic == "" {
		t.Fatal("unresolved entry must carry a diagnostic")
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)

	// A later top-up must not retry work whose evidence is gone, even when the
	// event is recoverable again.
	third := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	retryRec := &recordingAdapter{}
	third.Dispatcher().RegisterAdapter("cursor", retryRec)
	if err := third.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if retryRec.calls != 0 {
		t.Fatalf("adapter calls = %d after top-up, want unresolved work to stay unretried", retryRec.calls)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchUnresolved)
}
