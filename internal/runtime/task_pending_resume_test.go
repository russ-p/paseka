package runtime_test

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// taskProbeAdapter records each dispatch and whether the task was already
// running when the adapter was entered, so a resume test can prove the normal
// task dispatch path was taken instead of a parallel shortcut.
type taskProbeAdapter struct {
	ledger   taskledger.Ledger
	traceID  string
	calls    int
	running  []bool
	requests []adapters.RunRequest
}

func (a *taskProbeAdapter) Name() string { return "cursor" }

func (a *taskProbeAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	a.calls++
	a.requests = append(a.requests, req)
	status := protocol.TaskStatus("")
	if snap, err := a.ledger.Snapshot(a.traceID); err == nil {
		status = snap.Tasks[req.TaskID].Status
	}
	a.running = append(a.running, status == protocol.TaskStatusRunning)
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
}

func taskAndDirectBees() map[string]colony.Bee {
	bees := taskDispatchBees()
	for role, bee := range directGuardBees() {
		bees[role] = bee
	}
	return bees
}

// seedTaskStatuses plans the tasks and then applies the requested lifecycle
// status directly, so a test can build a ready task or one that closed while a
// dispatch was pending.
func seedTaskStatuses(t *testing.T, ledger *taskledger.MemoryLedger, traceID string, statuses map[string]protocol.TaskStatus) {
	t.Helper()
	ids := make([]string, 0, len(statuses))
	for id := range statuses {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	specs := make([]protocol.TaskSpec, 0, len(ids))
	for _, id := range ids {
		specs = append(specs, protocol.TaskSpec{
			TaskID: id,
			Title:  "implement",
			Body:   "do work",
			Bee:    "builder",
		})
	}
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: specs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		status, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
			Kind:   protocol.TaskEventStatus,
			TaskID: id,
			Status: statuses[id],
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.Apply(status); err != nil {
			t.Fatal(err)
		}
	}
}

func deferTaskEntry(t *testing.T, ledger *taskledger.MemoryLedger, traceID, dispatchID, taskID, bee, sourceEventID string, createdAt time.Time) {
	t.Helper()
	added, err := ledger.DeferPendingDispatch(traceID, taskledger.PendingDispatch{
		ID:            dispatchID,
		Kind:          taskledger.PendingDispatchTask,
		TaskID:        taskID,
		Bee:           bee,
		SourceEventID: sourceEventID,
		Reason:        protocol.HoneyReserveExhaustedSummary,
		Status:        taskledger.PendingDispatchPending,
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatalf("pending task dispatch %q was not recorded", dispatchID)
	}
}

func assertPendingTask(t *testing.T, ledger taskledger.Ledger, traceID, dispatchID string, want taskledger.PendingDispatchStatus) {
	t.Helper()
	snap := mustSnapshot(t, ledger, traceID)
	for _, item := range snap.PendingDispatches {
		if item.ID == dispatchID {
			if item.Status != want {
				t.Fatalf("pending task %q status = %q, want %q", dispatchID, item.Status, want)
			}
			return
		}
	}
	t.Fatalf("pending task dispatch %q not found in trace %s", dispatchID, traceID)
}

// energy.add is the only thing that makes deferred task work runnable: the
// resume goes through the normal task dispatch path, so the task becomes
// running while the adapter runs and can complete.
func TestEnergyAddResumesDeferredReadyTask(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)

	ready := mustTaskReady(t, "task-1")
	if err := r.ProcessEvent(context.Background(), mustTaskPlan(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 0 {
		t.Fatalf("adapter calls = %d before the top-up, want 0", probe.calls)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusReady)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchPending)

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d after the top-up, want 1", probe.calls)
	}
	if !probe.running[0] {
		t.Fatal("resumed dispatch must see the task running while the adapter executes")
	}
	if probe.requests[0].TaskID != "task-1" || probe.requests[0].Bee != "builder" {
		t.Fatalf("dispatch = %+v, want task-1 through builder", probe.requests[0])
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCompleted)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchConsumed)
}

