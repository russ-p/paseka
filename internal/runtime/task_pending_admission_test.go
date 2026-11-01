package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// A dispatch admitted by task.ready consumes the deferral entry in the same
// mutation that starts the task. This is the non-resume trigger: the honey
// arrived externally, and no energy.add drain ran in this reactor.
func TestTaskReadyDispatchConsumesDeferredEntry(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)

	if err := r.ProcessEvent(context.Background(), mustTaskPlan(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), mustTaskReady(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchPending)

	// The top-up lands externally; this reactor does not process energy.add, so
	// only a later task.ready can admit the deferred work.
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if _, err := ledger.Apply(add); err != nil {
		t.Fatal(err)
	}
	// A configuration revision can change the effective Bee between deferral
	// and admission, so a second entry exists under another key.
	if added, err := ledger.DeferPendingDispatch("trace-1", taskledger.PendingDispatch{
		ID:        "task:task-1:reviewer",
		Kind:      taskledger.PendingDispatchTask,
		TaskID:    "task-1",
		Bee:       "reviewer",
		Reason:    protocol.HoneyReserveExhaustedSummary,
		Status:    taskledger.PendingDispatchPending,
		CreatedAt: time.Now().UTC().Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	} else if !added {
		t.Fatal("expected the drifted Bee entry to be recorded")
	}
	// Re-kick the deferred task through the ordinary ready path.
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
	if err := r.ProcessEvent(context.Background(), mustTaskReady(t, "task-1")); err != nil {
		t.Fatal(err)
	}

	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d, want one admitted dispatch", probe.calls)
	}
	if !probe.running[0] {
		t.Fatal("admitted dispatch must see the task running")
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCompleted)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchConsumed)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:reviewer", taskledger.PendingDispatchConsumed)
	if got := taskledger.ActivePendingCount(mustSnapshot(t, ledger, "trace-1")); got != 0 {
		t.Fatalf("pending count = %d, want 0 after admission", got)
	}
}

// A review-gate rework starts the task through the same dispatch path, so it
// consumes a deferral entry left by an earlier energy denial.
func TestReviewReworkDispatchConsumesDeferredEntry(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-1")
	exhaustEnergy(t, ledger)
	if added, err := ledger.DeferPendingDispatch("trace-1", taskledger.PendingDispatch{
		ID:        "task:task-1:builder",
		Kind:      taskledger.PendingDispatchTask,
		TaskID:    "task-1",
		Bee:       "builder",
		Reason:    protocol.HoneyReserveExhaustedSummary,
		Status:    taskledger.PendingDispatchPending,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	} else if !added {
		t.Fatal("expected the deferral to be recorded")
	}

	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if _, err := ledger.Apply(add); err != nil {
		t.Fatal(err)
	}

	feedback, err := protocol.NewEvent("trace-1", "console", 0, protocol.EventInsight, protocol.HumanFeedbackPayload{
		Kind:    protocol.InsightHumanFeedback,
		TaskID:  "task-1",
		Message: "apply the review comments",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), feedback); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d, want one rework dispatch", probe.calls)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchConsumed)
	if got := taskledger.ActivePendingCount(mustSnapshot(t, ledger, "trace-1")); got != 0 {
		t.Fatalf("pending count = %d, want 0 after rework", got)
	}
}

// The legacy energy-unblock path starts a honey-blocked task through the same
// dispatch path, so it also consumes a task entry left for that task.
func TestLegacyEnergyUnblockConsumesDeferredEntry(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)

	if err := r.ProcessEvent(context.Background(), mustTaskPlan(t, "task-1")); err != nil {
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
	if _, err := ledger.Apply(blocked); err != nil {
		t.Fatal(err)
	}
	if added, err := ledger.DeferPendingDispatch("trace-1", taskledger.PendingDispatch{
		ID:        "task:task-1:builder",
		Kind:      taskledger.PendingDispatchTask,
		TaskID:    "task-1",
		Bee:       "builder",
		Reason:    protocol.HoneyReserveExhaustedSummary,
		Status:    taskledger.PendingDispatchPending,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	} else if !added {
		t.Fatal("expected the deferral to be recorded")
	}

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d, want the unblocked task to dispatch", probe.calls)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCompleted)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchConsumed)
	if got := taskledger.ActivePendingCount(mustSnapshot(t, ledger, "trace-1")); got != 0 {
		t.Fatalf("pending count = %d, want 0 after the unblock", got)
	}
}

// The energy.add drain still reports one dispatch for one admitted resume. The
// admission consumes the entry, and the resumed observation counts it once; a
// second startup sees nothing left to drain.
func TestEnergyAddResumeCountsTaskDispatchOnce(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	pub := &recordingPublisher{}
	r := newEnergyObservationReactor(t, ledger, fakeReplayer{}, taskDispatchBees(), pub)
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)

	if err := r.ProcessEvent(context.Background(), mustTaskPlan(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), mustTaskReady(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchPending)

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d, want one resumed dispatch", probe.calls)
	}
	resumed := observationEvents(pub, protocol.SignalEnergyResumed)
	if len(resumed) != 1 {
		t.Fatalf("resumed observations = %d, want one", len(resumed))
	}
	payload := decodeObservation[protocol.EnergyResumedPayload](t, resumed[0])
	if payload.Dispatched != 1 {
		t.Fatalf("dispatched = %d, want exactly one", payload.Dispatched)
	}
	if payload.PendingBefore != 1 || payload.PendingAfter != 0 {
		t.Fatalf("pending = %d -> %d, want 1 -> 0", payload.PendingBefore, payload.PendingAfter)
	}
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchConsumed)

	if err := r.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d after restart, want the resume counted once", probe.calls)
	}
}
