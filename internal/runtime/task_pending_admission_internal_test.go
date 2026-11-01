package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// A resume that finds the reserve already spent by another actor re-defers the
// task: the entry stays active and the drain does not claim a dispatch it never
// admitted. This is the window between the drain's reserve check and the
// energy gate.
func TestResumeTaskPendingDispatchKeepsEntryActiveWhenItReDefers(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	ledger := taskledger.NewMemoryLedger()

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
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: "task-1",
		Status: protocol.TaskStatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(ready); err != nil {
		t.Fatal(err)
	}
	// Seed then exhaust the reserve: an unseeded trail would seed fresh honey in
	// consumeEnergy, which would admit the dispatch instead of re-deferring it.
	if err := ledger.SeedEnergy("trace-1", 1); err != nil {
		t.Fatal(err)
	}
	consume, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
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

	item := taskledger.PendingDispatch{
		ID:        "task:task-1:builder",
		Kind:      taskledger.PendingDispatchTask,
		TaskID:    "task-1",
		Bee:       "builder",
		Reason:    protocol.HoneyReserveExhaustedSummary,
		Status:    taskledger.PendingDispatchPending,
		CreatedAt: time.Now().UTC(),
	}
	if added, err := ledger.DeferPendingDispatch("trace-1", item); err != nil {
		t.Fatal(err)
	} else if !added {
		t.Fatal("expected the deferral to be recorded")
	}

	r := NewTestReactor(TestReactorOptions{
		ColonyRoot: repo,
		Dispatcher: NewDispatcher(),
		Ledger:     ledger,
	})

	// The ledger has no reserve, so the dispatch re-defers instead of starting.
	admitted, err := r.resumeTaskPendingDispatch(context.Background(), "trace-1", item)
	if err != nil {
		t.Fatal(err)
	}
	if admitted {
		t.Fatal("a re-deferred dispatch must not be reported as dispatched")
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks["task-1"].Status; got != protocol.TaskStatusReady {
		t.Fatalf("task status = %q, want ready while the dispatch re-defers", got)
	}
	if active := taskledger.ActivePendingDispatches(snap); len(active) != 1 || active[0].ID != item.ID {
		t.Fatalf("active pending = %+v, want the entry left active", active)
	}
}
