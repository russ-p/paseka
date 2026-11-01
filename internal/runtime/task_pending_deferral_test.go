package runtime_test

import (
	"context"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func taskDispatchBees() map[string]colony.Bee {
	return map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	}
}

func mustTaskPlan(t *testing.T, taskID string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: taskID,
			Title:  "implement",
			Body:   "do work",
			Bee:    "builder",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func mustTaskReady(t *testing.T, taskID string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: taskID,
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// A re-kick of the same deferred task reaction dedupes on task:<taskId>:<bee>.
// The first entry keeps the source identity it was recorded with, because that
// identity is the attempt dispatch marker a later resume relies on.
func TestRepeatedTaskDeferralKeepsFirstSourceEventIdentity(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	firstReady := mustTaskReady(t, "task-1")
	secondReady := mustTaskReady(t, "task-1")
	if firstReady.ID == secondReady.ID {
		t.Fatal("test needs two distinct source events")
	}
	if err := r.ProcessEvent(context.Background(), mustTaskPlan(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), firstReady); err != nil {
		t.Fatal(err)
	}

	// The task must look dispatchable again before the same reaction is
	// deferred a second time; a plain duplicate task.ready on a ready task is
	// not a dispatch at all.
	failed, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: "task-1",
		Status: protocol.TaskStatusFailed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(failed); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), secondReady); err != nil {
		t.Fatal(err)
	}

	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0 while the reserve is exhausted", rec.calls)
	}
	pending := pendingTaskDispatches(mustSnapshot(t, ledger, "trace-1"))
	if len(pending) != 1 {
		t.Fatalf("pending task entries = %d, want exactly one", len(pending))
	}
	if pending[0].SourceEventID != firstReady.ID {
		t.Fatalf("source event = %q, want the first kick %q", pending[0].SourceEventID, firstReady.ID)
	}
	if pending[0].Status != taskledger.PendingDispatchPending {
		t.Fatalf("status = %q, want pending", pending[0].Status)
	}
}

// Kill is the hard stop: it abandons deferred task work in the kill mutation,
// and a ready event arriving afterwards records no new pending work.
func TestKilledTrailRecordsNoNewPendingTaskWork(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), mustTaskPlan(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), mustTaskReady(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	if got := len(pendingTaskDispatches(mustSnapshot(t, ledger, "trace-1"))); got != 1 {
		t.Fatalf("pending task entries = %d, want 1 before kill", got)
	}

	kill := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"system.kill","reason":"stop"}`)
	if err := r.ProcessEvent(context.Background(), kill); err != nil {
		t.Fatal(err)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCancelled)
	pending := pendingTaskDispatches(mustSnapshot(t, ledger, "trace-1"))
	if len(pending) != 1 || pending[0].Status != taskledger.PendingDispatchAbandoned {
		t.Fatalf("pending task entries = %+v, want the deferred entry abandoned", pending)
	}

	if err := r.ProcessEvent(context.Background(), mustTaskReady(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	pending = pendingTaskDispatches(mustSnapshot(t, ledger, "trace-1"))
	if len(pending) != 1 {
		t.Fatalf("pending task entries = %d after kill, want no new work", len(pending))
	}
	if active := taskledger.ActivePendingDispatches(mustSnapshot(t, ledger, "trace-1")); len(active) != 0 {
		t.Fatalf("active pending = %+v, want none on a killed trail", active)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0 on a killed trail", rec.calls)
	}
}

func pendingTaskDispatches(snap taskledger.TraceSnapshot) []taskledger.PendingDispatch {
	var pending []taskledger.PendingDispatch
	for _, item := range snap.PendingDispatches {
		if item.Kind == taskledger.PendingDispatchTask {
			pending = append(pending, item)
		}
	}
	return pending
}