// The pending entry keeps the first source event identity. That identity is the
// attempt dispatch marker a resume uses; a duplicate task.ready while the entry
// is pending adds no second entry and does not replace the identity.
func TestTaskResumeUsesStoredSourceEventAsAttemptMarker(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)

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

	// A plain duplicate task.ready on a ready task is not a dispatch at all.
	if err := r.ProcessEvent(context.Background(), secondReady); err != nil {
		t.Fatal(err)
	}
	// Make the task dispatchable again, so the duplicate reaction is deferred
	// under the same task key and must keep the first identity.
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

	pending := pendingTaskDispatches(mustSnapshot(t, ledger, "trace-1"))
	if len(pending) != 1 {
		t.Fatalf("pending task entries = %d, want the duplicate to add none", len(pending))
	}
	if pending[0].SourceEventID != firstReady.ID {
		t.Fatalf("pending source event = %q, want the first kick %q", pending[0].SourceEventID, firstReady.ID)
	}

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d, want one resumed dispatch", probe.calls)
	}
	attempt, ok, err := trailconfig.LatestTaskAttempt(r.ColonyRoot(), "trace-1", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("resumed dispatch recorded no attempt")
	}
	if attempt.SourceEventID != firstReady.ID {
		t.Fatalf("attempt source event = %q, want the stored %q", attempt.SourceEventID, firstReady.ID)
	}
	if !strings.HasSuffix(attempt.LastDispatchKey, "|"+firstReady.ID) {
		t.Fatalf("attempt dispatch marker = %q, want it keyed by the stored source event %q", attempt.LastDispatchKey, firstReady.ID)
	}
	if attempt.DispatchCount != 1 {
		t.Fatalf("attempt dispatch count = %d, want 1", attempt.DispatchCount)
	}
}

// A top-up applied while the reactor was down is durable in the ledger, so the
// next reactor start resumes add-approved task work. A killed trail and a
// zero-reserve trail are skipped even if the ledger still carries an active
// entry for them.
func TestStartupResumesAddApprovedPendingTaskAndSkipsClosedTrails(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	seedTaskStatuses(t, ledger, "trace-live", map[string]protocol.TaskStatus{"task-live": protocol.TaskStatusReady})
	seedTaskStatuses(t, ledger, "trace-killed", map[string]protocol.TaskStatus{"task-kill": protocol.TaskStatusReady})
	seedTaskStatuses(t, ledger, "trace-zero", map[string]protocol.TaskStatus{"task-zero": protocol.TaskStatusReady})

	base := time.Now().UTC()
	deferTaskEntry(t, ledger, "trace-live", "task:task-live:builder", "task-live", "builder", "ready-live", base)
	deferTaskEntry(t, ledger, "trace-killed", "task:task-kill:builder", "task-kill", "builder", "ready-kill", base)
	deferTaskEntry(t, ledger, "trace-zero", "task:task-zero:builder", "task-zero", "builder", "ready-zero", base)

	// The top-up lands while no reactor is running.
	add := mustEvent(t, "trace-live", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if _, err := ledger.Apply(add); err != nil {
		t.Fatal(err)
	}
	// Kill abandons pending work, but a legacy ledger can still carry an active
	// entry: the startup skip must come from the trail state itself.
	kill := mustEvent(t, "trace-killed", "cli", protocol.EventSignal, `{"kind":"system.kill","reason":"stop"}`)
	if _, err := ledger.Apply(kill); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ResolvePendingDispatch("trace-killed", "task:task-kill:builder", taskledger.PendingDispatchPending); err != nil {
		t.Fatal(err)
	}

	started := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	probe := &taskProbeAdapter{ledger: ledger}
	started.Dispatcher().RegisterAdapter("cursor", probe)
	if err := started.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d on startup, want only the add-approved trail to run", probe.calls)
	}
	if probe.requests[0].TaskID != "task-live" {
		t.Fatalf("startup dispatched %q, want task-live", probe.requests[0].TaskID)
	}
	assertPendingTask(t, ledger, "trace-live", "task:task-live:builder", taskledger.PendingDispatchConsumed)
	assertPendingTask(t, ledger, "trace-killed", "task:task-kill:builder", taskledger.PendingDispatchPending)
	assertPendingTask(t, ledger, "trace-zero", "task:task-zero:builder", taskledger.PendingDispatchPending)
}

