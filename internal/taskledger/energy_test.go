package taskledger_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestEnsureEnergySeeded(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1"}
	updated, changed := taskledger.EnsureEnergySeeded(trace, 12)
	if !changed {
		t.Fatal("expected changed")
	}
	if updated.EnergyBudget != 12 || updated.EnergyRemaining != 12 {
		t.Fatalf("energy = %+v", updated)
	}

	again, changed := taskledger.EnsureEnergySeeded(updated, 20)
	if changed {
		t.Fatal("expected no change when already seeded")
	}
	if again.EnergyRemaining != 12 {
		t.Fatalf("remaining = %d", again.EnergyRemaining)
	}
}

func TestApplyEventEnergyAdd(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 2,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 7 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
	if res.Trace.EnergyBudget != 12 {
		t.Fatalf("budget changed = %d", res.Trace.EnergyBudget)
	}
}

func TestApplyEventEnergyAddDoesNotLockDefaultBudget(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks:   map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5", res.Trace.EnergyRemaining)
	}
	if res.Trace.EnergyBudget != 0 {
		t.Fatalf("budget = %d, want 0 so SeedEnergy can apply colony defaults", res.Trace.EnergyBudget)
	}

	seeded, changed := taskledger.EnsureEnergySeeded(res.Trace, 20)
	if !changed {
		t.Fatal("expected SeedEnergy to set custom budget")
	}
	if seeded.EnergyBudget != 20 {
		t.Fatalf("budget = %d, want 20", seeded.EnergyBudget)
	}
	if seeded.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5 (prior add preserved)", seeded.EnergyRemaining)
	}
}

func TestApplyEventEnergyConsume(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 3,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
		Reason: "task.dispatch",
		TaskID: "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 2 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
}

func TestApplyEventEnergyConsumeInsufficient(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 0,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = taskledger.ApplyEvent(trace, ev)
	if err == nil {
		t.Fatal("expected insufficient honey reserve error")
	}
}

func TestMemoryLedgerSeedEnergy(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := ledger.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 12 || snap.EnergyRemaining != 12 {
		t.Fatalf("snap = %+v", snap)
	}
	if err := ledger.SeedEnergy("trace-1", 20); err != nil {
		t.Fatal(err)
	}
	snap, err = ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 12 {
		t.Fatalf("seed should be idempotent, remaining = %d", snap.EnergyRemaining)
	}
}
