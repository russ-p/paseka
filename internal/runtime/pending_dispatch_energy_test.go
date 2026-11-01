package runtime_test

import (
	"context"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// A partial top-up pays for exactly as much of the queue as it can: each
// admitted dispatch charges one honey, and the entries it cannot pay for stay
// pending for the next top-up instead of being claimed as resumed.
func TestPartialTopUpResumesOneAndLeavesTheRestPending(t *testing.T) {
	evA := mustCodeProposal(t, "trace-1", "task-a", 1, "+func A() {}")
	evB := mustCodeProposal(t, "trace-1", "task-b", 2, "+func B() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTasks(t, ledger, "trace-1", "task-a", "task-b")
	exhaustEnergy(t, ledger)

	r := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{evA, evB}}, directGuardBees())
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)
	for _, ev := range []protocol.Event{evA, evB} {
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d before top-up, want 0", rec.calls)
	}
	if got := len(taskledger.ActivePendingDispatches(mustSnapshot(t, ledger, "trace-1"))); got != 2 {
		t.Fatalf("active pending = %d, want 2", got)
	}

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d after one honey, want 1", rec.calls)
	}
	statuses := pendingStatusesByTask(t, ledger, "trace-1")
	if statuses["task-a"] != taskledger.PendingDispatchConsumed {
		t.Fatalf("task-a pending status = %q, want consumed (oldest first)", statuses["task-a"])
	}
	if statuses["task-b"] != taskledger.PendingDispatchPending {
		t.Fatalf("task-b pending status = %q, want pending", statuses["task-b"])
	}
	// One admitted dispatch costs one honey and the resumed event is not
	// applied to the ledger a second time, so the reserve is exactly spent.
	if got := remainingHoney(t, ledger, "trace-1"); got != 0 {
		t.Fatalf("remaining honey = %d, want 0 after one admitted dispatch", got)
	}

	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d after the second honey, want 2", rec.calls)
	}
	statuses = pendingStatusesByTask(t, ledger, "trace-1")
	if statuses["task-a"] != taskledger.PendingDispatchConsumed || statuses["task-b"] != taskledger.PendingDispatchConsumed {
		t.Fatalf("pending statuses = %+v, want both consumed", statuses)
	}
	if active := taskledger.ActivePendingDispatches(mustSnapshot(t, ledger, "trace-1")); len(active) != 0 {
		t.Fatalf("active pending = %+v, want none", active)
	}
	// Resuming recovers the source event for dispatch; it never replays it
	// through the ledger reducer, so the tasks keep the stage they reached.
	for _, taskID := range []string{"task-a", "task-b"} {
		assertTaskStatus(t, ledger, taskID, protocol.TaskStatusWaitingReview)
	}
}

// A retried top-up must not dispatch work that already ran, and must not charge
// honey for a dispatch it does not start.
func TestRepeatedEnergyAddDoesNotDispatchConsumedWork(t *testing.T) {
	ev := mustCodeProposal(t, "trace-1", "task-1", 1, "+func Login() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)

	r := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, directGuardBees())
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d after the first top-up, want 1", rec.calls)
	}
	if got := remainingHoney(t, ledger, "trace-1"); got != 1 {
		t.Fatalf("remaining honey = %d, want 1 (2 added, one dispatch)", got)
	}

	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d after the second top-up, want 1", rec.calls)
	}
	if got := remainingHoney(t, ledger, "trace-1"); got != 3 {
		t.Fatalf("remaining honey = %d, want 3 (no charge for an already consumed dispatch)", got)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchConsumed)
	if active := taskledger.ActivePendingDispatches(mustSnapshot(t, ledger, "trace-1")); len(active) != 0 {
		t.Fatalf("active pending = %+v, want none", active)
	}
}