// Task and direct entries share one creation-order queue. The ledger slice order
// is deliberately not the creation order.
func TestPendingQueueDrainsTaskAndDirectOldestFirst(t *testing.T) {
	directEv := mustCodeProposal(t, "trace-1", "task-direct", 1, "+func Direct() {}")
	ledger := taskledger.NewMemoryLedger()
	seedTaskStatuses(t, ledger, "trace-1", map[string]protocol.TaskStatus{
		"task-ready":  protocol.TaskStatusReady,
		"task-direct": protocol.TaskStatusWaitingReview,
	})
	if err := ledger.SeedEnergy("trace-1", 2); err != nil {
		t.Fatal(err)
	}

	base := time.Now().UTC()
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "direct-newer",
		TaskID:        "task-direct",
		Bee:           "guard",
		SourceEventID: directEv.ID,
		CreatedAt:     base.Add(time.Minute),
	})
	deferTaskEntry(t, ledger, "trace-1", "task:task-ready:builder", "task-ready", "builder", "ready-older", base)

	r := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{directEv}}, taskAndDirectBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 2 {
		t.Fatalf("adapter calls = %d, want both entries to drain", probe.calls)
	}
	if probe.requests[0].TaskID != "task-ready" || probe.requests[0].Bee != "builder" {
		t.Fatalf("first dispatch = %+v, want the older task entry", probe.requests[0])
	}
	if probe.requests[1].TaskID != "task-direct" || probe.requests[1].Bee != "guard" {
		t.Fatalf("second dispatch = %+v, want the newer direct entry", probe.requests[1])
	}
}

// Entries created at the same instant drain by id, so the queue order is stable
// across restarts instead of depending on ledger slice order.
func TestPendingQueueTieBreaksOnStableID(t *testing.T) {
	directEv := mustCodeProposal(t, "trace-1", "task-direct", 1, "+func Direct() {}")
	ledger := taskledger.NewMemoryLedger()
	seedTaskStatuses(t, ledger, "trace-1", map[string]protocol.TaskStatus{
		"task-ready":  protocol.TaskStatusReady,
		"task-direct": protocol.TaskStatusWaitingReview,
	})
	if err := ledger.SeedEnergy("trace-1", 2); err != nil {
		t.Fatal(err)
	}

	same := time.Now().UTC()
	deferTaskEntry(t, ledger, "trace-1", "z-task", "task-ready", "builder", "ready-z", same)
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "a-direct",
		TaskID:        "task-direct",
		Bee:           "guard",
		SourceEventID: directEv.ID,
		CreatedAt:     same,
	})

	r := newPendingDispatchReactor(t, ledger, fakeReplayer{events: []protocol.Event{directEv}}, taskAndDirectBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 2 {
		t.Fatalf("adapter calls = %d, want both entries to drain", probe.calls)
	}
	if probe.requests[0].TaskID != "task-direct" || probe.requests[1].TaskID != "task-ready" {
		t.Fatalf("dispatch order = [%s %s], want the lower id first", probe.requests[0].TaskID, probe.requests[1].TaskID)
	}
}

// A top-up pays for as much of the task queue as it can. The entry it cannot
// pay for stays pending — not consumed — so the trail never claims a resume it
// did not perform.
func TestPartialTopUpResumesOnePendingTaskAndLeavesTheRest(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	seedTaskStatuses(t, ledger, "trace-1", map[string]protocol.TaskStatus{
		"task-old": protocol.TaskStatusReady,
		"task-new": protocol.TaskStatusReady,
	})
	base := time.Now().UTC()
	deferTaskEntry(t, ledger, "trace-1", "task:task-old:builder", "task-old", "builder", "ready-old", base)
	deferTaskEntry(t, ledger, "trace-1", "task:task-new:builder", "task-new", "builder", "ready-new", base.Add(time.Second))

	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
	probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
	r.Dispatcher().RegisterAdapter("cursor", probe)

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("adapter calls = %d after one honey, want 1", probe.calls)
	}
	if probe.requests[0].TaskID != "task-old" {
		t.Fatalf("resumed %q, want the oldest task-old", probe.requests[0].TaskID)
	}
	assertPendingTask(t, ledger, "trace-1", "task:task-old:builder", taskledger.PendingDispatchConsumed)
	assertPendingTask(t, ledger, "trace-1", "task:task-new:builder", taskledger.PendingDispatchPending)
	if remaining := remainingHoney(t, ledger, "trace-1"); remaining != 0 {
		t.Fatalf("remaining honey = %d, want 0 after one admitted dispatch", remaining)
	}

	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 2 {
		t.Fatalf("adapter calls = %d after the second honey, want 2", probe.calls)
	}
	assertPendingTask(t, ledger, "trace-1", "task:task-new:builder", taskledger.PendingDispatchConsumed)
}

