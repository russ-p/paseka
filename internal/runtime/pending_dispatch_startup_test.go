package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func directGuardBees() map[string]colony.Bee {
	return map[string]colony.Bee{
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		}},
	}
}

func mustCodeProposal(t *testing.T, traceID, taskID string, seq int, diff string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "builder-1", seq, protocol.EventMutation, protocol.MutationPayload{
		Kind:   protocol.MutationCodeProposal,
		TaskID: taskID,
		Diff:   diff,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func mustDeferDirect(t *testing.T, ledger *taskledger.MemoryLedger, traceID string, dispatch taskledger.PendingDispatch) {
	t.Helper()
	dispatch.Kind = taskledger.PendingDispatchDirect
	dispatch.Status = taskledger.PendingDispatchPending
	if dispatch.Reason == "" {
		dispatch.Reason = protocol.HoneyReserveExhaustedSummary
	}
	added, err := ledger.DeferPendingDispatch(traceID, dispatch)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatalf("pending dispatch %q was not recorded", dispatch.ID)
	}
}

func pendingByID(t *testing.T, ledger taskledger.Ledger, traceID, dispatchID string) taskledger.PendingDispatch {
	t.Helper()
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.PendingDispatches {
		if item.ID == dispatchID {
			return item
		}
	}
	t.Fatalf("pending dispatch %q not found in trace %s", dispatchID, traceID)
	return taskledger.PendingDispatch{}
}

// A top-up applied while the reactor was down is already durable in the ledger.
// The next reactor start must resume the direct work it approved instead of
// waiting for another `energy.add`.
func TestStartupResumesAddApprovedPendingDirectDispatch(t *testing.T) {
	ev := mustCodeProposal(t, "trace-1", "task-1", 1, "+func Login() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)

	bees := directGuardBees()
	first := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	first.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := first.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	assertPendingDirect(t, ledger, taskledger.PendingDispatchPending)

	// The top-up lands while no reactor is running: the ledger is the only
	// actor, as with `paseka energy add` when the hive is stopped.
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if _, err := ledger.Apply(add); err != nil {
		t.Fatal(err)
	}

	second := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, bees)
	rec := &recordingAdapter{}
	second.Dispatcher().RegisterAdapter("cursor", rec)
	if err := second.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d on startup, want 1", rec.calls)
	}
	if rec.lastReq.Bee != "guard" {
		t.Fatalf("bee = %q, want guard", rec.lastReq.Bee)
	}
	if rec.lastReq.TaskID != "task-1" {
		t.Fatalf("taskId = %q, want task-1", rec.lastReq.TaskID)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
	assertPendingDirect(t, ledger, taskledger.PendingDispatchConsumed)
}

// Kill and an empty reserve both make deferred work unrunnable. A killed trail
// keeps its entry abandoned, and a legacy ledger whose kill predates
// abandonment still must not dispatch it on startup.
func TestStartupSkipsKilledAndZeroReserveTrails(t *testing.T) {
	killEv := mustCodeProposal(t, "trace-killed", "task-k", 1, "+func Kill() {}")
	zeroEv := mustCodeProposal(t, "trace-zero", "task-z", 1, "+func Zero() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTasks(t, ledger, "trace-killed", "task-k")
	seedWaitingReviewTasks(t, ledger, "trace-zero", "task-z")
	exhaustEnergyIn(t, ledger, "trace-killed")
	exhaustEnergyIn(t, ledger, "trace-zero")

	bees := directGuardBees()
	killed := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{killEv}}, bees)
	killed.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := killed.ProcessEvent(context.Background(), killEv); err != nil {
		t.Fatal(err)
	}
	kill := mustEvent(t, "trace-killed", "cli", protocol.EventSignal, `{"kind":"system.kill","reason":"stop"}`)
	if err := killed.ProcessEvent(context.Background(), kill); err != nil {
		t.Fatal(err)
	}
	// A ledger written before kill abandoned pending work still carries an
	// active entry: the startup skip must come from the trail state itself.
	killID := pendingDirectIDFor(t, ledger, "trace-killed")
	if err := ledger.ResolvePendingDispatch("trace-killed", killID, taskledger.PendingDispatchPending); err != nil {
		t.Fatal(err)
	}

	zero := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{zeroEv}}, bees)
	zero.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := zero.ProcessEvent(context.Background(), zeroEv); err != nil {
		t.Fatal(err)
	}
	zeroID := pendingDirectIDFor(t, ledger, "trace-zero")

	started := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{killEv, zeroEv}}, bees)
	rec := &recordingAdapter{}
	started.Dispatcher().RegisterAdapter("cursor", rec)
	if err := started.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d on startup, want skipped killed and zero-reserve trails", rec.calls)
	}
	if got := pendingByID(t, ledger, "trace-killed", killID).Status; got != taskledger.PendingDispatchPending {
		t.Fatalf("killed trail entry status = %q, want untouched pending", got)
	}
	if got := pendingByID(t, ledger, "trace-zero", zeroID).Status; got != taskledger.PendingDispatchPending {
		t.Fatalf("zero-reserve entry status = %q, want untouched pending", got)
	}
}

