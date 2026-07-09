package runtime_test

import (
	"context"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestReactorBlocksDispatchWhenEnergyExhausted(t *testing.T) {
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
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 1); err != nil {
		t.Fatal(err)
	}
	mustConsumeEnergy(t, ledger, "trace-1", 1)

	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0 when energy exhausted", rec.calls)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	task := snap.Tasks["task-1"]
	if task.Status != protocol.TaskStatusBlocked {
		t.Fatalf("status = %q, want blocked", task.Status)
	}
}

func TestReactorUnblocksAfterEnergyAdd(t *testing.T) {
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
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	add, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 1); err != nil {
		t.Fatal(err)
	}
	mustConsumeEnergy(t, ledger, "trace-1", 1)

	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d before add, want 0", rec.calls)
	}
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls == 0 {
		t.Fatal("expected dispatch after energy.add")
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Summary == protocol.HoneyReserveExhaustedSummary {
		t.Fatal("expected honey-exhausted summary cleared after unblock")
	}
}

func TestReactorSkipsLocalEnergyConsumeEcho(t *testing.T) {
	r := newTestReactor(t, map[string]colony.Bee{})
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 1); err != nil {
		t.Fatal(err)
	}

	ev, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
		Reason: "task.dispatch",
		TaskID: "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// applyAndSync order: apply locally, remember fingerprint, then publish.
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
	r.RememberLocalEvent(ev)

	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 0 {
		t.Fatalf("remaining = %d, want 0 (echo must not double-consume)", snap.EnergyRemaining)
	}
}

func TestReactorApplyAndSyncFailsBeforePublishWhenInsufficient(t *testing.T) {
	r := newTestReactor(t, map[string]colony.Bee{})
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 1); err != nil {
		t.Fatal(err)
	}
	mustConsumeEnergy(t, ledger, "trace-1", 1)

	ev, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
		Reason: "task.dispatch",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = r.ApplyAndSyncForTest(context.Background(), ev)
	if err == nil {
		t.Fatal("expected apply failure when reserve is empty")
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 0 {
		t.Fatalf("remaining = %d, want 0", snap.EnergyRemaining)
	}
}

func mustConsumeEnergy(t *testing.T, ledger *taskledger.MemoryLedger, traceID string, amount int) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "test", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: amount,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
}
