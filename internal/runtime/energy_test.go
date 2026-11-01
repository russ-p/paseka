package runtime_test

import (
	"context"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestReactorDefersTaskDispatchWhenEnergyExhausted(t *testing.T) {
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
	if task.Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q, want ready (energy must not write a task status)", task.Status)
	}
	if task.Summary == protocol.HoneyReserveExhaustedSummary {
		t.Fatal("energy exhaustion must not be written into the task summary")
	}
	pending := pendingTaskDispatches(snap)
	if len(pending) != 1 {
		t.Fatalf("pending task entries = %d, want exactly one", len(pending))
	}
	entry := pending[0]
	if entry.ID != "task:task-1:builder" {
		t.Fatalf("pending id = %q, want task:task-1:builder", entry.ID)
	}
	if entry.Kind != taskledger.PendingDispatchTask {
		t.Fatalf("pending kind = %q, want task", entry.Kind)
	}
	if entry.TaskID != "task-1" || entry.Bee != "builder" {
		t.Fatalf("pending entry = %+v, want task-1/builder", entry)
	}
	if entry.SourceEventID != ready.ID {
		t.Fatalf("source event = %q, want %q", entry.SourceEventID, ready.ID)
	}
	if entry.Reason != protocol.HoneyReserveExhaustedSummary {
		t.Fatalf("reason = %q, want %q", entry.Reason, protocol.HoneyReserveExhaustedSummary)
	}
	if entry.Status != taskledger.PendingDispatchPending {
		t.Fatalf("status = %q, want pending", entry.Status)
	}
}

// A legacy honey-blocked task keeps its old meaning: energy.add still unblocks
// it and dispatches the task. New deferrals never create this state.
func TestReactorUnblocksLegacyEnergyBlockedTask(t *testing.T) {
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
	blocked, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  "task-1",
		Status:  protocol.TaskStatusBlocked,
		Summary: protocol.HoneyReserveExhaustedSummary,
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
	for _, ev := range []protocol.Event{plan, blocked} {
		if _, err := ledger.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}

	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls == 0 {
		t.Fatal("expected legacy blocked task to dispatch after energy.add")
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Summary == protocol.HoneyReserveExhaustedSummary {
		t.Fatal("expected honey-exhausted summary cleared after unblock")
	}
}

func TestReactorDoesNotDispatchDeferredTaskAfterEnergyStipend(t *testing.T) {
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
	stipend, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
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
		t.Fatalf("adapter calls = %d before stipend, want 0", rec.calls)
	}
	if err := r.ProcessEvent(context.Background(), stipend); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d after stipend, want 0", rec.calls)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 4 {
		t.Fatalf("remaining = %d, want 4", snap.EnergyRemaining)
	}
	// A stipend starts a new allocation; it supersedes the previous tick's
	// deferred task work instead of resuming it. The entry and the task are
	// closed in the same mutation, so the task cannot look resumable.
	if snap.Tasks["task-1"].Status != protocol.TaskStatusSkipped {
		t.Fatalf("status = %q, want skipped", snap.Tasks["task-1"].Status)
	}
	if got, want := snap.Tasks["task-1"].Summary, protocol.SupersededByStipendSummary(4); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	pending := pendingTaskDispatches(snap)
	if len(pending) != 1 || pending[0].Status != taskledger.PendingDispatchAbandoned {
		t.Fatalf("pending task entries = %+v, want one abandoned", pending)
	}
	if active := taskledger.ActivePendingDispatches(snap); len(active) != 0 {
		t.Fatalf("active pending = %+v, want nothing left to resume", active)
	}

	// A later top-up cannot revive superseded work, even on a fresh reactor.
	fresh := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	freshRec := &recordingAdapter{}
	fresh.Dispatcher().RegisterAdapter("cursor", freshRec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := fresh.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if freshRec.calls != 0 {
		t.Fatalf("adapter calls = %d after top-up, want superseded work to stay closed", freshRec.calls)
	}
	snap, err = ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Status != protocol.TaskStatusSkipped {
		t.Fatalf("status after top-up = %q, want skipped", snap.Tasks["task-1"].Status)
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