// Startup drains oldest-first even when the ledger slice order disagrees.
func TestStartupDrainsPendingDirectOldestFirst(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTasks(t, ledger, "trace-1", "task-old", "task-new")
	if err := ledger.SeedEnergy("trace-1", 3); err != nil {
		t.Fatal(err)
	}
	oldEv := mustCodeProposal(t, "trace-1", "task-old", 1, "+func Old() {}")
	newEv := mustCodeProposal(t, "trace-1", "task-new", 2, "+func New() {}")

	base := time.Now().UTC()
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "pending-new",
		TaskID:        "task-new",
		Bee:           "guard",
		SourceEventID: newEv.ID,
		CreatedAt:     base.Add(time.Minute),
	})
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "pending-old",
		TaskID:        "task-old",
		Bee:           "guard",
		SourceEventID: oldEv.ID,
		CreatedAt:     base,
	})

	started := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{newEv, oldEv}}, directGuardBees())
	rec := &recordingAdapter{}
	started.Dispatcher().RegisterAdapter("cursor", rec)
	if err := started.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d on startup, want 2", rec.calls)
	}
	if rec.requests[0].TaskID != "task-old" || rec.requests[1].TaskID != "task-new" {
		t.Fatalf("dispatch order = [%s %s], want oldest task-old first", rec.requests[0].TaskID, rec.requests[1].TaskID)
	}
	if got := pendingByID(t, ledger, "trace-1", "pending-old").Status; got != taskledger.PendingDispatchConsumed {
		t.Fatalf("oldest entry status = %q, want consumed", got)
	}
	if got := pendingByID(t, ledger, "trace-1", "pending-new").Status; got != taskledger.PendingDispatchConsumed {
		t.Fatalf("newest entry status = %q, want consumed", got)
	}
}

// A missing source event is recorded as unresolved and the rest of the queue
// still drains in the same startup pass.
func TestStartupUnresolvedMissingSourceDoesNotBlockDrain(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTasks(t, ledger, "trace-1", "task-missing", "task-live")
	if err := ledger.SeedEnergy("trace-1", 2); err != nil {
		t.Fatal(err)
	}
	missingEv := mustCodeProposal(t, "trace-1", "task-missing", 1, "+func Missing() {}")
	liveEv := mustCodeProposal(t, "trace-1", "task-live", 2, "+func Live() {}")

	base := time.Now().UTC()
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "pending-missing",
		TaskID:        "task-missing",
		Bee:           "guard",
		SourceEventID: missingEv.ID,
		CreatedAt:     base,
	})
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "pending-live",
		TaskID:        "task-live",
		Bee:           "guard",
		SourceEventID: liveEv.ID,
		CreatedAt:     base.Add(time.Second),
	})

	// The replayer no longer holds the oldest source event.
	started := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{liveEv}}, directGuardBees())
	rec := &recordingAdapter{}
	started.Dispatcher().RegisterAdapter("cursor", rec)
	if err := started.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d on startup, want the live entry to drain", rec.calls)
	}
	if rec.lastReq.TaskID != "task-live" {
		t.Fatalf("taskId = %q, want task-live", rec.lastReq.TaskID)
	}
	missing := pendingByID(t, ledger, "trace-1", "pending-missing")
	if missing.Status != taskledger.PendingDispatchUnresolved {
		t.Fatalf("missing entry status = %q, want unresolved", missing.Status)
	}
	if missing.Diagnostic == "" {
		t.Fatal("unresolved entry must carry a diagnostic")
	}
	if got := pendingByID(t, ledger, "trace-1", "pending-live").Status; got != taskledger.PendingDispatchConsumed {
		t.Fatalf("live entry status = %q, want consumed", got)
	}
}

func pendingDirectIDFor(t *testing.T, ledger taskledger.Ledger, traceID string) string {
	t.Helper()
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.PendingDispatches {
		if item.Kind == taskledger.PendingDispatchDirect {
			return item.ID
		}
	}
	t.Fatalf("direct pending dispatch not found in trace %s", traceID)
	return ""
}

// A redelivered source event can defer the same reaction again after its entry
// was consumed, appending a second entry with the same dispatch key. The next
// top-up must close that active duplicate, not the stale consumed one.
func TestResumeClosesDuplicateDispatchEntryAfterRestart(t *testing.T) {
	ev := mustCodeProposal(t, "trace-1", "task-1", 1, "+func Login() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)

	first := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, directGuardBees())
	first.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := first.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	dispatchID := pendingDirectIDFor(t, ledger, "trace-1")

	// The resume ran while the reactor was up: the entry is consumed, but the
	// process that would remember the dispatch is gone.
	if err := ledger.ResolvePendingDispatch("trace-1", dispatchID, taskledger.PendingDispatchConsumed); err != nil {
		t.Fatal(err)
	}

	// A fresh reactor sees the same source event redelivered with no reserve,
	// so it defers the reaction again under the same dispatch key.
	second := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{ev}}, directGuardBees())
	second.Dispatcher().RegisterAdapter("cursor", &recordingAdapter{})
	if err := second.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	snap := mustSnapshot(t, ledger, "trace-1")
	if len(snap.PendingDispatches) != 2 {
		t.Fatalf("pending entries = %d, want the duplicate to be recorded", len(snap.PendingDispatches))
	}

	rec := &recordingAdapter{}
	second.Dispatcher().RegisterAdapter("cursor", rec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	if err := second.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want the duplicate to run once", rec.calls)
	}
	snap = mustSnapshot(t, ledger, "trace-1")
	for i, item := range snap.PendingDispatches {
		if item.Status != taskledger.PendingDispatchConsumed {
			t.Fatalf("entry[%d] status = %q, want consumed", i, item.Status)
		}
	}
	if active := taskledger.ActivePendingDispatches(snap); len(active) != 0 {
		t.Fatalf("active pending = %+v, want none", active)
	}
}
