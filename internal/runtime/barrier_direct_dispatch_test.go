package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// barrierHoldReason is the literal persisted reason. A test compares it
// directly so a rename cannot silently make held work look like energy-deferred
// work to the resume paths.
const barrierHoldReason = "configuration change pending"

// barrierReplayer is a mutable trace replayer so a test can register the source
// event after the reactor was built.
type barrierReplayer struct {
	events []protocol.Event
}

func (b *barrierReplayer) ReplayTrace(string) ([]protocol.Event, error) {
	return b.events, nil
}

// barrierDirectHarness owns a colony whose "builder" bee runs one reviewable
// task (the durable blocker of a pending configuration change) and whose
// "guard" bee reacts directly to feature.requested signals.
type barrierDirectHarness struct {
	root     string
	traceID  string
	bees     map[string]colony.Bee
	ledger   taskledger.Ledger
	replayer *barrierReplayer
	r        *runtime.Reactor
	rec      *recordingAdapter
	held     protocol.Event
}

func newBarrierDirectHarness(t *testing.T, traceID string) *barrierDirectHarness {
	t.Helper()
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
		"guard": {Role: "guard", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "feature.requested"}, Dispatch: colony.DispatchDirect},
		}},
	}
	mustWriteTestColony(t, root, bees)

	h := &barrierDirectHarness{
		root:     root,
		traceID:  traceID,
		bees:     bees,
		ledger:   taskledger.NewMemoryLedger(),
		replayer: &barrierReplayer{},
		held:     featureEvent(t, traceID, "scout", "held work"),
	}
	h.r, h.rec = h.newReactor(t)

	// Capturing the trail and leaving task-1 waiting for review creates both the
	// configuration binding and the durable blocker the pending change sees.
	plan, ready := barrierTaskEvents(t, traceID, "task-1")
	mustProcess(t, h.r, plan)
	mustProcess(t, h.r, ready)
	if h.rec.calls != 1 {
		t.Fatalf("initial task calls = %d, want 1", h.rec.calls)
	}
	writeRunCompleted(t, root, traceID, h.rec.lastReq.AgentID)
	return h
}

func (h *barrierDirectHarness) newReactor(t *testing.T) (*runtime.Reactor, *recordingAdapter) {
	t.Helper()
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(h.bees)
	d.SetBeeRegistry(reg)
	rec := &recordingAdapter{}
	d.RegisterAdapter("cursor", rec)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: h.root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     h.ledger,
		Replayer:   h.replayer,
	})
	return r, rec
}

// applyPending rewrites the default prompt and records the change as pending.
func (h *barrierDirectHarness) applyPending(t *testing.T) trailconfig.Manifest {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.root, ".paseka", "prompts", "default.md"), []byte("NEW {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := trailconfig.Load(h.root, h.traceID)
	if err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: colony.Context{ColonyRoot: h.root, Slug: "test"},
		TraceID: h.traceID, Source: trailconfig.SourceColony, Actor: "operator",
		ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("apply outcome = %q, want pending", applied.Change.Outcome)
	}
	return manifest
}

// holdDirect runs the direct source event while the change is pending.
func (h *barrierDirectHarness) holdDirect(t *testing.T) {
	t.Helper()
	h.replayer.events = []protocol.Event{h.held}
	before := h.rec.calls
	mustProcess(t, h.r, h.held)
	if h.rec.calls != before {
		t.Fatalf("held direct work ran while pending: calls %d -> %d", before, h.rec.calls)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchPending)
}

// clearBlocker finishes the reviewable task so the pending change can activate.
func (h *barrierDirectHarness) clearBlocker(t *testing.T) {
	t.Helper()
	ev, err := protocol.NewEvent(h.traceID, "runtime", 10, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusFailed, Summary: "give up",
	})
	if err != nil {
		t.Fatal(err)
	}
	mustProcess(t, h.r, ev)
}

