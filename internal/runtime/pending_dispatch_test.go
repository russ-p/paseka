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