// A task that closed or turned terminal while its dispatch waited is abandoned
// instead of replayed. The task keeps its own lifecycle status.
func TestPendingTaskClosedWhileWaitingBecomesAbandoned(t *testing.T) {
	cases := []struct {
		name   string
		status protocol.TaskStatus
	}{
		{name: "completed", status: protocol.TaskStatusCompleted},
		{name: "failed", status: protocol.TaskStatusFailed},
		{name: "cancelled", status: protocol.TaskStatusCancelled},
		{name: "waiting review", status: protocol.TaskStatusWaitingReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := taskledger.NewMemoryLedger()
			seedTaskStatuses(t, ledger, "trace-1", map[string]protocol.TaskStatus{"task-1": protocol.TaskStatusReady})
			deferTaskEntry(t, ledger, "trace-1", "task:task-1:builder", "task-1", "builder", "ready-1", time.Now().UTC())

			closed, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
				Kind:   protocol.TaskEventStatus,
				TaskID: "task-1",
				Status: tc.status,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.Apply(closed); err != nil {
				t.Fatal(err)
			}

			r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
			probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
			r.Dispatcher().RegisterAdapter("cursor", probe)
			add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
			if err := r.ProcessEvent(context.Background(), add); err != nil {
				t.Fatal(err)
			}
			if probe.calls != 0 {
				t.Fatalf("adapter calls = %d, want closed work abandoned", probe.calls)
			}
			assertTaskStatus(t, ledger, "task-1", tc.status)
			assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchAbandoned)
		})
	}
}

// Two close energy events must not start two live dispatches for the same
// entry. The process claims the entry for the dispatch it already started, so
// the second drain leaves it alone and the first marks it consumed.
func TestConcurrentTopUpsDoNotDuplicateLiveTaskDispatch(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	exhaustEnergy(t, ledger)
	r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFn := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseFn()
	blocking := &blockingTaskAdapter{entered: entered, release: release}
	r.Dispatcher().RegisterAdapter("cursor", blocking)

	if err := r.ProcessEvent(context.Background(), mustTaskPlan(t, "task-1")); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), mustTaskReady(t, "task-1")); err != nil {
		t.Fatal(err)
	}

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	done := make(chan error, 1)
	go func() { done <- r.ProcessEvent(context.Background(), add) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("resumed task dispatch never reached the adapter")
	}

	// A second top-up arrives while the first dispatch is live.
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		releaseFn()
		t.Fatal(err)
	}
	if got := blocking.Calls(); got != 1 {
		releaseFn()
		t.Fatalf("adapter calls = %d while the first dispatch is live, want no duplicate", got)
	}

	releaseFn()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resumed task dispatch did not finish")
	}
	if got := blocking.Calls(); got != 1 {
		t.Fatalf("adapter calls = %d after both top-ups, want one live dispatch", got)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCompleted)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchConsumed)
}

type blockingTaskAdapter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   int32
}

func (a *blockingTaskAdapter) Name() string { return "cursor" }

func (a *blockingTaskAdapter) Run(_ context.Context, _ adapters.RunRequest) (*adapters.RunResult, error) {
	atomic.AddInt32(&a.calls, 1)
	a.once.Do(func() { close(a.entered) })
	<-a.release
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
}

func (a *blockingTaskAdapter) Calls() int { return int(atomic.LoadInt32(&a.calls)) }

// Kill is the hard stop for deferred task work too: the kill mutation abandons
// the entry, and a later top-up must not dispatch it.
func TestKillAbandonsPendingTaskWorkAndTopUpDoesNotRevive(t *testing.T) {
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

	kill := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"system.kill","reason":"stop"}`)
	if err := r.ProcessEvent(context.Background(), kill); err != nil {
		t.Fatal(err)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCancelled)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchAbandoned)

	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 0 {
		t.Fatalf("adapter calls = %d after top-up, want killed task work to stay abandoned", probe.calls)
	}
	assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusCancelled)
	assertPendingTask(t, ledger, "trace-1", "task:task-1:builder", taskledger.PendingDispatchAbandoned)
}