func TestBarrierHoldRecordsDirectWorkUnderPendingConfiguration(t *testing.T) {
	h := newBarrierDirectHarness(t, "trace-barrier-record")
	h.applyPending(t)
	h.holdDirect(t)

	item := assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchPending)
	if item.Reason != barrierHoldReason {
		t.Fatalf("held reason = %q, want %q", item.Reason, barrierHoldReason)
	}
	if item.Bee != "guard" || item.TaskID != "" {
		t.Fatalf("held item = %+v", item)
	}
	if item.SourceEventID == "" {
		t.Fatalf("held item has no source event: %+v", item)
	}
	manifest, err := trailconfig.Load(h.root, h.traceID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Pending == nil || manifest.ConfigRevision != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}

	// Recovering the barrier while the change is still blocked must not drop,
	// dispatch, or consume the held item.
	if err := h.r.RecoverPendingConfigs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.rec.calls != 1 {
		t.Fatalf("recovery dispatched held work: calls = %d", h.rec.calls)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchPending)
	after, err := trailconfig.Load(h.root, h.traceID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Pending == nil || after.ConfigRevision != 1 {
		t.Fatalf("recovery changed the pending state: %+v", after)
	}
}

func TestBarrierHoldResumesUnderActivatedConfiguration(t *testing.T) {
	h := newBarrierDirectHarness(t, "trace-barrier-activate")
	h.applyPending(t)
	h.holdDirect(t)

	h.clearBlocker(t)
	if h.rec.calls != 2 {
		t.Fatalf("held work calls = %d, want 2 after activation", h.rec.calls)
	}
	if h.rec.lastReq.Bee != "guard" {
		t.Fatalf("resumed bee = %q, want guard", h.rec.lastReq.Bee)
	}
	if h.rec.lastReq.ConfigRevision != 2 || !strings.Contains(h.rec.lastReq.Prompt, "NEW") {
		t.Fatalf("resumed work ran under rev %d prompt %q, want rev 2 with the activated prompt", h.rec.lastReq.ConfigRevision, h.rec.lastReq.Prompt)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchConsumed)

	manifest, err := trailconfig.Load(h.root, h.traceID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Pending != nil || manifest.ConfigRevision != 2 {
		t.Fatalf("manifest after activation = %+v", manifest)
	}

	// The held event was resumed exactly once: a later boundary has nothing left
	// to drain.
	mustProcess(t, h.r, mustEvent(t, h.traceID, "cli", protocol.EventSignal, `{"kind":"system.note","note":"tick"}`))
	if h.rec.calls != 2 {
		t.Fatalf("held work ran again: calls = %d", h.rec.calls)
	}
}

func TestBarrierHoldResumesUnderCancelledConfiguration(t *testing.T) {
	h := newBarrierDirectHarness(t, "trace-barrier-cancel")
	h.applyPending(t)
	h.holdDirect(t)

	if _, err := trailconfig.Cancel(h.root, h.traceID, "", "operator", "abandon change"); err != nil {
		t.Fatal(err)
	}
	mustProcess(t, h.r, mustEvent(t, h.traceID, "cli", protocol.EventSignal, `{"kind":"system.note","note":"tick"}`))

	if h.rec.calls != 2 {
		t.Fatalf("held work calls = %d, want 2 after cancel", h.rec.calls)
	}
	if h.rec.lastReq.ConfigRevision != 1 || !strings.Contains(h.rec.lastReq.Prompt, "held work") {
		t.Fatalf("resumed work ran under rev %d prompt %q, want rev 1", h.rec.lastReq.ConfigRevision, h.rec.lastReq.Prompt)
	}
	if strings.Contains(h.rec.lastReq.Prompt, "NEW") {
		t.Fatalf("cancelled configuration leaked into the resumed run: %q", h.rec.lastReq.Prompt)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchConsumed)
	mustProcess(t, h.r, mustEvent(t, h.traceID, "cli", protocol.EventSignal, `{"kind":"system.note","note":"again"}`))
	if h.rec.calls != 2 {
		t.Fatalf("held work ran again after cancel: calls = %d", h.rec.calls)
	}
}

func TestBarrierHoldIgnoresEnergyAddWhilePending(t *testing.T) {
	h := newBarrierDirectHarness(t, "trace-barrier-energy")
	h.applyPending(t)
	h.holdDirect(t)

	mustProcess(t, h.r, mustEvent(t, h.traceID, "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`))
	if h.rec.calls != 1 {
		t.Fatalf("energy.add released barrier-held work: calls = %d", h.rec.calls)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchPending)

	// The change is still pending, so the item is still owned by the barrier.
	manifest, err := trailconfig.Load(h.root, h.traceID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Pending == nil || manifest.ConfigRevision != 1 {
		t.Fatalf("energy.add changed the pending state: %+v", manifest)
	}
}

func TestBarrierHoldFreshReactorResumesHeldWork(t *testing.T) {
	h := newBarrierDirectHarness(t, "trace-barrier-fresh")
	h.applyPending(t)
	h.holdDirect(t)
	if _, err := trailconfig.Cancel(h.root, h.traceID, "", "operator", "abandon change"); err != nil {
		t.Fatal(err)
	}

	restarted, rec := h.newReactor(t)
	mustProcess(t, restarted, mustEvent(t, h.traceID, "cli", protocol.EventSignal, `{"kind":"system.note","note":"tick"}`))

	if rec.calls != 1 {
		t.Fatalf("fresh reactor calls = %d, want 1 resumed held item", rec.calls)
	}
	if rec.lastReq.Bee != "guard" || rec.lastReq.ConfigRevision != 1 {
		t.Fatalf("fresh reactor dispatched bee %q rev %d, want guard rev 1", rec.lastReq.Bee, rec.lastReq.ConfigRevision)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchConsumed)
}

func TestBarrierHoldKeepsWorkDurableWhenTraceIsKilled(t *testing.T) {
	h := newBarrierDirectHarness(t, "trace-barrier-kill")
	h.applyPending(t)
	h.holdDirect(t)

	kill, err := protocol.NewEvent(h.traceID, "cli", 11, protocol.EventSignal, protocol.SystemKillPayload{
		Kind: protocol.SignalSystemKill, Reason: "stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	mustProcess(t, h.r, kill)
	if h.rec.calls != 1 {
		t.Fatalf("killed trace dispatched held work: calls = %d", h.rec.calls)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchPending)

	manifest, err := trailconfig.Load(h.root, h.traceID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Pending != nil {
		t.Fatalf("kill did not cancel the pending change: %+v", manifest.Pending)
	}

	// A restart must not resume or consume held work on a killed trail either.
	restarted, rec := h.newReactor(t)
	mustProcess(t, restarted, mustEvent(t, h.traceID, "cli", protocol.EventSignal, `{"kind":"system.note","note":"tick"}`))
	if rec.calls != 0 {
		t.Fatalf("killed trace resumed held work after restart: calls = %d", rec.calls)
	}
	assertBarrierHoldStatus(t, h.ledger, h.traceID, taskledger.PendingDispatchPending)
}

func assertBarrierHoldStatus(t *testing.T, ledger taskledger.Ledger, traceID string, want taskledger.PendingDispatchStatus) taskledger.PendingDispatch {
	t.Helper()
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.PendingDispatches {
		if item.Kind != taskledger.PendingDispatchDirect || item.Reason != barrierHoldReason {
			continue
		}
		if item.Status != want {
			t.Fatalf("barrier-held status = %q, want %q", item.Status, want)
		}
		return item
	}
	t.Fatal("barrier-held direct dispatch not found")
	return taskledger.PendingDispatch{}
}

func mustProcess(t *testing.T, r *runtime.Reactor, ev protocol.Event) {
	t.Helper()
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}
